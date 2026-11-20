package mcp

import (
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"mailagent/config"
	"mailagent/oauth"
)

// testServer creates a server with two configured accounts: one password
// based and one OAuth2 based.
func testServer(t *testing.T) *Server {
	t.Helper()

	cfg := &config.Config{
		Mail: []config.MailAccount{
			{
				Name:        "Michael Brundins",
				Server:      "imap.one.com",
				Port:        993,
				Username:    "michael@brundins.se",
				PasswordEnv: "TEST_MAIL_PASSWORD_MICHAEL",
				Folder:      "INBOX.Simon",
			},
			{
				Name:           "Soulmate",
				Server:         "outlook.office365.com",
				Port:           993,
				Username:       "michael.brundin@soulmate.se",
				AzureClientID:  "client-123",
				AzureTenantID:  "tenant-1",
				OAuthCachePath: t.TempDir() + "/token.json",
				Folder:         "INBOX",
				UseOAuth2:      true,
			},
		},
	}

	return NewServer(cfg, nil, nil)
}

func TestListAccountsReportsAuthMethodAndLoginState(t *testing.T) {
	t.Setenv("TEST_MAIL_PASSWORD_MICHAEL", "secret")

	s := testServer(t)

	rec := httptest.NewRecorder()
	s.httpListAccounts(rec, httptest.NewRequest(http.MethodGet, "/api/v1/accounts", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var body struct {
		Accounts []AccountInfo `json:"accounts"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.Accounts) != 2 {
		t.Fatalf("expected 2 accounts, got %d", len(body.Accounts))
	}

	first := body.Accounts[0]
	if first.Name != "Michael Brundins" {
		t.Errorf("unexpected account name %q", first.Name)
	}
	if first.AuthMethod != "password" {
		t.Errorf("expected auth method password, got %q", first.AuthMethod)
	}
	if !first.LoggedIn {
		t.Error("expected the password account to be logged in")
	}
	if first.LoginNeeded {
		t.Error("did not expect a login to be needed for the password account")
	}

	second := body.Accounts[1]
	if second.AuthMethod != "oauth2" {
		t.Errorf("expected auth method oauth2, got %q", second.AuthMethod)
	}
	if second.LoggedIn {
		t.Error("expected the OAuth account not to be logged in without a token")
	}
	if !second.LoginNeeded {
		t.Error("expected a login to be needed for the OAuth account")
	}
}

func TestListAccountsMarksOAuthAccountLoggedInWithToken(t *testing.T) {
	s := testServer(t)

	cachePath := s.cfg.Mail[1].OAuthCachePath
	if err := oauth.SaveToken(cachePath, &oauth.Token{AccessToken: "token", ExpiresAt: timeNowPlusHour()}); err != nil {
		t.Fatalf("SaveToken: %v", err)
	}

	rec := httptest.NewRecorder()
	s.httpListAccounts(rec, httptest.NewRequest(http.MethodGet, "/api/v1/accounts", nil))

	var body struct {
		Accounts []AccountInfo `json:"accounts"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !body.Accounts[1].LoggedIn {
		t.Error("expected the OAuth account to be logged in with a cached token")
	}
	if body.Accounts[1].LoginNeeded {
		t.Error("did not expect a login to be needed with a cached token")
	}
}

func TestStartAccountLoginRejectsUnknownAccount(t *testing.T) {
	s := testServer(t)

	rec := httptest.NewRecorder()
	s.httpStartAccountLogin(rec, httptest.NewRequest(http.MethodPost, "/api/v1/accounts/Nope/auth/login", nil), "/api/v1/accounts/Nope/auth/login")

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

func TestStartAccountLoginRejectsPasswordAccount(t *testing.T) {
	s := testServer(t)

	// net/http decodes the path before the handler sees it, so the account
	// name arrives with a real space.
	rec := httptest.NewRecorder()
	s.httpStartAccountLogin(rec, httptest.NewRequest(http.MethodPost, "/api/v1/accounts/Michael%20Brundins/auth/login", nil), "/api/v1/accounts/Michael Brundins/auth/login")

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "OAuth2") {
		t.Errorf("expected the error to mention OAuth2, got %s", rec.Body.String())
	}
}

func TestAccountLoginStatusWithoutPendingLoginIsIdle(t *testing.T) {
	s := testServer(t)

	rec := httptest.NewRecorder()
	s.httpAccountLoginStatus(rec, httptest.NewRequest(http.MethodGet, "/api/v1/accounts/Soulmate/auth/status", nil), "/api/v1/accounts/Soulmate/auth/status")

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var body struct {
		Status string `json:"status"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Status != "idle" {
		t.Errorf("expected status idle, got %q", body.Status)
	}
}

func TestAccountByNameReturnsConfiguredAccount(t *testing.T) {
	s := testServer(t)

	account, err := s.accountByName("Soulmate")
	if err != nil {
		t.Fatalf("accountByName: %v", err)
	}
	if account.Username != "michael.brundin@soulmate.se" {
		t.Errorf("unexpected username %q", account.Username)
	}

	if _, err := s.accountByName("Unknown"); err == nil {
		t.Error("expected an error for an unknown account")
	}
}

func TestAccountByNameFallsBackToFirstAccount(t *testing.T) {
	s := testServer(t)

	account, err := s.accountByName("")
	if err != nil {
		t.Fatalf("accountByName: %v", err)
	}
	if account.Name != "Michael Brundins" {
		t.Errorf("expected the first account, got %q", account.Name)
	}
}

func TestCredentialForPasswordAccountReadsEnvironment(t *testing.T) {
	t.Setenv("TEST_MAIL_PASSWORD_MICHAEL", "secret")

	s := testServer(t)

	credential, err := credentialFor(&s.cfg.Mail[0])
	if err != nil {
		t.Fatalf("credentialFor: %v", err)
	}
	if credential != "secret" {
		t.Errorf("expected secret, got %q", credential)
	}
}

func TestCredentialForOAuthAccountFailsWithoutToken(t *testing.T) {
	s := testServer(t)

	_, err := credentialFor(&s.cfg.Mail[1])
	if err == nil {
		t.Fatal("expected an error without a cached token")
	}
	if !strings.Contains(err.Error(), "not logged in") {
		t.Errorf("expected the error to say the account is not logged in, got %v", err)
	}
}

func TestSessionStoreKeepsSessionPerAccount(t *testing.T) {
	store := newSessionStore()

	store.put("A", &accountSession{credential: "a"})
	store.put("B", &accountSession{credential: "b"})

	session, ok := store.get("A")
	if !ok {
		t.Fatal("expected a session for A")
	}
	if session.credential != "a" {
		t.Errorf("expected credential a, got %q", session.credential)
	}

	store.remove("A")
	if _, ok := store.get("A"); ok {
		t.Error("expected the session for A to be gone")
	}
	if _, ok := store.get("B"); !ok {
		t.Error("expected the session for B to be kept")
	}
}

func TestMarkBrokenFlagsSessionForReconnect(t *testing.T) {
	store := newSessionStore()
	store.put("A", &accountSession{credential: "a"})

	if store.isBroken("A") {
		t.Error("expected a fresh session not to be broken")
	}

	store.markBroken("A")

	if !store.isBroken("A") {
		t.Error("expected the session to be marked as broken")
	}
}

func TestIsConnectionError(t *testing.T) {
	tests := []struct {
		err  error
		want bool
	}{
		{nil, false},
		{errors.New("failed to select folder INBOX: no such mailbox"), false},
		{errors.New("write tcp 1.2.3.4:5000->5.6.7.8:993: write: broken pipe"), true},
		{errors.New("read tcp: connection reset by peer"), true},
		{errors.New("use of closed network connection"), true},
		{io.EOF, true},
		{net.ErrClosed, true},
	}

	for _, tt := range tests {
		if got := isConnectionError(tt.err); got != tt.want {
			t.Errorf("isConnectionError(%v) = %v, want %v", tt.err, got, tt.want)
		}
	}
}

func TestEnsureConnectionForReconnectsBrokenSession(t *testing.T) {
	t.Setenv("TEST_MAIL_PASSWORD_MICHAEL", "secret")

	s := testServer(t)

	// Point the account at a port that refuses connections, so the reconnect
	// fails fast and without touching the network.
	s.cfg.Mail[0].Server = "127.0.0.1"
	s.cfg.Mail[0].Port = 1

	// A broken session must not be reused.
	s.sessions.put("Michael Brundins", &accountSession{
		account:    &s.cfg.Mail[0],
		broken:     true,
		credential: "secret",
	})

	if _, err := s.ensureConnectionFor("Michael Brundins"); err == nil {
		t.Fatal("expected the reconnect to fail against an unreachable server")
	}

	// The broken session must have been discarded.
	if _, ok := s.sessions.get("Michael Brundins"); ok {
		t.Error("expected the broken session to be discarded")
	}
}

func TestFolderForAccountUsesConfiguredFolder(t *testing.T) {
	s := testServer(t)

	if got := s.defaultFolder(&s.cfg.Mail[0]); got != "INBOX.Simon" {
		t.Errorf("expected INBOX.Simon, got %q", got)
	}
	if got := s.defaultFolder(&s.cfg.Mail[1]); got != "INBOX" {
		t.Errorf("expected INBOX, got %q", got)
	}
	if got := s.defaultFolder(nil); got != "INBOX" {
		t.Errorf("expected INBOX, got %q", got)
	}
}

// timeNowPlusHour is a small helper keeping the test readable.
func timeNowPlusHour() time.Time {
	return time.Now().Add(time.Hour)
}
