package cli

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
)

const vpsServerJSON = `{"id":7,"display_name":"web1","display_provider":"zeabur",` +
	`"region_id":"sg-singapore","os":"ubuntu-22.04","status":"running",` +
	`"ip_address":"203.0.113.10","ssh_port":22,"auto_renew":true,` +
	`"expires_at":"2024-12-31T00:00:00Z","created_at":"2024-01-01T00:00:00Z",` +
	`"ssh_setup_completed":true,"action_status":"",` +
	`"plan":{"provider_name":"zeabur","plan_id":1,"external_plan_id":"sg-1c-1g",` +
	`"name":"Small","cpu":1,"memory":1024,"disk":20,"selling_price":"4.99"}}`

const vpsStoppedJSON = `{"id":8,"display_name":"db1","display_provider":"zeabur",` +
	`"region_id":"sg-singapore","status":"stopped","ip_address":"203.0.113.11",` +
	`"ssh_port":22,"auto_renew":true,"expires_at":"2024-12-31T00:00:00Z",` +
	`"created_at":"2024-01-01T00:00:00Z","ssh_setup_completed":true,"action_status":""}`

func TestVPSList(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/vps/servers", func(w http.ResponseWriter, _ *http.Request) {
		writeEnvelope(w, http.StatusOK, "["+vpsServerJSON+"]")
	})
	srv := newServer(t, mux)
	mockEnv(t, srv.URL)

	out, _, err := runCLI("vps", "list")
	if err != nil {
		t.Fatalf("vps list: %v", err)
	}
	for _, want := range []string{"NAME", "STATUS", "PROVIDER", "web1", "running", "zeabur", "203.0.113.10"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q: %s", want, out)
		}
	}
}

func TestVPSListJSON(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/vps/servers", func(w http.ResponseWriter, _ *http.Request) {
		writeEnvelope(w, http.StatusOK, "["+vpsServerJSON+"]")
	})
	srv := newServer(t, mux)
	mockEnv(t, srv.URL)

	out, _, err := runCLI("vps", "list", "-o", "json")
	if err != nil {
		t.Fatalf("vps list json: %v", err)
	}
	var got []map[string]any
	if err := json.NewDecoder(strings.NewReader(out)).Decode(&got); err != nil && err != io.EOF {
		t.Fatalf("decode: %v", err)
	}
	if len(got) != 1 || got[0]["display_name"] != "web1" {
		t.Errorf("unexpected json: %s", out)
	}
}

func TestVPSGet(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/vps/servers/web1", func(w http.ResponseWriter, _ *http.Request) {
		writeEnvelope(w, http.StatusOK, vpsServerJSON)
	})
	srv := newServer(t, mux)
	mockEnv(t, srv.URL)

	out, _, err := runCLI("vps", "get", "web1")
	if err != nil {
		t.Fatalf("vps get: %v", err)
	}
	for _, want := range []string{"ID:", "web1", "running", "zeabur", "sg-singapore", "Small"} {
		if !strings.Contains(out, want) {
			t.Errorf("detail missing %q: %s", want, out)
		}
	}
}

func TestVPSGetNotFound(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/vps/servers/missing", func(w http.ResponseWriter, _ *http.Request) {
		writeError(w, http.StatusNotFound, "INSTANCE_NOT_FOUND", "not found")
	})
	srv := newServer(t, mux)
	mockEnv(t, srv.URL)

	_, _, err := runCLI("vps", "get", "missing")
	if err == nil || !strings.Contains(err.Error(), `no vps named "missing"`) {
		t.Fatalf("expected friendly not-found error, got: %v", err)
	}
}

func TestVPSRegions(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/vps/regions", func(w http.ResponseWriter, _ *http.Request) {
		writeEnvelope(w, http.StatusOK, `[{"id":"sg-singapore","name":"Singapore"},{"id":"us-east","name":"US East"}]`)
	})
	srv := newServer(t, mux)
	mockEnv(t, srv.URL)

	out, _, err := runCLI("vps", "regions")
	if err != nil {
		t.Fatalf("vps regions: %v", err)
	}
	for _, want := range []string{"ID", "NAME", "sg-singapore", "Singapore", "us-east"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q: %s", want, out)
		}
	}
}

