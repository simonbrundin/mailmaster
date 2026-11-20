package mcp

import (
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"

	"mailagent/actions"
	"mailagent/config"
	"mailagent/imap"
	"mailagent/oauth"
	"mailagent/rules"
)

// accountSession holds the live state for one mail account: the IMAP
// connection, the action executor and the credentials in use.
type accountSession struct {
	account    *config.MailAccount
	imapClient *imap.Client
	executor   *actions.Executor
	// credential is the password or OAuth2 access token used for the
	// connection. It is needed for SMTP as well.
	credential string
	// broken marks a session whose connection died. The next request for the
	// account reconnects instead of reusing the dead connection.
	broken bool
}

// sessionStore keeps one session per account so switching accounts in the UI
// does not require a new login every time.
type sessionStore struct {
	mu       sync.Mutex
	sessions map[string]*accountSession
}

func newSessionStore() *sessionStore {
	return &sessionStore{sessions: make(map[string]*accountSession)}
}

// get returns the cached session for an account name.
func (s *sessionStore) get(account string) (*accountSession, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	session, ok := s.sessions[account]
	return session, ok
}

// put stores a session for an account name.
func (s *sessionStore) put(account string, session *accountSession) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessions[account] = session
}

// remove closes and forgets the session for an account name.
func (s *sessionStore) remove(account string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if session, ok := s.sessions[account]; ok {
		if session.imapClient != nil {
			_ = session.imapClient.Close()
		}
		delete(s.sessions, account)
	}
}

// markBroken flags a session as dead so the next request reconnects.
func (s *sessionStore) markBroken(account string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if session, ok := s.sessions[account]; ok {
		session.broken = true
	}
}

// isBroken reports whether the session for an account is dead.
func (s *sessionStore) isBroken(account string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	session, ok := s.sessions[account]
	return ok && session.broken
}

// markIfBroken flags the session as dead when the error came from a dead
// connection, so the next request reconnects instead of failing again.
func (s *Server) markIfBroken(session *accountSession, err error) error {
	if isConnectionError(err) {
		s.sessions.markBroken(session.account.Name)
	}
	return err
}

// listEmails lists the emails of a folder. A connection that died between
// requests is reestablished once, since mail servers regularly drop idle
// connections.
func (s *Server) listEmails(session *accountSession, folder string, limit uint32) ([]*rules.EmailContext, error) {
	emails, err := session.imapClient.ListEmails(folder, limit)
	if err == nil || !isConnectionError(err) {
		return emails, s.markIfBroken(session, err)
	}

	// The connection died. Reconnect and try once more.
	s.sessions.remove(session.account.Name)
	fresh, connErr := s.ensureConnectionFor(session.account.Name)
	if connErr != nil {
		return nil, err
	}
	emails, err = fresh.imapClient.ListEmails(folder, limit)
	return emails, s.markIfBroken(fresh, err)
}

// isConnectionError reports whether an error came from a dead connection
// rather than from the mail server rejecting the request.
func isConnectionError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, io.EOF) || errors.Is(err, net.ErrClosed) {
		return true
	}
	message := err.Error()
	for _, fragment := range []string{
		"broken pipe",
		"connection reset",
		"connection refused",
		"use of closed network connection",
		"i/o timeout",
		"EOF",
	} {
		if strings.Contains(message, fragment) {
			return true
		}
	}
	return false
}

// credentialFor returns the credential used to log in to an account. For
// OAuth2 accounts it is a cached access token, for the rest it is the
// password from the environment.
func credentialFor(account *config.MailAccount) (string, error) {
	if account.UseOAuth2 {
		token, err := oauth.CachedToken(account.OAuthConfig())
		if err != nil {
			return "", fmt.Errorf("account is not logged in: %w", err)
		}
		return token, nil
	}
	return account.GetPassword()
}

// accountByName returns the configured account with the given name. An empty
// name returns the first configured account.
func (s *Server) accountByName(name string) (*config.MailAccount, error) {
	if s.cfg == nil || len(s.cfg.Mail) == 0 {
		return nil, fmt.Errorf("no mail accounts configured")
	}
	if name == "" {
		return &s.cfg.Mail[0], nil
	}
	for i := range s.cfg.Mail {
		if s.cfg.Mail[i].Name == name {
			return &s.cfg.Mail[i], nil
		}
	}
	return nil, fmt.Errorf("unknown account %q", name)
}
