package cli

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

// TestIdempotencyKeyThreaded asserts --idempotency-key reaches the write as the
// Idempotency-Key header.
func TestIdempotencyKeyThreaded(t *testing.T) {
	var gotKey string
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/volumes", func(w http.ResponseWriter, r *http.Request) {
		gotKey = r.Header.Get("Idempotency-Key")
		writeEnvelope(w, http.StatusOK, volumeDetachedJSON)
	})
	srv := newServer(t, mux)
	mockEnv(t, srv.URL)

	_, _, err := runCLI("volume", "create", "--name", "vault", "--tier", "ssd", "--size", "5",
		"--wait=false", "--idempotency-key", "my-fixed-key")
	if err != nil {
		t.Fatalf("volume create: %v", err)
	}
	if gotKey != "my-fixed-key" {
		t.Errorf("Idempotency-Key header = %q, want my-fixed-key", gotKey)
	}
}

// TestNumericIDLookup asserts a bare numeric positional resolves by id (GET
// /secrets/{id}) rather than by name.
func TestNumericIDLookup(t *testing.T) {
	var hitByID bool
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/secrets/42", func(w http.ResponseWriter, _ *http.Request) {
		hitByID = true
		writeEnvelope(w, http.StatusOK, `{"id":42,"name":"dockerhub","type":"registry"}`)
	})
	srv := newServer(t, mux)
	mockEnv(t, srv.URL)

	if _, _, err := runCLI("secret", "get", "42"); err != nil {
		t.Fatalf("secret get by id: %v", err)
	}
	if !hitByID {
		t.Error("numeric positional should resolve via GET /secrets/42")
	}
}

// TestRegistryPasswordStdin asserts the password is read from stdin and never
// needs to appear on the command line.
func TestRegistryPasswordStdin(t *testing.T) {
	var rawBody string
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/secrets", func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		rawBody = string(b)
		writeEnvelope(w, http.StatusOK, `{"id":1,"name":"dockerhub","type":"registry"}`)
	})
	srv := newServer(t, mux)
	mockEnv(t, srv.URL)

	root := NewRootCmd()
	var out strings.Builder
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetIn(strings.NewReader("s3cr3t-pass\n"))
	root.SetArgs([]string{"secret", "create", "--name", "dockerhub", "--type", "registry",
		"--registry-username", "alice", "--registry-password-stdin"})
	if err := root.Execute(); err != nil {
		t.Fatalf("secret create stdin: %v (%s)", err, out.String())
	}
	if !strings.Contains(rawBody, "s3cr3t-pass") {
		t.Errorf("request body missing stdin password: %s", rawBody)
	}
}
