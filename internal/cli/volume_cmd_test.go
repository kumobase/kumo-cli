package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
)

const volumeReadyJSON = `{"id":7,"name":"data","app_id":42,"app_name":"web-app",` +
	`"storage_tier":{"id":1,"slug":"ssd","name":"SSD","price_per_gb_hour":"0.0001",` +
	`"min_size_gb":1,"max_size_gb":1000},"size_gb":10,"mount_path":"/data",` +
	`"status":"ready","created_at":"2024-01-01T00:00:00Z","updated_at":"2024-01-02T00:00:00Z"}`

const volumeDetachedJSON = `{"id":8,"name":"vault","app_id":null,` +
	`"storage_tier":{"id":1,"slug":"ssd","name":"SSD","price_per_gb_hour":"0.0001",` +
	`"min_size_gb":1,"max_size_gb":1000},"size_gb":5,"mount_path":"",` +
	`"status":"detached","created_at":"2024-01-01T00:00:00Z","updated_at":"2024-01-02T00:00:00Z"}`

func TestVolumeList(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/volumes", func(w http.ResponseWriter, _ *http.Request) {
		writeEnvelope(w, http.StatusOK, "["+volumeReadyJSON+"]")
	})
	srv := newServer(t, mux)
	mockEnv(t, srv.URL)

	out, _, err := runCLI("volume", "list")
	if err != nil {
		t.Fatalf("volume list: %v", err)
	}
	for _, want := range []string{"ID", "NAME", "TIER", "SIZE", "STATUS", "APP", "MOUNT", "CREATED", "data", "ssd", "10 GB", "ready", "web-app", "/data"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q: %s", want, out)
		}
	}
}

func TestVolumeListJSON(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/volumes", func(w http.ResponseWriter, _ *http.Request) {
		writeEnvelope(w, http.StatusOK, "["+volumeReadyJSON+"]")
	})
	srv := newServer(t, mux)
	mockEnv(t, srv.URL)

	out, _, err := runCLI("volume", "list", "-o", "json")
	if err != nil {
		t.Fatalf("volume list json: %v", err)
	}
	var got []map[string]any
	if err := json.NewDecoder(strings.NewReader(out)).Decode(&got); err != nil && err != io.EOF {
		t.Fatalf("decode json: %v", err)
	}
	if len(got) != 1 || got[0]["name"] != "data" {
		t.Errorf("unexpected json: %s", out)
	}
}

func TestVolumeGet(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/volumes/data", func(w http.ResponseWriter, _ *http.Request) {
		writeEnvelope(w, http.StatusOK, volumeReadyJSON)
	})
	srv := newServer(t, mux)
	mockEnv(t, srv.URL)

	out, _, err := runCLI("volume", "get", "data")
	if err != nil {
		t.Fatalf("volume get: %v", err)
	}
	for _, want := range []string{"ID:", "Name:", "data", "Status:", "ready", "Tier:", "ssd", "Size:", "10 GB", "Mount path:", "/data", "App:", "web-app"} {
		if !strings.Contains(out, want) {
			t.Errorf("detail missing %q: %s", want, out)
		}
	}
}

func TestVolumeCreate(t *testing.T) {
	var gotBody map[string]any
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/volumes", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		writeEnvelope(w, http.StatusOK, volumeDetachedJSON)
	})
	srv := newServer(t, mux)
	mockEnv(t, srv.URL)

	out, _, err := runCLI("volume", "create", "--name", "vault", "--tier", "ssd", "--size", "5", "--wait=false")
	if err != nil {
		t.Fatalf("volume create: %v", err)
	}
	if gotBody["name"] != "vault" || gotBody["storage_tier"] != "ssd" {
		t.Errorf("unexpected request body: %v", gotBody)
	}
	if sz, _ := gotBody["size_gb"].(float64); int(sz) != 5 {
		t.Errorf("unexpected size_gb: %v", gotBody["size_gb"])
	}
	if !strings.Contains(out, "Created volume") {
		t.Errorf("missing created message: %s", out)
	}
}

