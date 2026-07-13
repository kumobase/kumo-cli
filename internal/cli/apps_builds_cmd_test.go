package cli

import (
	"net/http"
	"strings"
	"testing"
)

// handleAppByName serves the canonical app fixture for name resolution
// (GET /api/v1/apps/web-app → id 42).
func handleAppByName(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("ETag", `W/"abc123"`)
	writeEnvelope(w, http.StatusOK, appDetailJSON)
}

const buildJSON = `{"id":7,"app_id":42,"commit_sha":"abcdef1234567890abcdef","ref":"refs/heads/main",` +
	`"status":"succeeded","image_digest":"sha256:deadbeef","created_at":"2026-07-01T00:00:00Z",` +
	`"finished_at":"2026-07-01T00:05:00Z"}`

func TestAppsBuildsList(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/apps/web-app", handleAppByName)
	mux.HandleFunc("GET /api/v1/apps/42/builds", func(w http.ResponseWriter, _ *http.Request) {
		writeEnvelope(w, http.StatusOK, "["+buildJSON+"]")
	})
	srv := newServer(t, mux)
	mockEnv(t, srv.URL)

	out, _, err := runCLI("apps", "builds", "list", "web-app")
	if err != nil {
		t.Fatalf("apps builds list: %v", err)
	}
	for _, want := range []string{"COMMIT", "abcdef123456", "refs/heads/main", "succeeded"} {
		if !strings.Contains(out, want) {
			t.Errorf("builds list missing %q: %s", want, out)
		}
	}
}

func TestAppsBuildsLogs(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/apps/web-app", handleAppByName)
	mux.HandleFunc("GET /api/v1/apps/42/builds/7/log-url", func(w http.ResponseWriter, _ *http.Request) {
		writeEnvelope(w, http.StatusOK, `{"log_url":"https://logs.example/abc"}`)
	})
	srv := newServer(t, mux)
	mockEnv(t, srv.URL)

	out, _, err := runCLI("apps", "builds", "logs", "web-app", "7")
	if err != nil {
		t.Fatalf("apps builds logs: %v", err)
	}
	if !strings.Contains(out, "https://logs.example/abc") {
		t.Errorf("expected log url, got: %q", out)
	}
}

func TestAppsBuildsRebuild(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/apps/web-app", handleAppByName)
	rebuilt := false
	mux.HandleFunc("POST /api/v1/apps/42/builds", func(w http.ResponseWriter, _ *http.Request) {
		rebuilt = true
		writeEnvelope(w, http.StatusOK, buildJSON)
	})
	srv := newServer(t, mux)
	mockEnv(t, srv.URL)

	out, _, err := runCLI("apps", "builds", "rebuild", "web-app", "-y")
	if err != nil {
		t.Fatalf("apps builds rebuild: %v", err)
	}
	if !rebuilt {
		t.Error("expected rebuild POST to be called")
	}
	if !strings.Contains(out, "Build 7 queued") {
		t.Errorf("unexpected rebuild output: %q", out)
	}
}

func TestAppsCreateGitBuild(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/source-connections/3/apps", func(w http.ResponseWriter, r *http.Request) {
		var req map[string]any
		decodeBody(t, r, &req)
		if req["repo_full_name"] != "acme/web" {
			t.Errorf("repo_full_name = %v, want acme/web", req["repo_full_name"])
		}
		if req["language"] != "railpack" {
			t.Errorf("language = %v, want railpack", req["language"])
		}
		writeEnvelope(w, http.StatusOK,
			`{"id":50,"name":"gitapp","deployment_status":"pending","operation_id":"op-9","updated_at":"2026-07-01T00:00:00Z"}`)
	})
	srv := newServer(t, mux)
	mockEnv(t, srv.URL)

	out, _, err := runCLI("apps", "create", "--git", "3", "--repo", "acme/web",
		"--name", "gitapp", "--language", "railpack", "--pricing-slug", "app-small")
	if err != nil {
		t.Fatalf("apps create git-build: %v", err)
	}
	if !strings.Contains(out, "git-build app") || !strings.Contains(out, "gitapp") {
		t.Errorf("unexpected git-build create output: %q", out)
	}
}

func TestAppsCreateGitImageMutuallyExclusive(t *testing.T) {
	mockEnv(t, "http://127.0.0.1:0")
	_, _, err := runCLI("apps", "create", "--git", "3", "--repo", "acme/web",
		"--image", "nginx:1", "--name", "x", "--pricing-slug", "s")
	if err == nil || !strings.Contains(err.Error(), "mutually exclusive") {
		t.Fatalf("expected mutual-exclusion error, got: %v", err)
	}
}

func TestAppsCreateGitBuildDockerfilePathValidation(t *testing.T) {
	mockEnv(t, "http://127.0.0.1:0")
	_, _, err := runCLI("apps", "create", "--git", "3", "--repo", "acme/web",
		"--name", "x", "--pricing-slug", "s", "--dockerfile-path", "Dockerfile", "--language", "railpack")
	if err == nil || !strings.Contains(err.Error(), "--dockerfile-path only applies with --language dockerfile") {
		t.Fatalf("expected dockerfile-path validation error, got: %v", err)
	}
}
