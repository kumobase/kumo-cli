package cli

import (
	"testing"
)

func TestNewRootCmdStructure(t *testing.T) {
	root := NewRootCmd()
	if root.Use != "kumo" {
		t.Errorf("root Use = %q", root.Use)
	}
	want := map[string]bool{"apps": false, "auth": false, "secret": false}
	for _, c := range root.Commands() {
		if _, ok := want[c.Name()]; ok {
			want[c.Name()] = true
		}
	}
	for name, found := range want {
		if !found {
			t.Errorf("missing subcommand %q", name)
		}
	}
}

func TestAppsSubcommands(t *testing.T) {
	root := NewRootCmd()
	for _, c := range root.Commands() {
		if c.Name() != "apps" {
			continue
		}
		got := map[string]bool{}
		for _, sub := range c.Commands() {
			got[sub.Name()] = true
		}
		for _, name := range []string{"list", "get", "create", "update", "delete", "start", "stop", "operations", "domain"} {
			if !got[name] {
				t.Errorf("apps missing subcommand %q", name)
			}
		}
		return
	}
	t.Fatal("apps command not found")
}

func TestParseEnvFlags(t *testing.T) {
	ev, err := parseEnvFlags([]string{"FOO=bar", "EMPTY="})
	if err != nil {
		t.Fatalf("parseEnvFlags: %v", err)
	}
	if len(ev) != 2 || ev[0].Key != "FOO" || ev[0].Value != "bar" || ev[1].Key != "EMPTY" || ev[1].Value != "" {
		t.Errorf("unexpected env vars: %+v", ev)
	}
	if _, err := parseEnvFlags([]string{"noequals"}); err == nil {
		t.Error("expected error for missing =")
	}
	if _, err := parseEnvFlags([]string{"=novalue"}); err == nil {
		t.Error("expected error for empty key")
	}
}

func TestMaskKey(t *testing.T) {
	if got := maskKey(""); got != "(none)" {
		t.Errorf("maskKey(empty) = %q", got)
	}
	if got := maskKey("kumo_sk_1234567890"); got != "kumo_sk_…7890" {
		t.Errorf("maskKey = %q", got)
	}
}