func TestVolumeCreateRequiresMountWithApp(t *testing.T) {
	mux := http.NewServeMux()
	srv := newServer(t, mux)
	mockEnv(t, srv.URL)

	_, _, err := runCLI("volume", "create", "--name", "vault", "--tier", "ssd", "--size", "5", "--app", "web-app")
	if err == nil || !strings.Contains(err.Error(), "--mount is required") {
		t.Fatalf("expected --mount required error, got: %v", err)
	}
}

func TestVolumeDeleteAttachedHint(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/volumes/data", func(w http.ResponseWriter, _ *http.Request) {
		writeEnvelope(w, http.StatusOK, volumeReadyJSON)
	})
	mux.HandleFunc("DELETE /api/v1/volumes/7", func(w http.ResponseWriter, _ *http.Request) {
		writeError(w, http.StatusConflict, "VOLUME_ATTACHED", "volume is attached")
	})
	srv := newServer(t, mux)
	mockEnv(t, srv.URL)

	_, _, err := runCLI("volume", "delete", "data", "-y")
	if err == nil || !strings.Contains(err.Error(), "kumo volume detach") {
		t.Fatalf("expected detach hint, got: %v", err)
	}
}

func TestVolumeResize(t *testing.T) {
	var gotBody map[string]any
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/volumes/data", func(w http.ResponseWriter, _ *http.Request) {
		writeEnvelope(w, http.StatusOK, volumeReadyJSON)
	})
	mux.HandleFunc("GET /api/v1/apps/42", handleAppDetail())
	mux.HandleFunc("PATCH /api/v1/volumes/7/resize", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		resized := strings.Replace(volumeReadyJSON, `"size_gb":10`, `"size_gb":20`, 1)
		writeEnvelope(w, http.StatusOK, resized)
	})
	srv := newServer(t, mux)
	mockEnv(t, srv.URL)

	out, _, err := runCLI("volume", "resize", "data", "--size", "20", "--force", "--wait=false")
	if err != nil {
		t.Fatalf("volume resize: %v", err)
	}
	if sz, _ := gotBody["size_gb"].(float64); int(sz) != 20 {
		t.Errorf("unexpected size_gb in request body: %v", gotBody)
	}
	if !strings.Contains(out, "Resize queued") {
		t.Errorf("missing queued message: %s", out)
	}
}

func TestVolumeAttach(t *testing.T) {
	var gotBody map[string]any
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/volumes/vault", func(w http.ResponseWriter, _ *http.Request) {
		writeEnvelope(w, http.StatusOK, volumeDetachedJSON)
	})
	mux.HandleFunc("POST /api/v1/volumes/8/attach", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		writeEnvelope(w, http.StatusOK, volumeReadyJSON)
	})
	srv := newServer(t, mux)
	mockEnv(t, srv.URL)

	out, _, err := runCLI("volume", "attach", "vault", "--app", "web-app", "--mount", "/data")
	if err != nil {
		t.Fatalf("volume attach: %v", err)
	}
	if gotBody["app_name"] != "web-app" || gotBody["mount_path"] != "/data" {
		t.Errorf("unexpected attach body: %v", gotBody)
	}
	if !strings.Contains(out, "attached to web-app at /data") {
		t.Errorf("missing attach output: %s", out)
	}
}

// volumeCreatingJSON has id=7 so polling hits the same GET /volumes/7 mock.
const volumeCreatingJSON = `{"id":7,"name":"data","app_id":null,` +
	`"storage_tier":{"id":1,"slug":"ssd","name":"SSD","price_per_gb_hour":"0.0001",` +
	`"min_size_gb":1,"max_size_gb":1000},"size_gb":10,"mount_path":"",` +
	`"status":"creating","created_at":"2024-01-01T00:00:00Z","updated_at":"2024-01-02T00:00:00Z"}`

