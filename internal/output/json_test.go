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

func TestPrintBareJSON(t *testing.T) {
	var buf bytes.Buffer
	if err := Print(&buf, FormatJSON, map[string]any{"id": 7, "name": "web"}, nil); err != nil {
		t.Fatalf("Print: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("not bare json: %v (%s)", err, buf.String())
	}
	if got["name"] != "web" {
		t.Errorf("bare object should have name at top level: %s", buf.String())
	}
	if _, wrapped := got["data"]; wrapped {
		t.Errorf("success output must not be wrapped: %s", buf.String())
	}
}

func TestPrintResultBareAndTable(t *testing.T) {
	r := ActionResult{Resource: "app", ID: 42, Action: "delete", Status: "done", Message: "App 42 deleted"}

	var js bytes.Buffer
	if err := PrintResult(&js, FormatJSON, false, r); err != nil {
		t.Fatalf("PrintResult json: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(js.Bytes(), &got); err != nil {
		t.Fatalf("not bare json: %v (%s)", err, js.String())
	}
	if got["resource"] != "app" || got["action"] != "delete" || got["status"] != "done" {
		t.Errorf("unexpected action result: %s", js.String())
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
	if strings.TrimSpace(js.String()) == "" {
		t.Errorf("quiet json should still emit the bare result")
	}
}

func TestPrintListBareArrayWithTableFooter(t *testing.T) {
	items := []map[string]any{{"id": 1}}
	meta := &types.Meta{Page: 1, PageSize: 20, TotalItems: 40, TotalPages: 2}

	var js bytes.Buffer
	if err := PrintList(&js, FormatJSON, items, meta, nil); err != nil {
		t.Fatalf("PrintList json: %v", err)
	}
	var arr []map[string]any
	if err := json.Unmarshal(js.Bytes(), &arr); err != nil {
		t.Fatalf("list json should be a bare array: %v (%s)", err, js.String())
	}
	if len(arr) != 1 || arr[0]["id"].(float64) != 1 {
		t.Errorf("unexpected bare list: %s", js.String())
	}

	// meta is table-only.
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

func TestErrorView(t *testing.T) {
	api := &client.APIError{StatusCode: 409, Code: "NAME_TAKEN", Message: "taken"}
	v := ErrorView(api)
	if v.Code != "NAME_TAKEN" || v.HTTPStatus != 409 || v.Message != "taken" {
		t.Errorf("unexpected view: %+v", v)
	}
	if ErrorView(fmt.Errorf("context: %w", api)).Code != "NAME_TAKEN" {
		t.Errorf("wrapped view lost code")
	}
	if got := ErrorView(errString("boom")); got.Code != "" || got.Message != "boom" {
		t.Errorf("plain view = %+v", got)
	}
}

func TestPrintErrorFormats(t *testing.T) {
	api := &client.APIError{StatusCode: 404, Code: "APP_NOT_FOUND", Message: "no app"}

	var js bytes.Buffer
	PrintError(&js, FormatJSON, api)
	var got struct {
		Error APIErrorView `json:"error"`
	}
	if err := json.Unmarshal(js.Bytes(), &got); err != nil {
		t.Fatalf("error json malformed: %v (%s)", err, js.String())
	}
	if got.Error.Code != "APP_NOT_FOUND" || got.Error.HTTPStatus != 404 {
		t.Errorf("unexpected error payload: %s", js.String())
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
	var got map[string]bool
	if err := json.Unmarshal(js.Bytes(), &got); err != nil || !got["aborted"] {
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
	friendly := &wrapErrForTest{msg: `no app named "web"`, cause: api}
	if got := FormatError(friendly); got != `no app named "web"` {
		t.Errorf("FormatError should show friendly text, got %q", got)
	}
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
