package manifest

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kumobase/kumo-go/types"
)

// TestApplyUpdatePartialPreservesSeed is the regression guard for the
// wholesale-replace data-loss bug: a partial manifest must overwrite only the
// fields it set and leave the seeded live-spec fields untouched.
func TestApplyUpdatePartialPreservesSeed(t *testing.T) {
	path := writeManifest(t, "image: nginx:2\n")
	m, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	name, replicas, port, exposed := "existing", 3, uint16(8080), true
	req := &types.UpdateAppRequest{Name: &name, Replicas: &replicas, Port: &port, IsExposed: &exposed}
	m.ApplyUpdate(req)

	if req.Image == nil || *req.Image != "nginx:2" {
		t.Errorf("image should be applied from manifest: %v", req.Image)
	}
	if req.Name == nil || *req.Name != "existing" {
		t.Errorf("name should be preserved, got %v", req.Name)
	}
	if req.Replicas == nil || *req.Replicas != 3 {
		t.Errorf("replicas should be preserved (not zeroed), got %v", req.Replicas)
	}
	if req.Port == nil || *req.Port != 8080 {
		t.Errorf("port should be preserved, got %v", req.Port)
	}
	if req.IsExposed == nil || !*req.IsExposed {
		t.Errorf("isExposed should be preserved, got %v", req.IsExposed)
	}
}

// TestApplyUpdateExplicitZeroHonored confirms an explicitly-set zero is applied
// (distinct from an omitted field).
func TestApplyUpdateExplicitZeroHonored(t *testing.T) {
	path := writeManifest(t, "replicas: 0\n")
	m, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	req := &types.UpdateAppRequest{}
	m.ApplyUpdate(req)
	if req.Replicas == nil || *req.Replicas != 0 {
		t.Errorf("explicit replicas:0 should be applied, got %v", req.Replicas)
	}
}

func writeManifest(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "app.yaml")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	return path
}

func TestLoadAndConvert(t *testing.T) {
	path := writeManifest(t, `
name: demo
image: nginx:1.27
port: 8080
isExposed: true
replicas: 3
pricingSlug: app-small
registryCredential: my-registry
environmentVariables:
  - key: FOO
    value: bar
healthCheck:
  type: http
  path: /healthz
  port: 8080
autoscaling:
  enabled: true
  minReplicas: 2
  maxReplicas: 6
  cpuTargetPercentage: 75
`)

	m, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	req := m.ToCreateRequest()
	if req.Name != "demo" || req.Image != "nginx:1.27" {
		t.Errorf("base fields wrong: %+v", req.BaseCreateApp)
	}
	if req.Port != 8080 || req.Replicas != 3 || !req.IsExposed {
		t.Errorf("scalar fields wrong: %+v", req.BaseCreateApp)
	}
	if req.PricingSlug != "app-small" || req.RegistryCredentialName != "my-registry" {
		t.Errorf("metadata fields wrong: %+v", req)
	}
	if len(req.EnvironmentVariables) != 1 || req.EnvironmentVariables[0].Key != "FOO" {
		t.Errorf("env vars wrong: %+v", req.EnvironmentVariables)
	}
	if req.HealthCheck == nil || req.HealthCheck.Type != "http" || req.HealthCheck.Path != "/healthz" {
		t.Errorf("healthcheck wrong: %+v", req.HealthCheck)
	}
	if req.BaseCreateApp.Autoscaling == nil || !req.BaseCreateApp.Autoscaling.Enabled {
		t.Fatalf("autoscaling missing: %+v", req.BaseCreateApp.Autoscaling)
	}
	as := req.BaseCreateApp.Autoscaling
	if as.MinReplicas != 2 || as.MaxReplicas != 6 {
		t.Errorf("autoscaling replicas wrong: %+v", as)
	}
	if as.CPUTargetPercentage == nil || *as.CPUTargetPercentage != 75 {
		t.Errorf("cpu target wrong: %+v", as.CPUTargetPercentage)
	}

	// Update request should carry the same shared fields.
	upd := m.ToUpdateRequest()
	if upd.Name == nil || *upd.Name != "demo" {
		t.Errorf("update request name wrong: %+v", upd.Name)
	}
	if upd.RegistryCredentialName == nil || *upd.RegistryCredentialName != "my-registry" {
		t.Errorf("update request registry-credential wrong: %+v", upd.RegistryCredentialName)
	}
}

func TestLoadSecretAttachments(t *testing.T) {
	path := writeManifest(t, `
name: demo
image: nginx:1.27
port: 80
replicas: 1
secretVars:
  - secretName: db-creds
    restartWhenUpdated: true
secretFileMounts:
  - secretName: tls-bundle
    mountTo: /etc/tls/cert.pem
`)
	m, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	req := m.ToCreateRequest()
	if len(req.SecretVars) != 1 || req.SecretVars[0].SecretName != "db-creds" || !req.SecretVars[0].RestartWhenUpdated {
		t.Errorf("secret vars wrong: %+v", req.SecretVars)
	}
	if len(req.SecretFileMounts) != 1 {
		t.Fatalf("secret file mounts wrong: %+v", req.SecretFileMounts)
	}
	fm := req.SecretFileMounts[0]
	if fm.SecretName != "tls-bundle" || fm.MountTo != "/etc/tls/cert.pem" || string(fm.Type) != "secret_file" {
		t.Errorf("secret file mount fields wrong: %+v", fm)
	}

	upd := m.ToUpdateRequest()
	if len(upd.SecretVars) != 1 || len(upd.SecretFileMounts) != 1 {
		t.Errorf("update request dropped secret attachments: %+v", upd)
	}
}

func TestLoadMissingFile(t *testing.T) {
	if _, err := Load(filepath.Join(t.TempDir(), "nope.yaml")); err == nil {
		t.Fatal("expected error for missing manifest")
	}
}

func TestLoadOmitsEmptyOptional(t *testing.T) {
	path := writeManifest(t, "name: demo\nimage: nginx\nport: 80\nreplicas: 1\n")
	m, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	req := m.ToCreateRequest()
	if req.EnvironmentVariables != nil {
		t.Errorf("expected nil env vars, got %+v", req.EnvironmentVariables)
	}
	if req.HealthCheck != nil {
		t.Errorf("expected nil healthcheck, got %+v", req.HealthCheck)
	}
	if req.BaseCreateApp.Autoscaling != nil {
		t.Errorf("expected nil autoscaling, got %+v", req.BaseCreateApp.Autoscaling)
	}
}
