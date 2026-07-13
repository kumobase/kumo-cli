package cli

import (
	"net/http"
	"strings"
	"testing"
)

// TestMutationSuccessEnvelope asserts a state-changing command emits the
// {"ok":true,"data":{resource,action,status}} envelope under -o json instead
// of a plain success line.
func TestMutationSuccessEnvelope(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/volumes/data", func(w http.ResponseWriter, _ *http.Request) {
		writeEnvelope(w, http.StatusOK, volumeReadyJSON)
	})
	mux.HandleFunc("DELETE /api/v1/volumes/7", func(w http.ResponseWriter, _ *http.Request) {
		writeEnvelope(w, http.StatusOK, "")
	})
	srv := newServer(t, mux)
	mockEnv(t, srv.URL)

	out, _, err := runCLI("volume", "delete", "data", "-y", "-o", "json")
	if err != nil {
		t.Fatalf("volume delete json: %v", err)
	}
	var got map[string]any
	decodeData(t, out, &got)
	if got["resource"] != "volume" || got["action"] != "delete" || got["status"] != "done" {
		t.Errorf("unexpected action-result envelope: %s", out)
	}
}

// TestQuietSuppressesHumanLine asserts --quiet drops the table success line but
// still returns success (exit 0).
func TestQuietSuppressesHumanLine(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/volumes/data", func(w http.ResponseWriter, _ *http.Request) {
		writeEnvelope(w, http.StatusOK, volumeReadyJSON)
	})
	mux.HandleFunc("DELETE /api/v1/volumes/7", func(w http.ResponseWriter, _ *http.Request) {
		writeEnvelope(w, http.StatusOK, "")
	})
	srv := newServer(t, mux)
	mockEnv(t, srv.URL)

	out, _, err := runCLI("volume", "delete", "data", "-y", "--quiet")
	if err != nil {
		t.Fatalf("volume delete quiet: %v", err)
	}
	if strings.TrimSpace(out) != "" {
		t.Errorf("--quiet should suppress the success line, got: %q", out)
	}
}

// TestDeclinedConfirmAbortsCleanly asserts a declined confirmation exits 0 with
// an ABORTED envelope in JSON mode.
func TestDeclinedConfirmAbortsCleanly(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/volumes/data", func(w http.ResponseWriter, _ *http.Request) {
		writeEnvelope(w, http.StatusOK, volumeReadyJSON)
	})
	srv := newServer(t, mux)
	mockEnv(t, srv.URL)

	// Inject "n" on stdin so confirm() reads a decline instead of refusing.
	root := NewRootCmd()
	var out strings.Builder
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetIn(strings.NewReader("n\n"))
	root.SetArgs([]string{"volume", "delete", "data", "-o", "json"})
	if err := root.Execute(); err != nil {
		t.Fatalf("declined confirm should not error: %v", err)
	}
	if !strings.Contains(out.String(), `"code": "ABORTED"`) {
		t.Errorf("expected ABORTED envelope, got: %s", out.String())
	}
}

func TestIntrospectEmitsTree(t *testing.T) {
	mockEnv(t, "http://127.0.0.1:0")
	out, _, err := runCLI("introspect")
	if err != nil {
		t.Fatalf("introspect: %v", err)
	}
	var tree map[string]any
	decodeData(t, out, &tree)
	if tree["path"] != "kumo" {
		t.Errorf("introspect root path = %v, want kumo", tree["path"])
	}
	subs, ok := tree["subcommands"].([]any)
	if !ok || len(subs) == 0 {
		t.Errorf("introspect should list subcommands: %s", out)
	}
}
