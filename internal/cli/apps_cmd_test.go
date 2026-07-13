package cli

import (
	"net/http"
	"strings"
	"testing"
	"time"
)

const appDetailJSON = `{"id":42,"name":"web-app","image":"nginx:1.27","port":80,"is_exposed":true,"replicas":2,` +
	`"pricing_slug":"app-small","app_status":"running","desired_replicas":2,"ready_replicas":2,` +
	`"generated_sub_domain":"web-app.kumo.run","internal_dns":"web-app.internal",` +
	`"created_at":"2024-01-01T00:00:00Z","updated_at":"2024-01-02T00:00:00Z"}`

func TestAppsList(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/apps", func(w http.ResponseWriter, _ *http.Request) {
		writeEnvelope(w, http.StatusOK, "["+appDetailJSON+"]")
	})
	srv := newServer(t, mux)
	mockEnv(t, srv.URL)

	out, _, err := runCLI("apps", "list")
	if err != nil {
		t.Fatalf("apps list: %v", err)
	}
	if !strings.Contains(out, "web-app") || !strings.Contains(out, "running") {
		t.Errorf("unexpected list output: %q", out)
	}
	if !strings.Contains(out, "NAME") {
		t.Errorf("missing table header: %q", out)
	}
}

func TestAppsListJSON(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/apps", func(w http.ResponseWriter, _ *http.Request) {
		writeEnvelope(w, http.StatusOK, "["+appDetailJSON+"]")
	})
	srv := newServer(t, mux)
	mockEnv(t, srv.URL)

	out, _, err := runCLI("apps", "list", "-o", "json")
	if err != nil {
		t.Fatalf("apps list json: %v", err)
	}
	if !strings.Contains(out, `"name": "web-app"`) {
		t.Errorf("unexpected list json: %q", out)
	}
}

func TestAppsGet(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/apps/{id}", func(w http.ResponseWriter, _ *http.Request) {
		writeEnvelope(w, http.StatusOK, appDetailJSON)
	})
	srv := newServer(t, mux)
	mockEnv(t, srv.URL)

	out, _, err := runCLI("apps", "get", "42")
	if err != nil {
		t.Fatalf("apps get: %v", err)
	}
	if !strings.Contains(out, "web-app") || !strings.Contains(out, "web-app.kumo.run") {
		t.Errorf("unexpected get output: %q", out)
	}
}

func TestAppsGetNotFound(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/apps/{id}", func(w http.ResponseWriter, _ *http.Request) {
		writeError(w, http.StatusNotFound, "APP_NOT_FOUND", "app not found")
	})
	srv := newServer(t, mux)
	mockEnv(t, srv.URL)

	if _, _, err := runCLI("apps", "get", "99"); err == nil {
		t.Fatal("expected error for missing app")
	}
}

func TestAppsGetByName(t *testing.T) {
	mux := http.NewServeMux()
	got := ""
	mux.HandleFunc("GET /api/v1/apps/{id}", func(w http.ResponseWriter, r *http.Request) {
		got = r.PathValue("id")
		writeEnvelope(w, http.StatusOK, appDetailJSON)
	})
	srv := newServer(t, mux)
	mockEnv(t, srv.URL)

	out, _, err := runCLI("apps", "get", "web-app")
	if err != nil {
		t.Fatalf("apps get by name: %v", err)
	}
	if got != "web-app" {
		t.Errorf("expected name in path, got %q", got)
	}
	if !strings.Contains(out, "Instances:") {
		t.Errorf("expected Instances line in detail output: %q", out)
	}
}

func TestAppsGetAmbiguousName(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/apps/{id}", func(w http.ResponseWriter, _ *http.Request) {
		writeError(w, http.StatusConflict, "AMBIGUOUS_NAME", "name matches multiple apps")
	})
	srv := newServer(t, mux)
	mockEnv(t, srv.URL)

	_, _, err := runCLI("apps", "get", "dup-name")
	if err == nil {
		t.Fatal("expected error on ambiguous name")
	}
	if !strings.Contains(err.Error(), "disambiguate") {
		t.Errorf("expected disambiguation hint, got %q", err.Error())
	}
}

