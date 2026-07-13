package cli

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

const secretListJSON = `[{"id":5,"name":"app-env","type":"env_var","used_by_count":1,` +
	`"created_at":"2024-01-01T00:00:00Z","updated_at":"2024-01-02T00:00:00Z"}]`

const secretEnvDetailJSON = `{"id":5,"name":"app-env","type":"env_var","used_by_count":1,` +
	`"used_by":[{"app_id":42,"app_name":"web-app","usage_type":"attached"}],` +
	`"environment_variables":[{"key":"FOO","value":"super-secret"}],` +
	`"created_at":"2024-01-01T00:00:00Z","updated_at":"2024-01-02T00:00:00Z"}`

const secretRegistryDetailJSON = `{"id":6,"name":"reg","type":"registry","used_by_count":0,` +
	`"secret_registry":{"registry_host":"ghcr.io","username":"alice","password":"hunter2"},` +
	`"created_at":"2024-01-01T00:00:00Z","updated_at":"2024-01-02T00:00:00Z"}`

func TestSecretList(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/secrets", func(w http.ResponseWriter, _ *http.Request) {
		writeEnvelope(w, http.StatusOK, secretListJSON)
	})
	srv := newServer(t, mux)
	mockEnv(t, srv.URL)

	out, _, err := runCLI("secret", "list")
	if err != nil {
		t.Fatalf("secret list: %v", err)
	}
	if !strings.Contains(out, "app-env") || !strings.Contains(out, "env_var") {
		t.Errorf("unexpected list output: %q", out)
	}
	if !strings.Contains(out, "USED BY") {
		t.Errorf("missing table header: %q", out)
	}
}

func TestSecretListJSON(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/secrets", func(w http.ResponseWriter, _ *http.Request) {
		writeEnvelope(w, http.StatusOK, secretListJSON)
	})
	srv := newServer(t, mux)
	mockEnv(t, srv.URL)

	out, _, err := runCLI("secret", "list", "-o", "json")
	if err != nil {
		t.Fatalf("secret list json: %v", err)
	}
	if !strings.Contains(out, `"name": "app-env"`) {
		t.Errorf("unexpected list json: %q", out)
	}
}

func TestSecretListFilters(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/secrets", func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("type"); got != "registry" {
			t.Errorf("type query = %q, want registry", got)
		}
		if got := r.URL.Query().Get("search"); got != "reg" {
			t.Errorf("search query = %q, want reg", got)
		}
		writeEnvelope(w, http.StatusOK, "[]")
	})
	srv := newServer(t, mux)
	mockEnv(t, srv.URL)

	if _, _, err := runCLI("secret", "list", "--type", "registry", "--search", "reg"); err != nil {
		t.Fatalf("secret list filtered: %v", err)
	}
}

func TestSecretGetMasked(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/secrets/{id}", func(w http.ResponseWriter, _ *http.Request) {
		writeEnvelope(w, http.StatusOK, secretEnvDetailJSON)
	})
	srv := newServer(t, mux)
	mockEnv(t, srv.URL)

	out, _, err := runCLI("secret", "get", "5")
	if err != nil {
		t.Fatalf("secret get: %v", err)
	}
	if strings.Contains(out, "super-secret") {
		t.Errorf("masked get must not reveal value: %q", out)
	}
	if !strings.Contains(out, "FOO") || !strings.Contains(out, "web-app") {
		t.Errorf("expected key and consumer in output: %q", out)
	}
}

func TestSecretGetReveal(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/secrets/{id}", func(w http.ResponseWriter, _ *http.Request) {
		writeEnvelope(w, http.StatusOK, secretRegistryDetailJSON)
	})
	srv := newServer(t, mux)
	mockEnv(t, srv.URL)

	out, _, err := runCLI("secret", "get", "6", "--reveal")
	if err != nil {
		t.Fatalf("secret get --reveal: %v", err)
	}
	if !strings.Contains(out, "hunter2") || !strings.Contains(out, "alice") || !strings.Contains(out, "ghcr.io") {
		t.Errorf("reveal should show registry credentials: %q", out)
	}
}

func TestSecretGetJSON(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/secrets/{id}", func(w http.ResponseWriter, _ *http.Request) {
		writeEnvelope(w, http.StatusOK, secretEnvDetailJSON)
	})
	srv := newServer(t, mux)
	mockEnv(t, srv.URL)

	out, _, err := runCLI("secret", "get", "5", "-o", "json")
	if err != nil {
		t.Fatalf("secret get json: %v", err)
	}
	if !strings.Contains(out, `"super-secret"`) {
		t.Errorf("json output should carry raw payload: %q", out)
	}
}

