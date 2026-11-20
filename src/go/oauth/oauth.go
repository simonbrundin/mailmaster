// Package oauth handles Microsoft Entra ID (Azure AD) OAuth2 logins for mail
// accounts. It supports two flows:
//
//   - Device code flow: the backend asks Microsoft for a code, the user enters
//     it in a browser on any device. This is the flow used by the web UI,
//     because the browser does not have to run on the same machine as the
//     server.
//   - Authorization Code + PKCE: a local browser is opened on the server
//     machine. Useful for command line usage.
//
// Access tokens are cached on disk per client ID and username so a restart
// does not require a new login.
package oauth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// DefaultScopes are the delegated Exchange Online permissions needed to read
// and send mail on behalf of the signed-in user.
const DefaultScopes = "openid profile offline_access https://outlook.office.com/IMAP.AccessAsUser.All https://outlook.office.com/SMTP.Send"

// ErrAuthorizationPending is returned while the user has not yet completed
// the device code login.
var ErrAuthorizationPending = errors.New("authorization pending")

// ErrExpired is returned when the device code expired before the user
// completed the login.
var ErrExpired = errors.New("device code expired")

// The Microsoft endpoints. They are variables so tests can point them at a
// local server.
var (
	deviceCodeEndpoint = func(tenantID string) string {
		return fmt.Sprintf("https://login.microsoftonline.com/%s/oauth2/v2.0/devicecode", url.PathEscape(tenantID))
	}
	tokenEndpoint = func(tenantID string) string {
		return fmt.Sprintf("https://login.microsoftonline.com/%s/oauth2/v2.0/token", url.PathEscape(tenantID))
	}
)

// Config describes which Entra ID app and account the login is for.
type Config struct {
	ClientID    string
	TenantID    string
	Username    string
	RedirectURI string
	CachePath   string
	Scopes      string
}

// Token is an access/refresh token pair returned by Microsoft Entra ID.
type Token struct {
	AccessToken  string    `json:"access_token"`
	TokenType    string    `json:"token_type"`
	ExpiresIn    int       `json:"expires_in"`
	RefreshToken string    `json:"refresh_token,omitempty"`
	ExpiresAt    time.Time `json:"expires_at"`
}

// DeviceFlow is a started device code login.
type DeviceFlow struct {
	// DeviceCode is the code the backend polls the token endpoint with.
	DeviceCode string
	// UserCode is the short code the user types into the browser.
	UserCode string
	// VerificationURI is where the user enters the code.
	VerificationURI string
	// Message is Microsoft's own instruction text.
	Message string
	// Interval is the minimum number of seconds between two polls.
	Interval int
	// ExpiresIn is how long the code stays valid, in seconds.
	ExpiresIn int
}

// withDefaults fills in the default values for redirect URI, scopes and the
// token cache path.
func (c Config) withDefaults() Config {
	if c.RedirectURI == "" {
		c.RedirectURI = "http://localhost:8400/callback"
	}
	if c.Scopes == "" {
		c.Scopes = DefaultScopes
	}
	if c.CachePath == "" {
		c.CachePath = DefaultCachePath(c.ClientID, c.Username)
	}
	return c
}

// Validate checks that the config has everything needed for a login.
func (c Config) Validate() error {
	if c.ClientID == "" || c.TenantID == "" || c.Username == "" {
		return errors.New("OAuth2 requires client ID, tenant ID and username")
	}
	return nil
}

// DefaultCachePath returns the token cache path used for a client/user pair.
func DefaultCachePath(clientID, username string) string {
	hash := sha256.Sum256([]byte(clientID + "\x00" + username))
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		cacheDir = ".cache"
	}
	return filepath.Join(cacheDir, "mailmaster", fmt.Sprintf("oauth-%x.json", hash[:8]))
}

