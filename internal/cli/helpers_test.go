package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
)

// decodeData decodes a bare -o json success response (an object on stdout) into
// v. Success output is emitted bare (aws/gh/kubectl convention).
func decodeData(t *testing.T, out string, v any) {
	t.Helper()
	if err := json.Unmarshal([]byte(out), v); err != nil {
		t.Fatalf("decode json: %v (out=%s)", err, out)
	}
}

// decodeItems decodes a bare -o json list response (an array on stdout) into v.
func decodeItems(t *testing.T, out string, v any) {
	t.Helper()
	if err := json.Unmarshal([]byte(out), v); err != nil {
		t.Fatalf("decode json list: %v (out=%s)", err, out)
	}
}

// runCLI builds a fresh root command, runs it with the given args, and
// returns captured stdout, stderr, and the execution error.
//
// Tests must NOT run in parallel: the persistent flags are package-level
// vars that NewRootCmd re-registers (resetting to defaults) on each call,
// and the environment is shared process-wide.
func runCLI(args ...string) (stdout, stderr string, err error) {
	root := NewRootCmd()
	var out, errBuf bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&errBuf)
	root.SetArgs(args)
	err = root.Execute()
	if err != nil {
		// Mirror Execute's error rendering so tests see the same stderr
		// (JSON error envelope or human line) the real entrypoint produces.
		renderExecError(&errBuf, err)
	}
	return out.String(), errBuf.String(), err
}

// mockEnv points the CLI at the mock server and an isolated config home.
// Call once per test (a single TempDir is reused across the test's runCLI
// calls so login/logout state persists within the test).
func mockEnv(t *testing.T, srvURL string) {
	t.Helper()
	t.Setenv("KUMO_HOME", t.TempDir())
	t.Setenv("KUMO_PROFILE", "")
	t.Setenv("KUMO_API_KEY", "kumo_sk_test1234567890")
	t.Setenv("KUMO_BASE_URL", srvURL)
	t.Setenv("KUMO_OUTPUT", "")
}

// newServer starts an httptest server with the given mux and registers
// cleanup.
func newServer(t *testing.T, mux *http.ServeMux) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// writeEnvelope writes a success StructureResponse with the given raw JSON
// data payload. Pass an empty dataJSON to omit the data field.
func writeEnvelope(w http.ResponseWriter, status int, dataJSON string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if dataJSON == "" {
		fmt.Fprint(w, `{"code":"OK","message":"ok"}`)
		return
	}
	fmt.Fprintf(w, `{"code":"OK","message":"ok","data":%s}`, dataJSON)
}

// writeError writes an error StructureResponse the SDK decodes into an
// *client.APIError.
func writeError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	fmt.Fprintf(w, `{"code":%q,"message":%q}`, code, message)
}

// profileJSON is a stock GetProfileResponse body used by login/whoami tests.
const profileJSON = `{"full_name":"Test User","email":"test@example.com","is_verified":true,"is_admin":false,"has_password":true,"has_google":false}`

// handleProfile responds to GET /api/v1/profile with profileJSON.
func handleProfile(w http.ResponseWriter, _ *http.Request) {
	writeEnvelope(w, http.StatusOK, profileJSON)
}

// handlePullable responds to POST /api/v1/apps/validate-image reporting the
// image is pullable on both platforms — the pre-flight every deploy runs.
func handlePullable(w http.ResponseWriter, _ *http.Request) {
	writeEnvelope(w, http.StatusOK, `{"linux_amd64":true,"linux_arm64":true}`)
}

// handleSecretType returns a GET /api/v1/secrets/{id} handler that echoes the
// requested id back with the given secret type, satisfying the app↔secret
// pre-flight guard. The path segment may be either a numeric id or a name —
// in the name case, the returned id is a fixed sentinel (99) since the
// server resolves names to ids before returning a body.
func handleSecretType(secretType string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		seg := r.PathValue("id")
		id := seg
		if _, err := strconv.ParseUint(seg, 10, 64); err != nil {
			id = "99"
		}
		writeEnvelope(w, http.StatusOK, fmt.Sprintf(`{"id":%s,"name":%q,"type":%q}`, id, seg, secretType))
	}
}

// handleAppDetail returns a GET /api/v1/apps/{id} handler that serves the
// canonical appDetailJSON fixture with an ETag header so callers can drive
// id-or-name resolution and update-with-If-Match flows.
func handleAppDetail() http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("ETag", `W/"abc123"`)
		writeEnvelope(w, http.StatusOK, appDetailJSON)
	}
}

// handleSecretDetail returns a GET /api/v1/secrets/{id} handler that serves
// a stock env_var secret fixture for tests that just need the resolver to
// succeed.
func handleSecretDetail() http.HandlerFunc {
	return handleSecretType("env_var")
}