func TestVolumeDetach(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/volumes/data", func(w http.ResponseWriter, _ *http.Request) {
		writeEnvelope(w, http.StatusOK, volumeReadyJSON)
	})
	mux.HandleFunc("POST /api/v1/volumes/7/detach", func(w http.ResponseWriter, _ *http.Request) {
		// Server returns the volume in the detached/ready post-detach shape.
		detached := strings.Replace(volumeReadyJSON, `"app_id":42`, `"app_id":null`, 1)
		writeEnvelope(w, http.StatusOK, detached)
	})
	srv := newServer(t, mux)
	mockEnv(t, srv.URL)

	out, _, err := runCLI("volume", "detach", "data")
	if err != nil {
		t.Fatalf("volume detach: %v", err)
	}
	if !strings.Contains(out, "Volume 7 detached") {
		t.Errorf("missing detach output: %s", out)
	}
}

func TestVolumeCreateWaitPolls(t *testing.T) {
	var getCalls atomic.Int32
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/volumes", func(w http.ResponseWriter, _ *http.Request) {
		writeEnvelope(w, http.StatusOK, volumeCreatingJSON)
	})
	mux.HandleFunc("GET /api/v1/volumes/7", func(w http.ResponseWriter, _ *http.Request) {
		n := getCalls.Add(1)
		if n == 1 {
			writeEnvelope(w, http.StatusOK, volumeCreatingJSON)
			return
		}
		writeEnvelope(w, http.StatusOK, volumeReadyJSON)
	})
	srv := newServer(t, mux)
	mockEnv(t, srv.URL)

	out, _, err := runCLI("volume", "create", "--name", "data", "--tier", "ssd", "--size", "10")
	if err != nil {
		t.Fatalf("volume create wait: %v", err)
	}
	if got := getCalls.Load(); got < 2 {
		t.Errorf("expected >=2 GET polls, got %d", got)
	}
	if !strings.Contains(out, "ready") {
		t.Errorf("expected final status ready in output: %s", out)
	}
}

func TestVolumeCreateNoWait(t *testing.T) {
	var postCalls, getCalls atomic.Int32
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/volumes", func(w http.ResponseWriter, _ *http.Request) {
		postCalls.Add(1)
		writeEnvelope(w, http.StatusOK, volumeCreatingJSON)
	})
	mux.HandleFunc("GET /api/v1/volumes/", func(w http.ResponseWriter, _ *http.Request) {
		getCalls.Add(1)
		writeEnvelope(w, http.StatusOK, volumeCreatingJSON)
	})
	srv := newServer(t, mux)
	mockEnv(t, srv.URL)

	out, _, err := runCLI("volume", "create", "--name", "data", "--tier", "ssd", "--size", "10", "--wait=false")
	if err != nil {
		t.Fatalf("volume create no-wait: %v", err)
	}
	if postCalls.Load() != 1 {
		t.Errorf("expected 1 POST, got %d", postCalls.Load())
	}
	if getCalls.Load() != 0 {
		t.Errorf("expected 0 GET polls with --wait=false, got %d", getCalls.Load())
	}
	if !strings.Contains(out, "Run `kumo volume get") {
		t.Errorf("missing poll hint: %s", out)
	}
}

func TestVolumeResizeForce(t *testing.T) {
	var gotBody map[string]any
	var appGetCalls atomic.Int32
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/volumes/data", func(w http.ResponseWriter, _ *http.Request) {
		writeEnvelope(w, http.StatusOK, volumeReadyJSON)
	})
	// Catch any GET on /api/v1/apps/* — preflight must be skipped with --force.
	mux.HandleFunc("GET /api/v1/apps/", func(w http.ResponseWriter, _ *http.Request) {
		appGetCalls.Add(1)
		writeEnvelope(w, http.StatusOK, appDetailJSON)
	})
	mux.HandleFunc("PATCH /api/v1/volumes/7/resize", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		resized := strings.Replace(volumeReadyJSON, `"size_gb":10`, `"size_gb":20`, 1)
		writeEnvelope(w, http.StatusOK, resized)
	})
	srv := newServer(t, mux)
	mockEnv(t, srv.URL)

	_, _, err := runCLI("volume", "resize", "data", "--size", "20", "--force", "--wait=false")
	if err != nil {
		t.Fatalf("volume resize --force: %v", err)
	}
	if appGetCalls.Load() != 0 {
		t.Errorf("expected preflight GET /apps/* skipped with --force, got %d calls", appGetCalls.Load())
	}
	if sz, _ := gotBody["size_gb"].(float64); int(sz) != 20 {
		t.Errorf("expected size_gb=20 in PATCH body, got: %v", gotBody)
	}
}

