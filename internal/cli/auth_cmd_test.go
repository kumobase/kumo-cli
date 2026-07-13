package cli

import (
	"net/http"
	"strings"
	"testing"
)

func TestAuthLoginAPIKey(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/profile", handleProfile)
	srv := newServer(t, mux)
	mockEnv(t, srv.URL)
	// Clear the env key so login must rely on --api-key and persist it.
	t.Setenv("KUMO_API_KEY", "")

	out, _, err := runCLI("auth", "login", "--api-key", "kumo_sk_logintest")
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	if !strings.Contains(out, "Test User") || !strings.Contains(out, "test@example.com") {
		t.Errorf("unexpected login output: %q", out)
	}

	// The key should now be saved; whoami should succeed without --api-key.
	out, _, err = runCLI("auth", "whoami")
	if err != nil {
		t.Fatalf("whoami after login: %v", err)
	}
	if !strings.Contains(out, "test@example.com") {
		t.Errorf("whoami output missing email: %q", out)
	}
}

func TestAuthLoginRejectsBadKey(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/profile", func(w http.ResponseWriter, _ *http.Request) {
		writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "invalid api key")
	})
	srv := newServer(t, mux)
	mockEnv(t, srv.URL)
	t.Setenv("KUMO_API_KEY", "")

	if _, _, err := runCLI("auth", "login", "--api-key", "kumo_sk_bad"); err == nil {
		t.Fatal("expected login to fail for an invalid key")
	}
}

func TestAuthWhoami(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/profile", handleProfile)
	srv := newServer(t, mux)
	mockEnv(t, srv.URL)

	out, _, err := runCLI("auth", "whoami")
	if err != nil {
		t.Fatalf("whoami: %v", err)
	}
	// API key should be masked, never printed in full.
	if strings.Contains(out, "kumo_sk_test1234567890") {
		t.Errorf("whoami leaked the full API key: %q", out)
	}
	if !strings.Contains(out, "test@example.com") || !strings.Contains(out, "Test User") {
		t.Errorf("unexpected whoami output: %q", out)
	}
}

func TestAuthWhoamiJSON(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/profile", handleProfile)
	srv := newServer(t, mux)
	mockEnv(t, srv.URL)

	out, _, err := runCLI("auth", "whoami", "--output", "json")
	if err != nil {
		t.Fatalf("whoami json: %v", err)
	}
	if !strings.Contains(out, `"email": "test@example.com"`) {
		t.Errorf("unexpected whoami json: %q", out)
	}
}

func TestAuthLogout(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/profile", handleProfile)
	srv := newServer(t, mux)
	mockEnv(t, srv.URL)
	t.Setenv("KUMO_API_KEY", "")

	// Log in to persist a credential, then log out.
	if _, _, err := runCLI("auth", "login", "--api-key", "kumo_sk_logintest"); err != nil {
		t.Fatalf("login: %v", err)
	}
	out, _, err := runCLI("auth", "logout")
	if err != nil {
		t.Fatalf("logout: %v", err)
	}
	if !strings.Contains(out, "Logged out") {
		t.Errorf("unexpected logout output: %q", out)
	}

	// A second logout reports nothing stored.
	out, _, err = runCLI("auth", "logout")
	if err != nil {
		t.Fatalf("second logout: %v", err)
	}
	if !strings.Contains(out, "No credentials stored") {
		t.Errorf("unexpected second logout output: %q", out)
	}
}