func TestVPSPlans(t *testing.T) {
	mux := http.NewServeMux()
	gotRegion := ""
	mux.HandleFunc("GET /api/v1/vps/plans", func(w http.ResponseWriter, r *http.Request) {
		gotRegion = r.URL.Query().Get("region")
		writeEnvelope(w, http.StatusOK,
			`[{"provider_name":"zeabur","plan_id":1,"external_plan_id":"sg-1c-1g","name":"Small","cpu":1,"memory":1024,"disk":20,"selling_price":"4.99"}]`)
	})
	srv := newServer(t, mux)
	mockEnv(t, srv.URL)

	out, _, err := runCLI("vps", "plans", "--region", "sg-singapore")
	if err != nil {
		t.Fatalf("vps plans: %v", err)
	}
	if gotRegion != "sg-singapore" {
		t.Errorf("expected region query param, got %q", gotRegion)
	}
	for _, want := range []string{"PROVIDER", "PLAN ID", "zeabur", "sg-1c-1g", "4.99"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q: %s", want, out)
		}
	}
}

func TestVPSPlansRequiresRegion(t *testing.T) {
	mockEnv(t, "http://127.0.0.1:0")
	_, _, err := runCLI("vps", "plans")
	if err == nil || !strings.Contains(err.Error(), "--region is required") {
		t.Fatalf("expected --region required error, got %v", err)
	}
}

func TestVPSRent(t *testing.T) {
	mux := http.NewServeMux()
	var gotBody map[string]any
	mux.HandleFunc("POST /api/v1/vps/servers", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		writeEnvelope(w, http.StatusCreated, vpsServerJSON)
	})
	srv := newServer(t, mux)
	mockEnv(t, srv.URL)

	out, _, err := runCLI("vps", "rent",
		"--name", "web1",
		"--provider", "zeabur",
		"--region", "sg-singapore",
		"--plan", "sg-1c-1g")
	if err != nil {
		t.Fatalf("vps rent: %v", err)
	}
	if gotBody["name"] != "web1" || gotBody["provider"] != "zeabur" ||
		gotBody["region"] != "sg-singapore" || gotBody["plan"] != "sg-1c-1g" {
		t.Errorf("unexpected rent body: %v", gotBody)
	}
	if !strings.Contains(out, "Rented vps") {
		t.Errorf("missing success line: %s", out)
	}
}

func TestVPSStartNoWait(t *testing.T) {
	mux := http.NewServeMux()
	var getCount int32
	mux.HandleFunc("GET /api/v1/vps/servers/db1", func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&getCount, 1)
		writeEnvelope(w, http.StatusOK, vpsStoppedJSON)
	})
	mux.HandleFunc("POST /api/v1/vps/servers/8/poweron", func(w http.ResponseWriter, _ *http.Request) {
		writeEnvelope(w, http.StatusOK, "")
	})
	srv := newServer(t, mux)
	mockEnv(t, srv.URL)

	out, _, err := runCLI("vps", "start", "db1", "--wait=false")
	if err != nil {
		t.Fatalf("vps start --wait=false: %v", err)
	}
	if atomic.LoadInt32(&getCount) != 1 {
		t.Errorf("expected exactly 1 GET (resolve only); got %d", getCount)
	}
	if !strings.Contains(out, "Action queued") || !strings.Contains(out, "kumo vps get db1") {
		t.Errorf("missing polling hint: %s", out)
	}
}

func TestVPSPasswordReveal(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/vps/servers/web1", func(w http.ResponseWriter, _ *http.Request) {
		writeEnvelope(w, http.StatusOK, vpsServerJSON)
	})
	mux.HandleFunc("POST /api/v1/vps/servers/7/password", func(w http.ResponseWriter, _ *http.Request) {
		writeEnvelope(w, http.StatusOK, `{"password":"s3cr3t-init"}`)
	})
	srv := newServer(t, mux)
	mockEnv(t, srv.URL)

	out, _, err := runCLI("vps", "password", "web1")
	if err != nil {
		t.Fatalf("vps password: %v", err)
	}
	if !strings.Contains(out, "s3cr3t-init") {
		t.Errorf("missing password in output: %s", out)
	}
}

