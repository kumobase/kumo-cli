package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"text/tabwriter"
	"time"

	"github.com/kumobase/kumo-go/types"
)

func newTabWriter(buf *bytes.Buffer) *tabwriter.Writer {
	return tabwriter.NewWriter(buf, 0, 4, 2, ' ', 0)
}

func TestBuildSSHArgs(t *testing.T) {
	// Default port 22 and no identity → just user@ip plus extras.
	got := buildSSHArgs(22, "", "root", "1.2.3.4", []string{"uptime"})
	if !reflect.DeepEqual(got, []string{"root@1.2.3.4", "uptime"}) {
		t.Errorf("default = %v", got)
	}
	// Custom port + identity prepend flags.
	got = buildSSHArgs(2222, "/key.pem", "admin", "5.6.7.8", nil)
	want := []string{"-p", "2222", "-i", "/key.pem", "admin@5.6.7.8"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("custom = %v, want %v", got, want)
	}
	// Port 0 is treated as unset.
	got = buildSSHArgs(0, "", "u", "h", nil)
	if !reflect.DeepEqual(got, []string{"u@h"}) {
		t.Errorf("port 0 = %v", got)
	}
}

func TestCertificateContent(t *testing.T) {
	p := &secretPayloadFlags{}
	if _, err := certificateContent(p); err == nil {
		t.Error("missing files should error")
	}

	dir := t.TempDir()
	certPath := filepath.Join(dir, "cert.pem")
	keyPath := filepath.Join(dir, "key.pem")
	if err := os.WriteFile(certPath, []byte("CERTDATA"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keyPath, []byte("KEYDATA"), 0o600); err != nil {
		t.Fatal(err)
	}

	// Missing key file → read error.
	if _, err := certificateContent(&secretPayloadFlags{certFile: certPath, keyFile: filepath.Join(dir, "nope")}); err == nil {
		t.Error("unreadable key should error")
	}

	got, err := certificateContent(&secretPayloadFlags{certFile: certPath, keyFile: keyPath})
	if err != nil {
		t.Fatalf("certificateContent: %v", err)
	}
	if got.Certificate != "CERTDATA" || got.PrivateKey != "KEYDATA" {
		t.Errorf("content = %+v", got)
	}
}

func TestParseJobSecretFlags(t *testing.T) {
	refs, err := parseJobSecretFlags([]string{"DB:PASSWORD:DB_PASS"}, []string{"TLS:/etc/tls"})
	if err != nil {
		t.Fatalf("parseJobSecretFlags: %v", err)
	}
	if len(refs) != 2 {
		t.Fatalf("got %d refs, want 2", len(refs))
	}
	if refs[0].SecretName != "DB" || refs[0].SourceFrom != "PASSWORD" || refs[0].MountTo != "DB_PASS" {
		t.Errorf("env ref = %+v", refs[0])
	}
	if refs[1].SecretName != "TLS" || refs[1].MountTo != "/etc/tls" {
		t.Errorf("mount ref = %+v", refs[1])
	}

	for _, bad := range []string{"DB:PASSWORD", "DB::ENV", ":KEY:ENV"} {
		if _, err := parseJobSecretFlags([]string{bad}, nil); err == nil {
			t.Errorf("--secret-env %q should be rejected", bad)
		}
	}
	for _, bad := range []string{"TLS", "TLS:relative/path", ":/abs"} {
		if _, err := parseJobSecretFlags(nil, []string{bad}); err == nil {
			t.Errorf("--secret-mount %q should be rejected", bad)
		}
	}
}

func TestPrintJobExecutionDetail(t *testing.T) {
	var buf bytes.Buffer
	tw := newTabWriter(&buf)
	code := 0
	dur := int64(2500)
	started := time.Date(2026, 7, 13, 10, 0, 0, 0, time.UTC)
	finished := started.Add(2500 * time.Millisecond)
	billed := "$0.01"
	printJobExecutionDetail(tw, &types.JobExecution{
		ID: 5, JobID: 2, Trigger: "manual", Status: "succeeded",
		ExitCode: &code, DurationMS: &dur,
		PodStartedAt: &started, PodFinishedAt: &finished,
		CPUvCPU: "0.5", MemoryMB: 256, BilledAmount: &billed,
		CreatedAt: started,
	})
	tw.Flush()
	out := buf.String()
	for _, want := range []string{"ID:", "Exit code:", "Duration:", "CPU:", "Memory:", "Billed:", "0.5 vCPU", "256 MB", "$0.01"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}

func TestPrintRegistryRepoDetail(t *testing.T) {
	var buf bytes.Buffer
	tw := newTabWriter(&buf)
	now := time.Date(2026, 7, 13, 0, 0, 0, 0, time.UTC)
	printRegistryRepoDetail(tw, "acme", &types.RepositoryResponse{
		ID: 9, Name: "web", TagMutability: "immutable", CreatedAt: now, UpdatedAt: now,
	})
	tw.Flush()
	out := buf.String()
	for _, want := range []string{"web", "acme", "immutable"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}

func TestPrintJobDetailScheduled(t *testing.T) {
	var buf bytes.Buffer
	tw := newTabWriter(&buf)
	app := "api"
	last := time.Date(2026, 7, 13, 9, 0, 0, 0, time.UTC)
	printJobDetail(tw, &types.JobResponse{
		ID: 1, Name: "nightly", Kind: types.JobKindAppAttached, AppName: &app,
		Image: "img:1", Command: []string{"run"}, Args: []string{"--flag"},
		Schedule: "0 0 * * *", Timezone: "UTC", ConcurrencyPolicy: types.JobConcurrencyForbid,
		LastExecutionAt: &last, NextRunTimes: []time.Time{last.Add(24 * time.Hour)},
	})
	tw.Flush()
	out := buf.String()
	for _, want := range []string{"nightly", "App:", "Image:", "Command:", "Args:", "Timezone:", "Concurrency:", "Next run:"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}