func TestAppsCreateNoWait(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/apps/validate-image", handlePullable)
	mux.HandleFunc("POST /api/v1/apps", func(w http.ResponseWriter, _ *http.Request) {
		writeEnvelope(w, http.StatusAccepted, `{"id":42,"name":"web-app","operation_id":"op-123","deployment_status":"deploying","updated_at":"2024-01-01T00:00:00Z"}`)
	})
	srv := newServer(t, mux)
	mockEnv(t, srv.URL)

	out, _, err := runCLI("apps", "create", "--name", "web-app", "--image", "nginx:1.27", "--port", "80", "--wait=false")
	if err != nil {
		t.Fatalf("apps create: %v", err)
	}
	if !strings.Contains(out, "op-123") || !strings.Contains(out, "Created app") {
		t.Errorf("unexpected create output: %q", out)
	}
}

func TestAppsCreateWait(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/apps/validate-image", handlePullable)
	mux.HandleFunc("POST /api/v1/apps", func(w http.ResponseWriter, _ *http.Request) {
		writeEnvelope(w, http.StatusAccepted, `{"id":42,"name":"web-app","operation_id":"op-123","deployment_status":"deploying","updated_at":"2024-01-01T00:00:00Z"}`)
	})
	mux.HandleFunc("GET /api/v1/apps/{id}/operations/{opid}", func(w http.ResponseWriter, _ *http.Request) {
		writeEnvelope(w, http.StatusOK, `{"operation_id":"op-123","app_id":42,"action_type":"create","status":"succeeded","queued_at":"2024-01-01T00:00:00Z"}`)
	})
	mux.HandleFunc("GET /api/v1/apps/{id}", func(w http.ResponseWriter, _ *http.Request) {
		writeEnvelope(w, http.StatusOK, appDetailJSON)
	})
	srv := newServer(t, mux)
	mockEnv(t, srv.URL)

	out, _, err := runCLI("apps", "create", "--name", "web-app", "--image", "nginx:1.27", "--port", "80")
	if err != nil {
		t.Fatalf("apps create --wait: %v", err)
	}
	if !strings.Contains(out, "deployed") || !strings.Contains(out, "web-app.kumo.run") {
		t.Errorf("unexpected create wait output: %q", out)
	}
}

func TestAppsCreateWaitNotExposed(t *testing.T) {
	// App detail with is_exposed=false: the deploy summary must not print a URL.
	const notExposed = `{"id":42,"name":"web-app","image":"nginx:1.27","port":80,"is_exposed":false,"replicas":2,` +
		`"app_status":"running","desired_replicas":2,"ready_replicas":2,` +
		`"generated_sub_domain":"web-app.kumo.run","internal_dns":"web-app.internal",` +
		`"created_at":"2024-01-01T00:00:00Z","updated_at":"2024-01-02T00:00:00Z"}`
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/apps/validate-image", handlePullable)
	mux.HandleFunc("POST /api/v1/apps", func(w http.ResponseWriter, _ *http.Request) {
		writeEnvelope(w, http.StatusAccepted, `{"id":42,"name":"web-app","operation_id":"op-123","deployment_status":"deploying","updated_at":"2024-01-01T00:00:00Z"}`)
	})
	mux.HandleFunc("GET /api/v1/apps/{id}/operations/{opid}", func(w http.ResponseWriter, _ *http.Request) {
		writeEnvelope(w, http.StatusOK, `{"operation_id":"op-123","app_id":42,"action_type":"create","status":"succeeded","queued_at":"2024-01-01T00:00:00Z"}`)
	})
	mux.HandleFunc("GET /api/v1/apps/{id}", func(w http.ResponseWriter, _ *http.Request) {
		writeEnvelope(w, http.StatusOK, notExposed)
	})
	srv := newServer(t, mux)
	mockEnv(t, srv.URL)

	out, _, err := runCLI("apps", "create", "--name", "web-app", "--image", "nginx:1.27", "--port", "80")
	if err != nil {
		t.Fatalf("apps create: %v", err)
	}
	if !strings.Contains(out, "deployed") {
		t.Errorf("expected deploy summary: %q", out)
	}
	if strings.Contains(out, "URL:") || strings.Contains(out, "web-app.kumo.run") {
		t.Errorf("non-exposed app should not print a URL: %q", out)
	}
}