func TestVPSPasswordRevealJSON(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/vps/servers/web1", func(w http.ResponseWriter, _ *http.Request) {
		writeEnvelope(w, http.StatusOK, vpsServerJSON)
	})
	mux.HandleFunc("POST /api/v1/vps/servers/7/password", func(w http.ResponseWriter, _ *http.Request) {
		writeEnvelope(w, http.StatusOK, `{"password":"s3cr3t-init"}`)
	})
	srv := newServer(t, mux)
	mockEnv(t, srv.URL)

	out, _, err := runCLI("vps", "password", "web1", "-o", "json")
	if err != nil {
		t.Fatalf("vps password json: %v", err)
	}
	var got map[string]string
	if err := json.NewDecoder(strings.NewReader(out)).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got["password"] != "s3cr3t-init" {
		t.Errorf("unexpected json: %s", out)
	}
}

func TestVPSCancel(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/vps/servers/web1", func(w http.ResponseWriter, _ *http.Request) {
		writeEnvelope(w, http.StatusOK, vpsServerJSON)
	})
	mux.HandleFunc("POST /api/v1/vps/servers/7/cancel", func(w http.ResponseWriter, _ *http.Request) {
		writeEnvelope(w, http.StatusOK, "")
	})
	srv := newServer(t, mux)
	mockEnv(t, srv.URL)

	out, _, err := runCLI("vps", "cancel", "web1", "-y")
	if err != nil {
		t.Fatalf("vps cancel: %v", err)
	}
	if !strings.Contains(out, "Auto-renewal cancelled") || !strings.Contains(out, "2024-12-31") {
		t.Errorf("unexpected cancel output: %s", out)
	}
}

func TestVPSStartWaitPolls(t *testing.T) {
	mux := http.NewServeMux()
	var getCount int32
	// First name-resolve GET returns idle stopped server (action_status="").
	mux.HandleFunc("GET /api/v1/vps/servers/db1", func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&getCount, 1)
		writeEnvelope(w, http.StatusOK, vpsStoppedJSON)
	})
	mux.HandleFunc("POST /api/v1/vps/servers/8/poweron", func(w http.ResponseWriter, _ *http.Request) {
		writeEnvelope(w, http.StatusAccepted, "")
	})
	// Numeric-id GETs come from the poll loop after POST.
	var pollCount int32
	mux.HandleFunc("GET /api/v1/vps/servers/8", func(w http.ResponseWriter, _ *http.Request) {
		n := atomic.AddInt32(&pollCount, 1)
		if n == 1 {
			writeEnvelope(w, http.StatusOK,
				`{"id":8,"display_name":"db1","status":"stopped","action_status":"powering_on","action_status_updated_at":"2024-01-01T00:00:00Z"}`)
			return
		}
		writeEnvelope(w, http.StatusOK,
			`{"id":8,"display_name":"db1","status":"running","action_status":""}`)
	})
	srv := newServer(t, mux)
	mockEnv(t, srv.URL)

	out, _, err := runCLI("vps", "start", "db1")
	if err != nil {
		t.Fatalf("vps start --wait: %v", err)
	}
	if atomic.LoadInt32(&pollCount) < 2 {
		t.Errorf("expected at least 2 poll GETs, got %d", pollCount)
	}
	if !strings.Contains(out, "powered on") {
		t.Errorf("missing success line: %s", out)
	}
}

func TestVPSReboot(t *testing.T) {
	mux := http.NewServeMux()
	gotPath := ""
	mux.HandleFunc("GET /api/v1/vps/servers/web1", func(w http.ResponseWriter, _ *http.Request) {
		writeEnvelope(w, http.StatusOK, vpsServerJSON)
	})
	mux.HandleFunc("POST /api/v1/vps/servers/7/reboot", func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		writeEnvelope(w, http.StatusAccepted, "")
	})
	srv := newServer(t, mux)
	mockEnv(t, srv.URL)

	out, _, err := runCLI("vps", "reboot", "web1", "--wait=false")
	if err != nil {
		t.Fatalf("vps reboot: %v", err)
	}
	if gotPath != "/api/v1/vps/servers/7/reboot" {
		t.Errorf("unexpected reboot path: %q", gotPath)
	}
	if !strings.Contains(out, "Action queued") || !strings.Contains(out, "kumo vps get web1") {
		t.Errorf("missing polling hint: %s", out)
	}
}