func TestSecretGetNotFound(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/secrets/{id}", func(w http.ResponseWriter, _ *http.Request) {
		writeError(w, http.StatusNotFound, "SECRET_NOT_FOUND", "secret not found")
	})
	srv := newServer(t, mux)
	mockEnv(t, srv.URL)

	if _, _, err := runCLI("secret", "get", "99"); err == nil {
		t.Fatal("expected error for missing secret")
	}
}

func TestSecretGetInvalidID(t *testing.T) {
	mockEnv(t, "http://127.0.0.1:0")
	if _, _, err := runCLI("secret", "get", "abc"); err == nil {
		t.Fatal("expected error for non-numeric id")
	}
}

func TestSecretCreateEnvVar(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/secrets", func(w http.ResponseWriter, r *http.Request) {
		var req map[string]any
		decodeBody(t, r, &req)
		if req["type"] != "env_var" {
			t.Errorf("type = %v, want env_var", req["type"])
		}
		if _, ok := req["environment_variables"]; !ok {
			t.Errorf("missing environment_variables in body: %v", req)
		}
		writeEnvelope(w, http.StatusOK, `{"id":5,"name":"app-env","type":"env_var"}`)
	})
	srv := newServer(t, mux)
	mockEnv(t, srv.URL)

	out, _, err := runCLI("secret", "create", "--type", "env_var", "--name", "app-env", "--env", "FOO=bar")
	if err != nil {
		t.Fatalf("secret create env_var: %v", err)
	}
	if !strings.Contains(out, "Created secret") || !strings.Contains(out, "app-env") {
		t.Errorf("unexpected create output: %q", out)
	}
}

func TestSecretCreateRegistry(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/secrets", func(w http.ResponseWriter, r *http.Request) {
		var req map[string]any
		decodeBody(t, r, &req)
		reg, ok := req["secret_registry"].(map[string]any)
		if !ok || reg["username"] != "alice" || reg["password"] != "hunter2" {
			t.Errorf("registry payload wrong: %v", req)
		}
		writeEnvelope(w, http.StatusOK, `{"id":6,"name":"reg","type":"registry"}`)
	})
	srv := newServer(t, mux)
	mockEnv(t, srv.URL)

	_, _, err := runCLI("secret", "create", "--type", "registry", "--name", "reg",
		"--registry-username", "alice", "--registry-password", "hunter2")
	if err != nil {
		t.Fatalf("secret create registry: %v", err)
	}
}

func TestSecretCreateFile(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/secrets", func(w http.ResponseWriter, r *http.Request) {
		var req map[string]any
		decodeBody(t, r, &req)
		if req["file_content"] != "hello" {
			t.Errorf("file_content = %v, want hello", req["file_content"])
		}
		writeEnvelope(w, http.StatusOK, `{"id":7,"name":"f","type":"file"}`)
	})
	srv := newServer(t, mux)
	mockEnv(t, srv.URL)

	if _, _, err := runCLI("secret", "create", "--type", "file", "--name", "f", "--content", "hello"); err != nil {
		t.Fatalf("secret create file: %v", err)
	}
}

func TestSecretCreateRequiresName(t *testing.T) {
	mockEnv(t, "http://127.0.0.1:0")
	if _, _, err := runCLI("secret", "create", "--type", "env_var", "--env", "FOO=bar"); err == nil {
		t.Fatal("expected error when --name is missing")
	}
}

func TestSecretCreateTypeMismatch(t *testing.T) {
	mockEnv(t, "http://127.0.0.1:0")
	// --registry-username belongs to registry, not env_var.
	if _, _, err := runCLI("secret", "create", "--type", "env_var", "--name", "x",
		"--env", "FOO=bar", "--registry-username", "alice"); err == nil {
		t.Fatal("expected error for foreign payload flag")
	}
}

func TestSecretCreateUnknownType(t *testing.T) {
	mockEnv(t, "http://127.0.0.1:0")
	if _, _, err := runCLI("secret", "create", "--type", "bogus", "--name", "x"); err == nil {
		t.Fatal("expected error for unknown type")
	}
}

func TestSecretCreateCertificateRejected(t *testing.T) {
	if certificateSecretsEnabled {
		t.Skip("certificate gate is on")
	}
	mockEnv(t, "http://127.0.0.1:0")
	if _, _, err := runCLI("secret", "create", "--type", "certificate", "--name", "x"); err == nil {
		t.Fatal("expected certificate type to be rejected while gated off")
	}
}

