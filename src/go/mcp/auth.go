package mcp

import (
	"errors"
	"net/http"
	"time"

	"mailagent/config"
	"mailagent/oauth"
)

// pendingLogin is a device code login waiting for the user to complete it in
// a browser.
type pendingLogin struct {
	flow      *oauth.DeviceFlow
	startedAt time.Time
	lastPoll  time.Time
}

// AccountInfo describes one configured mail account for the UI.
type AccountInfo struct {
	Name        string `json:"name"`
	Username    string `json:"username"`
	Server      string `json:"server"`
	Folder      string `json:"folder"`
	AuthMethod  string `json:"auth_method"` // "password" or "oauth2"
	LoggedIn    bool   `json:"logged_in"`
	LoginNeeded bool   `json:"login_needed"`
	Message     string `json:"message,omitempty"`
}

// httpListAccounts lists all configured accounts and their login state.
func (s *Server) httpListAccounts(w http.ResponseWriter, r *http.Request) {
	if s.cfg == nil || len(s.cfg.Mail) == 0 {
		writeError(w, 500, "no mail accounts configured")
		return
	}

	accounts := make([]AccountInfo, 0, len(s.cfg.Mail))
	for i := range s.cfg.Mail {
		accounts = append(accounts, s.accountInfo(&s.cfg.Mail[i]))
	}

	writeJSON(w, 200, map[string]interface{}{"accounts": accounts})
}

// accountInfo describes one account, including whether it is logged in.
func (s *Server) accountInfo(account *config.MailAccount) AccountInfo {
	info := AccountInfo{
		Name:       account.Name,
		Username:   account.Username,
		Server:     account.Server,
		Folder:     account.Folder,
		AuthMethod: "password",
	}
	if account.UseOAuth2 {
		info.AuthMethod = "oauth2"
	}

	if _, ok := s.sessions.get(account.Name); ok {
		info.LoggedIn = true
		return info
	}

	if account.UseOAuth2 {
		if oauth.HasCachedToken(account.OAuthConfig()) {
			info.LoggedIn = true
		} else {
			info.LoginNeeded = true
			info.Message = "Logga in med Microsoft för att kunna läsa mejlen."
		}
		return info
	}

	if _, err := account.GetPassword(); err != nil {
		info.LoginNeeded = true
		info.Message = "Lösenord saknas i miljön."
	} else {
		info.LoggedIn = true
	}
	return info
}

// httpStartAccountLogin starts a device code login for an account and returns
// the code the user has to enter in a browser.
func (s *Server) httpStartAccountLogin(w http.ResponseWriter, r *http.Request, path string) {
	account := accountFromPath(path)

	accountCfg, err := s.accountByName(account)
	if err != nil {
		writeError(w, 404, err.Error())
		return
	}
	if !accountCfg.UseOAuth2 {
		writeError(w, 400, "account does not use OAuth2")
		return
	}

	flow, err := oauth.StartDeviceLogin(r.Context(), accountCfg.OAuthConfig())
	if err != nil {
		writeError(w, 502, err.Error())
		return
	}

	s.loginsMu.Lock()
	s.logins[accountCfg.Name] = &pendingLogin{flow: flow, startedAt: time.Now()}
	s.loginsMu.Unlock()

	writeJSON(w, 200, map[string]interface{}{
		"user_code":        flow.UserCode,
		"verification_uri": flow.VerificationURI,
		"message":          flow.Message,
		"expires_in":       flow.ExpiresIn,
		"interval":         flow.Interval,
	})
}

// httpAccountLoginStatus polls a pending login and reports whether the
// account is now logged in.
func (s *Server) httpAccountLoginStatus(w http.ResponseWriter, r *http.Request, path string) {
	account := accountFromPath(path)

	accountCfg, err := s.accountByName(account)
	if err != nil {
		writeError(w, 404, err.Error())
		return
	}

	s.loginsMu.Lock()
	pending, ok := s.logins[accountCfg.Name]
	s.loginsMu.Unlock()

	if !ok {
		writeJSON(w, 200, map[string]interface{}{
			"status":    "idle",
			"logged_in": s.accountInfo(accountCfg).LoggedIn,
		})
		return
	}

	// Respect the polling interval Microsoft asked for.
	if !pending.lastPoll.IsZero() && time.Since(pending.lastPoll) < time.Duration(pending.flow.Interval)*time.Second {
		writeJSON(w, 200, map[string]interface{}{
			"status":    "waiting",
			"logged_in": false,
		})
		return
	}
	pending.lastPoll = time.Now()

	token, err := oauth.CompleteDeviceLogin(r.Context(), accountCfg.OAuthConfig(), pending.flow)
	switch {
	case err == nil:
		s.loginsMu.Lock()
		delete(s.logins, accountCfg.Name)
		s.loginsMu.Unlock()
		_ = token
		writeJSON(w, 200, map[string]interface{}{
			"status":    "complete",
			"logged_in": true,
		})
	case errors.Is(err, oauth.ErrAuthorizationPending):
		writeJSON(w, 200, map[string]interface{}{
			"status":    "waiting",
			"logged_in": false,
		})
	case errors.Is(err, oauth.ErrExpired):
		s.loginsMu.Lock()
		delete(s.logins, accountCfg.Name)
		s.loginsMu.Unlock()
		writeJSON(w, 200, map[string]interface{}{
			"status":    "expired",
			"logged_in": false,
			"message":   "Koden gick ut. Starta en ny inloggning.",
		})
	default:
		writeJSON(w, 200, map[string]interface{}{
			"status":    "error",
			"logged_in": false,
			"message":   err.Error(),
		})
	}
}

// httpAccountFolders lists the folders of an account. Connecting is required,
// so the account has to be logged in.
func (s *Server) httpAccountFolders(w http.ResponseWriter, r *http.Request, path string) {
	account := accountFromPath(path)

	session, err := s.ensureConnectionFor(account)
	if err != nil {
		writeError(w, 500, err.Error())
		return
	}

	folders, err := session.imapClient.ListFolders()
	if err != nil {
		writeError(w, 500, err.Error())
		return
	}
	if folders == nil {
		folders = []string{}
	}

	writeJSON(w, 200, map[string]interface{}{"folders": folders})
}
