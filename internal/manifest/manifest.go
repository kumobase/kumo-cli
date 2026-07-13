// Package manifest parses a declarative app spec file (app.yaml) into the
// kumo-go request types used to create or update an application.
package manifest

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"

	"github.com/kumobase/kumo-go/types"
)

// Manifest is the YAML shape of an app spec file. Field names mirror the SDK
// request JSON in lowerCamelCase for readability in YAML.
//
// The scalar fields are pointers so an absent key (nil) is distinguishable from
// a zero value. This matters for `apps update -f`, where only the fields the
// manifest actually set are applied — an omitted field must leave the live spec
// unchanged rather than overwrite it with "" / 0 / false.
type Manifest struct {
	Name                 *string               `yaml:"name"`
	Image                *string               `yaml:"image"`
	Port                 *uint16               `yaml:"port"`
	IsExposed            *bool                 `yaml:"isExposed"`
	Replicas             *int                  `yaml:"replicas"`
	PricingSlug          *string               `yaml:"pricingSlug"`
	RegistryCredential   string                `yaml:"registryCredential"`
	EnvironmentVariables []EnvVar              `yaml:"environmentVariables"`
	SecretVars           []SecretVarSpec       `yaml:"secretVars"`
	SecretFileMounts     []SecretFileMountSpec `yaml:"secretFileMounts"`
	TLSSecret            string                `yaml:"tlsSecret"`
	HealthCheck          *HealthCheck          `yaml:"healthCheck"`
	Autoscaling          *Autoscaling          `yaml:"autoscaling"`
}

func derefStr(p *string) string {
	if p != nil {
		return *p
	}
	return ""
}

func derefU16(p *uint16) uint16 {
	if p != nil {
		return *p
	}
	return 0
}

func derefBool(p *bool) bool {
	if p != nil {
		return *p
	}
	return false
}

func derefInt(p *int) int {
	if p != nil {
		return *p
	}
	return 0
}

// SecretVarSpec attaches a Kumo Secret as environment variables to the app.
type SecretVarSpec struct {
	SecretName         string `yaml:"secretName"`
	RestartWhenUpdated bool   `yaml:"restartWhenUpdated"`
}

// SecretFileMountSpec mounts a Kumo Secret as a file inside the app container.
type SecretFileMountSpec struct {
	SecretName         string `yaml:"secretName"`
	MountTo            string `yaml:"mountTo"`
	RestartWhenUpdated bool   `yaml:"restartWhenUpdated"`
}

// EnvVar is a single environment variable entry.
type EnvVar struct {
	Key   string `yaml:"key"`
	Value string `yaml:"value"`
}

// HealthCheck mirrors types.HealthCheck.
type HealthCheck struct {
	Type string `yaml:"type"`
	Path string `yaml:"path"`
	Port uint16 `yaml:"port"`
}

// Autoscaling mirrors types.AutoscalingConfig.
type Autoscaling struct {
	Enabled                bool `yaml:"enabled"`
	MinReplicas            int  `yaml:"minReplicas"`
	MaxReplicas            int  `yaml:"maxReplicas"`
	CPUTargetPercentage    *int `yaml:"cpuTargetPercentage"`
	MemoryTargetPercentage *int `yaml:"memoryTargetPercentage"`
}

// Load reads and parses a manifest file.
func Load(path string) (*Manifest, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read manifest %s: %w", path, err)
	}
	var m Manifest
	if err := yaml.Unmarshal(b, &m); err != nil {
		return nil, fmt.Errorf("parse manifest %s: %w", path, err)
	}
	return &m, nil
}

func (m *Manifest) base() types.BaseCreateApp {
	b := types.BaseCreateApp{
		Name:      derefStr(m.Name),
		Image:     derefStr(m.Image),
		Port:      derefU16(m.Port),
		IsExposed: derefBool(m.IsExposed),
		Replicas:  derefInt(m.Replicas),
	}
	if m.Autoscaling != nil {
		b.Autoscaling = &types.AutoscalingConfig{
			Enabled:                m.Autoscaling.Enabled,
			MinReplicas:            m.Autoscaling.MinReplicas,
			MaxReplicas:            m.Autoscaling.MaxReplicas,
			CPUTargetPercentage:    m.Autoscaling.CPUTargetPercentage,
			MemoryTargetPercentage: m.Autoscaling.MemoryTargetPercentage,
		}
	}
	return b
}

