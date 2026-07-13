// Package config loads and persists kumo-cli settings using the AWS/gcloud
// convention: a dedicated home directory (~/.kumo) holding non-secret config
// (config.yaml) and secret credentials (credentials.yaml, mode 0600) in
// separate files, with support for multiple named profiles.
package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Defaults applied when nothing else provides a value.
const (
	DefaultBaseURL = "https://api.kumo.run"
	DefaultProfile = "default"
	DefaultOutput  = "table"
)

// CredentialTypeAPIKey is the only credential type today. The Type field is a
// seam for future auth methods (e.g. browser/OAuth login).
const CredentialTypeAPIKey = "api_key"

// Environment variables consulted during resolution.
const (
	EnvHome    = "KUMO_HOME"
	EnvProfile = "KUMO_PROFILE"
	EnvAPIKey  = "KUMO_API_KEY"
	EnvBaseURL = "KUMO_BASE_URL"
	EnvOutput  = "KUMO_OUTPUT"
)

// Profile holds non-secret per-profile settings (stored in config.yaml).
type Profile struct {
	BaseURL string `yaml:"base_url,omitempty"`
	Output  string `yaml:"output,omitempty"`
}

// Credential holds secret per-profile auth material (stored in
// credentials.yaml, mode 0600).
type Credential struct {
	Type   string `yaml:"type"`
	APIKey string `yaml:"api_key,omitempty"`
}

type configFile struct {
	CurrentProfile string             `yaml:"current_profile,omitempty"`
	Profiles       map[string]Profile `yaml:"profiles,omitempty"`
}

type credentialsFile struct {
	Profiles map[string]Credential `yaml:"profiles,omitempty"`
}

// Store is the in-memory view of the on-disk config + credentials files.
type Store struct {
	home   string
	config configFile
	creds  credentialsFile
}

// Home returns the kumo-cli home directory: $KUMO_HOME if set, else ~/.kumo.
func Home() (string, error) {
	if h := os.Getenv(EnvHome); h != "" {
		return h, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cannot determine home directory: %w", err)
	}
	return filepath.Join(home, ".kumo"), nil
}

func (s *Store) configPath() string      { return filepath.Join(s.home, "config.yaml") }
func (s *Store) credentialsPath() string { return filepath.Join(s.home, "credentials.yaml") }

// Load reads config.yaml and credentials.yaml from the home directory. Missing
// files are treated as empty (first run), not an error.
func Load() (*Store, error) {
	home, err := Home()
	if err != nil {
		return nil, err
	}
	s := &Store{home: home}
	if err := readYAML(s.configPath(), &s.config); err != nil {
		return nil, err
	}
	if err := readYAML(s.credentialsPath(), &s.creds); err != nil {
		return nil, err
	}
	if s.config.Profiles == nil {
		s.config.Profiles = map[string]Profile{}
	}
	if s.creds.Profiles == nil {
		s.creds.Profiles = map[string]Credential{}
	}
	return s, nil
}

func readYAML(path string, out any) error {
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read %s: %w", path, err)
	}
	if err := yaml.Unmarshal(b, out); err != nil {
		return fmt.Errorf("parse %s: %w", path, err)
	}
	return nil
}

// Save writes both files back to the home directory, creating it if needed.
// credentials.yaml is written with mode 0600 so it can be locked down
// independently of the non-secret config.
func (s *Store) Save() error {
	if err := os.MkdirAll(s.home, 0o700); err != nil {
		return fmt.Errorf("create %s: %w", s.home, err)
	}
	cfgBytes, err := yaml.Marshal(s.config)
	if err != nil {
		return fmt.Errorf("encode config: %w", err)
	}
	if err := os.WriteFile(s.configPath(), cfgBytes, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", s.configPath(), err)
	}
	credBytes, err := yaml.Marshal(s.creds)
	if err != nil {
		return fmt.Errorf("encode credentials: %w", err)
	}
	if err := os.WriteFile(s.credentialsPath(), credBytes, 0o600); err != nil {
		return fmt.Errorf("write %s: %w", s.credentialsPath(), err)
	}
	return nil
}

// CurrentProfile returns the profile marked active in config.yaml, or
// DefaultProfile if none is set.
func (s *Store) CurrentProfile() string {
	if s.config.CurrentProfile != "" {
		return s.config.CurrentProfile
	}
	return DefaultProfile
}

// SetCurrentProfile records the active profile name.
func (s *Store) SetCurrentProfile(name string) { s.config.CurrentProfile = name }

// GetProfile returns the non-secret settings for a profile.
func (s *Store) GetProfile(name string) (Profile, bool) {
	p, ok := s.config.Profiles[name]
	return p, ok
}

// SetProfile stores non-secret settings for a profile.
func (s *Store) SetProfile(name string, p Profile) {
	if s.config.Profiles == nil {
		s.config.Profiles = map[string]Profile{}
	}
	s.config.Profiles[name] = p
}

// Credential returns the stored credential for a profile.
func (s *Store) Credential(name string) (Credential, bool) {
	c, ok := s.creds.Profiles[name]
	return c, ok
}

// SetCredential stores a credential for a profile.
func (s *Store) SetCredential(name string, c Credential) {
	if s.creds.Profiles == nil {
		s.creds.Profiles = map[string]Credential{}
	}
	s.creds.Profiles[name] = c
}

// RemoveCredential deletes a profile's stored credential.
func (s *Store) RemoveCredential(name string) { delete(s.creds.Profiles, name) }

// Overrides carries command-line flag values. Empty strings mean "not set".
type Overrides struct {
	Profile string
	BaseURL string
	Output  string
}

// Settings is the fully-resolved configuration for one command invocation.
type Settings struct {
	Profile string
	BaseURL string
	Output  string
	APIKey  string
}

// Resolve merges flags, environment variables, the selected profile, and
// built-in defaults (in that precedence order) into Settings.
func (s *Store) Resolve(o Overrides) Settings {
	profile := firstNonEmpty(o.Profile, os.Getenv(EnvProfile), s.CurrentProfile())
	p := s.config.Profiles[profile]
	cred := s.creds.Profiles[profile]

	return Settings{
		Profile: profile,
		BaseURL: firstNonEmpty(o.BaseURL, os.Getenv(EnvBaseURL), p.BaseURL, DefaultBaseURL),
		Output:  firstNonEmpty(o.Output, os.Getenv(EnvOutput), p.Output, DefaultOutput),
		APIKey:  firstNonEmpty(os.Getenv(EnvAPIKey), cred.APIKey),
	}
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
