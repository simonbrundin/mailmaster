package oauth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// withTestEndpoints points the Microsoft endpoints at a local test server and
// restores them afterwards.
func withTestEndpoints(t *testing.T, deviceURL, tokenURL string) {
	t.Helper()

	oldDevice := deviceCodeEndpoint
	oldToken := tokenEndpoint

	deviceCodeEndpoint = func(string) string { return deviceURL }
	tokenEndpoint = func(string) string { return tokenURL }

	t.Cleanup(func() {
		deviceCodeEndpoint = oldDevice
		tokenEndpoint = oldToken
	})
}

func TestStartDeviceLoginReturnsUserCode(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Errorf("parse form: %v", err)
		}
		if r.Form.Get("client_id") != "client-123" {
			t.Errorf("unexpected client_id %q", r.Form.Get("client_id"))
		}
		if r.Form.Get("scope") == "" {
			t.Error("expected a scope to be requested")
		}
		json.NewEncoder(w).Encode(map[string]interface{}{
			"device_code":      "device-abc",
			"user_code":        "ABCD-1234",
			"verification_uri": "https://microsoft.com/devicelogin",
			"interval":         7,
			"expires_in":       900,
		})
	}))
	defer server.Close()

	withTestEndpoints(t, server.URL, server.URL)

	cfg := Config{
		ClientID: "client-123",
		TenantID: "tenant-1",
		Username: "user@example.com",
	}

	flow, err := StartDeviceLogin(context.Background(), cfg)
	if err != nil {
		t.Fatalf("StartDeviceLogin: %v", err)
	}
	if flow.UserCode != "ABCD-1234" {
		t.Errorf("expected user code ABCD-1234, got %q", flow.UserCode)
	}
	if flow.DeviceCode != "device-abc" {
		t.Errorf("expected device code device-abc, got %q", flow.DeviceCode)
	}
	if flow.Interval != 7 {
		t.Errorf("expected interval 7, got %d", flow.Interval)
	}
}

func TestStartDeviceLoginFailsOnErrorResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error":             "invalid_client",
			"error_description": "unknown client",
		})
	}))
	defer server.Close()

	withTestEndpoints(t, server.URL, server.URL)

	_, err := StartDeviceLogin(context.Background(), Config{
		ClientID: "client-123",
		TenantID: "tenant-1",
		Username: "user@example.com",
	})
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), "invalid_client") {
		t.Errorf("expected the error to mention invalid_client, got %v", err)
	}
}

func TestCompleteDeviceLoginPending(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error":             "authorization_pending",
			"error_description": "waiting for the user",
		})
	}))
	defer server.Close()

	withTestEndpoints(t, server.URL, server.URL)

	_, err := CompleteDeviceLogin(context.Background(), Config{
		ClientID: "client-123",
		TenantID: "tenant-1",
		Username: "user@example.com",
	}, &DeviceFlow{DeviceCode: "device-abc"})
	if err != ErrAuthorizationPending {
		t.Fatalf("expected ErrAuthorizationPending, got %v", err)
	}
}

func TestCompleteDeviceLoginSavesToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Errorf("parse form: %v", err)
		}
		if r.Form.Get("grant_type") != "urn:ietf:params:oauth:grant-type:device_code" {
			t.Errorf("unexpected grant_type %q", r.Form.Get("grant_type"))
		}
		json.NewEncoder(w).Encode(map[string]interface{}{
			"access_token":  "access-token-1",
			"token_type":    "Bearer",
			"expires_in":    3600,
			"refresh_token": "refresh-token-1",
		})
	}))
	defer server.Close()

	withTestEndpoints(t, server.URL, server.URL)

	cachePath := filepath.Join(t.TempDir(), "token.json")
	cfg := Config{
		ClientID:  "client-123",
		TenantID:  "tenant-1",
		Username:  "user@example.com",
		CachePath: cachePath,
	}

	token, err := CompleteDeviceLogin(context.Background(), cfg, &DeviceFlow{DeviceCode: "device-abc"})
	if err != nil {
		t.Fatalf("CompleteDeviceLogin: %v", err)
	}
	if token != "access-token-1" {
		t.Errorf("expected access token access-token-1, got %q", token)
	}

	saved, err := LoadToken(cachePath)
	if err != nil {
		t.Fatalf("LoadToken: %v", err)
	}
	if saved.RefreshToken != "refresh-token-1" {
		t.Errorf("expected the refresh token to be saved, got %q", saved.RefreshToken)
	}
	if !HasCachedToken(cfg) {
		t.Error("expected HasCachedToken to be true after a successful login")
	}
}

