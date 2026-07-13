package cli

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

// TestAppCreateAutoscalingFlags asserts the shared autoscaling flags reach the
// create request (they previously could only be set via a manifest).
func TestAppCreateAutoscalingFlags(t *testing.T) {
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
		"--wait=false", "--autoscale", "--min-replicas", "2", "--max-replicas", "5", "--cpu-target", "70")
	if err != nil {
		t.Fatalf("apps create with autoscaling: %v", err)
	}
	for _, want := range []string{"autoscaling", "\"min_replicas\":2", "\"max_replicas\":5"} {
		if !strings.Contains(body, want) {
			t.Errorf("create body missing %q: %s", want, body)
		}
	}
}
