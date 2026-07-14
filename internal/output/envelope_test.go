package output

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"text/tabwriter"

	"github.com/kumobase/kumo-go/client"
	"github.com/kumobase/kumo-go/types"
)

func decodeEnv(t *testing.T, s string) Envelope {
	t.Helper()
	var e Envelope
	if err := json.Unmarshal([]byte(s), &e); err != nil {
		t.Fatalf("decode envelope: %v (%s)", err, s)
	}
	return e
}

func TestPrintWrapsJSONInEnvelope(t *testing.T) {
	var buf bytes.Buffer
	if err := Print(&buf, FormatJSON, map[string]any{"id": 7}, nil); err != nil {
		t.Fatalf("Print: %v", err)
	}
	env := decodeEnv(t, buf.String())
	if !env.OK || env.Error != nil {
		t.Errorf("expected ok:true, no error: %s", buf.String())
	}
}

func TestPrintResultJSONAndTable(t *testing.T) {
	r := ActionResult{Resource: "app", ID: 42, Action: "delete", Status: "done", Message: "App 42 deleted"}

	var js bytes.Buffer
	if err := PrintResult(&js, FormatJSON, false, r); err != nil {
		t.Fatalf("PrintResult json: %v", err)
	}
	env := decodeEnv(t, js.String())
	if !env.OK {
		t.Errorf("json ok should be true: %s", js.String())
	}
	data, _ := env.Data.(map[string]any)
	if data["resource"] != "app" || data["action"] != "delete" || data["status"] != "done" {
		t.Errorf("unexpected data: %v", env.Data)
	}
	if strings.Contains(js.String(), "App 42 deleted") {
		t.Errorf("human Message must not be serialized: %s", js.String())
	}

	var tbl bytes.Buffer
	if err := PrintResult(&tbl, FormatTable, false, r); err != nil {
		t.Fatalf("PrintResult table: %v", err)
	}
	if strings.TrimSpace(tbl.String()) != "App 42 deleted" {
		t.Errorf("table line = %q", tbl.String())
	}
}

func TestPrintResultQuiet(t *testing.T) {
	r := ActionResult{Resource: "app", Action: "stop", Status: "done", Message: "App 1 stopped"}

	var tbl bytes.Buffer
	if err := PrintResult(&tbl, FormatTable, true, r); err != nil {
		t.Fatalf("PrintResult quiet table: %v", err)
	}
	if tbl.String() != "" {
		t.Errorf("quiet table should be empty, got %q", tbl.String())
	}

	// quiet must NOT suppress JSON.
	var js bytes.Buffer
	if err := PrintResult(&js, FormatJSON, true, r); err != nil {
		t.Fatalf("PrintResult quiet json: %v", err)
	}
	if !decodeEnv(t, js.String()).OK {
		t.Errorf("quiet json should still emit envelope: %s", js.String())
	}
}

func TestPrintListMetaInBothFormats(t *testing.T) {
	items := []map[string]any{{"id": 1}}
	meta := &types.Meta{Page: 1, PageSize: 20, TotalItems: 40, TotalPages: 2}

	var js bytes.Buffer
	if err := PrintList(&js, FormatJSON, items, meta, nil); err != nil {
		t.Fatalf("PrintList json: %v", err)
	}
	env := decodeEnv(t, js.String())
	data, _ := env.Data.(map[string]any)
	if _, ok := data["items"]; !ok {
		t.Errorf("json list missing items: %s", js.String())
	}
	if _, ok := data["meta"]; !ok {
		t.Errorf("json list missing meta: %s", js.String())
	}

	var tbl bytes.Buffer
	err := PrintList(&tbl, FormatTable, items, meta, func(tw *tabwriter.Writer) {
		fmt.Fprintln(tw, "ID")
		fmt.Fprintln(tw, "1")
	})
	if err != nil {
		t.Fatalf("PrintList table: %v", err)
	}
	if !strings.Contains(tbl.String(), "Page 1/2") {
		t.Errorf("table should show page footer: %q", tbl.String())
	}
}

func TestPrintListNoFooterSinglePage(t *testing.T) {
	var tbl bytes.Buffer
	meta := &types.Meta{Page: 1, TotalPages: 1, TotalItems: 3}
	_ = PrintList(&tbl, FormatTable, []int{1}, meta, func(tw *tabwriter.Writer) {
		fmt.Fprintln(tw, "x")
	})
	if strings.Contains(tbl.String(), "Page ") {
		t.Errorf("single page should have no footer: %q", tbl.String())
	}
}

func TestErrorView(t *testing.T) {
	api := &client.APIError{StatusCode: 409, Code: "NAME_TAKEN", Message: "taken"}
	v := ErrorView(api)
	if v.Code != "NAME_TAKEN" || v.HTTPStatus != 409 || v.Message != "taken" {
		t.Errorf("unexpected view: %+v", v)
	}
	// wrapped API error still unwraps
	wrapped := fmt.Errorf("context: %w", api)
	if ErrorView(wrapped).Code != "NAME_TAKEN" {
		t.Errorf("wrapped view lost code: %+v", ErrorView(wrapped))
	}
	// plain error
	if got := ErrorView(errString("boom")); got.Code != "" || got.Message != "boom" {
		t.Errorf("plain view = %+v", got)
	}
}

func TestPrintErrorFormats(t *testing.T) {
	api := &client.APIError{StatusCode: 404, Code: "APP_NOT_FOUND", Message: "no app"}

	var js bytes.Buffer
	PrintError(&js, FormatJSON, api)
	env := decodeEnv(t, js.String())
	if env.OK || env.Error == nil || env.Error.Code != "APP_NOT_FOUND" {
		t.Errorf("json error envelope wrong: %s", js.String())
	}

	var tbl bytes.Buffer
	PrintError(&tbl, FormatTable, api)
	if !strings.HasPrefix(tbl.String(), "Error: ") {
		t.Errorf("table error should start with 'Error: ': %q", tbl.String())
	}
}

func TestPrintAborted(t *testing.T) {
	var js bytes.Buffer
	if err := PrintAborted(&js, FormatJSON); err != nil {
		t.Fatalf("PrintAborted json: %v", err)
	}
	env := decodeEnv(t, js.String())
	if env.OK || env.Error == nil || env.Error.Code != "ABORTED" {
		t.Errorf("aborted json wrong: %s", js.String())
	}

	var tbl bytes.Buffer
	_ = PrintAborted(&tbl, FormatTable)
	if strings.TrimSpace(tbl.String()) != "Aborted." {
		t.Errorf("aborted table = %q", tbl.String())
	}
}

func TestFormatErrorPreservesFriendlyContext(t *testing.T) {
	api := &client.APIError{StatusCode: 404, Code: "APP_NOT_FOUND", Message: "not found"}
	// A friendly wrapper whose Error() is just the friendly text.
	friendly := &wrapErrForTest{msg: `no app named "web"`, cause: api}
	if got := FormatError(friendly); got != `no app named "web"` {
		t.Errorf("FormatError should show friendly text, got %q", got)
	}
	// A bare API error still shows the compact message (code) form.
	if got := FormatError(api); got != "not found (APP_NOT_FOUND)" {
		t.Errorf("FormatError bare = %q", got)
	}
}

type wrapErrForTest struct {
	msg   string
	cause error
}

func (e *wrapErrForTest) Error() string { return e.msg }
func (e *wrapErrForTest) Unwrap() error { return e.cause }