func TestVPSStop(t *testing.T) {
	mux := http.NewServeMux()
	gotPath := ""
	mux.HandleFunc("GET /api/v1/vps/servers/web1", func(w http.ResponseWriter, _ *http.Request) {
		writeEnvelope(w, http.StatusOK, vpsServerJSON)
	})
	mux.HandleFunc("POST /api/v1/vps/servers/7/poweroff", func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		writeEnvelope(w, http.StatusAccepted, "")
	})
	srv := newServer(t, mux)
	mockEnv(t, srv.URL)

	out, _, err := runCLI("vps", "stop", "web1", "--wait=false")
	if err != nil {
		t.Fatalf("vps stop: %v", err)
	}
	if gotPath != "/api/v1/vps/servers/7/poweroff" {
		t.Errorf("unexpected poweroff path: %q", gotPath)
	}
	if !strings.Contains(out, "Action queued") || !strings.Contains(out, "kumo vps get web1") {
		t.Errorf("missing polling hint: %s", out)
	}
}

func TestVPSReinstallRequiresConfirm(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/vps/servers/web1", func(w http.ResponseWriter, _ *http.Request) {
		writeEnvelope(w, http.StatusOK, vpsServerJSON)
	})
	var posted int32
	mux.HandleFunc("POST /api/v1/vps/servers/7/reinstall", func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&posted, 1)
		writeEnvelope(w, http.StatusAccepted, "")
	})
	srv := newServer(t, mux)
	mockEnv(t, srv.URL)

	// stdin is not a TTY under go test, so confirm() refuses without -y.
	_, _, err := runCLI("vps", "reinstall", "web1", "--wait=false")
	if err == nil || !strings.Contains(err.Error(), "--yes") {
		t.Fatalf("expected confirm refusal, got: %v", err)
	}
	if atomic.LoadInt32(&posted) != 0 {
		t.Errorf("expected no POST /reinstall, got %d", posted)
	}
}

func TestVPSReinstallWithYes(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/vps/servers/web1", func(w http.ResponseWriter, _ *http.Request) {
		writeEnvelope(w, http.StatusOK, vpsServerJSON)
	})
	var posted int32
	mux.HandleFunc("POST /api/v1/vps/servers/7/reinstall", func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&posted, 1)
		writeEnvelope(w, http.StatusAccepted, "")
	})
	srv := newServer(t, mux)
	mockEnv(t, srv.URL)

	out, _, err := runCLI("vps", "reinstall", "web1", "-y", "--wait=false")
	if err != nil {
		t.Fatalf("vps reinstall: %v", err)
	}
	if atomic.LoadInt32(&posted) != 1 {
		t.Errorf("expected 1 POST /reinstall, got %d", posted)
	}
	if !strings.Contains(out, "Action queued") {
		t.Errorf("missing queued line: %s", out)
	}
}

func TestVPSRenew(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/vps/servers/web1", func(w http.ResponseWriter, _ *http.Request) {
		writeEnvelope(w, http.StatusOK, vpsServerJSON)
	})
	mux.HandleFunc("POST /api/v1/vps/servers/7/renew", func(w http.ResponseWriter, _ *http.Request) {
		writeEnvelope(w, http.StatusOK, "")
	})
	srv := newServer(t, mux)
	mockEnv(t, srv.URL)

	out, _, err := runCLI("vps", "renew", "web1")
	if err != nil {
		t.Fatalf("vps renew: %v", err)
	}
	if !strings.Contains(out, "Renewed vps 7") {
		t.Errorf("missing success: %s", out)
	}
}