// CachedToken returns a valid access token from the cache, refreshing it with
// the refresh token when the cached one has expired. It never starts an
// interactive login, so it is safe to call from a request handler.
func CachedToken(cfg Config) (string, error) {
	cfg = cfg.withDefaults()
	if err := cfg.Validate(); err != nil {
		return "", err
	}

	token, err := LoadToken(cfg.CachePath)
	if err != nil {
		return "", fmt.Errorf("no cached OAuth token: %w", err)
	}
	if accessToken, ok := validAccessToken(token); ok {
		return accessToken, nil
	}
	if token.RefreshToken == "" {
		return "", errors.New("cached OAuth token has expired and cannot be refreshed")
	}

	refreshed, err := refreshToken(cfg, token.RefreshToken)
	if err != nil {
		return "", fmt.Errorf("refresh OAuth token: %w", err)
	}
	if refreshed.RefreshToken == "" {
		refreshed.RefreshToken = token.RefreshToken
	}
	if err := SaveToken(cfg.CachePath, refreshed); err != nil {
		return "", fmt.Errorf("save refreshed OAuth token: %w", err)
	}
	return refreshed.AccessToken, nil
}

// HasCachedToken reports whether a usable token is stored on disk. It does
// not contact Microsoft and never refreshes.
func HasCachedToken(cfg Config) bool {
	cfg = cfg.withDefaults()
	token, err := LoadToken(cfg.CachePath)
	if err != nil {
		return false
	}
	if _, ok := validAccessToken(token); ok {
		return true
	}
	return token.RefreshToken != ""
}

// StartDeviceLogin asks Microsoft for a device code. The returned code is
// shown to the user, who enters it in a browser.
func StartDeviceLogin(ctx context.Context, cfg Config) (*DeviceFlow, error) {
	cfg = cfg.withDefaults()
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	form := url.Values{}
	form.Set("client_id", cfg.ClientID)
	form.Set("scope", cfg.Scopes)

	endpoint := deviceCodeEndpoint(cfg.TenantID)
	var response struct {
		DeviceCode      string `json:"device_code"`
		UserCode        string `json:"user_code"`
		VerificationURI string `json:"verification_uri"`
		Message         string `json:"message"`
		Interval        int    `json:"interval"`
		ExpiresIn       int    `json:"expires_in"`
		Error           string `json:"error"`
		Description     string `json:"error_description"`
	}
	if err := postForm(ctx, endpoint, form, &response); err != nil {
		return nil, err
	}
	if response.Error != "" {
		return nil, fmt.Errorf("device code request failed (%s): %s", response.Error, response.Description)
	}
	if response.DeviceCode == "" || response.UserCode == "" {
		return nil, errors.New("device code response was incomplete")
	}
	if response.Interval <= 0 {
		response.Interval = 5
	}

	return &DeviceFlow{
		DeviceCode:      response.DeviceCode,
		UserCode:        response.UserCode,
		VerificationURI: response.VerificationURI,
		Message:         response.Message,
		Interval:        response.Interval,
		ExpiresIn:       response.ExpiresIn,
	}, nil
}

// CompleteDeviceLogin polls the token endpoint once. It returns
// ErrAuthorizationPending while the user has not finished the login and
// ErrExpired when the code is no longer valid.
func CompleteDeviceLogin(ctx context.Context, cfg Config, flow *DeviceFlow) (string, error) {
	cfg = cfg.withDefaults()
	if flow == nil || flow.DeviceCode == "" {
		return "", errors.New("no device code to complete")
	}

	form := url.Values{}
	form.Set("client_id", cfg.ClientID)
	form.Set("grant_type", "urn:ietf:params:oauth:grant-type:device_code")
	form.Set("device_code", flow.DeviceCode)

	var response struct {
		AccessToken  string `json:"access_token"`
		TokenType    string `json:"token_type"`
		ExpiresIn    int    `json:"expires_in"`
		RefreshToken string `json:"refresh_token"`
		Error        string `json:"error"`
		Description  string `json:"error_description"`
	}
	err := postForm(ctx, tokenEndpoint(cfg.TenantID), form, &response)
	if err != nil {
		switch {
		case strings.Contains(err.Error(), "authorization_pending"):
			return "", ErrAuthorizationPending
		case strings.Contains(err.Error(), "expired_token"):
			return "", ErrExpired
		case strings.Contains(err.Error(), "access_denied"):
			return "", errors.New("login was declined")
		}
		return "", err
	}
	if response.AccessToken == "" {
		return "", errors.New("token response did not contain an access token")
	}

	token := &Token{
		AccessToken:  response.AccessToken,
		TokenType:    response.TokenType,
		ExpiresIn:    response.ExpiresIn,
		RefreshToken: response.RefreshToken,
	}
	if token.ExpiresIn > 0 {
		token.ExpiresAt = time.Now().Add(time.Duration(token.ExpiresIn-60) * time.Second)
	}
	if err := SaveToken(cfg.CachePath, token); err != nil {
		return "", fmt.Errorf("save OAuth token: %w", err)
	}
	return token.AccessToken, nil
}