func TestAppsCreateAbortsWhenNoAmd64(t *testing.T) {
	// A plain deploy (no --validate) still pre-flights the image. linux/amd64
	// is the required minimum, so an arm64-only image must abort before deploy.
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/apps/validate-image", func(w http.ResponseWriter, _ *http.Request) {
		writeEnvelope(w, http.StatusOK, `{"linux_amd64":false,"linux_arm64":true}`)
	})
	mux.HandleFunc("POST /api/v1/apps", func(w http.ResponseWriter, _ *http.Request) {
		t.Error("deploy must not proceed when linux/amd64 is not pullable")
		writeEnvelope(w, http.StatusAccepted, "")
	})
	srv := newServer(t, mux)
	mockEnv(t, srv.URL)

	_, _, err := runCLI("apps", "create", "--name", "web-app", "--image", "arm-only:latest", "--port", "80")
	if err == nil {
		t.Fatal("expected deploy to abort without linux/amd64")
	}
	if !strings.Contains(err.Error(), "linux/amd64") {
		t.Errorf("unexpected abort error: %v", err)
	}
}

func TestAppsCreateValidateFails(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/apps/validate-image", func(w http.ResponseWriter, _ *http.Request) {
		writeEnvelope(w, http.StatusOK, `{"linux_amd64":false,"linux_arm64":false}`)
	})
	mux.HandleFunc("POST /api/v1/apps", func(w http.ResponseWriter, _ *http.Request) {
		t.Error("create must not be called when validation fails")
		writeEnvelope(w, http.StatusAccepted, "")
	})
	srv := newServer(t, mux)
	mockEnv(t, srv.URL)

	_, _, err := runCLI("apps", "create", "--name", "web-app", "--image", "bad:image", "--port", "80", "--validate")
	if err == nil {
		t.Fatal("expected create to fail when image is not pullable")
	}
	if !strings.Contains(err.Error(), "not pullable") {
		t.Errorf("unexpected validate error: %v", err)
	}
}

func TestAppsCreateValidateOnly(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/apps/validate-image", func(w http.ResponseWriter, _ *http.Request) {
		writeEnvelope(w, http.StatusOK, `{"linux_amd64":true,"linux_arm64":true}`)
	})
	mux.HandleFunc("POST /api/v1/apps", func(w http.ResponseWriter, _ *http.Request) {
		t.Error("create must not be called for a validation-only run")
		writeEnvelope(w, http.StatusAccepted, "")
	})
	srv := newServer(t, mux)
	mockEnv(t, srv.URL)

	out, _, err := runCLI("apps", "create", "--name", "web-app", "--image", "nginx:alpine", "--port", "80", "--validate")
	if err != nil {
		t.Fatalf("validate-only create: %v", err)
	}
	if !strings.Contains(out, "is pullable") || !strings.Contains(out, "validation only") {
		t.Errorf("unexpected validate-only output: %q", out)
	}
	if strings.Contains(out, "deployed") {
		t.Errorf("validate-only run should not deploy: %q", out)
	}
}

func TestAppsCreateRequiresNameAndImage(t *testing.T) {
	mockEnv(t, "http://127.0.0.1:0")
	if _, _, err := runCLI("apps", "create", "--name", "web-app", "--wait=false"); err == nil {
		t.Fatal("expected error when --image is missing")
	}
}

func TestAppsCreateWithSecretVar(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/apps/validate-image", handlePullable)
	mux.HandleFunc("GET /api/v1/secrets/{id}", handleSecretType("env_var"))
	mux.HandleFunc("POST /api/v1/apps", func(w http.ResponseWriter, r *http.Request) {
		var req map[string]any
		decodeBody(t, r, &req)
		sv, ok := req["secret_vars"].([]any)
		if !ok || len(sv) != 1 {
			t.Fatalf("expected one secret_var, got %v", req["secret_vars"])
		}
		first := sv[0].(map[string]any)
		if first["secret_name"] != "db-creds" || first["restart_when_updated"] != true {
			t.Errorf("secret_var fields wrong: %v", first)
		}
		writeEnvelope(w, http.StatusAccepted, `{"id":42,"name":"web-app","operation_id":"op-1","deployment_status":"deploying","updated_at":"2024-01-01T00:00:00Z"}`)
	})
	srv := newServer(t, mux)
	mockEnv(t, srv.URL)

	if _, _, err := runCLI("apps", "create", "--name", "web-app", "--image", "nginx:1.27", "--port", "80",
		"--secret-var", "db-creds:restart", "--wait=false"); err != nil {
		t.Fatalf("apps create --secret-var: %v", err)
	}
}

