package cli

import (
	"net/http"
	"os"
	"strings"
	"testing"
)

func TestAppsBuildsGet(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/apps/web-app", handleAppByName)
	mux.HandleFunc("GET /api/v1/apps/42/builds/7", func(w http.ResponseWriter, _ *http.Request) {
		writeEnvelope(w, http.StatusOK, buildJSON)
	})
	srv := newServer(t, mux)
	mockEnv(t, srv.URL)

	out, _, err := runCLI("apps", "builds", "get", "web-app", "7")
	if err != nil {
		t.Fatalf("apps builds get: %v", err)
	}
	for _, want := range []string{"ID:", "Status:", "succeeded", "Commit:", "Image digest:"} {
		if !strings.Contains(out, want) {
			t.Errorf("build detail missing %q: %s", want, out)
		}
	}
}

func TestAppsBuildsCancel(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/apps/web-app", handleAppByName)
	cancelled := false
	mux.HandleFunc("POST /api/v1/apps/42/builds/7/cancel", func(w http.ResponseWriter, _ *http.Request) {
		cancelled = true
		writeEnvelope(w, http.StatusOK,
			`{"id":7,"app_id":42,"status":"cancelled","commit_sha":"abc","created_at":"2026-07-01T00:00:00Z"}`)
	})
	srv := newServer(t, mux)
	mockEnv(t, srv.URL)

	out, _, err := runCLI("apps", "builds", "cancel", "web-app", "7")
	if err != nil {
		t.Fatalf("apps builds cancel: %v", err)
	}
	if !cancelled {
		t.Error("expected cancel POST to be called")
	}
	if !strings.Contains(out, "Build 7 cancelled") {
		t.Errorf("unexpected cancel output: %q", out)
	}
}

func TestAppsBuilders(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/builders", func(w http.ResponseWriter, _ *http.Request) {
		writeEnvelope(w, http.StatusOK,
			`{"builders":[{"kind":"auto","label":"Auto","default":true},{"kind":"railpack","label":"Railpack","default":false}],`+
				`"languages":[{"value":"nodejs"},{"value":"go"}]}`)
	})
	srv := newServer(t, mux)
	mockEnv(t, srv.URL)

	out, _, err := runCLI("apps", "builders")
	if err != nil {
		t.Fatalf("apps builders: %v", err)
	}
	for _, want := range []string{"BUILDER", "auto", "Railpack", "LANGUAGES", "nodejs", "go"} {
		if !strings.Contains(out, want) {
			t.Errorf("builders missing %q: %s", want, out)
		}
	}
}

func TestJobsUpdate(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/jobs/backup", func(w http.ResponseWriter, _ *http.Request) {
		writeEnvelope(w, http.StatusOK, jobStandaloneJSON)
	})
	var patched bool
	mux.HandleFunc("PATCH /api/v1/jobs/1", func(w http.ResponseWriter, r *http.Request) {
		patched = true
		var req map[string]any
		decodeBody(t, r, &req)
		if req["pricing_slug"] != "job-large" {
			t.Errorf("pricing_slug = %v, want job-large", req["pricing_slug"])
		}
		writeEnvelope(w, http.StatusOK,
			`{"id":1,"name":"backup","deployment_status":"pending","operation_id":"op-1","updated_at":"2026-07-03T00:00:00Z"}`)
	})
	srv := newServer(t, mux)
	mockEnv(t, srv.URL)

	out, _, err := runCLI("jobs", "update", "backup",
		"--pricing-slug", "job-large", "--command", "sh", "--arg", "-c",
		"--env", "K=V", "--schedule", "0 3 * * *", "--concurrency-policy", "Allow",
		"--active-deadline", "60", "--backoff-limit", "2")
	if err != nil {
		t.Fatalf("jobs update: %v", err)
	}
	if !patched {
		t.Error("expected PATCH to be called")
	}
	if !strings.Contains(out, "Updated job") || !strings.Contains(out, "backup") {
		t.Errorf("unexpected update output: %q", out)
	}
}

