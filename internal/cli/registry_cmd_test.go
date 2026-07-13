package cli

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

const orgAcmeJSON = `{"id":1,"slug":"acme","display_name":"Acme Inc",` +
	`"owner_user_id":1,"registry_auto_create_repos":true,` +
	`"created_at":"2024-01-01T00:00:00Z","updated_at":"2024-01-02T00:00:00Z"}`

const orgBetaJSON = `{"id":2,"slug":"beta","display_name":"Beta Org",` +
	`"owner_user_id":1,"registry_auto_create_repos":false,` +
	`"created_at":"2024-01-03T00:00:00Z","updated_at":"2024-01-04T00:00:00Z"}`

// orgDefaultJSON is the platform-provisioned default org (undeletable).
const orgDefaultJSON = `{"id":1,"slug":"acme","display_name":"Acme Inc","is_default":true,` +
	`"owner_user_id":1,"registry_auto_create_repos":true,` +
	`"created_at":"2024-01-01T00:00:00Z","updated_at":"2024-01-02T00:00:00Z"}`

const repoWebJSON = `{"id":10,"name":"web","tag_mutability":"MUTABLE","soft_delete_days":7,` +
	`"created_at":"2024-02-01T00:00:00Z","updated_at":"2024-02-02T00:00:00Z"}`

const manifestImageJSON = `{"id":100,"digest":"sha256:abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890",` +
	`"tag":"v1","media_type":"application/vnd.oci.image.manifest.v1+json","kind":"image",` +
	`"size_bytes":2048,"image_size_bytes":10485760,"architecture":"amd64","os":"linux",` +
	`"platform":"linux/amd64","pushed_at":"2024-03-01T00:00:00Z"}`

func TestRegistryOrgList(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/registry/organizations", func(w http.ResponseWriter, _ *http.Request) {
		writeEnvelope(w, http.StatusOK, "["+orgAcmeJSON+","+orgBetaJSON+"]")
	})
	srv := newServer(t, mux)
	mockEnv(t, srv.URL)

	out, _, err := runCLI("registry", "org", "list")
	if err != nil {
		t.Fatalf("registry org list: %v", err)
	}
	for _, want := range []string{"SLUG", "DISPLAY NAME", "DEFAULT", "AUTO-CREATE REPOS", "acme", "beta", "Acme Inc"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q: %s", want, out)
		}
	}
}

func TestRegistryOrgListJSON(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/registry/organizations", func(w http.ResponseWriter, _ *http.Request) {
		writeEnvelope(w, http.StatusOK, "["+orgAcmeJSON+"]")
	})
	srv := newServer(t, mux)
	mockEnv(t, srv.URL)

	out, _, err := runCLI("registry", "org", "list", "-o", "json")
	if err != nil {
		t.Fatalf("registry org list json: %v", err)
	}
	var got []map[string]any
	if err := json.NewDecoder(strings.NewReader(out)).Decode(&got); err != nil && err != io.EOF {
		t.Fatalf("decode json: %v", err)
	}
	if len(got) != 1 || got[0]["slug"] != "acme" {
		t.Errorf("unexpected json: %s", out)
	}
}

func TestRegistryOrgGet(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/registry/organizations/acme", func(w http.ResponseWriter, _ *http.Request) {
		writeEnvelope(w, http.StatusOK, orgAcmeJSON)
	})
	srv := newServer(t, mux)
	mockEnv(t, srv.URL)

	out, _, err := runCLI("registry", "org", "get", "acme")
	if err != nil {
		t.Fatalf("registry org get: %v", err)
	}
	for _, want := range []string{"Slug:", "acme", "Acme Inc", "Auto-create repos:"} {
		if !strings.Contains(out, want) {
			t.Errorf("detail missing %q: %s", want, out)
		}
	}
}

func TestRegistryOrgGetNotFound(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/registry/organizations/missing", func(w http.ResponseWriter, _ *http.Request) {
		writeError(w, http.StatusNotFound, "ORG_NOT_FOUND", "not found")
	})
	srv := newServer(t, mux)
	mockEnv(t, srv.URL)

	_, _, err := runCLI("registry", "org", "get", "missing")
	if err == nil || !strings.Contains(err.Error(), `no org named "missing"`) {
		t.Fatalf("expected friendly not-found error, got: %v", err)
	}
}