func TestAppsCreateWithSecretFileMount(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/apps/validate-image", handlePullable)
	mux.HandleFunc("GET /api/v1/secrets/{id}", handleSecretType("file"))
	mux.HandleFunc("POST /api/v1/apps", func(w http.ResponseWriter, r *http.Request) {
		var req map[string]any
		decodeBody(t, r, &req)
		sm, ok := req["secret_file_mounts"].([]any)
		if !ok || len(sm) != 1 {
			t.Fatalf("expected one secret_file_mount, got %v", req["secret_file_mounts"])
		}
		first := sm[0].(map[string]any)
		if first["secret_name"] != "tls-bundle" || first["mount_to"] != "/etc/tls" || first["type"] != "secret_file" {
			t.Errorf("secret_file_mount fields wrong: %v", first)
		}
		writeEnvelope(w, http.StatusAccepted, `{"id":42,"name":"web-app","operation_id":"op-1","deployment_status":"deploying","updated_at":"2024-01-01T00:00:00Z"}`)
	})
	srv := newServer(t, mux)
	mockEnv(t, srv.URL)

	if _, _, err := runCLI("apps", "create", "--name", "web-app", "--image", "nginx:1.27", "--port", "80",
		"--secret-file-mount", "tls-bundle:/etc/tls", "--wait=false"); err != nil {
		t.Fatalf("apps create --secret-file-mount: %v", err)
	}
}

func TestAppsUpdateWithSecretVar(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/apps/{id}", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("ETag", `W/"abc123"`)
		writeEnvelope(w, http.StatusOK, appDetailJSON)
	})
	mux.HandleFunc("GET /api/v1/secrets/{id}", handleSecretType("env_var"))
	mux.HandleFunc("PATCH /api/v1/apps/{id}", func(w http.ResponseWriter, r *http.Request) {
		var req map[string]any
		decodeBody(t, r, &req)
		sv, ok := req["secret_vars"].([]any)
		if !ok || len(sv) != 1 {
			t.Fatalf("expected one secret_var in update, got %v", req["secret_vars"])
		}
		writeEnvelope(w, http.StatusAccepted, "")
	})
	mux.HandleFunc("GET /api/v1/apps/{id}/operations", func(w http.ResponseWriter, _ *http.Request) {
		now := time.Now().UTC().Format(time.RFC3339Nano)
		writeEnvelope(w, http.StatusOK,
			`[{"operation_id":"op-upd","app_id":42,"action_type":"update","status":"succeeded","queued_at":"`+now+`"}]`)
	})
	srv := newServer(t, mux)
	mockEnv(t, srv.URL)

	if _, _, err := runCLI("apps", "update", "42", "--secret-var", "9"); err != nil {
		t.Fatalf("apps update --secret-var: %v", err)
	}
}

func TestAppsTLSFlagHidden(t *testing.T) {
	if certificateSecretsEnabled {
		t.Skip("certificate gate is on")
	}
	out, _, err := runCLI("apps", "create", "--help")
	if err != nil {
		t.Fatalf("apps create --help: %v", err)
	}
	if strings.Contains(out, "--tls-secret-id") {
		t.Errorf("tls-secret-id flag must be hidden: %q", out)
	}
}

func TestAppsCreateSecretVarNotFound(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/apps/validate-image", handlePullable)
	mux.HandleFunc("GET /api/v1/secrets/{id}", func(w http.ResponseWriter, _ *http.Request) {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "record not found")
	})
	mux.HandleFunc("POST /api/v1/apps", func(w http.ResponseWriter, _ *http.Request) {
		t.Error("POST /apps must not be reached when the secret is missing")
		writeEnvelope(w, http.StatusAccepted, "{}")
	})
	srv := newServer(t, mux)
	mockEnv(t, srv.URL)

	_, _, err := runCLI("apps", "create", "--name", "web-app", "--image", "nginx:1.27", "--port", "80",
		"--secret-var", "9", "--wait=false")
	if err == nil {
		t.Fatal("expected error for missing secret")
	}
	if !strings.Contains(err.Error(), "does not exist") || !strings.Contains(err.Error(), "--secret-var") {
		t.Errorf("error should name the flag and that it does not exist, got %q", err.Error())
	}
}

