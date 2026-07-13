package cli

import (
	"net/http"
	"strings"
	"testing"
)

const apikeyPersonalJSON = `{"id":1,"name":"laptop","key_prefix":"kumo_sk_live_abcd",` +
	`"last_used_at":"2024-01-10T00:00:00Z","expires_at":null,` +
	`"created_at":"2024-01-01T00:00:00Z","scopes":["read","write"]}`

const apikeyRegistryJSON = `{"id":2,"name":"docker-bot","key_prefix":"kumo_sk_live_efgh",` +
	`"last_used_at":null,"expires_at":"2025-01-01T00:00:00Z",` +
	`"created_at":"2024-01-01T00:00:00Z","scopes":[],` +
	`"registry_org_slug":"acme","registry_repo_name":"web",` +
	`"registry_permissions":["pull","push"]}`

// apikeyGrantsJSON is a unified-model key: it carries grants (and conditions)
// instead of the legacy scopes / registry_* fields.
const apikeyGrantsJSON = `{"id":3,"name":"ci-bot","key_prefix":"kumo_sk_live_ijkl",` +
	`"last_used_at":null,"expires_at":null,"created_at":"2024-01-01T00:00:00Z","scopes":[],` +
	`"grants":[{"domain":"registry","actions":["pull","push"],"orgs":["acme"]},` +
	`{"domain":"control_plane","actions":["read"]}],` +
	`"conditions":{"ip_allowlist":["10.0.0.0/8"]}}`

func TestAPIKeyList(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/api-keys", func(w http.ResponseWriter, _ *http.Request) {
		writeEnvelope(w, http.StatusOK, "["+apikeyPersonalJSON+","+apikeyRegistryJSON+"]")
	})
	srv := newServer(t, mux)
	mockEnv(t, srv.URL)

	out, _, err := runCLI("apikey", "list")
	if err != nil {
		t.Fatalf("apikey list: %v", err)
	}
	for _, want := range []string{"NAME", "KIND", "laptop", "docker-bot", "personal", "registry", "pull,push", "read,write"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q: %s", want, out)
		}
	}
}

func TestAPIKeyListJSON(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/api-keys", func(w http.ResponseWriter, _ *http.Request) {
		writeEnvelope(w, http.StatusOK, "["+apikeyPersonalJSON+"]")
	})
	srv := newServer(t, mux)
	mockEnv(t, srv.URL)

	out, _, err := runCLI("apikey", "list", "-o", "json")
	if err != nil {
		t.Fatalf("apikey list json: %v", err)
	}
	var got []map[string]any
	decodeData(t, out, &got)
	if len(got) != 1 || got[0]["name"] != "laptop" {
		t.Errorf("unexpected json: %s", out)
	}
}

func TestAPIKeyGetRegistry(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/api-keys/docker-bot", func(w http.ResponseWriter, _ *http.Request) {
		writeEnvelope(w, http.StatusOK, apikeyRegistryJSON)
	})
	srv := newServer(t, mux)
	mockEnv(t, srv.URL)

	out, _, err := runCLI("apikey", "get", "docker-bot")
	if err != nil {
		t.Fatalf("apikey get: %v", err)
	}
	for _, want := range []string{"docker-bot", "registry", "acme", "web", "pull,push"} {
		if !strings.Contains(out, want) {
			t.Errorf("detail missing %q: %s", want, out)
		}
	}
}

func TestAPIKeyGetPersonalShowsOrgWide(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/api-keys/laptop", func(w http.ResponseWriter, _ *http.Request) {
		writeEnvelope(w, http.StatusOK, apikeyPersonalJSON)
	})
	srv := newServer(t, mux)
	mockEnv(t, srv.URL)

	out, _, err := runCLI("apikey", "get", "laptop")
	if err != nil {
		t.Fatalf("apikey get personal: %v", err)
	}
	if !strings.Contains(out, "Scopes:") || !strings.Contains(out, "read,write") {
		t.Errorf("personal detail missing scopes: %s", out)
	}
	if strings.Contains(out, "Registry org:") {
		t.Errorf("personal detail should not list registry fields: %s", out)
	}
}