func TestRegistryRepoList(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/registry/organizations", func(w http.ResponseWriter, _ *http.Request) {
		writeEnvelope(w, http.StatusOK, "["+orgAcmeJSON+"]")
	})
	mux.HandleFunc("GET /api/v1/registry/organizations/acme/repositories", func(w http.ResponseWriter, _ *http.Request) {
		writeEnvelope(w, http.StatusOK, "["+repoWebJSON+"]")
	})
	srv := newServer(t, mux)
	mockEnv(t, srv.URL)

	out, _, err := runCLI("registry", "repo", "list")
	if err != nil {
		t.Fatalf("registry repo list: %v", err)
	}
	for _, want := range []string{"NAME", "TAG MUTABILITY", "web", "MUTABLE"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q: %s", want, out)
		}
	}
}

func TestRegistryRepoListMultipleOrgsRequiresFlag(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/registry/organizations", func(w http.ResponseWriter, _ *http.Request) {
		writeEnvelope(w, http.StatusOK, "["+orgAcmeJSON+","+orgBetaJSON+"]")
	})
	srv := newServer(t, mux)
	mockEnv(t, srv.URL)

	_, _, err := runCLI("registry", "repo", "list")
	if err == nil || !strings.Contains(err.Error(), "specify --org") {
		t.Fatalf("expected multi-org error, got: %v", err)
	}
}

func TestRegistryRepoCreate(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/registry/organizations", func(w http.ResponseWriter, _ *http.Request) {
		writeEnvelope(w, http.StatusOK, "["+orgAcmeJSON+"]")
	})
	mux.HandleFunc("POST /api/v1/registry/organizations/acme/repositories", func(w http.ResponseWriter, r *http.Request) {
		var req map[string]any
		decodeBody(t, r, &req)
		if req["name"] != "web" {
			t.Errorf("name = %v, want web", req["name"])
		}
		if req["tag_mutability"] != "IMMUTABLE" {
			t.Errorf("tag_mutability = %v, want IMMUTABLE", req["tag_mutability"])
		}
		writeEnvelope(w, http.StatusOK, repoWebJSON)
	})
	srv := newServer(t, mux)
	mockEnv(t, srv.URL)

	out, _, err := runCLI("registry", "repo", "create", "web",
		"--tag-mutability", "IMMUTABLE")
	if err != nil {
		t.Fatalf("registry repo create: %v", err)
	}
	if !strings.Contains(out, "Created repo acme/web") {
		t.Errorf("unexpected create output: %q", out)
	}
}

func TestRegistryRepoCreateOmitsUnsetFlags(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/registry/organizations", func(w http.ResponseWriter, _ *http.Request) {
		writeEnvelope(w, http.StatusOK, "["+orgAcmeJSON+"]")
	})
	mux.HandleFunc("POST /api/v1/registry/organizations/acme/repositories", func(w http.ResponseWriter, r *http.Request) {
		var req map[string]any
		decodeBody(t, r, &req)
		if _, ok := req["tag_mutability"]; ok {
			t.Errorf("tag_mutability should be omitted when --tag-mutability not set: %v", req)
		}
		writeEnvelope(w, http.StatusOK, repoWebJSON)
	})
	srv := newServer(t, mux)
	mockEnv(t, srv.URL)

	if _, _, err := runCLI("registry", "repo", "create", "web"); err != nil {
		t.Fatalf("registry repo create defaults: %v", err)
	}
}

func TestRegistryRepoDelete(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/registry/organizations", func(w http.ResponseWriter, _ *http.Request) {
		writeEnvelope(w, http.StatusOK, "["+orgAcmeJSON+"]")
	})
	mux.HandleFunc("DELETE /api/v1/registry/organizations/acme/repositories/web", func(w http.ResponseWriter, _ *http.Request) {
		writeEnvelope(w, http.StatusOK, "")
	})
	srv := newServer(t, mux)
	mockEnv(t, srv.URL)

	out, _, err := runCLI("registry", "repo", "delete", "web", "-y")
	if err != nil {
		t.Fatalf("registry repo delete: %v", err)
	}
	if !strings.Contains(out, "deleted") {
		t.Errorf("unexpected delete output: %q", out)
	}
}