// GetAccessToken returns a cached token, refreshes it when possible, or
// starts an interactive Authorization Code + PKCE flow.
func GetAccessToken(cfg Config) (string, error) {
	cfg = cfg.withDefaults()
	if err := cfg.Validate(); err != nil {
		return "", err
	}

	if token, err := LoadToken(cfg.CachePath); err == nil {
		if accessToken, ok := validAccessToken(token); ok {
			return accessToken, nil
		}
		if token.RefreshToken != "" {
			refreshed, refreshErr := refreshToken(cfg, token.RefreshToken)
			if refreshErr == nil {
				if refreshed.RefreshToken == "" {
					refreshed.RefreshToken = token.RefreshToken
				}
				if err := SaveToken(cfg.CachePath, refreshed); err != nil {
					return "", fmt.Errorf("save refreshed OAuth token: %w", err)
				}
				return refreshed.AccessToken, nil
			}
			fmt.Fprintf(os.Stderr, "OAuth-token kunde inte förnyas, startar interaktiv inloggning: %v\n", refreshErr)
		}
	}

	token, err := authorizationCodePKCE(cfg)
	if err != nil {
		return "", err
	}
	if err := SaveToken(cfg.CachePath, token); err != nil {
		return "", fmt.Errorf("save OAuth token: %w", err)
	}
	return token.AccessToken, nil
}

func authorizationCodePKCE(cfg Config) (*Token, error) {
	redirect, err := url.Parse(cfg.RedirectURI)
	if err != nil || redirect.Scheme != "http" || redirect.Hostname() != "localhost" {
		return nil, fmt.Errorf("OAuth redirect URI must be a localhost HTTP URI, got %q", cfg.RedirectURI)
	}

	listener, err := net.Listen("tcp", redirect.Host)
	if err != nil {
		return nil, fmt.Errorf("listen on OAuth redirect %s: %w", redirect.Host, err)
	}
	defer listener.Close()

	state, err := randomURLValue(32)
	if err != nil {
		return nil, fmt.Errorf("generate OAuth state: %w", err)
	}
	verifier, err := randomURLValue(64)
	if err != nil {
		return nil, fmt.Errorf("generate PKCE verifier: %w", err)
	}
	hash := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(hash[:])

	callbackCode := make(chan string, 1)
	callbackErr := make(chan error, 1)
	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != redirect.Path {
			http.NotFound(w, r)
			return
		}
		if r.URL.Query().Get("state") != state {
			callbackErr <- errors.New("OAuth state validation failed")
			http.Error(w, "Invalid OAuth state", http.StatusBadRequest)
			return
		}
		if oauthErr := r.URL.Query().Get("error"); oauthErr != "" {
			description := r.URL.Query().Get("error_description")
			callbackErr <- fmt.Errorf("OAuth authorization failed: %s: %s", oauthErr, description)
			fmt.Fprintln(w, "Inloggningen misslyckades. Du kan stänga fönstret.")
			return
		}
		code := r.URL.Query().Get("code")
		if code == "" {
			callbackErr <- errors.New("OAuth callback did not contain an authorization code")
			http.Error(w, "Missing authorization code", http.StatusBadRequest)
			return
		}
		callbackCode <- code
		fmt.Fprintln(w, "Inloggningen lyckades. Du kan stänga detta fönstret.")
	})}

	go func() {
		if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			callbackErr <- fmt.Errorf("OAuth callback server: %w", err)
		}
	}()
	defer server.Shutdown(context.Background())

	authorizeURL, err := buildAuthorizeURL(cfg, state, challenge)
	if err != nil {
		return nil, err
	}
	fmt.Printf("\nÖppnar Microsoft-inloggning för %s. Om webbläsaren inte öppnas, öppna:\n%s\n\n", cfg.Username, authorizeURL)
	_ = openBrowser(authorizeURL)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	select {
	case code := <-callbackCode:
		return exchangeAuthorizationCode(cfg, code, verifier)
	case err := <-callbackErr:
		return nil, err
	case <-ctx.Done():
		return nil, errors.New("OAuth login timed out")
	}
}