func TestCompleteDeviceLoginExpired(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error": "expired_token",
		})
	}))
	defer server.Close()

	withTestEndpoints(t, server.URL, server.URL)

	_, err := CompleteDeviceLogin(context.Background(), Config{
		ClientID: "client-123",
		TenantID: "tenant-1",
		Username: "user@example.com",
	}, &DeviceFlow{DeviceCode: "device-abc"})
	if err != ErrExpired {
		t.Fatalf("expected ErrExpired, got %v", err)
	}
}

func TestCachedTokenWithoutCacheFails(t *testing.T) {
	cfg := Config{
		ClientID:  "client-123",
		TenantID:  "tenant-1",
		Username:  "user@example.com",
		CachePath: filepath.Join(t.TempDir(), "missing.json"),
	}

	if _, err := CachedToken(cfg); err == nil {
		t.Fatal("expected an error when there is no cached token")
	}
	if HasCachedToken(cfg) {
		t.Error("expected HasCachedToken to be false without a cached token")
	}
}

func TestCachedTokenRefreshesExpiredToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Errorf("parse form: %v", err)
		}
		if r.Form.Get("grant_type") != "refresh_token" {
			t.Errorf("unexpected grant_type %q", r.Form.Get("grant_type"))
		}
		json.NewEncoder(w).Encode(map[string]interface{}{
			"access_token": "fresh-token",
			"expires_in":   3600,
		})
	}))
	defer server.Close()

	withTestEndpoints(t, server.URL, server.URL)

	cachePath := filepath.Join(t.TempDir(), "token.json")
	expired := &Token{
		AccessToken:  "old-token",
		RefreshToken: "refresh-token-1",
		ExpiresAt:    time.Now().Add(-time.Hour),
	}
	if err := SaveToken(cachePath, expired); err != nil {
		t.Fatalf("SaveToken: %v", err)
	}

	cfg := Config{
		ClientID:  "client-123",
		TenantID:  "tenant-1",
		Username:  "user@example.com",
		CachePath: cachePath,
	}

	token, err := CachedToken(cfg)
	if err != nil {
		t.Fatalf("CachedToken: %v", err)
	}
	if token != "fresh-token" {
		t.Errorf("expected fresh-token, got %q", token)
	}

	// The old refresh token must be kept when Microsoft does not send a new one.
	saved, err := LoadToken(cachePath)
	if err != nil {
		t.Fatalf("LoadToken: %v", err)
	}
	if saved.RefreshToken != "refresh-token-1" {
		t.Errorf("expected the refresh token to be kept, got %q", saved.RefreshToken)
	}
}

func TestSaveTokenUsesRestrictivePermissions(t *testing.T) {
	cachePath := filepath.Join(t.TempDir(), "token.json")
	if err := SaveToken(cachePath, &Token{AccessToken: "x"}); err != nil {
		t.Fatalf("SaveToken: %v", err)
	}

	info, err := os.Stat(cachePath)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0600 {
		t.Errorf("expected permissions 0600, got %o", perm)
	}
}

func TestDefaultCachePathDependsOnClientAndUser(t *testing.T) {
	first := DefaultCachePath("client-1", "user@example.com")
	second := DefaultCachePath("client-2", "user@example.com")
	if first == second {
		t.Error("expected different cache paths for different clients")
	}
	if first != DefaultCachePath("client-1", "user@example.com") {
		t.Error("expected the cache path to be stable")
	}
}
