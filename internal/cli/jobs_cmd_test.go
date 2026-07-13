package cli

import (
	"net/http"
	"strings"
	"testing"
)

const jobStandaloneJSON = `{"id":1,"name":"backup","kind":"standalone","image":"busybox:1",` +
	`"schedule":"0 2 * * *","timezone":"UTC","concurrency_policy":"Forbid",` +
	`"suspended":false,"deployment_status":"active",` +
	`"resource_template":{"slug":"job-small","name":"Small","cpu_vcpu":"0.25","memory_mb":256,"price_per_hour":"100.00"},` +
	`"created_at":"2026-07-01T00:00:00Z","updated_at":"2026-07-02T00:00:00Z"}`

func TestJobsList(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/jobs", func(w http.ResponseWriter, _ *http.Request) {
		writeEnvelope(w, http.StatusOK,
			`[{"id":1,"name":"backup","kind":"standalone","schedule":"0 2 * * *","timezone":"UTC",`+
				`"suspended":false,"deployment_status":"active","created_at":"2026-07-01T00:00:00Z","updated_at":"2026-07-02T00:00:00Z"}]`)
	})
	srv := newServer(t, mux)
	mockEnv(t, srv.URL)

	out, _, err := runCLI("jobs", "list")
	if err != nil {
		t.Fatalf("jobs list: %v", err)
	}
	for _, want := range []string{"NAME", "backup", "standalone", "0 2 * * *", "active"} {
		if !strings.Contains(out, want) {
			t.Errorf("list missing %q: %s", want, out)
		}
	}
}

func TestJobsGet(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/jobs/backup", func(w http.ResponseWriter, _ *http.Request) {
		writeEnvelope(w, http.StatusOK, jobStandaloneJSON)
	})
	srv := newServer(t, mux)
	mockEnv(t, srv.URL)

	out, _, err := runCLI("jobs", "get", "backup")
	if err != nil {
		t.Fatalf("jobs get: %v", err)
	}
	for _, want := range []string{"Name:", "backup", "Image:", "busybox:1", "Schedule:", "job-small"} {
		if !strings.Contains(out, want) {
			t.Errorf("detail missing %q: %s", want, out)
		}
	}
}

func TestJobsCreateStandalone(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/jobs", func(w http.ResponseWriter, r *http.Request) {
		var req map[string]any
		decodeBody(t, r, &req)
		if req["kind"] != "standalone" {
			t.Errorf("kind = %v, want standalone", req["kind"])
		}
		if req["image"] != "busybox:1" {
			t.Errorf("image = %v, want busybox:1", req["image"])
		}
		writeEnvelope(w, http.StatusOK,
			`{"id":1,"name":"backup","deployment_status":"pending","operation_id":"op-1","updated_at":"2026-07-01T00:00:00Z"}`)
	})
	srv := newServer(t, mux)
	mockEnv(t, srv.URL)

	out, _, err := runCLI("jobs", "create", "--name", "backup", "--kind", "standalone",
		"--image", "busybox:1", "--pricing-slug", "job-small")
	if err != nil {
		t.Fatalf("jobs create: %v", err)
	}
	if !strings.Contains(out, "Created job") || !strings.Contains(out, "pending") {
		t.Errorf("unexpected create output: %q", out)
	}
}

func TestJobsCreateStandaloneRequiresImage(t *testing.T) {
	mockEnv(t, "http://127.0.0.1:0")
	_, _, err := runCLI("jobs", "create", "--name", "backup", "--kind", "standalone", "--pricing-slug", "job-small")
	if err == nil || !strings.Contains(err.Error(), "standalone job requires --image") {
		t.Fatalf("expected standalone-needs-image error, got: %v", err)
	}
}

func TestJobsCreateInvalidKind(t *testing.T) {
	mockEnv(t, "http://127.0.0.1:0")
	_, _, err := runCLI("jobs", "create", "--name", "x", "--kind", "bogus", "--pricing-slug", "s")
	if err == nil || !strings.Contains(err.Error(), "invalid --kind") {
		t.Fatalf("expected invalid-kind error, got: %v", err)
	}
}

func TestJobsRun(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/jobs/backup", func(w http.ResponseWriter, _ *http.Request) {
		writeEnvelope(w, http.StatusOK, jobStandaloneJSON)
	})
	mux.HandleFunc("POST /api/v1/jobs/1/run", func(w http.ResponseWriter, _ *http.Request) {
		writeEnvelope(w, http.StatusOK, `{"execution_id":42,"status":"pending","operation_id":"op-2"}`)
	})
	srv := newServer(t, mux)
	mockEnv(t, srv.URL)

	out, _, err := runCLI("jobs", "run", "backup")
	if err != nil {
		t.Fatalf("jobs run: %v", err)
	}
	if !strings.Contains(out, "execution 42") {
		t.Errorf("unexpected run output: %q", out)
	}
}

func TestJobsSuspendAlreadySuspended(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/jobs/backup", func(w http.ResponseWriter, _ *http.Request) {
		writeEnvelope(w, http.StatusOK, jobStandaloneJSON)
	})
	mux.HandleFunc("POST /api/v1/jobs/1/suspend", func(w http.ResponseWriter, _ *http.Request) {
		writeError(w, http.StatusConflict, "JOB_ALREADY_SUSPENDED", "already")
	})
	srv := newServer(t, mux)
	mockEnv(t, srv.URL)

	_, _, err := runCLI("jobs", "suspend", "backup")
	if err == nil || !strings.Contains(err.Error(), "already suspended") {
		t.Fatalf("expected already-suspended mapping, got: %v", err)
	}
}

func TestJobsExecutions(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/jobs/backup", func(w http.ResponseWriter, _ *http.Request) {
		writeEnvelope(w, http.StatusOK, jobStandaloneJSON)
	})
	mux.HandleFunc("GET /api/v1/jobs/1/executions", func(w http.ResponseWriter, _ *http.Request) {
		writeEnvelope(w, http.StatusOK,
			`[{"id":42,"job_id":1,"trigger":"manual","k8s_job_name":"j-1","status":"succeeded","exit_code":0,"created_at":"2026-07-01T00:00:00Z"}]`)
	})
	srv := newServer(t, mux)
	mockEnv(t, srv.URL)

	out, _, err := runCLI("jobs", "executions", "backup")
	if err != nil {
		t.Fatalf("jobs executions: %v", err)
	}
	for _, want := range []string{"TRIGGER", "manual", "succeeded"} {
		if !strings.Contains(out, want) {
			t.Errorf("executions missing %q: %s", want, out)
		}
	}
}