func TestVPSRenewInsufficientBalance(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/vps/servers/web1", func(w http.ResponseWriter, _ *http.Request) {
		writeEnvelope(w, http.StatusOK, vpsServerJSON)
	})
	mux.HandleFunc("POST /api/v1/vps/servers/7/renew", func(w http.ResponseWriter, _ *http.Request) {
		writeError(w, http.StatusPaymentRequired, "INSUFFICIENT_BALANCE", "not enough credit")
	})
	srv := newServer(t, mux)
	mockEnv(t, srv.URL)

	_, _, err := runCLI("vps", "renew", "web1")
	if err == nil {
		t.Fatalf("expected error on insufficient balance")
	}
	msg := err.Error()
	if !strings.Contains(msg, "top up") && !strings.Contains(msg, "dashboard") {
		t.Errorf("expected top-up/dashboard hint, got: %v", err)
	}
}

func TestVPSStartInstanceExpired(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/vps/servers/web1", func(w http.ResponseWriter, _ *http.Request) {
		writeEnvelope(w, http.StatusOK, vpsServerJSON)
	})
	mux.HandleFunc("POST /api/v1/vps/servers/7/poweron", func(w http.ResponseWriter, _ *http.Request) {
		writeError(w, http.StatusBadRequest, "INSTANCE_EXPIRED", "expired")
	})
	srv := newServer(t, mux)
	mockEnv(t, srv.URL)

	_, _, err := runCLI("vps", "start", "web1", "--wait=false")
	if err == nil || !strings.Contains(err.Error(), "kumo vps renew") {
		t.Fatalf("expected renew hint, got %v", err)
	}
}

func TestVPSStartActionInProgress(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/vps/servers/web1", func(w http.ResponseWriter, _ *http.Request) {
		writeEnvelope(w, http.StatusOK, vpsServerJSON)
	})
	mux.HandleFunc("POST /api/v1/vps/servers/7/poweron", func(w http.ResponseWriter, _ *http.Request) {
		writeError(w, http.StatusConflict, "ACTION_IN_PROGRESS", "another action")
	})
	srv := newServer(t, mux)
	mockEnv(t, srv.URL)

	_, _, err := runCLI("vps", "start", "web1", "--wait=false")
	if err == nil || !strings.Contains(err.Error(), "another action") {
		t.Fatalf("expected friendly in-progress error, got %v", err)
	}
}

func TestVPSStopInstanceNotRunning(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/vps/servers/web1", func(w http.ResponseWriter, _ *http.Request) {
		writeEnvelope(w, http.StatusOK, vpsServerJSON)
	})
	mux.HandleFunc("POST /api/v1/vps/servers/7/poweroff", func(w http.ResponseWriter, _ *http.Request) {
		writeError(w, http.StatusBadRequest, "INSTANCE_NOT_RUNNING", "not running")
	})
	srv := newServer(t, mux)
	mockEnv(t, srv.URL)

	_, _, err := runCLI("vps", "stop", "web1", "--wait=false")
	if err == nil || !strings.Contains(err.Error(), "not running") {
		t.Fatalf("expected not-running error, got %v", err)
	}
}

func TestVPSResetPassword(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/vps/servers/web1", func(w http.ResponseWriter, _ *http.Request) {
		writeEnvelope(w, http.StatusOK, vpsServerJSON)
	})
	mux.HandleFunc("POST /api/v1/vps/servers/7/reset-password", func(w http.ResponseWriter, _ *http.Request) {
		writeEnvelope(w, http.StatusOK, `{"password":"newpw123"}`)
	})
	srv := newServer(t, mux)
	mockEnv(t, srv.URL)

	out, _, err := runCLI("vps", "reset-password", "web1", "-y")
	if err != nil {
		t.Fatalf("vps reset-password: %v", err)
	}
	if !strings.Contains(out, "newpw123") {
		t.Errorf("missing new password in output: %s", out)
	}
}

func TestVPSResetPasswordSSHNotReady(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/vps/servers/web1", func(w http.ResponseWriter, _ *http.Request) {
		writeEnvelope(w, http.StatusOK, vpsServerJSON)
	})
	mux.HandleFunc("POST /api/v1/vps/servers/7/reset-password", func(w http.ResponseWriter, _ *http.Request) {
		writeError(w, http.StatusConflict, "SSH_NOT_READY", "ssh not ready")
	})
	srv := newServer(t, mux)
	mockEnv(t, srv.URL)

	_, _, err := runCLI("vps", "reset-password", "web1", "-y")
	if err == nil || !strings.Contains(err.Error(), "SSH") {
		t.Fatalf("expected SSH-not-ready error, got %v", err)
	}
}

