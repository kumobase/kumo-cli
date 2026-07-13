package cli

import (
	"net/http"
	"strings"
	"testing"
)

const sourceConnJSON = `{"id":3,"provider":"github","installation_id":111,"account_login":"acme",` +
	`"account_type":"Organization","repo_selection":"selected","repo_count":2,` +
	`"status":"active","app_kind":"build","created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-02T00:00:00Z"}`

func TestSourceList(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/source-connections", func(w http.ResponseWriter, _ *http.Request) {
		writeEnvelope(w, http.StatusOK, "["+sourceConnJSON+"]")
	})
	srv := newServer(t, mux)
	mockEnv(t, srv.URL)

	out, _, err := runCLI("source", "list")
	if err != nil {
		t.Fatalf("source list: %v", err)
	}
	for _, want := range []string{"PROVIDER", "github", "acme", "build", "active", "selected (2)"} {
		if !strings.Contains(out, want) {
			t.Errorf("list missing %q: %s", want, out)
		}
	}
}

func TestSourceRepos(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/source-connections/3/repos", func(w http.ResponseWriter, _ *http.Request) {
		writeEnvelope(w, http.StatusOK,
			`[{"id":1,"full_name":"acme/web","private":true,"default_branch":"main"}]`)
	})
	srv := newServer(t, mux)
	mockEnv(t, srv.URL)

	out, _, err := runCLI("source", "repos", "3")
	if err != nil {
		t.Fatalf("source repos: %v", err)
	}
	for _, want := range []string{"REPO", "acme/web", "main"} {
		if !strings.Contains(out, want) {
			t.Errorf("repos missing %q: %s", want, out)
		}
	}
}

func TestSourceDisconnect(t *testing.T) {
	mux := http.NewServeMux()
	deleted := false
	mux.HandleFunc("DELETE /api/v1/source-connections/3", func(w http.ResponseWriter, _ *http.Request) {
		deleted = true
		writeEnvelope(w, http.StatusOK, sourceConnJSON)
	})
	srv := newServer(t, mux)
	mockEnv(t, srv.URL)

	out, _, err := runCLI("source", "disconnect", "3", "-y")
	if err != nil {
		t.Fatalf("source disconnect: %v", err)
	}
	if !deleted {
		t.Error("expected DELETE to be called")
	}
	if !strings.Contains(out, "disconnected") {
		t.Errorf("unexpected disconnect output: %q", out)
	}
}

func TestSourceDisconnectInUse(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("DELETE /api/v1/source-connections/3", func(w http.ResponseWriter, _ *http.Request) {
		writeError(w, http.StatusConflict, "BUILD_CONNECTION_IN_USE", "in use")
	})
	srv := newServer(t, mux)
	mockEnv(t, srv.URL)

	_, _, err := runCLI("source", "disconnect", "3", "-y")
	if err == nil || !strings.Contains(err.Error(), "still used by a git-build app") {
		t.Fatalf("expected in-use mapping, got: %v", err)
	}
}