func TestAppsCreateSecretVarWrongType(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/apps/validate-image", handlePullable)
	mux.HandleFunc("GET /api/v1/secrets/{id}", handleSecretType("registry"))
	mux.HandleFunc("POST /api/v1/apps", func(w http.ResponseWriter, _ *http.Request) {
		t.Error("POST /apps must not be reached when the secret type is wrong")
		writeEnvelope(w, http.StatusAccepted, "{}")
	})
	srv := newServer(t, mux)
	mockEnv(t, srv.URL)

	_, _, err := runCLI("apps", "create", "--name", "web-app", "--image", "nginx:1.27", "--port", "80",
		"--secret-var", "9", "--wait=false")
	if err == nil {
		t.Fatal("expected error for wrong secret type")
	}
	if !strings.Contains(err.Error(), `"env_var"`) || !strings.Contains(err.Error(), `"registry"`) {
		t.Errorf("error should cite expected env_var vs actual registry, got %q", err.Error())
	}
}

func TestAppsCreateSecretVarValid(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/apps/validate-image", handlePullable)
	mux.HandleFunc("GET /api/v1/secrets/{id}", handleSecretType("env_var"))
	posted := false
	mux.HandleFunc("POST /api/v1/apps", func(w http.ResponseWriter, _ *http.Request) {
		posted = true
		writeEnvelope(w, http.StatusAccepted, `{"id":42,"name":"web-app","operation_id":"op-1","deployment_status":"deploying","updated_at":"2024-01-01T00:00:00Z"}`)
	})
	srv := newServer(t, mux)
	mockEnv(t, srv.URL)

	if _, _, err := runCLI("apps", "create", "--name", "web-app", "--image", "nginx:1.27", "--port", "80",
		"--secret-var", "9", "--wait=false"); err != nil {
		t.Fatalf("apps create with valid secret: %v", err)
	}
	if !posted {
		t.Error("POST /apps should have been reached for a valid secret")
	}
}

func TestAppsCreateSkipSecretChecks(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/apps/validate-image", handlePullable)
	mux.HandleFunc("GET /api/v1/secrets/{id}", func(w http.ResponseWriter, _ *http.Request) {
		t.Error("GET /secrets must not be reached when --skip-secret-checks is set")
		writeError(w, http.StatusNotFound, "NOT_FOUND", "record not found")
	})
	posted := false
	mux.HandleFunc("POST /api/v1/apps", func(w http.ResponseWriter, _ *http.Request) {
		posted = true
		writeEnvelope(w, http.StatusAccepted, `{"id":42,"name":"web-app","operation_id":"op-1","deployment_status":"deploying","updated_at":"2024-01-01T00:00:00Z"}`)
	})
	srv := newServer(t, mux)
	mockEnv(t, srv.URL)

	if _, _, err := runCLI("apps", "create", "--name", "web-app", "--image", "nginx:1.27", "--port", "80",
		"--secret-var", "9", "--skip-secret-checks", "--wait=false"); err != nil {
		t.Fatalf("apps create --skip-secret-checks: %v", err)
	}
	if !posted {
		t.Error("POST /apps should have been reached when the guard is skipped")
	}
}

func TestAppsCreateRegistryCredWrongType(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/apps/validate-image", handlePullable)
	mux.HandleFunc("GET /api/v1/secrets/{id}", handleSecretType("env_var"))
	mux.HandleFunc("POST /api/v1/apps", func(w http.ResponseWriter, _ *http.Request) {
		t.Error("POST /apps must not be reached for a wrong-typed registry credential")
		writeEnvelope(w, http.StatusAccepted, "{}")
	})
	srv := newServer(t, mux)
	mockEnv(t, srv.URL)

	_, _, err := runCLI("apps", "create", "--name", "web-app", "--image", "nginx:1.27", "--port", "80",
		"--registry-credential", "my-registry", "--wait=false")
	if err == nil {
		t.Fatal("expected error for wrong registry credential type")
	}
	if !strings.Contains(err.Error(), "--registry-credential") || !strings.Contains(err.Error(), `"registry"`) {
		t.Errorf("error should name the flag and the registry type, got %q", err.Error())
	}
}

