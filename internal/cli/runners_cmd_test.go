package cli

import (
	"net/http"
	"strings"
	"testing"
)

const runnerJobGithubJSON = `{"id":5,"provider":"github","spec_label":"kumo-2cpu",` +
	`"github_job_id":88,"run_id":99,"repo_full_name":"acme/web",` +
	`"web_url":"https://github.com/acme/web/actions/runs/99",` +
	`"state":"completed","conclusion":"success","queued_at":"2026-07-01T00:00:00Z"}`

func TestRunnersList(t *testing.T) {
	mux := http.NewServeMux()
	gotState := ""
	mux.HandleFunc("GET /api/v1/runner-jobs", func(w http.ResponseWriter, r *http.Request) {
		gotState = r.URL.Query().Get("state")
		writeEnvelope(w, http.StatusOK, "["+runnerJobGithubJSON+"]")
	})
	srv := newServer(t, mux)
	mockEnv(t, srv.URL)

	out, _, err := runCLI("runners", "list", "--state", "completed")
	if err != nil {
		t.Fatalf("runners list: %v", err)
	}
	if gotState != "completed" {
		t.Errorf("expected state query, got %q", gotState)
	}
	for _, want := range []string{"PROVIDER", "github", "acme/web", "kumo-2cpu", "completed", "success"} {
		if !strings.Contains(out, want) {
			t.Errorf("list missing %q: %s", want, out)
		}
	}
}

func TestRunnersGet(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/runner-jobs/5", func(w http.ResponseWriter, _ *http.Request) {
		writeEnvelope(w, http.StatusOK, runnerJobGithubJSON)
	})
	srv := newServer(t, mux)
	mockEnv(t, srv.URL)

	out, _, err := runCLI("runners", "get", "5")
	if err != nil {
		t.Fatalf("runners get: %v", err)
	}
	for _, want := range []string{"Provider:", "github", "GitHub job id:", "URL:", "github.com/acme/web"} {
		if !strings.Contains(out, want) {
			t.Errorf("detail missing %q: %s", want, out)
		}
	}
}

func TestRunnersGetNotFound(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/runner-jobs/7", func(w http.ResponseWriter, _ *http.Request) {
		writeError(w, http.StatusNotFound, "RUNNER_JOB_NOT_FOUND", "not found")
	})
	srv := newServer(t, mux)
	mockEnv(t, srv.URL)

	_, _, err := runCLI("runners", "get", "7")
	if err == nil || !strings.Contains(err.Error(), "no runner job with id 7") {
		t.Fatalf("expected not-found mapping, got: %v", err)
	}
}

func TestRunnersGetInvalidID(t *testing.T) {
	mockEnv(t, "http://127.0.0.1:0")
	_, _, err := runCLI("runners", "get", "abc")
	if err == nil || !strings.Contains(err.Error(), "invalid runner job id") {
		t.Fatalf("expected invalid-id error, got: %v", err)
	}
}