// TestJobsUpdateSecretsAndTimezone covers the remaining update branches:
// --timezone, --secret-env, and --secret-mount.
func TestJobsUpdateSecretsAndTimezone(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/jobs/backup", func(w http.ResponseWriter, _ *http.Request) {
		writeEnvelope(w, http.StatusOK, jobStandaloneJSON)
	})
	var req map[string]any
	mux.HandleFunc("PATCH /api/v1/jobs/1", func(w http.ResponseWriter, r *http.Request) {
		decodeBody(t, r, &req)
		writeEnvelope(w, http.StatusOK,
			`{"id":1,"name":"backup","deployment_status":"pending","operation_id":"op-3","updated_at":"2026-07-03T00:00:00Z"}`)
	})
	srv := newServer(t, mux)
	mockEnv(t, srv.URL)

	_, _, err := runCLI("jobs", "update", "backup",
		"--timezone", "Asia/Jakarta",
		"--secret-env", "DB:PASSWORD:DB_PASS",
		"--secret-mount", "TLS:/etc/tls")
	if err != nil {
		t.Fatalf("jobs update secrets/timezone: %v", err)
	}
	if req["timezone"] != "Asia/Jakarta" {
		t.Errorf("timezone = %v, want Asia/Jakarta", req["timezone"])
	}
}

func TestJobsUpdateInvalidConcurrency(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/jobs/backup", func(w http.ResponseWriter, _ *http.Request) {
		writeEnvelope(w, http.StatusOK, jobStandaloneJSON)
	})
	srv := newServer(t, mux)
	mockEnv(t, srv.URL)

	_, _, err := runCLI("jobs", "update", "backup", "--concurrency-policy", "Nope")
	if err == nil || !strings.Contains(err.Error(), "invalid --concurrency-policy") {
		t.Fatalf("expected concurrency validation error, got: %v", err)
	}
}

func TestJobsDelete(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/jobs/backup", func(w http.ResponseWriter, _ *http.Request) {
		writeEnvelope(w, http.StatusOK, jobStandaloneJSON)
	})
	var deleted bool
	mux.HandleFunc("DELETE /api/v1/jobs/1", func(w http.ResponseWriter, _ *http.Request) {
		deleted = true
		writeEnvelope(w, http.StatusOK,
			`{"id":1,"name":"backup","deployment_status":"terminating","operation_id":"op-2","updated_at":"2026-07-03T00:00:00Z"}`)
	})
	srv := newServer(t, mux)
	mockEnv(t, srv.URL)

	out, _, err := runCLI("jobs", "delete", "backup", "-y")
	if err != nil {
		t.Fatalf("jobs delete: %v", err)
	}
	if !deleted {
		t.Error("expected DELETE to be called")
	}
	if !strings.Contains(out, "Deletion queued for job 1") {
		t.Errorf("unexpected delete output: %q", out)
	}
}

func TestJobsResume(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/jobs/backup", func(w http.ResponseWriter, _ *http.Request) {
		writeEnvelope(w, http.StatusOK, jobStandaloneJSON)
	})
	var resumed bool
	mux.HandleFunc("POST /api/v1/jobs/1/resume", func(w http.ResponseWriter, _ *http.Request) {
		resumed = true
		writeEnvelope(w, http.StatusOK, jobStandaloneJSON)
	})
	srv := newServer(t, mux)
	mockEnv(t, srv.URL)

	out, _, err := runCLI("jobs", "resume", "backup")
	if err != nil {
		t.Fatalf("jobs resume: %v", err)
	}
	if !resumed {
		t.Error("expected resume POST to be called")
	}
	if !strings.Contains(out, "Job 1 resumed") {
		t.Errorf("unexpected resume output: %q", out)
	}
}

func TestSecretUpdateFileFromFile(t *testing.T) {
	dir := t.TempDir()
	filePath := dir + "/payload.txt"
	if err := os.WriteFile(filePath, []byte("NEW-FILE-BODY"), 0o600); err != nil {
		t.Fatal(err)
	}
	const fileDetailJSON = `{"id":7,"name":"f","type":"file","used_by_count":0,` +
		`"created_at":"2026-07-01T00:00:00Z","updated_at":"2026-07-01T00:00:00Z","file_content":"OLD"}`

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/secrets/{id}", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("ETag", `W/"abc123"`)
		writeEnvelope(w, http.StatusOK, fileDetailJSON)
	})
	var body string
	mux.HandleFunc("PATCH /api/v1/secrets/{id}", func(w http.ResponseWriter, r *http.Request) {
		var req map[string]any
		decodeBody(t, r, &req)
		if fc, ok := req["file_content"].(string); ok {
			body = fc
		}
		writeEnvelope(w, http.StatusOK, `{"id":7,"name":"f","type":"file"}`)
	})
	srv := newServer(t, mux)
	mockEnv(t, srv.URL)

	if _, _, err := runCLI("secret", "update", "7", "--from-file", filePath); err != nil {
		t.Fatalf("secret update file: %v", err)
	}
	if body != "NEW-FILE-BODY" {
		t.Errorf("file_content = %q, want NEW-FILE-BODY", body)
	}
}