func TestRegistryRepoDeleteNotFound(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/registry/organizations", func(w http.ResponseWriter, _ *http.Request) {
		writeEnvelope(w, http.StatusOK, "["+orgAcmeJSON+"]")
	})
	mux.HandleFunc("DELETE /api/v1/registry/organizations/acme/repositories/missing", func(w http.ResponseWriter, _ *http.Request) {
		writeError(w, http.StatusNotFound, "REGISTRY_REPOSITORY_NOT_FOUND", "not found")
	})
	srv := newServer(t, mux)
	mockEnv(t, srv.URL)

	_, _, err := runCLI("registry", "repo", "delete", "missing", "-y")
	if err == nil || !strings.Contains(err.Error(), `no repo named "missing"`) {
		t.Fatalf("expected friendly not-found error, got: %v", err)
	}
}

func TestRegistryOrgDeleteHasRepos(t *testing.T) {
	mux := http.NewServeMux()
	// Delete pre-fetches the org to check IsDefault before the DELETE.
	mux.HandleFunc("GET /api/v1/registry/organizations/acme", func(w http.ResponseWriter, _ *http.Request) {
		writeEnvelope(w, http.StatusOK, orgAcmeJSON)
	})
	mux.HandleFunc("DELETE /api/v1/registry/organizations/acme", func(w http.ResponseWriter, _ *http.Request) {
		writeError(w, http.StatusConflict, "ORG_HAS_REPOS", "org has repos")
	})
	srv := newServer(t, mux)
	mockEnv(t, srv.URL)

	_, _, err := runCLI("registry", "org", "delete", "acme", "-y")
	if err == nil || !strings.Contains(err.Error(), "delete them first") {
		t.Fatalf("expected has-repos error, got: %v", err)
	}
}

func TestRegistryOrgDeleteDefaultRefused(t *testing.T) {
	mux := http.NewServeMux()
	// GET reports the org as default → the CLI refuses client-side, no DELETE.
	mux.HandleFunc("GET /api/v1/registry/organizations/acme", func(w http.ResponseWriter, _ *http.Request) {
		writeEnvelope(w, http.StatusOK, orgDefaultJSON)
	})
	mux.HandleFunc("DELETE /api/v1/registry/organizations/acme", func(w http.ResponseWriter, _ *http.Request) {
		t.Error("DELETE must not be called for the default org")
		writeEnvelope(w, http.StatusOK, "")
	})
	srv := newServer(t, mux)
	mockEnv(t, srv.URL)

	_, _, err := runCLI("registry", "org", "delete", "acme", "-y")
	if err == nil || !strings.Contains(err.Error(), "default organization and cannot be deleted") {
		t.Fatalf("expected default-org refusal, got: %v", err)
	}
}

func TestRegistryOrgDeleteDefaultServerCode(t *testing.T) {
	mux := http.NewServeMux()
	// GET reports non-default, but the server rejects the DELETE (race) — the
	// CLI must map the server code to the same friendly message.
	mux.HandleFunc("GET /api/v1/registry/organizations/acme", func(w http.ResponseWriter, _ *http.Request) {
		writeEnvelope(w, http.StatusOK, orgAcmeJSON)
	})
	mux.HandleFunc("DELETE /api/v1/registry/organizations/acme", func(w http.ResponseWriter, _ *http.Request) {
		writeError(w, http.StatusConflict, "ORG_CANNOT_DELETE_DEFAULT", "cannot delete default")
	})
	srv := newServer(t, mux)
	mockEnv(t, srv.URL)

	_, _, err := runCLI("registry", "org", "delete", "acme", "-y")
	if err == nil || !strings.Contains(err.Error(), "default organization and cannot be deleted") {
		t.Fatalf("expected default-org server-code mapping, got: %v", err)
	}
}