func TestAppsCreateFileMountValid(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/apps/validate-image", handlePullable)
	mux.HandleFunc("GET /api/v1/secrets/{id}", handleSecretType("file"))
	posted := false
	mux.HandleFunc("POST /api/v1/apps", func(w http.ResponseWriter, _ *http.Request) {
		posted = true
		writeEnvelope(w, http.StatusAccepted, `{"id":42,"name":"web-app","operation_id":"op-1","deployment_status":"deploying","updated_at":"2024-01-01T00:00:00Z"}`)
	})
	srv := newServer(t, mux)
	mockEnv(t, srv.URL)

	if _, _, err := runCLI("apps", "create", "--name", "web-app", "--image", "nginx:1.27", "--port", "80",
		"--secret-file-mount", "tls-bundle:/etc/tls", "--wait=false"); err != nil {
		t.Fatalf("apps create with valid file mount: %v", err)
	}
	if !posted {
		t.Error("POST /apps should have been reached for a valid file mount")
	}
}

func TestAppsUpdateSecretValidation(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/apps/{id}", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("ETag", `W/"abc123"`)
		writeEnvelope(w, http.StatusOK, appDetailJSON)
	})
	mux.HandleFunc("GET /api/v1/secrets/{id}", func(w http.ResponseWriter, _ *http.Request) {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "record not found")
	})
	mux.HandleFunc("PATCH /api/v1/apps/{id}", func(w http.ResponseWriter, _ *http.Request) {
		t.Error("PATCH /apps must not be reached when the secret is missing")
		writeEnvelope(w, http.StatusAccepted, "")
	})
	srv := newServer(t, mux)
	mockEnv(t, srv.URL)

	_, _, err := runCLI("apps", "update", "42", "--secret-var", "9")
	if err == nil {
		t.Fatal("expected error for missing secret on update")
	}
	if !strings.Contains(err.Error(), "does not exist") {
		t.Errorf("update error should report the missing secret, got %q", err.Error())
	}
}

func TestAppsUpdateWait(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/apps/{id}", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("ETag", `W/"abc123"`)
		writeEnvelope(w, http.StatusOK, appDetailJSON)
	})
	mux.HandleFunc("PATCH /api/v1/apps/{id}", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("If-Match") != `W/"abc123"` {
			t.Errorf("expected If-Match header, got %q", r.Header.Get("If-Match"))
		}
		writeEnvelope(w, http.StatusAccepted, "")
	})
	mux.HandleFunc("GET /api/v1/apps/{id}/operations", func(w http.ResponseWriter, _ *http.Request) {
		now := time.Now().UTC().Format(time.RFC3339Nano)
		writeEnvelope(w, http.StatusOK,
			`[{"operation_id":"op-upd","app_id":42,"action_type":"update","status":"succeeded","queued_at":"`+now+`"}]`)
	})
	srv := newServer(t, mux)
	mockEnv(t, srv.URL)

	out, _, err := runCLI("apps", "update", "42", "--replicas", "3")
	if err != nil {
		t.Fatalf("apps update: %v", err)
	}
	if !strings.Contains(out, "updated") {
		t.Errorf("unexpected update output: %q", out)
	}
}

func TestAppsUpdateETagMismatch(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/apps/{id}", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("ETag", `W/"abc123"`)
		writeEnvelope(w, http.StatusOK, appDetailJSON)
	})
	mux.HandleFunc("PATCH /api/v1/apps/{id}", func(w http.ResponseWriter, _ *http.Request) {
		writeError(w, http.StatusPreconditionFailed, "ETAG_MISMATCH", "resource was modified")
	})
	srv := newServer(t, mux)
	mockEnv(t, srv.URL)

	_, _, err := runCLI("apps", "update", "42", "--replicas", "3")
	if err == nil {
		t.Fatal("expected ETag mismatch error")
	}
	if !strings.Contains(err.Error(), "re-run the update") {
		t.Errorf("unexpected etag error: %v", err)
	}
}

func TestAppsDeleteWait(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("DELETE /api/v1/apps/{id}", func(w http.ResponseWriter, _ *http.Request) {
		writeEnvelope(w, http.StatusAccepted, "")
	})
	getCalls := 0
	mux.HandleFunc("GET /api/v1/apps/{id}", func(w http.ResponseWriter, r *http.Request) {
		// First call is the resolver pre-fetch; subsequent calls are the
		// post-delete poll that should observe the 404.
		getCalls++
		if getCalls == 1 {
			handleAppDetail()(w, r)
			return
		}
		writeError(w, http.StatusNotFound, "APP_NOT_FOUND", "app not found")
	})
	srv := newServer(t, mux)
	mockEnv(t, srv.URL)

	out, _, err := runCLI("apps", "delete", "42", "--yes", "--wait")
	if err != nil {
		t.Fatalf("apps delete: %v", err)
	}
	if !strings.Contains(out, "deleted") {
		t.Errorf("unexpected delete output: %q", out)
	}
}

