package cli

import (
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/kumobase/kumo-go/client"
)

// TestJSONErrorEnvelope asserts that under -o json a failed command emits a
// {"ok":false,"error":{…}} envelope to stderr carrying the stable wire code,
// while the table path keeps the friendly human line.
func TestJSONErrorEnvelope(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/apps/missing", func(w http.ResponseWriter, _ *http.Request) {
		writeError(w, http.StatusNotFound, "APP_NOT_FOUND", "no such app")
	})
	srv := newServer(t, mux)
	mockEnv(t, srv.URL)

	_, stderr, err := runCLI("apps", "get", "missing", "-o", "json")
	if err == nil {
		t.Fatal("expected error")
	}
	for _, want := range []string{`"ok": false`, `"code": "APP_NOT_FOUND"`, `"http_status": 404`} {
		if !strings.Contains(stderr, want) {
			t.Errorf("JSON error envelope missing %q: %s", want, stderr)
		}
	}

	_, stderr2, _ := runCLI("apps", "get", "missing")
	if !strings.Contains(stderr2, `Error: no app named "missing"`) {
		t.Errorf("table error path not friendly: %s", stderr2)
	}
}

// TestExitCodeClasses pins the error-class → exit-code mapping agents rely on.
func TestExitCodeClasses(t *testing.T) {
	apiErr := func(status int, code string) error {
		return &client.APIError{StatusCode: status, Code: code, Message: "x"}
	}
	cases := []struct {
		name string
		err  error
		want int
	}{
		{"nil", nil, 0},
		{"usage", usageError{err: errors.New("bad flag")}, 2},
		{"auth", apiErr(http.StatusUnauthorized, "UNAUTHORIZED"), 3},
		{"notfound", apiErr(http.StatusNotFound, "APP_NOT_FOUND"), 4},
		{"conflict", apiErr(http.StatusConflict, "NAME_TAKEN"), 5},
		{"etag", client.ErrETagMismatch, 7},
		{"generic", errors.New("boom"), 1},
		{"friendly-wrapped-notfound", friendlyf(apiErr(http.StatusNotFound, "APP_NOT_FOUND"), "no app named %q", "web"), 4},
	}
	for _, tc := range cases {
		if got := exitCodeFor(tc.err); got != tc.want {
			t.Errorf("%s: exitCodeFor = %d, want %d", tc.name, got, tc.want)
		}
	}
}