func TestRegistryImageList(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/registry/organizations", func(w http.ResponseWriter, _ *http.Request) {
		writeEnvelope(w, http.StatusOK, "["+orgAcmeJSON+"]")
	})
	mux.HandleFunc("GET /api/v1/registry/organizations/acme/repositories/web/manifests", func(w http.ResponseWriter, _ *http.Request) {
		writeEnvelope(w, http.StatusOK, "["+manifestImageJSON+"]")
	})
	srv := newServer(t, mux)
	mockEnv(t, srv.URL)

	out, _, err := runCLI("registry", "image", "list", "web")
	if err != nil {
		t.Fatalf("registry image list: %v", err)
	}
	for _, want := range []string{"TAG", "DIGEST", "ARCH/OS", "PUSHED", "v1", "abcdef123456", "linux/amd64"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q: %s", want, out)
		}
	}
	if strings.Contains(out, "sha256:abcdef") {
		t.Errorf("table should show short digest, got full: %s", out)
	}
}

func TestRegistryOrgCreate(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/registry/organizations", func(w http.ResponseWriter, r *http.Request) {
		var req map[string]any
		decodeBody(t, r, &req)
		if req["slug"] != "acme" {
			t.Errorf("slug = %v, want acme", req["slug"])
		}
		if req["display_name"] != "Acme Inc" {
			t.Errorf("display_name = %v, want Acme Inc", req["display_name"])
		}
		writeEnvelope(w, http.StatusOK, orgAcmeJSON)
	})
	srv := newServer(t, mux)
	mockEnv(t, srv.URL)

	out, _, err := runCLI("registry", "org", "create", "acme", "--display-name", "Acme Inc")
	if err != nil {
		t.Fatalf("registry org create: %v", err)
	}
	if !strings.Contains(out, "Created") || !strings.Contains(out, "acme") {
		t.Errorf("unexpected create output: %q", out)
	}
}

func TestRegistryOrgUpdate(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/registry/organizations/acme", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("ETag", `W/"abc123"`)
		writeEnvelope(w, http.StatusOK, orgAcmeJSON)
	})
	mux.HandleFunc("PATCH /api/v1/registry/organizations/acme", func(w http.ResponseWriter, r *http.Request) {
		var req map[string]any
		decodeBody(t, r, &req)
		if req["display_name"] != "New Name" {
			t.Errorf("display_name = %v, want New Name", req["display_name"])
		}
		writeEnvelope(w, http.StatusOK, orgAcmeJSON)
	})
	srv := newServer(t, mux)
	mockEnv(t, srv.URL)

	out, _, err := runCLI("registry", "org", "update", "acme", "--display-name", "New Name")
	if err != nil {
		t.Fatalf("registry org update: %v", err)
	}
	if !strings.Contains(out, "Updated") {
		t.Errorf("unexpected update output: %q", out)
	}
}

// The current implementation always issues the PATCH even when no flags are
// set; verify the request is made with an empty body rather than a local error.
func TestRegistryOrgUpdateNoChange(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/registry/organizations/acme", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("ETag", `W/"abc123"`)
		writeEnvelope(w, http.StatusOK, orgAcmeJSON)
	})
	patched := false
	mux.HandleFunc("PATCH /api/v1/registry/organizations/acme", func(w http.ResponseWriter, r *http.Request) {
		patched = true
		var req map[string]any
		decodeBody(t, r, &req)
		if _, ok := req["display_name"]; ok {
			t.Errorf("display_name should be omitted when flag unset: %v", req)
		}
		if _, ok := req["registry_auto_create_repos"]; ok {
			t.Errorf("registry_auto_create_repos should be omitted when flag unset: %v", req)
		}
		writeEnvelope(w, http.StatusOK, orgAcmeJSON)
	})
	srv := newServer(t, mux)
	mockEnv(t, srv.URL)

	if _, _, err := runCLI("registry", "org", "update", "acme"); err != nil {
		t.Fatalf("registry org update no-change: %v", err)
	}
	if !patched {
		t.Errorf("expected PATCH to be issued even with no flags set")
	}
}