func TestVolumeResizeNotAttached(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/volumes/data", func(w http.ResponseWriter, _ *http.Request) {
		writeEnvelope(w, http.StatusOK, volumeReadyJSON)
	})
	mux.HandleFunc("GET /api/v1/apps/42", handleAppDetail())
	mux.HandleFunc("PATCH /api/v1/volumes/7/resize", func(w http.ResponseWriter, _ *http.Request) {
		writeError(w, http.StatusConflict, "VOLUME_NOT_ATTACHED", "volume not attached")
	})
	srv := newServer(t, mux)
	mockEnv(t, srv.URL)

	// --force bypasses preflight (appDetailJSON has replicas:2) so the backend error path runs.
	_, _, err := runCLI("volume", "resize", "data", "--size", "20", "--force", "--wait=false")
	if err == nil || !strings.Contains(err.Error(), "kumo volume attach") {
		t.Fatalf("expected attach hint, got: %v", err)
	}
}

func TestVolumeResizeCannotShrink(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/volumes/data", func(w http.ResponseWriter, _ *http.Request) {
		writeEnvelope(w, http.StatusOK, volumeReadyJSON)
	})
	mux.HandleFunc("PATCH /api/v1/volumes/7/resize", func(w http.ResponseWriter, _ *http.Request) {
		writeError(w, http.StatusBadRequest, "CANNOT_SHRINK_VOLUME", "cannot shrink an existing volume")
	})
	srv := newServer(t, mux)
	mockEnv(t, srv.URL)

	_, _, err := runCLI("volume", "resize", "data", "--size", "5", "--force", "--wait=false")
	if err == nil || !strings.Contains(err.Error(), "cannot shrink") {
		t.Fatalf("expected backend shrink message, got: %v", err)
	}
}

func TestVolumeResizeAppMustHaveOneReplica(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/volumes/data", func(w http.ResponseWriter, _ *http.Request) {
		writeEnvelope(w, http.StatusOK, volumeReadyJSON)
	})
	mux.HandleFunc("PATCH /api/v1/volumes/7/resize", func(w http.ResponseWriter, _ *http.Request) {
		writeError(w, http.StatusConflict, "APP_MUST_HAVE_ONE_REPLICA", "app must have one replica")
	})
	srv := newServer(t, mux)
	mockEnv(t, srv.URL)

	_, _, err := runCLI("volume", "resize", "data", "--size", "20", "--force", "--wait=false")
	if err == nil {
		t.Fatal("expected error")
	}
	msg := err.Error()
	if !strings.Contains(msg, "--force") && !strings.Contains(msg, "scale the app down") {
		t.Errorf("expected --force or scale hint, got: %v", err)
	}
}

func TestVolumeResizeBusy(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/volumes/data", func(w http.ResponseWriter, _ *http.Request) {
		writeEnvelope(w, http.StatusOK, volumeReadyJSON)
	})
	mux.HandleFunc("PATCH /api/v1/volumes/7/resize", func(w http.ResponseWriter, _ *http.Request) {
		writeError(w, http.StatusConflict, "VOLUME_RESIZING", "already resizing")
	})
	srv := newServer(t, mux)
	mockEnv(t, srv.URL)

	_, _, err := runCLI("volume", "resize", "data", "--size", "20", "--force", "--wait=false")
	if err == nil || !strings.Contains(err.Error(), "busy") {
		t.Fatalf("expected busy message, got: %v", err)
	}
}

func TestVolumeAttachTargetHasVolume(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/volumes/vault", func(w http.ResponseWriter, _ *http.Request) {
		writeEnvelope(w, http.StatusOK, volumeDetachedJSON)
	})
	mux.HandleFunc("POST /api/v1/volumes/8/attach", func(w http.ResponseWriter, _ *http.Request) {
		writeError(w, http.StatusConflict, "TARGET_APP_ALREADY_HAS_VOLUME", "app already has a volume")
	})
	srv := newServer(t, mux)
	mockEnv(t, srv.URL)

	_, _, err := runCLI("volume", "attach", "vault", "--app", "web-app", "--mount", "/data")
	if err == nil || !strings.Contains(err.Error(), "already has a volume") {
		t.Fatalf("expected target-has-volume error, got: %v", err)
	}
}

