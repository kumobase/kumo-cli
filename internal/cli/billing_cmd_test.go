package cli

import (
	"net/http"
	"strings"
	"testing"
)

const billingSummaryJSON = `{"currency":"IDR","previous_period_total":"120000.00",` +
	`"current_period":{"start":"2026-07-01T00:00:00Z","end":"2026-07-31T00:00:00Z",` +
	`"total_charged":"45000.00","accruing_total":"5000.00",` +
	`"by_product":{"vps":"0","app":"45000.00","storage":"0","container_registry":"0","database":"0","jobs":"0","vm_runners":"0"},` +
	`"accruing":{"vps":"0","app":"5000.00","storage":"0","container_registry":"0","database":"0","jobs":"0","vm_runners":"0"}}}`

const billingChargeJSON = `{"id":1,"subscription_id":9,"product_type":"app","plan_name":"app-small",` +
	`"amount":"1000000.00","currency":"IDR","period_start":"2026-06-01T00:00:00Z",` +
	`"period_end":"2026-07-01T00:00:00Z","charge_type":"subscription","status":"paid","reference_id":"ref-1",` +
	`"created_at":"2026-07-01T00:00:00Z"}`

func TestBillingSummary(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/billing/v2/summary", func(w http.ResponseWriter, _ *http.Request) {
		writeEnvelope(w, http.StatusOK, billingSummaryJSON)
	})
	srv := newServer(t, mux)
	mockEnv(t, srv.URL)

	out, _, err := runCLI("billing", "summary")
	if err != nil {
		t.Fatalf("billing summary: %v", err)
	}
	for _, want := range []string{"Currency:", "IDR", "Charged so far:", "45000.00", "Accruing", "5000.00",
		// per-product section
		"PRODUCT", "CHARGED", "ACCRUING", "app"} {
		if !strings.Contains(out, want) {
			t.Errorf("summary missing %q: %s", want, out)
		}
	}
}

func TestBillingCharges(t *testing.T) {
	mux := http.NewServeMux()
	gotFrom := ""
	mux.HandleFunc("GET /api/v1/billing/v2/charges", func(w http.ResponseWriter, r *http.Request) {
		gotFrom = r.URL.Query().Get("from")
		writeEnvelope(w, http.StatusOK, "["+billingChargeJSON+"]")
	})
	srv := newServer(t, mux)
	mockEnv(t, srv.URL)

	out, _, err := runCLI("billing", "charges", "--from", "2026-06-01")
	if err != nil {
		t.Fatalf("billing charges: %v", err)
	}
	if gotFrom != "2026-06-01" {
		t.Errorf("expected from query param, got %q", gotFrom)
	}
	for _, want := range []string{"PRODUCT", "app", "app-small", "1000000.00", "paid"} {
		if !strings.Contains(out, want) {
			t.Errorf("charges missing %q: %s", want, out)
		}
	}
}

func TestBillingChargesGrouped(t *testing.T) {
	mux := http.NewServeMux()
	gotGroupBy := ""
	mux.HandleFunc("GET /api/v1/billing/v2/charges", func(w http.ResponseWriter, r *http.Request) {
		gotGroupBy = r.URL.Query().Get("group_by")
		writeEnvelope(w, http.StatusOK,
			`[{"group_key":"2026-07-01","total_amount":"5000.00","currency":"IDR","charge_count":3,"status_breakdown":{"paid":3}}]`)
	})
	srv := newServer(t, mux)
	mockEnv(t, srv.URL)

	out, _, err := runCLI("billing", "charges", "--group", "--group-by", "date")
	if err != nil {
		t.Fatalf("billing charges grouped: %v", err)
	}
	if gotGroupBy != "date" {
		t.Errorf("expected group_by=date, got %q", gotGroupBy)
	}
	for _, want := range []string{"GROUP", "2026-07-01", "5000.00", "CHARGES"} {
		if !strings.Contains(out, want) {
			t.Errorf("grouped missing %q: %s", want, out)
		}
	}
}

func TestBillingBreakdownInvalidDateRange(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/billing/v2/breakdown", func(w http.ResponseWriter, _ *http.Request) {
		writeError(w, http.StatusBadRequest, "INVALID_DATE_RANGE", "bad range")
	})
	srv := newServer(t, mux)
	mockEnv(t, srv.URL)

	_, _, err := runCLI("billing", "breakdown", "--from", "2026-07-31", "--to", "2026-07-01")
	if err == nil || !strings.Contains(err.Error(), "invalid date range") {
		t.Fatalf("expected date-range error mapping, got: %v", err)
	}
}
