package cli

import (
	"net/http"
	"strings"
	"testing"
)

func TestBillingBalance(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/profile/balance", func(w http.ResponseWriter, _ *http.Request) {
		writeEnvelope(w, http.StatusOK, `{"balance":"152340.50"}`)
	})
	srv := newServer(t, mux)
	mockEnv(t, srv.URL)

	out, _, err := runCLI("billing", "balance")
	if err != nil {
		t.Fatalf("billing balance: %v", err)
	}
	if !strings.Contains(out, "152340.50") {
		t.Errorf("balance missing from output: %s", out)
	}

	outJSON, _, err := runCLI("billing", "balance", "-o", "json")
	if err != nil {
		t.Fatalf("billing balance json: %v", err)
	}
	var got map[string]any
	decodeData(t, outJSON, &got)
	if got["balance"] != "152340.50" {
		t.Errorf("unexpected balance json: %s", outJSON)
	}
}

func TestBillingChargesGroupByRequiresGroup(t *testing.T) {
	mockEnv(t, "http://127.0.0.1:0")
	_, _, err := runCLI("billing", "charges", "--group-by", "subscription")
	if err == nil || !strings.Contains(err.Error(), "--group-by requires --group") {
		t.Fatalf("expected group-by guard error, got: %v", err)
	}
	if code := exitCodeFor(err); code != 2 {
		t.Errorf("group-by guard exit = %d, want 2 (usage)", code)
	}
}

func TestJobsListInvalidKindIsUsageError(t *testing.T) {
	mockEnv(t, "http://127.0.0.1:0")
	_, _, err := runCLI("jobs", "list", "--kind", "bogus")
	if err == nil || !strings.Contains(err.Error(), "invalid --kind") {
		t.Fatalf("expected invalid --kind error, got: %v", err)
	}
	if code := exitCodeFor(err); code != 2 {
		t.Errorf("invalid kind exit = %d, want 2 (usage)", code)
	}
}

func TestJobsListInvalidSortOrder(t *testing.T) {
	mockEnv(t, "http://127.0.0.1:0")
	_, _, err := runCLI("jobs", "list", "--sort", "name", "--sort-order", "sideways")
	if err == nil || !strings.Contains(err.Error(), "invalid --sort-order") {
		t.Fatalf("expected invalid --sort-order error, got: %v", err)
	}
}

// gitlabConnJSON is a GitLab source connection: the GitHub-centric top-level
// fields are empty and the identity lives under the gitlab sub-object.
const gitlabConnJSON = `{"id":5,"provider":"gitlab","account_login":"",` +
	`"status":"active","app_kind":"build",` +
	`"gitlab":{"id":5,"provider":"gitlab","instance_id":1,"base_url":"https://gitlab.com",` +
	`"kind":"group","namespace_id":42,"namespace_path":"acme/platform","display_name":"Platform",` +
	`"status":"active","created_at":"2024-01-01T00:00:00Z","updated_at":"2024-01-02T00:00:00Z"},` +
	`"created_at":"2024-01-01T00:00:00Z","updated_at":"2024-01-02T00:00:00Z"}`

func TestSourceListGitLabRow(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/source-connections", func(w http.ResponseWriter, _ *http.Request) {
		writeEnvelope(w, http.StatusOK, "["+gitlabConnJSON+"]")
	})
	srv := newServer(t, mux)
	mockEnv(t, srv.URL)

	out, _, err := runCLI("source", "list")
	if err != nil {
		t.Fatalf("source list: %v", err)
	}
	// GitLab identity (namespace path + kind) must render, not a blank account.
	for _, want := range []string{"gitlab", "acme/platform", "group"} {
		if !strings.Contains(out, want) {
			t.Errorf("source list missing GitLab field %q: %s", want, out)
		}
	}
}
