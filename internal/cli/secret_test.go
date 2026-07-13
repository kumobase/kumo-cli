package cli

import (
	"slices"
	"testing"

	"github.com/kumobase/kumo-go/types"
)

func TestParseSecretVarFlags(t *testing.T) {
	sv, err := parseSecretVarFlags([]string{"db-creds", "tokens:restart"})
	if err != nil {
		t.Fatalf("parseSecretVarFlags: %v", err)
	}
	if len(sv) != 2 {
		t.Fatalf("expected 2 secret vars, got %+v", sv)
	}
	if sv[0].SecretName != "db-creds" || sv[0].RestartWhenUpdated {
		t.Errorf("first secret var wrong: %+v", sv[0])
	}
	if sv[1].SecretName != "tokens" || !sv[1].RestartWhenUpdated {
		t.Errorf("second secret var wrong: %+v", sv[1])
	}

	for _, bad := range []string{"", "has/slash", "spaced name:restart", "name:nope"} {
		if _, err := parseSecretVarFlags([]string{bad}); err == nil {
			t.Errorf("expected error for --secret-var %q", bad)
		}
	}
}

func TestParseSecretFileMountFlags(t *testing.T) {
	sm, err := parseSecretFileMountFlags([]string{"tls-cert:/etc/tls", "tokens:/data/x:restart"})
	if err != nil {
		t.Fatalf("parseSecretFileMountFlags: %v", err)
	}
	if len(sm) != 2 {
		t.Fatalf("expected 2 mounts, got %+v", sm)
	}
	if sm[0].SecretName != "tls-cert" || sm[0].MountTo != "/etc/tls" || sm[0].RestartWhenUpdated {
		t.Errorf("first mount wrong: %+v", sm[0])
	}
	if sm[0].Type != types.SecretFileMountTypeSecretFile {
		t.Errorf("mount type wrong: %q", sm[0].Type)
	}
	if sm[1].SecretName != "tokens" || sm[1].MountTo != "/data/x" || !sm[1].RestartWhenUpdated {
		t.Errorf("second mount wrong: %+v", sm[1])
	}

	for _, bad := range []string{"name-only", ":/etc", "name:relative/path", "name:/etc:nope", ""} {
		if _, err := parseSecretFileMountFlags([]string{bad}); err == nil {
			t.Errorf("expected error for --secret-file-mount %q", bad)
		}
	}
}

func TestMaskValue(t *testing.T) {
	got := maskValue("super-secret-password")
	if got == "super-secret-password" {
		t.Error("maskValue must not echo the input")
	}
	if got != maskValue("x") {
		t.Error("maskValue must not leak input length")
	}
}

func TestRevealOrMask(t *testing.T) {
	if got := revealOrMask("", false); got != "(empty)" {
		t.Errorf("empty value = %q", got)
	}
	if got := revealOrMask("secret", true); got != "secret" {
		t.Errorf("reveal should show plaintext, got %q", got)
	}
	if got := revealOrMask("secret", false); got == "secret" {
		t.Error("masked value must not equal plaintext")
	}
}

func TestEnabledSecretTypes(t *testing.T) {
	ts := enabledSecretTypes()
	for _, want := range []types.SecretType{types.SecretTypeRegistry, types.SecretTypeEnvVar, types.SecretTypeFile} {
		if !slices.Contains(ts, want) {
			t.Errorf("expected %q to be enabled", want)
		}
	}
	if certificateSecretsEnabled {
		t.Skip("certificate gate is on; skipping the gated-off assertion")
	}
	if slices.Contains(ts, types.SecretTypeCertificate) {
		t.Error("certificate type must be hidden while the gate is off")
	}
	if isEnabledSecretType(types.SecretTypeCertificate) {
		t.Error("isEnabledSecretType(certificate) must be false while gated off")
	}
}

func TestBuildSecretPayloadValidation(t *testing.T) {
	cases := []struct {
		name    string
		typ     types.SecretType
		flags   secretPayloadFlags
		wantErr bool
	}{
		{"registry ok", types.SecretTypeRegistry, secretPayloadFlags{registryUser: "u", registryPass: "p"}, false},
		{"registry missing pass", types.SecretTypeRegistry, secretPayloadFlags{registryUser: "u"}, true},
		{"env ok", types.SecretTypeEnvVar, secretPayloadFlags{envs: []string{"K=V"}}, false},
		{"env empty", types.SecretTypeEnvVar, secretPayloadFlags{}, true},
		{"file content ok", types.SecretTypeFile, secretPayloadFlags{content: "hi"}, false},
		{"file missing", types.SecretTypeFile, secretPayloadFlags{}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := &types.CreateSecretRequest{
				RequestSecretBase: types.RequestSecretBase{Name: "n", Type: tc.typ},
			}
			err := buildSecretPayload(req, tc.typ, &tc.flags)
			if tc.wantErr != (err != nil) {
				t.Errorf("buildSecretPayload err = %v, wantErr %t", err, tc.wantErr)
			}
		})
	}
}

func TestCollectAppSecretRefs(t *testing.T) {
	refs := collectAppSecretRefs(
		"my-registry", "tls-cert",
		[]types.SecretVar{{SecretName: "db-creds"}, {SecretName: "tokens"}},
		[]types.SecretFileMount{{SecretName: "tls-bundle"}},
	)
	want := []appSecretRef{
		{name: "my-registry", flag: "--registry-credential", expected: types.SecretTypeRegistry},
		{name: "tls-cert", flag: "--tls-secret", expected: types.SecretTypeCertificate},
		{name: "db-creds", flag: "--secret-var", expected: types.SecretTypeEnvVar},
		{name: "tokens", flag: "--secret-var", expected: types.SecretTypeEnvVar},
		{name: "tls-bundle", flag: "--secret-file-mount", expected: types.SecretTypeFile},
	}
	if !slices.Equal(refs, want) {
		t.Errorf("collectAppSecretRefs = %+v, want %+v", refs, want)
	}

	// Empty names are "none" and must be skipped.
	if got := collectAppSecretRefs("", "", nil, nil); len(got) != 0 {
		t.Errorf("expected no refs for empty inputs, got %+v", got)
	}
}

func TestSecretSubcommands(t *testing.T) {
	root := NewRootCmd()
	for _, c := range root.Commands() {
		if c.Name() != "secret" {
			continue
		}
		got := map[string]bool{}
		for _, sub := range c.Commands() {
			got[sub.Name()] = true
		}
		for _, name := range []string{"list", "get", "create", "update", "delete"} {
			if !got[name] {
				t.Errorf("secret missing subcommand %q", name)
			}
		}
		return
	}
	t.Fatal("secret command not found")
}