func TestAppsDeleteRequiresConfirmation(t *testing.T) {
	mockEnv(t, "http://127.0.0.1:0")
	// stdin is not a terminal under `go test`, so confirm() refuses.
	if _, _, err := runCLI("apps", "delete", "42"); err == nil {
		t.Fatal("expected delete without --yes to be refused")
	}
}

func TestAppsStartNoWait(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/apps/{id}", handleAppDetail())
	mux.HandleFunc("POST /api/v1/apps/{id}/start", func(w http.ResponseWriter, _ *http.Request) {
		writeEnvelope(w, http.StatusAccepted, "")
	})
	srv := newServer(t, mux)
	mockEnv(t, srv.URL)

	out, _, err := runCLI("apps", "start", "42")
	if err != nil {
		t.Fatalf("apps start: %v", err)
	}
	if !strings.Contains(out, "queued") {
		t.Errorf("unexpected start output: %q", out)
	}
}

func TestAppsStartWait(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/apps/{id}", handleAppDetail())
	mux.HandleFunc("POST /api/v1/apps/{id}/start", func(w http.ResponseWriter, _ *http.Request) {
		writeEnvelope(w, http.StatusAccepted, "")
	})
	mux.HandleFunc("GET /api/v1/apps/{id}/operations", func(w http.ResponseWriter, _ *http.Request) {
		now := time.Now().UTC().Format(time.RFC3339Nano)
		writeEnvelope(w, http.StatusOK,
			`[{"operation_id":"op-start","app_id":42,"action_type":"start","status":"succeeded","queued_at":"`+now+`"}]`)
	})
	srv := newServer(t, mux)
	mockEnv(t, srv.URL)

	out, _, err := runCLI("apps", "start", "42", "--wait")
	if err != nil {
		t.Fatalf("apps start --wait: %v", err)
	}
	if !strings.Contains(out, "started") {
		t.Errorf("unexpected start wait output: %q", out)
	}
}

func TestAppsStopAlreadyStopped(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/apps/{id}", handleAppDetail())
	mux.HandleFunc("POST /api/v1/apps/{id}/stop", func(w http.ResponseWriter, _ *http.Request) {
		writeError(w, http.StatusConflict, "APP_ALREADY_STOPPED", "app is already stopped")
	})
	srv := newServer(t, mux)
	mockEnv(t, srv.URL)

	out, _, err := runCLI("apps", "stop", "42")
	if err != nil {
		t.Fatalf("apps stop already-stopped should not error: %v", err)
	}
	if !strings.Contains(out, "already stopped") {
		t.Errorf("unexpected stop output: %q", out)
	}
}

func TestAppsOperationsList(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/apps/{id}", handleAppDetail())
	mux.HandleFunc("GET /api/v1/apps/{id}/operations", func(w http.ResponseWriter, _ *http.Request) {
		writeEnvelope(w, http.StatusOK,
			`[{"operation_id":"op-1","app_id":42,"action_type":"create","status":"succeeded","queued_at":"2024-01-01T00:00:00Z"}]`)
	})
	srv := newServer(t, mux)
	mockEnv(t, srv.URL)

	out, _, err := runCLI("apps", "operations", "42")
	if err != nil {
		t.Fatalf("apps operations: %v", err)
	}
	if !strings.Contains(out, "op-1") || !strings.Contains(out, "OPERATION ID") {
		t.Errorf("unexpected operations output: %q", out)
	}
}

func TestAppsOperationsGet(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/apps/{id}", handleAppDetail())
	mux.HandleFunc("GET /api/v1/apps/{id}/operations/{opid}", func(w http.ResponseWriter, _ *http.Request) {
		writeEnvelope(w, http.StatusOK,
			`{"operation_id":"op-1","app_id":42,"action_type":"create","status":"succeeded","queued_at":"2024-01-01T00:00:00Z"}`)
	})
	srv := newServer(t, mux)
	mockEnv(t, srv.URL)

	out, _, err := runCLI("apps", "operations", "42", "op-1")
	if err != nil {
		t.Fatalf("apps operations get: %v", err)
	}
	if !strings.Contains(out, "Operation ID:") || !strings.Contains(out, "op-1") {
		t.Errorf("unexpected operation detail output: %q", out)
	}
}

