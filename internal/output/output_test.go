package output

import (
	"bytes"
	"strings"
	"testing"
	"text/tabwriter"

	"github.com/kumobase/kumo-go/client"
)

func TestPrintJSON(t *testing.T) {
	var buf bytes.Buffer
	data := map[string]any{"name": "demo", "id": 7}
	if err := Print(&buf, FormatJSON, data, nil); err != nil {
		t.Fatalf("Print json: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, `"name": "demo"`) || !strings.Contains(out, `"id": 7`) {
		t.Errorf("unexpected json: %s", out)
	}
}

func TestPrintTable(t *testing.T) {
	var buf bytes.Buffer
	err := Print(&buf, FormatTable, nil, func(tw *tabwriter.Writer) {
		tw.Write([]byte("A\tB\n"))
		tw.Write([]byte("1\t2\n"))
	})
	if err != nil {
		t.Fatalf("Print table: %v", err)
	}
	if !strings.Contains(buf.String(), "A") || !strings.Contains(buf.String(), "1") {
		t.Errorf("unexpected table: %q", buf.String())
	}
}

func TestPrintUnknownFormat(t *testing.T) {
	if err := Print(&bytes.Buffer{}, "xml", nil, nil); err == nil {
		t.Fatal("expected error for unknown format")
	}
}

func TestValid(t *testing.T) {
	if !Valid(FormatTable) || !Valid(FormatJSON) {
		t.Error("table/json should be valid")
	}
	if Valid("yaml") {
		t.Error("yaml should not be valid")
	}
}

func TestFormatErrorAPIError(t *testing.T) {
	err := &client.APIError{StatusCode: 404, Code: "APP_NOT_FOUND", Message: "app not found"}
	if got := FormatError(err); got != "app not found (APP_NOT_FOUND)" {
		t.Errorf("FormatError = %q", got)
	}
}

func TestFormatErrorPlain(t *testing.T) {
	if got := FormatError(errString("boom")); got != "boom" {
		t.Errorf("FormatError = %q", got)
	}
}

type errString string

func (e errString) Error() string { return string(e) }