func (m *Manifest) envVars() []types.EnvironmentVariable {
	if len(m.EnvironmentVariables) == 0 {
		return nil
	}
	out := make([]types.EnvironmentVariable, 0, len(m.EnvironmentVariables))
	for _, e := range m.EnvironmentVariables {
		out = append(out, types.EnvironmentVariable{Key: e.Key, Value: e.Value})
	}
	return out
}

func (m *Manifest) secretVars() []types.SecretVar {
	if len(m.SecretVars) == 0 {
		return nil
	}
	out := make([]types.SecretVar, 0, len(m.SecretVars))
	for _, sv := range m.SecretVars {
		out = append(out, types.SecretVar{
			SecretName:         sv.SecretName,
			RestartWhenUpdated: sv.RestartWhenUpdated,
		})
	}
	return out
}

func (m *Manifest) secretFileMounts() []types.SecretFileMount {
	if len(m.SecretFileMounts) == 0 {
		return nil
	}
	out := make([]types.SecretFileMount, 0, len(m.SecretFileMounts))
	for _, sm := range m.SecretFileMounts {
		out = append(out, types.SecretFileMount{
			Type:               types.SecretFileMountTypeSecretFile,
			MountTo:            sm.MountTo,
			SecretName:         sm.SecretName,
			RestartWhenUpdated: sm.RestartWhenUpdated,
		})
	}
	return out
}

func (m *Manifest) healthCheck() *types.HealthCheck {
	if m.HealthCheck == nil {
		return nil
	}
	return &types.HealthCheck{
		Type: m.HealthCheck.Type,
		Path: m.HealthCheck.Path,
		Port: m.HealthCheck.Port,
	}
}

// ToCreateRequest converts the manifest into a CreateAppRequest.
func (m *Manifest) ToCreateRequest() *types.CreateAppRequest {
	return &types.CreateAppRequest{
		BaseCreateApp:          m.base(),
		EnvironmentVariables:   m.envVars(),
		PricingSlug:            derefStr(m.PricingSlug),
		RegistryCredentialName: m.RegistryCredential,
		TLSSecretName:          m.TLSSecret,
		SecretVars:             m.secretVars(),
		SecretFileMounts:       m.secretFileMounts(),
		HealthCheck:            m.healthCheck(),
	}
}

// ToUpdateRequest converts the manifest into a fresh UpdateAppRequest carrying
// only the fields the manifest actually set.
func (m *Manifest) ToUpdateRequest() *types.UpdateAppRequest {
	req := &types.UpdateAppRequest{}
	m.ApplyUpdate(req)
	return req
}

// ApplyUpdate merges the manifest onto an existing UpdateAppRequest, overwriting
// only the fields the manifest actually set (non-nil pointers, non-empty
// slices/strings). Fields the manifest omits are left untouched so a partial
// manifest preserves — rather than zeroes — the rest of the live spec.
func (m *Manifest) ApplyUpdate(req *types.UpdateAppRequest) {
	if m.Name != nil {
		req.Name = m.Name
	}
	if m.Image != nil {
		req.Image = m.Image
	}
	if m.Port != nil {
		req.Port = m.Port
	}
	if m.IsExposed != nil {
		req.IsExposed = m.IsExposed
	}
	if m.Replicas != nil {
		req.Replicas = m.Replicas
	}
	if m.PricingSlug != nil {
		req.PricingSlug = m.PricingSlug
	}
	if len(m.EnvironmentVariables) > 0 {
		req.EnvironmentVariables = m.envVars()
	}
	if len(m.SecretVars) > 0 {
		req.SecretVars = m.secretVars()
	}
	if len(m.SecretFileMounts) > 0 {
		req.SecretFileMounts = m.secretFileMounts()
	}
	if m.HealthCheck != nil {
		req.HealthCheck = m.healthCheck()
	}
	if m.Autoscaling != nil {
		req.Autoscaling = &types.AutoscalingConfig{
			Enabled:                m.Autoscaling.Enabled,
			MinReplicas:            m.Autoscaling.MinReplicas,
			MaxReplicas:            m.Autoscaling.MaxReplicas,
			CPUTargetPercentage:    m.Autoscaling.CPUTargetPercentage,
			MemoryTargetPercentage: m.Autoscaling.MemoryTargetPercentage,
		}
	}
	if m.RegistryCredential != "" {
		rc := m.RegistryCredential
		req.RegistryCredentialName = &rc
	}
	if m.TLSSecret != "" {
		ts := m.TLSSecret
		req.TLSSecretName = &ts
	}
}