func TestSecretUpdateRegistryPassword(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/secrets/{id}", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("ETag", `W/"abc123"`)
		writeEnvelope(w, http.StatusOK, secretRegistryDetailJSON)
	})
	var pass string
	mux.HandleFunc("PATCH /api/v1/secrets/{id}", func(w http.ResponseWriter, r *http.Request) {
		var req map[string]any
		decodeBody(t, r, &req)
		if reg, ok := req["secret_registry"].(map[string]any); ok {
			pass, _ = reg["password"].(string)
		}
		writeEnvelope(w, http.StatusOK, `{"id":6,"name":"reg","type":"registry"}`)
	})
	srv := newServer(t, mux)
	mockEnv(t, srv.URL)

	if _, _, err := runCLI("secret", "update", "6", "--registry-password", "sekret"); err != nil {
		t.Fatalf("secret update registry: %v", err)
	}
	if pass != "sekret" {
		t.Errorf("password = %q, want sekret", pass)
	}
}

// TestAppsCreateGitBuildAutoResolveConn drives runGitBuildCreate without an
// explicit --git: the sole source connection is auto-resolved, and the static
// language + secret-var + skip-checks branches are exercised.
func TestAppsCreateGitBuildAutoResolveConn(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/source-connections", func(w http.ResponseWriter, _ *http.Request) {
		writeEnvelope(w, http.StatusOK, "["+sourceConnJSON+"]")
	})
	var got map[string]any
	mux.HandleFunc("POST /api/v1/source-connections/3/apps", func(w http.ResponseWriter, r *http.Request) {
		decodeBody(t, r, &got)
		writeEnvelope(w, http.StatusOK,
			`{"id":51,"name":"staticapp","deployment_status":"pending","operation_id":"op-1","updated_at":"2026-07-01T00:00:00Z"}`)
	})
	srv := newServer(t, mux)
	mockEnv(t, srv.URL)

	out, _, err := runCLI("apps", "create", "--repo", "acme/site", "--name", "staticapp",
		"--pricing-slug", "app-small", "--language", "static", "--output-dir", "dist",
		"--build-command", "npm run build", "--secret-var", "APIKEY:restart",
		"--skip-secret-checks")
	if err != nil {
		t.Fatalf("git-build create auto-resolve: %v", err)
	}
	if got["language"] != "static" || got["output_dir"] != "dist" {
		t.Errorf("request payload = %v", got)
	}
	if !strings.Contains(out, "git-build app") || !strings.Contains(out, "staticapp") {
		t.Errorf("unexpected output: %q", out)
	}
}

// TestAppsCreateGitBuildOutputDirRequiresStatic covers the static-only flag guard.
func TestAppsCreateGitBuildOutputDirRequiresStatic(t *testing.T) {
	mockEnv(t, "http://127.0.0.1:0")
	_, _, err := runCLI("apps", "create", "--git", "3", "--repo", "acme/web",
		"--name", "x", "--pricing-slug", "s", "--output-dir", "dist", "--language", "railpack")
	if err == nil || !strings.Contains(err.Error(), "only apply with --language static") {
		t.Fatalf("expected static-only guard error, got: %v", err)
	}
}

// TestVolumeCreateWaitFails covers pollVolumeUntilReady's failed-status branch:
// the volume reports "failed" with an error message, which must surface as an
// error without any polling delay.
func TestVolumeCreateWaitFails(t *testing.T) {
	const volumeFailedJSON = `{"id":7,"name":"data","app_id":null,` +
		`"storage_tier":{"id":1,"slug":"ssd","name":"SSD","price_per_gb_hour":"0.0001",` +
		`"min_size_gb":1,"max_size_gb":1000},"size_gb":10,"mount_path":"",` +
		`"status":"failed","error_message":"provisioner ran out of capacity",` +
		`"created_at":"2024-01-01T00:00:00Z","updated_at":"2024-01-02T00:00:00Z"}`

	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/volumes", func(w http.ResponseWriter, _ *http.Request) {
		writeEnvelope(w, http.StatusOK, `{"id":7,"name":"data","app_id":null,`+
			`"storage_tier":{"id":1,"slug":"ssd","name":"SSD","price_per_gb_hour":"0.0001","min_size_gb":1,"max_size_gb":1000},`+
			`"size_gb":10,"mount_path":"","status":"creating","created_at":"2024-01-01T00:00:00Z","updated_at":"2024-01-02T00:00:00Z"}`)
	})
	mux.HandleFunc("GET /api/v1/volumes/7", func(w http.ResponseWriter, _ *http.Request) {
		writeEnvelope(w, http.StatusOK, volumeFailedJSON)
	})
	srv := newServer(t, mux)
	mockEnv(t, srv.URL)

	_, _, err := runCLI("volume", "create", "--name", "data", "--tier", "ssd", "--size", "10")
	if err == nil || !strings.Contains(err.Error(), "provisioner ran out of capacity") {
		t.Fatalf("expected failed-volume error, got: %v", err)
	}
}

