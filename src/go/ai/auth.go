package ai

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
	"time"
)

const defaultOAuthScopes = "openid profile offline_access https://outlook.office.com/IMAP.AccessAsUser.All https://outlook.office.com/SMTP.Send"

// OAuthConfig configures an interactive Microsoft identity login.
type OAuthConfig struct {
	ClientID    string
	TenantID    string
	Username    string
	RedirectURI string
	CachePath   string
	Scopes      string
}

// OAuth2Token is an access/refresh token pair returned by Microsoft Entra ID.
type OAuth2Token struct {
	AccessToken  string    `json:"access_token"`
	TokenType    string    `json:"token_type"`
	ExpiresIn    int       `json:"expires_in"`
	RefreshToken string    `json:"refresh_token,omitempty"`
	ExpiresAt    time.Time `json:"expires_at"`
}

// GetOAuthAccessToken returns a cached token, refreshes it when possible, or
// starts an interactive Authorization Code + PKCE flow.
func GetOAuthAccessToken(cfg OAuthConfig) (string, error) {
	if cfg.ClientID == "" || cfg.TenantID == "" || cfg.Username == "" {
		return "", errors.New("OAuth2 requires client ID, tenant ID and username")
	}
	if cfg.RedirectURI == "" {
		cfg.RedirectURI = "http://localhost:8400/callback"
	}
	if cfg.Scopes == "" {
		cfg.Scopes = defaultOAuthScopes
	}
	if cfg.CachePath == "" {
		cfg.CachePath = defaultTokenCachePath(cfg.ClientID, cfg.Username)
	}

	if token, err := loadToken(cfg.CachePath); err == nil {
		if accessToken, ok := validAccessToken(token); ok {
			return accessToken, nil
		}
		if token.RefreshToken != "" {
			refreshed, refreshErr := refreshOAuthToken(cfg, token.RefreshToken)
			if refreshErr == nil {
				if refreshed.RefreshToken == "" {
					refreshed.RefreshToken = token.RefreshToken
				}
				if err := saveToken(cfg.CachePath, refreshed); err != nil {
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
	if err := saveToken(cfg.CachePath, token); err != nil {
		return "", fmt.Errorf("save OAuth token: %w", err)
	}
	return token.AccessToken, nil
}

func authorizationCodePKCE(cfg OAuthConfig) (*OAuth2Token, error) {
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
		fmt.Fprintln(w, "Inloggningen lyckades. Du kan stänga detta fönster.")
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

func buildAuthorizeURL(cfg OAuthConfig, state, challenge string) (string, error) {
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

func exchangeAuthorizationCode(cfg OAuthConfig, code, verifier string) (*OAuth2Token, error) {
	endpoint := tokenEndpoint(cfg.TenantID)
	form := url.Values{}
	form.Set("client_id", cfg.ClientID)
	form.Set("grant_type", "authorization_code")
	form.Set("code", code)
	form.Set("redirect_uri", cfg.RedirectURI)
	form.Set("code_verifier", verifier)
	form.Set("scope", cfg.Scopes)

	return postToken(endpoint, form)
}

func refreshOAuthToken(cfg OAuthConfig, refreshToken string) (*OAuth2Token, error) {
	form := url.Values{}
	form.Set("client_id", cfg.ClientID)
	form.Set("grant_type", "refresh_token")
	form.Set("refresh_token", refreshToken)
	form.Set("scope", cfg.Scopes)
	return postToken(tokenEndpoint(cfg.TenantID), form)
}

func postToken(endpoint string, form url.Values) (*OAuth2Token, error) {
	resp, err := http.PostForm(endpoint, form)
	if err != nil {
		return nil, fmt.Errorf("OAuth token request: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read OAuth token response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var oauthErr struct {
			Error       string `json:"error"`
			Description string `json:"error_description"`
		}
		_ = json.Unmarshal(body, &oauthErr)
		return nil, fmt.Errorf("OAuth token request failed (%s): %s", oauthErr.Error, oauthErr.Description)
	}

	var token OAuth2Token
	if err := json.Unmarshal(body, &token); err != nil {
		return nil, fmt.Errorf("decode OAuth token response: %w", err)
	}
	if token.AccessToken == "" {
		return nil, errors.New("OAuth token response did not contain an access token")
	}
	if token.ExpiresIn > 0 {
		token.ExpiresAt = time.Now().Add(time.Duration(token.ExpiresIn-60) * time.Second)
	}
	return &token, nil
}

func tokenEndpoint(tenantID string) string {
	return fmt.Sprintf("https://login.microsoftonline.com/%s/oauth2/v2.0/token", url.PathEscape(tenantID))
}

func validAccessToken(token *OAuth2Token) (string, bool) {
	if token == nil || token.AccessToken == "" || token.ExpiresAt.IsZero() {
		return "", false
	}
	if !time.Now().Before(token.ExpiresAt) {
		return "", false
	}
	return token.AccessToken, true
}

func loadToken(path string) (*OAuth2Token, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var token OAuth2Token
	if err := json.Unmarshal(data, &token); err != nil {
		return nil, err
	}
	return &token, nil
}

func saveToken(path string, token *OAuth2Token) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(token, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}

func defaultTokenCachePath(clientID, username string) string {
	hash := sha256.Sum256([]byte(clientID + "\x00" + username))
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		cacheDir = ".cache"
	}
	return filepath.Join(cacheDir, "mailmaster", fmt.Sprintf("oauth-%x.json", hash[:8]))
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

// DefaultOAuthScopes returns the delegated Exchange Online scopes needed by
// the IMAP and SMTP clients.
func DefaultOAuthScopes() string {
	return defaultOAuthScopes
}