func TestVolumeGetNotFound(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/volumes/ghost", func(w http.ResponseWriter, _ *http.Request) {
		writeError(w, http.StatusNotFound, "VOLUME_NOT_FOUND", "volume not found")
	})
	srv := newServer(t, mux)
	mockEnv(t, srv.URL)

	_, _, err := runCLI("volume", "get", "ghost")
	if err == nil || !strings.Contains(err.Error(), `no volume named "ghost"`) {
		t.Fatalf("expected not-found message, got: %v", err)
	}
}

func TestVolumeDeleteRequiresConfirm(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/volumes/data", func(w http.ResponseWriter, _ *http.Request) {
		writeEnvelope(w, http.StatusOK, volumeReadyJSON)
	})
	srv := newServer(t, mux)
	mockEnv(t, srv.URL)

	// Without -y and a non-TTY stdin (the test harness), confirm() must refuse.
	_, _, err := runCLI("volume", "delete", "data")
	if err == nil || !strings.Contains(err.Error(), "--yes") {
		t.Fatalf("expected --yes refusal, got: %v", err)
	}
}

func TestVolumeListFilters(t *testing.T) {
	var gotQuery string
	mux := http.NewServeMux()
	// resolveAppRef in --app filter hits GET /api/v1/apps/{name}.
	mux.HandleFunc("GET /api/v1/apps/web-app", handleAppDetail())
	mux.HandleFunc("GET /api/v1/volumes", func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		writeEnvelope(w, http.StatusOK, "["+volumeReadyJSON+"]")
	})
	srv := newServer(t, mux)
	mockEnv(t, srv.URL)

	_, _, err := runCLI("volume", "list",
		"--status", "ready",
		"--app", "web-app",
		"--attached", "true",
	)
	if err != nil {
		t.Fatalf("volume list filters: %v", err)
	}
	for _, want := range []string{"status=ready", "attached=true", "app_id=42"} {
		if !strings.Contains(gotQuery, want) {
			t.Errorf("query missing %q: %s", want, gotQuery)
		}
	}
}

const storageTierJSON = `{"id":1,"slug":"ssd","name":"SSD","price_per_gb_hour":"0.0001",` +
	`"min_size_gb":1,"max_size_gb":1000}`

func TestVolumePlans(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/volumes/plans", func(w http.ResponseWriter, _ *http.Request) {
		writeEnvelope(w, http.StatusOK, "["+storageTierJSON+"]")
	})
	srv := newServer(t, mux)
	mockEnv(t, srv.URL)

	out, _, err := runCLI("volume", "plans")
	if err != nil {
		t.Fatalf("volume plans: %v", err)
	}
	for _, want := range []string{"SLUG", "NAME", "MIN GB", "MAX GB", "PRICE/GB-HR", "ssd", "SSD", "0.0001"} {
		if !strings.Contains(out, want) {
			t.Errorf("plans missing %q: %s", want, out)
		}
	}
	// Single page → no pagination footer.
	if strings.Contains(out, "use --page") {
		t.Errorf("single-page output should not show a page footer: %s", out)
	}
}

func TestVolumePlansPaginatedShowsFooter(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/volumes/plans", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		// Envelope carries a meta block with more than one page.
		fmt.Fprintf(w, `{"code":"OK","message":"ok","data":[%s],`+
			`"meta":{"page":1,"page_size":1,"total_items":3,"total_pages":3}}`, storageTierJSON)
	})
	srv := newServer(t, mux)
	mockEnv(t, srv.URL)

	out, _, err := runCLI("volume", "plans")
	if err != nil {
		t.Fatalf("volume plans paginated: %v", err)
	}
	for _, want := range []string{"Page 1/3", "3 items", "use --page"} {
		if !strings.Contains(out, want) {
			t.Errorf("paginated footer missing %q: %s", want, out)
		}
	}
}
