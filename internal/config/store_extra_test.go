package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestHomeEnvOverride(t *testing.T) {
	t.Setenv(EnvHome, "/custom/kumo/home")
	got, err := Home()
	if err != nil {
		t.Fatalf("Home: %v", err)
	}
	if got != "/custom/kumo/home" {
		t.Errorf("Home = %q, want /custom/kumo/home", got)
	}
}

func TestHomeDefault(t *testing.T) {
	t.Setenv(EnvHome, "")
	got, err := Home()
	if err != nil {
		t.Fatalf("Home: %v", err)
	}
	base, _ := os.UserHomeDir()
	if want := filepath.Join(base, ".kumo"); got != want {
		t.Errorf("Home = %q, want %q", got, want)
	}
}

func TestGetProfile(t *testing.T) {
	withHome(t)
	s, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if _, ok := s.GetProfile("missing"); ok {
		t.Error("GetProfile(missing) should report ok=false")
	}
	s.SetProfile("staging", Profile{BaseURL: "https://staging.test", Output: "json"})
	p, ok := s.GetProfile("staging")
	if !ok || p.BaseURL != "https://staging.test" || p.Output != "json" {
		t.Errorf("GetProfile(staging) = %+v, ok=%v", p, ok)
	}
}

func TestRemoveCredential(t *testing.T) {
	withHome(t)
	s, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	s.SetCredential("default", Credential{Type: CredentialTypeAPIKey, APIKey: "kumo_sk_x"})
	if _, ok := s.Credential("default"); !ok {
		t.Fatal("credential should exist before removal")
	}
	s.RemoveCredential("default")
	if _, ok := s.Credential("default"); ok {
		t.Error("credential should be gone after RemoveCredential")
	}
	// Removing a missing profile is a no-op, not a panic.
	s.RemoveCredential("nope")
}

func TestSetCurrentProfileAndDefault(t *testing.T) {
	withHome(t)
	s, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if s.CurrentProfile() != DefaultProfile {
		t.Errorf("CurrentProfile default = %q, want %q", s.CurrentProfile(), DefaultProfile)
	}
	s.SetCurrentProfile("prod")
	if s.CurrentProfile() != "prod" {
		t.Errorf("CurrentProfile = %q, want prod", s.CurrentProfile())
	}
}

// TestLoadMalformedYAML exercises the parse-error path of readYAML/Load.
func TestLoadMalformedYAML(t *testing.T) {
	dir := withHome(t)
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte("::not valid yaml::\n\t- broken"), 0o644); err != nil {
		t.Fatalf("seed config: %v", err)
	}
	if _, err := Load(); err == nil {
		t.Error("Load should fail on malformed config.yaml")
	}
}