func TestAPIKeyListShowsGrants(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/api-keys", func(w http.ResponseWriter, _ *http.Request) {
		writeEnvelope(w, http.StatusOK, "["+apikeyGrantsJSON+"]")
	})
	srv := newServer(t, mux)
	mockEnv(t, srv.URL)

	out, _, err := runCLI("apikey", "list")
	if err != nil {
		t.Fatalf("apikey list grants: %v", err)
	}
	// The SCOPES column summarizes grants as "domain:actions" segments.
	for _, want := range []string{"ci-bot", "registry:pull,push", "control_plane:read"} {
		if !strings.Contains(out, want) {
			t.Errorf("list missing grant summary %q: %s", want, out)
		}
	}
}

func TestAPIKeyGetShowsGrants(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/api-keys/ci-bot", func(w http.ResponseWriter, _ *http.Request) {
		writeEnvelope(w, http.StatusOK, apikeyGrantsJSON)
	})
	srv := newServer(t, mux)
	mockEnv(t, srv.URL)

	out, _, err := runCLI("apikey", "get", "ci-bot")
	if err != nil {
		t.Fatalf("apikey get grants: %v", err)
	}
	for _, want := range []string{"Grant:", "registry", "pull,push", "acme", "control_plane", "all orgs", "IP allowlist:", "10.0.0.0/8"} {
		if !strings.Contains(out, want) {
			t.Errorf("detail missing %q: %s", want, out)
		}
	}
	// A grants key must not fall back to the legacy "Scopes:" line.
	if strings.Contains(out, "Scopes:") {
		t.Errorf("grants key should not print legacy Scopes line: %s", out)
	}
}

func TestAPIKeyGetNotFound(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/api-keys/missing", func(w http.ResponseWriter, _ *http.Request) {
		writeError(w, http.StatusNotFound, "API_KEY_NOT_FOUND", "not found")
	})
	srv := newServer(t, mux)
	mockEnv(t, srv.URL)

	_, _, err := runCLI("apikey", "get", "missing")
	if err == nil || !strings.Contains(err.Error(), `no api key named "missing"`) {
		t.Fatalf("expected friendly not-found error, got: %v", err)
	}
}

func TestAPIKeyListSessionRequired(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/api-keys", func(w http.ResponseWriter, _ *http.Request) {
		writeError(w, http.StatusForbidden, "API_KEY_SESSION_REQUIRED", "session required")
	})
	srv := newServer(t, mux)
	mockEnv(t, srv.URL)

	_, _, err := runCLI("apikey", "list")
	if err == nil {
		t.Fatal("expected session-required error, got nil")
	}
	msg := err.Error()
	if !strings.Contains(msg, "session login") || !strings.Contains(msg, "dashboard") {
		t.Errorf("error should point at session login + dashboard, got: %s", msg)
	}
}

func TestAPIKeyGetAmbiguous(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/api-keys/dup", func(w http.ResponseWriter, _ *http.Request) {
		writeError(w, http.StatusConflict, "AMBIGUOUS_NAME", "multiple keys")
	})
	srv := newServer(t, mux)
	mockEnv(t, srv.URL)

	_, _, err := runCLI("apikey", "get", "dup")
	if err == nil || !strings.Contains(err.Error(), "multiple api keys") || !strings.Contains(err.Error(), "disambiguate") {
		t.Fatalf("expected ambiguous-name guidance, got: %v", err)
	}
}

func TestAPIKeyGetEmptyName(t *testing.T) {
	srv := newServer(t, http.NewServeMux())
	mockEnv(t, srv.URL)

	_, _, err := runCLI("apikey", "get", "  ")
	if err == nil || !strings.Contains(err.Error(), "name is required") {
		t.Fatalf("expected name-required error, got: %v", err)
	}
}

func TestRootCommandLists4NewGroups(t *testing.T) {
	srv := newServer(t, http.NewServeMux())
	mockEnv(t, srv.URL)

	out, _, err := runCLI("--help")
	if err != nil {
		t.Fatalf("--help: %v", err)
	}
	for _, want := range []string{"apikey", "registry", "volume", "vps"} {
		if !strings.Contains(out, want) {
			t.Errorf("--help missing command %q: %s", want, out)
		}
	}
}