func TestRegistryRepoUpdate(t *testing.T) {
	t.Run("with flags", func(t *testing.T) {
		mux := http.NewServeMux()
		mux.HandleFunc("GET /api/v1/registry/organizations", func(w http.ResponseWriter, _ *http.Request) {
			writeEnvelope(w, http.StatusOK, "["+orgAcmeJSON+"]")
		})
		mux.HandleFunc("PATCH /api/v1/registry/organizations/acme/repositories/web", func(w http.ResponseWriter, r *http.Request) {
			var req map[string]any
			decodeBody(t, r, &req)
			if req["tag_mutability"] != "IMMUTABLE" {
				t.Errorf("tag_mutability = %v, want IMMUTABLE", req["tag_mutability"])
			}
			writeEnvelope(w, http.StatusOK, repoWebJSON)
		})
		srv := newServer(t, mux)
		mockEnv(t, srv.URL)

		out, _, err := runCLI("registry", "repo", "update", "web",
			"--tag-mutability", "IMMUTABLE")
		if err != nil {
			t.Fatalf("registry repo update: %v", err)
		}
		if !strings.Contains(out, "Updated repo acme/web") {
			t.Errorf("unexpected update output: %q", out)
		}
	})

	t.Run("omits unset flags", func(t *testing.T) {
		mux := http.NewServeMux()
		mux.HandleFunc("GET /api/v1/registry/organizations", func(w http.ResponseWriter, _ *http.Request) {
			writeEnvelope(w, http.StatusOK, "["+orgAcmeJSON+"]")
		})
		mux.HandleFunc("PATCH /api/v1/registry/organizations/acme/repositories/web", func(w http.ResponseWriter, r *http.Request) {
			var req map[string]json.RawMessage
			b, _ := io.ReadAll(r.Body)
			if err := json.Unmarshal(b, &req); err != nil {
				t.Fatalf("decode body %q: %v", b, err)
			}
			if _, ok := req["tag_mutability"]; ok {
				t.Errorf("tag_mutability should be omitted when flag unset: %s", b)
			}
			writeEnvelope(w, http.StatusOK, repoWebJSON)
		})
		srv := newServer(t, mux)
		mockEnv(t, srv.URL)

		if _, _, err := runCLI("registry", "repo", "update", "web"); err != nil {
			t.Fatalf("registry repo update defaults: %v", err)
		}
	})
}

func TestRegistryRepoListSuspended(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/registry/organizations", func(w http.ResponseWriter, _ *http.Request) {
		writeEnvelope(w, http.StatusOK, "["+orgAcmeJSON+"]")
	})
	mux.HandleFunc("GET /api/v1/registry/organizations/acme/repositories", func(w http.ResponseWriter, _ *http.Request) {
		writeError(w, http.StatusForbidden, "REGISTRY_SUSPENDED", "suspended")
	})
	srv := newServer(t, mux)
	mockEnv(t, srv.URL)

	_, _, err := runCLI("registry", "repo", "list")
	if err == nil || !strings.Contains(err.Error(), "billing") {
		t.Fatalf("expected suspended/billing error, got: %v", err)
	}
}

func TestRegistryRepoGetNotFound(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/registry/organizations", func(w http.ResponseWriter, _ *http.Request) {
		writeEnvelope(w, http.StatusOK, "["+orgAcmeJSON+"]")
	})
	mux.HandleFunc("GET /api/v1/registry/organizations/acme/repositories/missing", func(w http.ResponseWriter, _ *http.Request) {
		writeError(w, http.StatusNotFound, "REGISTRY_REPOSITORY_NOT_FOUND", "not found")
	})
	srv := newServer(t, mux)
	mockEnv(t, srv.URL)

	_, _, err := runCLI("registry", "repo", "get", "missing")
	if err == nil || !strings.Contains(err.Error(), `no repo named "missing"`) {
		t.Fatalf("expected friendly not-found error, got: %v", err)
	}
}

func TestRegistryOrgGetAmbiguous(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/registry/organizations/acme", func(w http.ResponseWriter, _ *http.Request) {
		writeError(w, http.StatusConflict, "AMBIGUOUS_NAME", "ambiguous")
	})
	srv := newServer(t, mux)
	mockEnv(t, srv.URL)

	_, _, err := runCLI("registry", "org", "get", "acme")
	if err == nil || !strings.Contains(err.Error(), "disambiguate") {
		t.Fatalf("expected ambiguous-name error, got: %v", err)
	}
}

