package cli

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

// --- filters actually reach the wire as query params ---

func TestJobsListKindQueryParam(t *testing.T) {
	var gotKind string
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/jobs", func(w http.ResponseWriter, r *http.Request) {
		gotKind = r.URL.Query().Get("kind")
		writeEnvelope(w, http.StatusOK, "[]")
	})
	srv := newServer(t, mux)
	mockEnv(t, srv.URL)

	if _, _, err := runCLI("jobs", "list", "--kind", "standalone"); err != nil {
		t.Fatalf("jobs list --kind: %v", err)
	}
	if gotKind != "standalone" {
		t.Errorf("kind query = %q, want standalone", gotKind)
	}
}

func TestVPSListExpiresBeforeQueryParam(t *testing.T) {
	var got string
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/vps/servers", func(w http.ResponseWriter, r *http.Request) {
		got = r.URL.Query().Get("expires_before")
		writeEnvelope(w, http.StatusOK, "[]")
	})
	srv := newServer(t, mux)
	mockEnv(t, srv.URL)

	if _, _, err := runCLI("vps", "list", "--expires-before", "2024-12-31T00:00:00Z"); err != nil {
		t.Fatalf("vps list --expires-before: %v", err)
	}
	if got != "2024-12-31T00:00:00Z" {
		t.Errorf("expires_before query = %q", got)
	}
}

func TestSourceReposFilterQueryParam(t *testing.T) {
	var got string
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/source-connections/5/repos", func(w http.ResponseWriter, r *http.Request) {
		got = r.URL.Query().Get("q")
		writeEnvelope(w, http.StatusOK, "[]")
	})
	srv := newServer(t, mux)
	mockEnv(t, srv.URL)

	if _, _, err := runCLI("source", "repos", "5", "--filter", "api"); err != nil {
		t.Fatalf("source repos --filter: %v", err)
	}
	if got != "api" {
		t.Errorf("q query = %q, want api", got)
	}
}

// --- health-check flags reach the create request ---

func TestAppCreateHealthCheckFlags(t *testing.T) {
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
		"--wait=false", "--health-check-type", "http", "--health-check-path", "/healthz", "--health-check-port", "80")
	if err != nil {
		t.Fatalf("apps create health check: %v", err)
	}
	for _, want := range []string{"healthcheck", "/healthz"} {
		if !strings.Contains(body, want) {
			t.Errorf("create body missing %q: %s", want, body)
		}
	}
}

// --- vps rent --wait polls until running ---

func TestVPSRentWaitPollsUntilRunning(t *testing.T) {
	var getCalls int
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/vps/servers", func(w http.ResponseWriter, _ *http.Request) {
		// server comes back provisioning
		prov := strings.Replace(vpsServerJSON, `"status":"running"`, `"status":"provisioning"`, 1)
		writeEnvelope(w, http.StatusCreated, prov)
	})
	mux.HandleFunc("GET /api/v1/vps/servers/7", func(w http.ResponseWriter, _ *http.Request) {
		getCalls++
		writeEnvelope(w, http.StatusOK, vpsServerJSON) // status running
	})
	srv := newServer(t, mux)
	mockEnv(t, srv.URL)

	out, _, err := runCLI("vps", "rent", "--name", "web1", "--provider", "zeabur",
		"--region", "sg-singapore", "--plan", "sg-1c-1g", "--wait")
	if err != nil {
		t.Fatalf("vps rent --wait: %v", err)
	}
	if getCalls < 1 {
		t.Error("rent --wait should poll GetServer at least once")
	}
	if !strings.Contains(out, "running") {
		t.Errorf("expected running status after wait: %q", out)
	}
}

// --- billing breakdown grouped rendering (summary per-product is covered by
// the extended TestBillingSummary in billing_cmd_test.go) ---

const billingBreakdownGroupedJSON = `{"currency":"IDR","granularity":"daily","group_by":"product_type",` +
	`"from":"2024-01-01","to":"2024-01-02","totals":{"amount":"100","groups":[{"key":"app","amount":"60"},` +
	`{"key":"vps","amount":"40"}]},"buckets":[{"period_start":"2024-01-01T00:00:00Z",` +
	`"period_end":"2024-01-02T00:00:00Z","amount":"100","groups":[{"key":"app","amount":"60"},{"key":"vps","amount":"40"}]}]}`

func TestBillingBreakdownGroupedRows(t *testing.T) {
	var gotGroupBy string
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/billing/v2/breakdown", func(w http.ResponseWriter, r *http.Request) {
		gotGroupBy = r.URL.Query().Get("group_by")
		writeEnvelope(w, http.StatusOK, billingBreakdownGroupedJSON)
	})
	srv := newServer(t, mux)
	mockEnv(t, srv.URL)

	out, _, err := runCLI("billing", "breakdown", "--group-by", "product_type")
	if err != nil {
		t.Fatalf("billing breakdown: %v", err)
	}
	if gotGroupBy != "product_type" {
		t.Errorf("group_by query = %q", gotGroupBy)
	}
	for _, want := range []string{"GROUP", "app", "60", "vps", "40"} {
		if !strings.Contains(out, want) {
			t.Errorf("breakdown missing grouped row %q: %s", want, out)
		}
	}
}

// --- idempotency key on a non-volume write (app delete) ---

func TestIdempotencyKeyOnAppDelete(t *testing.T) {
	var gotKey string
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/apps/web-app", handleAppDetail())
	mux.HandleFunc("DELETE /api/v1/apps/42", func(w http.ResponseWriter, r *http.Request) {
		gotKey = r.Header.Get("Idempotency-Key")
		writeEnvelope(w, http.StatusOK, "")
	})
	srv := newServer(t, mux)
	mockEnv(t, srv.URL)

	_, _, err := runCLI("apps", "delete", "web-app", "-y", "--wait=false", "--idempotency-key", "del-key-1")
	if err != nil {
		t.Fatalf("apps delete: %v", err)
	}
	if gotKey != "del-key-1" {
		t.Errorf("Idempotency-Key = %q, want del-key-1", gotKey)
	}
}