func TestSecretUpdate(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/secrets/{id}", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("ETag", `W/"abc123"`)
		writeEnvelope(w, http.StatusOK, secretEnvDetailJSON)
	})
	mux.HandleFunc("PATCH /api/v1/secrets/{id}", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("If-Match") != `W/"abc123"` {
			t.Errorf("expected If-Match header, got %q", r.Header.Get("If-Match"))
		}
		var req map[string]any
		decodeBody(t, r, &req)
		if req["type"] != "env_var" {
			t.Errorf("update must preserve type, got %v", req["type"])
		}
		writeEnvelope(w, http.StatusOK, secretEnvDetailJSON)
	})
	srv := newServer(t, mux)
	mockEnv(t, srv.URL)

	out, _, err := runCLI("secret", "update", "5", "--env", "FOO=baz")
	if err != nil {
		t.Fatalf("secret update: %v", err)
	}
	if !strings.Contains(out, "Updated secret") {
		t.Errorf("unexpected update output: %q", out)
	}
}

func TestSecretUpdateETagMismatch(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/secrets/{id}", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("ETag", `W/"abc123"`)
		writeEnvelope(w, http.StatusOK, secretEnvDetailJSON)
	})
	mux.HandleFunc("PATCH /api/v1/secrets/{id}", func(w http.ResponseWriter, _ *http.Request) {
		writeError(w, http.StatusPreconditionFailed, "ETAG_MISMATCH", "resource was modified")
	})
	srv := newServer(t, mux)
	mockEnv(t, srv.URL)

	_, _, err := runCLI("secret", "update", "5", "--env", "FOO=baz")
	if err == nil {
		t.Fatal("expected ETag mismatch error")
	}
	if !strings.Contains(err.Error(), "re-run the update") {
		t.Errorf("unexpected etag error: %v", err)
	}
}

func TestSecretDelete(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/secrets/{id}", handleSecretDetail())
	mux.HandleFunc("DELETE /api/v1/secrets/{id}", func(w http.ResponseWriter, _ *http.Request) {
		writeEnvelope(w, http.StatusOK, "")
	})
	srv := newServer(t, mux)
	mockEnv(t, srv.URL)

	out, _, err := runCLI("secret", "delete", "5", "--yes")
	if err != nil {
		t.Fatalf("secret delete: %v", err)
	}
	if !strings.Contains(out, "deleted") {
		t.Errorf("unexpected delete output: %q", out)
	}
}

func TestSecretDeleteRequiresConfirmation(t *testing.T) {
	mockEnv(t, "http://127.0.0.1:0")
	// stdin is not a terminal under `go test`, so confirm() refuses.
	if _, _, err := runCLI("secret", "delete", "5"); err == nil {
		t.Fatal("expected delete without --yes to be refused")
	}
}

func TestSecretDeleteInUse(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/secrets/{id}", handleSecretDetail())
	mux.HandleFunc("DELETE /api/v1/secrets/{id}", func(w http.ResponseWriter, _ *http.Request) {
		writeError(w, http.StatusConflict, "SECRET_IN_USE", "secret is referenced by apps")
	})
	srv := newServer(t, mux)
	mockEnv(t, srv.URL)

	_, _, err := runCLI("secret", "delete", "5", "--yes")
	if err == nil {
		t.Fatal("expected in-use delete to error")
	}
	if !strings.Contains(err.Error(), "in use") || !strings.Contains(err.Error(), "secret get") {
		t.Errorf("unexpected in-use error: %v", err)
	}
}

func TestSecretNotLoggedIn(t *testing.T) {
	t.Setenv("KUMO_HOME", t.TempDir())
	t.Setenv("KUMO_PROFILE", "")
	t.Setenv("KUMO_API_KEY", "")
	t.Setenv("KUMO_BASE_URL", "https://api.kumo.run")
	t.Setenv("KUMO_OUTPUT", "")

	if _, _, err := runCLI("secret", "list"); err == nil {
		t.Fatal("expected not-logged-in error")
	}
}

func TestSecretCertificateHidden(t *testing.T) {
	if certificateSecretsEnabled {
		t.Skip("certificate gate is on")
	}
	out, _, err := runCLI("secret", "create", "--help")
	if err != nil {
		t.Fatalf("secret create --help: %v", err)
	}
	if strings.Contains(out, "--cert-file") || strings.Contains(out, "--key-file") {
		t.Errorf("certificate flags must be hidden: %q", out)
	}
}

// decodeBody decodes a request's JSON body into v, failing the test on error.
func decodeBody(t *testing.T, r *http.Request, v any) {
	t.Helper()
	b, err := io.ReadAll(r.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if err := json.Unmarshal(b, v); err != nil {
		t.Fatalf("decode body %q: %v", b, err)
	}
}
