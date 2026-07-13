package config

import (
	"os"
	"path/filepath"
	"testing"
)

// withHome points KUMO_HOME at a temp dir for the duration of a test.
func withHome(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv(EnvHome, dir)
	// Clear resolution env so tests are deterministic.
	for _, k := range []string{EnvProfile, EnvAPIKey, EnvBaseURL, EnvOutput} {
		t.Setenv(k, "")
	}
	return dir
}

func TestSaveAndLoadRoundTrip(t *testing.T) {
	dir := withHome(t)

	s, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	s.SetProfile("default", Profile{BaseURL: "https://example.test", Output: "json"})
	s.SetCredential("default", Credential{Type: CredentialTypeAPIKey, APIKey: "kumo_sk_abc"})
	s.SetCurrentProfile("default")
	if err := s.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// credentials.yaml must be 0600.
	info, err := os.Stat(filepath.Join(dir, "credentials.yaml"))
	if err != nil {
		t.Fatalf("stat credentials: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("credentials.yaml perm = %o, want 600", perm)
	}

	got, err := Load()
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	cred, ok := got.Credential("default")
	if !ok || cred.APIKey != "kumo_sk_abc" {
		t.Errorf("credential = %+v, ok=%v", cred, ok)
	}
}

func TestResolvePrecedence(t *testing.T) {
	withHome(t)
	s, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	s.SetProfile("default", Profile{BaseURL: "https://profile.test", Output: "json"})
	s.SetCredential("default", Credential{Type: CredentialTypeAPIKey, APIKey: "from-file"})

	// Defaults applied when nothing set.
	got := s.Resolve(Overrides{})
	if got.BaseURL != "https://profile.test" || got.Output != "json" || got.APIKey != "from-file" {
		t.Errorf("profile values not used: %+v", got)
	}

	// Env overrides the file.
	t.Setenv(EnvBaseURL, "https://env.test")
	t.Setenv(EnvAPIKey, "from-env")
	got = s.Resolve(Overrides{})
	if got.BaseURL != "https://env.test" || got.APIKey != "from-env" {
		t.Errorf("env did not override file: %+v", got)
	}

	// Flag overrides env.
	got = s.Resolve(Overrides{BaseURL: "https://flag.test", Output: "table"})
	if got.BaseURL != "https://flag.test" || got.Output != "table" {
		t.Errorf("flag did not override env: %+v", got)
	}
}

func TestResolveDefaults(t *testing.T) {
	withHome(t)
	s, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	got := s.Resolve(Overrides{})
	if got.Profile != DefaultProfile {
		t.Errorf("profile = %q, want %q", got.Profile, DefaultProfile)
	}
	if got.BaseURL != DefaultBaseURL {
		t.Errorf("base URL = %q, want %q", got.BaseURL, DefaultBaseURL)
	}
	if got.Output != DefaultOutput {
		t.Errorf("output = %q, want %q", got.Output, DefaultOutput)
	}
	if got.APIKey != "" {
		t.Errorf("api key = %q, want empty", got.APIKey)
	}
}
