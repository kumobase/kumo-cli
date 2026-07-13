package cli

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

// TestAppCreateAutoscalingFlags asserts the shared autoscaling flags reach the
// create request (they previously could only be set via a manifest).
func TestAppCreateAutoscalingFlags(t *testing.T) {
	var body string
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/apps/validate-image", handlePullable)
	mux.HandleFunc("POST /api/v1/apps", func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		body = string(b)
		writeEnvelope(w, http.StatusOK, appDetailJSON)
	})
	srv := newServer(t, mux)
	mockEnv(t, srv.URL)

	_, _, err := runCLI("apps", "create", "--name", "web-app", "--image", "nginx", "--port", "80",
		"--wait=false", "--autoscale", "--min-replicas", "2", "--max-replicas", "5", "--cpu-target", "70")
	if err != nil {
		t.Fatalf("apps create with autoscaling: %v", err)
	}
	for _, want := range []string{"autoscaling", "\"min_replicas\":2", "\"max_replicas\":5"} {
		if !strings.Contains(body, want) {
			t.Errorf("create body missing %q: %s", want, body)
		}
	}
}

func TestJobsRunWaitSucceeded(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/jobs/backup", func(w http.ResponseWriter, _ *http.Request) {
		writeEnvelope(w, http.StatusOK, jobStandaloneJSON)
	})
	mux.HandleFunc("POST /api/v1/jobs/1/run", func(w http.ResponseWriter, _ *http.Request) {
		writeEnvelope(w, http.StatusOK, `{"execution_id":42,"status":"pending"}`)
	})
	mux.HandleFunc("GET /api/v1/jobs/1/executions/42", func(w http.ResponseWriter, _ *http.Request) {
		writeEnvelope(w, http.StatusOK, `{"id":42,"job_id":1,"status":"succeeded","trigger":"manual","created_at":"2024-01-01T00:00:00Z"}`)
	})
	srv := newServer(t, mux)
	mockEnv(t, srv.URL)

	out, _, err := runCLI("jobs", "run", "backup", "--wait")
	if err != nil {
		t.Fatalf("jobs run --wait: %v", err)
	}
	if !strings.Contains(out, "finished") || !strings.Contains(out, "succeeded") {
		t.Errorf("expected finished/succeeded, got: %q", out)
	}
}

func TestJobsRunWaitFailedExitsNonZero(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/jobs/backup", func(w http.ResponseWriter, _ *http.Request) {
		writeEnvelope(w, http.StatusOK, jobStandaloneJSON)
	})
	mux.HandleFunc("POST /api/v1/jobs/1/run", func(w http.ResponseWriter, _ *http.Request) {
		writeEnvelope(w, http.StatusOK, `{"execution_id":42,"status":"pending"}`)
	})
	mux.HandleFunc("GET /api/v1/jobs/1/executions/42", func(w http.ResponseWriter, _ *http.Request) {
		writeEnvelope(w, http.StatusOK, `{"id":42,"job_id":1,"status":"failed","trigger":"manual","created_at":"2024-01-01T00:00:00Z"}`)
	})
	srv := newServer(t, mux)
	mockEnv(t, srv.URL)

	_, _, err := runCLI("jobs", "run", "backup", "--wait")
	if err == nil || !strings.Contains(err.Error(), "failed") {
		t.Fatalf("expected error on failed execution, got: %v", err)
	}
}