func buildAuthorizeURL(cfg Config, state, challenge string) (string, error) {
	u := url.URL{
		Scheme: "https",
		Host:   "login.microsoftonline.com",
		Path:   "/" + url.PathEscape(cfg.TenantID) + "/oauth2/v2.0/authorize",
	}
	query := u.Query()
	query.Set("client_id", cfg.ClientID)
	query.Set("response_type", "code")
	query.Set("redirect_uri", cfg.RedirectURI)
	query.Set("response_mode", "query")
	query.Set("scope", cfg.Scopes)
	query.Set("state", state)
	query.Set("code_challenge", challenge)
	query.Set("code_challenge_method", "S256")
	u.RawQuery = query.Encode()
	return u.String(), nil
}

func exchangeAuthorizationCode(cfg Config, code, verifier string) (*Token, error) {
	form := url.Values{}
	form.Set("client_id", cfg.ClientID)
	form.Set("grant_type", "authorization_code")
	form.Set("code", code)
	form.Set("redirect_uri", cfg.RedirectURI)
	form.Set("code_verifier", verifier)
	form.Set("scope", cfg.Scopes)

	return postToken(tokenEndpoint(cfg.TenantID), form)
}

func refreshToken(cfg Config, refreshToken string) (*Token, error) {
	form := url.Values{}
	form.Set("client_id", cfg.ClientID)
	form.Set("grant_type", "refresh_token")
	form.Set("refresh_token", refreshToken)
	form.Set("scope", cfg.Scopes)
	return postToken(tokenEndpoint(cfg.TenantID), form)
}

func postToken(endpoint string, form url.Values) (*Token, error) {
	var response struct {
		AccessToken  string `json:"access_token"`
		TokenType    string `json:"token_type"`
		ExpiresIn    int    `json:"expires_in"`
		RefreshToken string `json:"refresh_token"`
		Error        string `json:"error"`
		Description  string `json:"error_description"`
	}
	if err := postForm(context.Background(), endpoint, form, &response); err != nil {
		return nil, err
	}
	if response.AccessToken == "" {
		return nil, errors.New("OAuth token response did not contain an access token")
	}

	token := &Token{
		AccessToken:  response.AccessToken,
		TokenType:    response.TokenType,
		ExpiresIn:    response.ExpiresIn,
		RefreshToken: response.RefreshToken,
	}
	if token.ExpiresIn > 0 {
		token.ExpiresAt = time.Now().Add(time.Duration(token.ExpiresIn-60) * time.Second)
	}
	return token, nil
}

// postForm posts a form and decodes a JSON response into result.
func postForm(ctx context.Context, endpoint string, form url.Values, result interface{}) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return fmt.Errorf("create OAuth request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("OAuth request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read OAuth response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var oauthErr struct {
			Error       string `json:"error"`
			Description string `json:"error_description"`
		}
		_ = json.Unmarshal(body, &oauthErr)
		return fmt.Errorf("OAuth request failed (%s): %s", oauthErr.Error, oauthErr.Description)
	}

	if err := json.Unmarshal(body, result); err != nil {
		return fmt.Errorf("decode OAuth response: %w", err)
	}
	return nil
}

func validAccessToken(token *Token) (string, bool) {
	if token == nil || token.AccessToken == "" || token.ExpiresAt.IsZero() {
		return "", false
	}
	if !time.Now().Before(token.ExpiresAt) {
		return "", false
	}
	return token.AccessToken, true
}

// LoadToken reads a cached token from disk.
func LoadToken(path string) (*Token, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var token Token
	if err := json.Unmarshal(data, &token); err != nil {
		return nil, err
	}
	return &token, nil
}

// SaveToken writes a token to the cache with restrictive permissions.
func SaveToken(path string, token *Token) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(token, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}

func randomURLValue(size int) (string, error) {
	buf := make([]byte, size)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func openBrowser(target string) error {
	var command string
	var args []string
	switch runtime.GOOS {
	case "linux":
		command, args = "xdg-open", []string{target}
	case "darwin":
		command, args = "open", []string{target}
	case "windows":
		command, args = "rundll32", []string{"url.dll,FileProtocolHandler", target}
	default:
		return errors.New("unsupported operating system")
	}
	return exec.Command(command, args...).Start()
}