func TestVPSStopWait(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/vps/servers/web1", func(w http.ResponseWriter, _ *http.Request) {
		writeEnvelope(w, http.StatusOK, vpsServerJSON)
	})
	var powered bool
	mux.HandleFunc("POST /api/v1/vps/servers/7/poweroff", func(w http.ResponseWriter, _ *http.Request) {
		powered = true
		writeEnvelope(w, http.StatusAccepted, "")
	})
	// Poll target: action_status "" → action complete.
	mux.HandleFunc("GET /api/v1/vps/servers/7", func(w http.ResponseWriter, _ *http.Request) {
		writeEnvelope(w, http.StatusOK, vpsServerJSON)
	})
	srv := newServer(t, mux)
	mockEnv(t, srv.URL)

	out, _, err := runCLI("vps", "stop", "web1", "--wait")
	if err != nil {
		t.Fatalf("vps stop --wait: %v", err)
	}
	if !powered {
		t.Error("expected poweroff POST")
	}
	if !strings.Contains(out, "powered off") {
		t.Errorf("expected completion message: %s", out)
	}
}

func TestVPSRebootWait(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/vps/servers/web1", func(w http.ResponseWriter, _ *http.Request) {
		writeEnvelope(w, http.StatusOK, vpsServerJSON)
	})
	var rebooted bool
	mux.HandleFunc("POST /api/v1/vps/servers/7/reboot", func(w http.ResponseWriter, _ *http.Request) {
		rebooted = true
		writeEnvelope(w, http.StatusAccepted, "")
	})
	mux.HandleFunc("GET /api/v1/vps/servers/7", func(w http.ResponseWriter, _ *http.Request) {
		writeEnvelope(w, http.StatusOK, vpsServerJSON)
	})
	srv := newServer(t, mux)
	mockEnv(t, srv.URL)

	out, _, err := runCLI("vps", "reboot", "web1", "--wait")
	if err != nil {
		t.Fatalf("vps reboot --wait: %v", err)
	}
	if !rebooted {
		t.Error("expected reboot POST")
	}
	if !strings.Contains(out, "rebooted") {
		t.Errorf("expected completion message: %s", out)
	}
}

// TestAppsUpdateManyFlags exercises the many `if f.Changed()` payload branches
// of newAppsUpdateCmd in one pass (name/image/port/replicas/exposed/pricing/
// registry-credential/env/secret-var/secret-file-mount), with --skip-secret-checks
// and --wait=false to keep it to a single PATCH.
func TestAppsUpdateManyFlags(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/apps/{id}", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("ETag", `W/"abc123"`)
		writeEnvelope(w, http.StatusOK, appDetailJSON)
	})
	var req map[string]any
	mux.HandleFunc("PATCH /api/v1/apps/{id}", func(w http.ResponseWriter, r *http.Request) {
		decodeBody(t, r, &req)
		writeEnvelope(w, http.StatusAccepted, "")
	})
	srv := newServer(t, mux)
	mockEnv(t, srv.URL)

	out, _, err := runCLI("apps", "update", "42",
		"--name", "renamed", "--image", "nginx:2", "--port", "9090",
		"--replicas", "3", "--exposed", "--pricing-slug", "app-large",
		"--registry-credential", "reg", "--env", "K=V",
		"--secret-var", "APIKEY", "--secret-file-mount", "TLS:/etc/tls",
		"--skip-secret-checks", "--wait=false")
	if err != nil {
		t.Fatalf("apps update many flags: %v", err)
	}
	if req["name"] != "renamed" || req["image"] != "nginx:2" {
		t.Errorf("payload = %v", req)
	}
	if !strings.Contains(out, "Update queued for app 42") {
		t.Errorf("unexpected output: %q", out)
	}
}