func TestRegistryImageListWithTagFilter(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/registry/organizations", func(w http.ResponseWriter, _ *http.Request) {
		writeEnvelope(w, http.StatusOK, "["+orgAcmeJSON+"]")
	})
	mux.HandleFunc("GET /api/v1/registry/organizations/acme/repositories/myrepo/manifests", func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("tag"); got != "v1.0" {
			t.Errorf("tag query = %q, want v1.0", got)
		}
		writeEnvelope(w, http.StatusOK, "["+manifestImageJSON+"]")
	})
	srv := newServer(t, mux)
	mockEnv(t, srv.URL)

	if _, _, err := runCLI("registry", "image", "list", "myrepo", "--tag", "v1.0"); err != nil {
		t.Fatalf("registry image list --tag: %v", err)
	}
}

func TestRegistryImageGetIndexManifest(t *testing.T) {
	const indexManifestJSON = `{"id":101,` +
		`"digest":"sha256:fedcba0987654321fedcba0987654321fedcba0987654321fedcba0987654321",` +
		`"tag":"latest","media_type":"application/vnd.oci.image.index.v1+json","kind":"index",` +
		`"size_bytes":1024,"pushed_at":"2024-03-02T00:00:00Z",` +
		`"platforms":[` +
		`{"os":"linux","architecture":"amd64","platform":"linux/amd64",` +
		`"digest":"sha256:1111111111111111111111111111111111111111111111111111111111111111","size":512,"image_size_bytes":5242880},` +
		`{"os":"linux","architecture":"arm64","platform":"linux/arm64",` +
		`"digest":"sha256:2222222222222222222222222222222222222222222222222222222222222222","size":512,"image_size_bytes":5242880}` +
		`]}`

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/registry/organizations", func(w http.ResponseWriter, _ *http.Request) {
		writeEnvelope(w, http.StatusOK, "["+orgAcmeJSON+"]")
	})
	mux.HandleFunc("GET /api/v1/registry/organizations/acme/repositories/web/manifests/latest", func(w http.ResponseWriter, _ *http.Request) {
		writeEnvelope(w, http.StatusOK, indexManifestJSON)
	})
	srv := newServer(t, mux)
	mockEnv(t, srv.URL)

	out, _, err := runCLI("registry", "image", "get", "web", "latest")
	if err != nil {
		t.Fatalf("registry image get index: %v", err)
	}
	for _, want := range []string{"Platforms:", "linux/amd64", "linux/arm64", "index"} {
		if !strings.Contains(out, want) {
			t.Errorf("index detail missing %q: %s", want, out)
		}
	}
}

func TestRegistryNoOrgsError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/registry/organizations", func(w http.ResponseWriter, _ *http.Request) {
		writeEnvelope(w, http.StatusOK, "[]")
	})
	srv := newServer(t, mux)
	mockEnv(t, srv.URL)

	_, _, err := runCLI("registry", "repo", "list")
	if err == nil || !strings.Contains(err.Error(), "no registry orgs") {
		t.Fatalf("expected no-orgs error, got: %v", err)
	}
}

func TestRegistryImageGet(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/registry/organizations", func(w http.ResponseWriter, _ *http.Request) {
		writeEnvelope(w, http.StatusOK, "["+orgAcmeJSON+"]")
	})
	mux.HandleFunc("GET /api/v1/registry/organizations/acme/repositories/web/manifests/v1", func(w http.ResponseWriter, _ *http.Request) {
		writeEnvelope(w, http.StatusOK, manifestImageJSON)
	})
	srv := newServer(t, mux)
	mockEnv(t, srv.URL)

	out, _, err := runCLI("registry", "image", "get", "web", "v1")
	if err != nil {
		t.Fatalf("registry image get: %v", err)
	}
	for _, want := range []string{"Digest:", "sha256:abcdef", "linux/amd64", "image"} {
		if !strings.Contains(out, want) {
			t.Errorf("detail missing %q: %s", want, out)
		}
	}
}

