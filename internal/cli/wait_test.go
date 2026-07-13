package cli

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/kumobase/kumo-go/client"
	"github.com/kumobase/kumo-go/types"
)

func ptr(s string) *string { return &s }

func TestLatestOperation(t *testing.T) {
	base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	ops := []types.AppOperation{
		{OperationID: "old-update", ActionType: types.AppOperationActionUpdate, QueuedAt: base.Add(-time.Hour)},
		{OperationID: "create", ActionType: types.AppOperationActionCreate, QueuedAt: base.Add(time.Minute)},
		{OperationID: "new-update", ActionType: types.AppOperationActionUpdate, QueuedAt: base.Add(2 * time.Minute)},
		{OperationID: "older-update", ActionType: types.AppOperationActionUpdate, QueuedAt: base.Add(time.Minute)},
	}

	got := latestOperation(ops, types.AppOperationActionUpdate, base)
	if got == nil || got.OperationID != "new-update" {
		t.Fatalf("latestOperation = %+v, want new-update", got)
	}

	// An action with no rows at/after since returns nil.
	if op := latestOperation(ops, types.AppOperationActionDelete, base); op != nil {
		t.Errorf("expected nil for absent action, got %+v", op)
	}

	// since filters out operations queued before it.
	if op := latestOperation(ops, types.AppOperationActionUpdate, base.Add(3*time.Minute)); op != nil {
		t.Errorf("expected nil when all ops precede since, got %+v", op)
	}
}

func TestOperationError(t *testing.T) {
	cases := []struct {
		name string
		op   types.AppOperation
		want string
	}{
		{"msg and code", types.AppOperation{ErrorMsg: ptr("boom"), ErrorCode: ptr("E_BOOM")}, "boom (E_BOOM)"},
		{"msg only", types.AppOperation{ErrorMsg: ptr("boom")}, "boom"},
		{"code only", types.AppOperation{ErrorCode: ptr("E_BOOM")}, "E_BOOM"},
		{"neither", types.AppOperation{}, "unknown error"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			op := tc.op
			if got := operationError(&op); got != tc.want {
				t.Errorf("operationError = %q, want %q", got, tc.want)
			}
		})
	}
}

// testClient builds an SDK client pointed at a mock server.
func testClient(t *testing.T, mux *http.ServeMux) *client.Client {
	t.Helper()
	srv := newServer(t, mux)
	c, err := client.New(srv.URL, client.WithAPIKey("kumo_sk_test"))
	if err != nil {
		t.Fatalf("client.New: %v", err)
	}
	return c
}

func TestWaitForOperationSucceeded(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/apps/{id}/operations", func(w http.ResponseWriter, _ *http.Request) {
		writeEnvelope(w, http.StatusOK,
			`[{"operation_id":"op-1","app_id":42,"action_type":"start","status":"succeeded","queued_at":"2024-06-01T00:00:00Z"}]`)
	})
	c := testClient(t, mux)

	op, err := waitForOperation(context.Background(), c, 42, types.AppOperationActionStart, time.Time{}, time.Minute)
	if err != nil {
		t.Fatalf("waitForOperation: %v", err)
	}
	if op.OperationID != "op-1" {
		t.Errorf("unexpected op: %+v", op)
	}
}

func TestWaitForOperationFailed(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/apps/{id}/operations", func(w http.ResponseWriter, _ *http.Request) {
		writeEnvelope(w, http.StatusOK,
			`[{"operation_id":"op-1","app_id":42,"action_type":"start","status":"failed","error_code":"DEPLOY_FAILED","error_message":"image pull error","queued_at":"2024-06-01T00:00:00Z"}]`)
	})
	c := testClient(t, mux)

	_, err := waitForOperation(context.Background(), c, 42, types.AppOperationActionStart, time.Time{}, time.Minute)
	if err == nil {
		t.Fatal("expected failure error")
	}
	if !strings.Contains(err.Error(), "image pull error") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestWaitForOperationCancelled(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/apps/{id}/operations", func(w http.ResponseWriter, _ *http.Request) {
		writeEnvelope(w, http.StatusOK,
			`[{"operation_id":"op-1","app_id":42,"action_type":"stop","status":"cancelled","queued_at":"2024-06-01T00:00:00Z"}]`)
	})
	c := testClient(t, mux)

	_, err := waitForOperation(context.Background(), c, 42, types.AppOperationActionStop, time.Time{}, time.Minute)
	if err == nil || !strings.Contains(err.Error(), "cancelled") {
		t.Fatalf("expected cancelled error, got %v", err)
	}
}

func TestWaitForOperationTimeout(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/apps/{id}/operations", func(w http.ResponseWriter, _ *http.Request) {
		writeEnvelope(w, http.StatusOK,
			`[{"operation_id":"op-1","app_id":42,"action_type":"update","status":"in_progress","queued_at":"2024-06-01T00:00:00Z"}]`)
	})
	c := testClient(t, mux)

	// A negative timeout makes the deadline already past, so the first
	// non-terminal poll returns the timeout error without sleeping.
	_, err := waitForOperation(context.Background(), c, 42, types.AppOperationActionUpdate, time.Time{}, -time.Second)
	if err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("expected timeout error, got %v", err)
	}
}