// TestJobsExecutionsGetOne drives the single-execution branch of
// newJobsExecutionsCmd (args len 2 → GetExecution → printJobExecutionDetail).
func TestJobsExecutionsGetOne(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/jobs/backup", func(w http.ResponseWriter, _ *http.Request) {
		writeEnvelope(w, http.StatusOK, jobStandaloneJSON)
	})
	mux.HandleFunc("GET /api/v1/jobs/1/executions/42", func(w http.ResponseWriter, _ *http.Request) {
		writeEnvelope(w, http.StatusOK,
			`{"id":42,"job_id":1,"trigger":"manual","k8s_job_name":"j-1","status":"succeeded",`+
				`"exit_code":0,"duration_ms":1500,"created_at":"2026-07-01T00:00:00Z"}`)
	})
	srv := newServer(t, mux)
	mockEnv(t, srv.URL)

	out, _, err := runCLI("jobs", "executions", "backup", "42")
	if err != nil {
		t.Fatalf("jobs executions get one: %v", err)
	}
	for _, want := range []string{"ID:", "Trigger:", "manual", "Status:", "succeeded"} {
		if !strings.Contains(out, want) {
			t.Errorf("execution detail missing %q: %s", want, out)
		}
	}
}

// TestJobsResolveAmbiguous covers resolveJobRef's ambiguous-name branch.
func TestJobsResolveAmbiguous(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/jobs/backup", func(w http.ResponseWriter, _ *http.Request) {
		writeError(w, http.StatusConflict, "AMBIGUOUS_NAME", "multiple jobs share this name")
	})
	srv := newServer(t, mux)
	mockEnv(t, srv.URL)

	_, _, err := runCLI("jobs", "resume", "backup")
	if err == nil || !strings.Contains(err.Error(), "multiple jobs named") {
		t.Fatalf("expected ambiguous-name error, got: %v", err)
	}
}

// TestJobsResolveNotFound covers resolveJobRef's not-found branch.
func TestJobsResolveNotFound(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/jobs/ghost", func(w http.ResponseWriter, _ *http.Request) {
		writeError(w, http.StatusNotFound, "JOB_NOT_FOUND", "no such job")
	})
	srv := newServer(t, mux)
	mockEnv(t, srv.URL)

	_, _, err := runCLI("jobs", "resume", "ghost")
	if err == nil || !strings.Contains(err.Error(), `no job named "ghost"`) {
		t.Fatalf("expected not-found error, got: %v", err)
	}
}

// TestJobsDeleteRequiresConfirmation drives newJobsDeleteCmd's confirm branch:
// with no --yes and a non-terminal stdin, confirm refuses to proceed.
func TestJobsDeleteRequiresConfirmation(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/jobs/backup", func(w http.ResponseWriter, _ *http.Request) {
		writeEnvelope(w, http.StatusOK, jobStandaloneJSON)
	})
	mux.HandleFunc("DELETE /api/v1/jobs/1", func(w http.ResponseWriter, _ *http.Request) {
		t.Error("DELETE must not be called without confirmation")
	})
	srv := newServer(t, mux)
	mockEnv(t, srv.URL)

	_, _, err := runCLI("jobs", "delete", "backup")
	if err == nil || !strings.Contains(err.Error(), "--yes") {
		t.Fatalf("expected confirmation-required error, got: %v", err)
	}
}

// TestAppsBuildsRebuildRequiresConfirmation covers the confirm branch of
// newAppsBuildsRebuildCmd (no --yes, non-terminal stdin).
func TestAppsBuildsRebuildRequiresConfirmation(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/apps/web-app", handleAppByName)
	mux.HandleFunc("POST /api/v1/apps/42/builds", func(w http.ResponseWriter, _ *http.Request) {
		t.Error("rebuild POST must not be called without confirmation")
	})
	srv := newServer(t, mux)
	mockEnv(t, srv.URL)

	_, _, err := runCLI("apps", "builds", "rebuild", "web-app")
	if err == nil || !strings.Contains(err.Error(), "--yes") {
		t.Fatalf("expected confirmation-required error, got: %v", err)
	}
}

func TestResolveRegistryHost(t *testing.T) {
	// Explicit flag wins.
	if got := resolveRegistryHost("my.registry"); got != "my.registry" {
		t.Errorf("explicit = %q", got)
	}
	// Env var next.
	t.Setenv("KUMO_REGISTRY_HOST", "env.registry")
	if got := resolveRegistryHost(""); got != "env.registry" {
		t.Errorf("env = %q", got)
	}
	// Default fallback.
	t.Setenv("KUMO_REGISTRY_HOST", "")
	if got := resolveRegistryHost(""); got != defaultRegistryHost {
		t.Errorf("default = %q, want %q", got, defaultRegistryHost)
	}
}