func TestRegistryPlans(t *testing.T) {
	mux := http.NewServeMux()
	gotSort := ""
	mux.HandleFunc("GET /api/v1/registry/plans", func(w http.ResponseWriter, r *http.Request) {
		gotSort = r.URL.Query().Get("sort")
		writeEnvelope(w, http.StatusOK,
			`{"plans":[{"id":1,"name":"Storage","unit":"GB-month","price_per_unit":"1500.00",`+
				`"currency":"IDR","charge_model":"metered","billing_period":"monthly"}]}`)
	})
	srv := newServer(t, mux)
	mockEnv(t, srv.URL)

	out, _, err := runCLI("registry", "plans", "--sort", "name")
	if err != nil {
		t.Fatalf("registry plans: %v", err)
	}
	if gotSort != "name" {
		t.Errorf("expected sort query param, got %q", gotSort)
	}
	for _, want := range []string{"NAME", "UNIT", "PRICE/UNIT", "Storage", "GB-month", "1500.00", "IDR", "metered", "monthly"} {
		if !strings.Contains(out, want) {
			t.Errorf("plans missing %q: %s", want, out)
		}
	}
}

const manifestDigest = "sha256:abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890"

func TestRegistryImageDeleteByDigest(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/registry/organizations", func(w http.ResponseWriter, _ *http.Request) {
		writeEnvelope(w, http.StatusOK, "["+orgAcmeJSON+"]")
	})
	deleted := false
	mux.HandleFunc("DELETE /api/v1/registry/organizations/acme/repositories/web/manifests/"+manifestDigest,
		func(w http.ResponseWriter, _ *http.Request) {
			deleted = true
			writeEnvelope(w, http.StatusOK, "")
		})
	srv := newServer(t, mux)
	mockEnv(t, srv.URL)

	out, _, err := runCLI("registry", "image", "delete", "web", manifestDigest, "-y")
	if err != nil {
		t.Fatalf("registry image delete: %v", err)
	}
	if !deleted {
		t.Error("expected DELETE to be called")
	}
	if !strings.Contains(out, "deleted from acme/web") {
		t.Errorf("unexpected delete output: %q", out)
	}
}

func TestRegistryImageDeleteByTagResolvesDigest(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/registry/organizations", func(w http.ResponseWriter, _ *http.Request) {
		writeEnvelope(w, http.StatusOK, "["+orgAcmeJSON+"]")
	})
	// A tag arg is resolved to its digest via GetManifest first.
	mux.HandleFunc("GET /api/v1/registry/organizations/acme/repositories/web/manifests/v1",
		func(w http.ResponseWriter, _ *http.Request) {
			writeEnvelope(w, http.StatusOK, manifestImageJSON)
		})
	deletedDigest := ""
	mux.HandleFunc("DELETE /api/v1/registry/organizations/acme/repositories/web/manifests/"+manifestDigest,
		func(w http.ResponseWriter, _ *http.Request) {
			deletedDigest = manifestDigest
			writeEnvelope(w, http.StatusOK, "")
		})
	srv := newServer(t, mux)
	mockEnv(t, srv.URL)

	out, _, err := runCLI("registry", "image", "delete", "web", "v1", "-y")
	if err != nil {
		t.Fatalf("registry image delete by tag: %v", err)
	}
	if deletedDigest != manifestDigest {
		t.Errorf("expected delete by resolved digest, got %q", deletedDigest)
	}
	if !strings.Contains(out, manifestDigest) {
		t.Errorf("output should show resolved digest: %q", out)
	}
}

func TestRegistryImageDeleteNotFound(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/registry/organizations", func(w http.ResponseWriter, _ *http.Request) {
		writeEnvelope(w, http.StatusOK, "["+orgAcmeJSON+"]")
	})
	mux.HandleFunc("DELETE /api/v1/registry/organizations/acme/repositories/web/manifests/"+manifestDigest,
		func(w http.ResponseWriter, _ *http.Request) {
			writeError(w, http.StatusNotFound, "REGISTRY_MANIFEST_NOT_FOUND", "not found")
		})
	srv := newServer(t, mux)
	mockEnv(t, srv.URL)

	_, _, err := runCLI("registry", "image", "delete", "web", manifestDigest, "-y")
	if err == nil || !strings.Contains(err.Error(), "no manifest") {
		t.Fatalf("expected manifest-not-found error, got: %v", err)
	}
}