func TestVPSResetPasswordRequiresConfirm(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/vps/servers/web1", func(w http.ResponseWriter, _ *http.Request) {
		writeEnvelope(w, http.StatusOK, vpsServerJSON)
	})
	var posted int32
	mux.HandleFunc("POST /api/v1/vps/servers/7/reset-password", func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&posted, 1)
		writeEnvelope(w, http.StatusOK, `{"password":"x"}`)
	})
	srv := newServer(t, mux)
	mockEnv(t, srv.URL)

	_, _, err := runCLI("vps", "reset-password", "web1")
	if err == nil || !strings.Contains(err.Error(), "--yes") {
		t.Fatalf("expected confirm refusal, got %v", err)
	}
	if atomic.LoadInt32(&posted) != 0 {
		t.Errorf("expected no POST, got %d", posted)
	}
}

func TestVPSCancelAlreadyCancelled(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/vps/servers/web1", func(w http.ResponseWriter, _ *http.Request) {
		writeEnvelope(w, http.StatusOK, vpsServerJSON)
	})
	mux.HandleFunc("POST /api/v1/vps/servers/7/cancel", func(w http.ResponseWriter, _ *http.Request) {
		writeError(w, http.StatusConflict, "AUTO_RENEW_ALREADY_CANCELLED", "already cancelled")
	})
	srv := newServer(t, mux)
	mockEnv(t, srv.URL)

	out, _, err := runCLI("vps", "cancel", "web1", "-y")
	if err != nil {
		t.Fatalf("already-cancelled should not error: %v", err)
	}
	if !strings.Contains(out, "already cancelled") || !strings.Contains(out, "2024-12-31") {
		t.Errorf("unexpected output: %s", out)
	}
}

func TestVPSRentValidation(t *testing.T) {
	mockEnv(t, "http://127.0.0.1:0")
	cases := []struct {
		name string
		args []string
		want string
	}{
		{"missing-name", []string{"vps", "rent", "--provider", "p", "--region", "r", "--plan", "x"}, "--name is required"},
		{"missing-provider", []string{"vps", "rent", "--name", "n", "--region", "r", "--plan", "x"}, "--provider is required"},
		{"missing-region", []string{"vps", "rent", "--name", "n", "--provider", "p", "--plan", "x"}, "--region is required"},
		{"missing-plan", []string{"vps", "rent", "--name", "n", "--provider", "p", "--region", "r"}, "--plan is required"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := runCLI(tc.args...)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("expected %q, got %v", tc.want, err)
			}
		})
	}
}

func TestVPSPlansSort(t *testing.T) {
	mux := http.NewServeMux()
	gotSort := ""
	mux.HandleFunc("GET /api/v1/vps/plans", func(w http.ResponseWriter, r *http.Request) {
		gotSort = r.URL.Query().Get("sort")
		writeEnvelope(w, http.StatusOK, "[]")
	})
	srv := newServer(t, mux)
	mockEnv(t, srv.URL)

	if _, _, err := runCLI("vps", "plans", "--region", "sg-singapore", "--sort", "selling_price"); err != nil {
		t.Fatalf("vps plans --sort: %v", err)
	}
	if gotSort != "selling_price" {
		t.Errorf("expected sort=selling_price, got %q", gotSort)
	}
}

func TestVPSRename(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/vps/servers/web1", func(w http.ResponseWriter, _ *http.Request) {
		writeEnvelope(w, http.StatusOK, vpsServerJSON)
	})
	var gotBody map[string]string
	mux.HandleFunc("PATCH /api/v1/vps/servers/7/name", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		writeEnvelope(w, http.StatusOK, "")
	})
	srv := newServer(t, mux)
	mockEnv(t, srv.URL)

	out, _, err := runCLI("vps", "rename", "web1", "--new-name", "web-prod")
	if err != nil {
		t.Fatalf("vps rename: %v", err)
	}
	if gotBody["name"] != "web-prod" {
		t.Errorf("expected new name in body, got %v", gotBody)
	}
	if !strings.Contains(out, "Renamed vps 7") {
		t.Errorf("unexpected rename output: %s", out)
	}
}