func TestAppsDomainAddGetVerifyRemove(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/apps/{id}", handleAppDetail())
	mux.HandleFunc("POST /api/v1/apps/{id}/custom-domain", func(w http.ResponseWriter, _ *http.Request) {
		writeEnvelope(w, http.StatusOK, `{"domain":"example.com","verification_status":"pending"}`)
	})
	mux.HandleFunc("GET /api/v1/apps/{id}/custom-domain", func(w http.ResponseWriter, _ *http.Request) {
		writeEnvelope(w, http.StatusOK, `{"domain":"example.com","verification_status":"pending"}`)
	})
	mux.HandleFunc("POST /api/v1/apps/{id}/custom-domain/verify", func(w http.ResponseWriter, _ *http.Request) {
		writeEnvelope(w, http.StatusOK, `{"domain":"example.com","verification_status":"verified"}`)
	})
	mux.HandleFunc("DELETE /api/v1/apps/{id}/custom-domain", func(w http.ResponseWriter, _ *http.Request) {
		writeEnvelope(w, http.StatusOK, "")
	})
	srv := newServer(t, mux)
	mockEnv(t, srv.URL)

	out, _, err := runCLI("apps", "domain", "add", "42", "example.com")
	if err != nil {
		t.Fatalf("domain add: %v", err)
	}
	if !strings.Contains(out, "example.com") || !strings.Contains(out, "pending") {
		t.Errorf("unexpected domain add output: %q", out)
	}

	if out, _, err = runCLI("apps", "domain", "get", "42"); err != nil {
		t.Fatalf("domain get: %v", err)
	} else if !strings.Contains(out, "example.com") {
		t.Errorf("unexpected domain get output: %q", out)
	}

	if out, _, err = runCLI("apps", "domain", "verify", "42"); err != nil {
		t.Fatalf("domain verify: %v", err)
	} else if !strings.Contains(out, "verified") {
		t.Errorf("unexpected domain verify output: %q", out)
	}

	if out, _, err = runCLI("apps", "domain", "remove", "42", "-y"); err != nil {
		t.Fatalf("domain remove: %v", err)
	} else if !strings.Contains(out, "detached") {
		t.Errorf("unexpected domain remove output: %q", out)
	}
}

func TestAppsPlans(t *testing.T) {
	mux := http.NewServeMux()
	gotSort := ""
	mux.HandleFunc("GET /api/v1/apps/plans", func(w http.ResponseWriter, r *http.Request) {
		gotSort = r.URL.Query().Get("sort")
		writeEnvelope(w, http.StatusOK,
			`{"templates":[{"slug":"app-small","name":"Small","description":"",`+
				`"cpu_request_vcpu":"0.25","cpu_limit_vcpu":"1","memory_request_mb":256,`+
				`"memory_limit_mb":512,"price_hour":"1500.00","price_day":"36000.00","price_month":"1000000.00"}]}`)
	})
	srv := newServer(t, mux)
	mockEnv(t, srv.URL)

	out, _, err := runCLI("apps", "plans", "--sort", "price_month")
	if err != nil {
		t.Fatalf("apps plans: %v", err)
	}
	if gotSort != "price_month" {
		t.Errorf("expected sort query param, got %q", gotSort)
	}
	for _, want := range []string{"SLUG", "PRICE/MO", "app-small", "Small", "0.25→1", "256→512", "1500.00", "1000000.00"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q: %s", want, out)
		}
	}
}

func TestAppsNotLoggedIn(t *testing.T) {
	t.Setenv("KUMO_HOME", t.TempDir())
	t.Setenv("KUMO_PROFILE", "")
	t.Setenv("KUMO_API_KEY", "")
	t.Setenv("KUMO_BASE_URL", "https://api.kumo.run")
	t.Setenv("KUMO_OUTPUT", "")

	if _, _, err := runCLI("apps", "list"); err == nil {
		t.Fatal("expected not-logged-in error")
	}
}
