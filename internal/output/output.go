// Package output renders command results either as human-readable tables or
// as JSON, selected by the --output flag.
package output

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"text/tabwriter"

	"github.com/kumobase/kumo-go/client"
	"github.com/kumobase/kumo-go/types"
)

// Supported output formats.
const (
	FormatTable = "table"
	FormatJSON  = "json"
)

// Valid reports whether format is a supported output format.
func Valid(format string) bool {
	return format == FormatTable || format == FormatJSON
}

// TableFunc writes a human-readable representation to the given tabwriter.
// Use tab ('\t') as the column separator.
type TableFunc func(tw *tabwriter.Writer)

// Print renders data as bare JSON, or invokes table to render a human table,
// depending on format. In JSON mode the data object/array is emitted directly on
// stdout (aws/gh/kubectl convention); errors go to stderr via PrintError.
func Print(w io.Writer, format string, data any, table TableFunc) error {
	switch format {
	case FormatJSON:
		return encodeJSON(w, data)
	case FormatTable:
		tw := tabwriter.NewWriter(w, 0, 2, 2, ' ', 0)
		table(tw)
		return tw.Flush()
	default:
		return fmt.Errorf("unknown output format %q (use %q or %q)", format, FormatTable, FormatJSON)
	}
}

// FormatError produces a concise, user-facing error message, unwrapping a
// kumo-go *client.APIError so the server's stable Code is surfaced alongside
// its human-readable message.
func FormatError(err error) string {
	var apiErr *client.APIError
	if errors.As(err, &apiErr) {
		// When the caller added no extra context, err.Error() == apiErr.Error();
		// show the compact "message (code)" form. When the caller wrapped it
		// with a friendlier message, show that instead so context is preserved
		// (the APIError stays in the chain for exit-code mapping).
		if err.Error() == apiErr.Error() {
			if apiErr.Code != "" {
				return fmt.Sprintf("%s (%s)", apiErr.Message, apiErr.Code)
			}
			return apiErr.Message
		}
		return err.Error()
	}
	return err.Error()
}

// APIErrorView is the stable JSON error shape. Code is the server's wire code
// (empty for local/client errors); Message may evolve and should not be parsed.
type APIErrorView struct {
	Code       string `json:"code,omitempty"`
	Message    string `json:"message"`
	HTTPStatus int    `json:"http_status,omitempty"`
}

// errorPayload is the -o json failure shape emitted on stderr: the view under an
// "error" key so it is self-describing and distinct from success data on stdout.
type errorPayload struct {
	Error APIErrorView `json:"error"`
}

// ActionResult is the canonical -o json payload for a mutation/lifecycle
// outcome, emitted bare on stdout. Message carries the human table line and is
// never serialized.
type ActionResult struct {
	Resource    string `json:"resource"`
	ID          uint   `json:"id,omitempty"`
	Action      string `json:"action"`
	Status      string `json:"status"`
	OperationID string `json:"operation_id,omitempty"`
	Message     string `json:"-"`
}

func encodeJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

// PrintResult renders a mutation/lifecycle outcome. In JSON it emits the bare
// action result; in table mode it writes the human line unless quiet is set.
func PrintResult(w io.Writer, format string, quiet bool, r ActionResult) error {
	if format == FormatJSON {
		return encodeJSON(w, r)
	}
	if quiet || r.Message == "" {
		return nil
	}
	_, err := fmt.Fprintln(w, r.Message)
	return err
}

// PrintList renders a list result. In JSON it emits the bare array (pagination
// metadata is table-only, matching the aws/kubectl convention). items is the
// slice to render; meta may be nil.
func PrintList(w io.Writer, format string, items any, meta *types.Meta, table TableFunc) error {
	switch format {
	case FormatJSON:
		return encodeJSON(w, items)
	case FormatTable:
		tw := tabwriter.NewWriter(w, 0, 2, 2, ' ', 0)
		table(tw)
		writePageFooter(tw, meta)
		return tw.Flush()
	default:
		return fmt.Errorf("unknown output format %q (use %q or %q)", format, FormatTable, FormatJSON)
	}
}

// writePageFooter renders a pagination hint when more than one page exists.
func writePageFooter(tw *tabwriter.Writer, meta *types.Meta) {
	if meta != nil && meta.TotalPages > 1 {
		fmt.Fprintf(tw, "\nPage %d/%d (%d items) — use --page to see more\n",
			meta.Page, meta.TotalPages, meta.TotalItems)
	}
}

// PrintAborted reports a user-declined confirmation. In JSON it emits
// {"aborted":true} (keeping stdout a valid JSON document); in table mode a short
// line. The caller returns nil (exit 0) — an abort is a user choice, not a failure.
func PrintAborted(w io.Writer, format string) error {
	if format == FormatJSON {
		return encodeJSON(w, map[string]bool{"aborted": true})
	}
	_, err := fmt.Fprintln(w, "Aborted.")
	return err
}

// ErrorView unwraps err into the stable JSON error shape.
func ErrorView(err error) APIErrorView {
	var apiErr *client.APIError
	if errors.As(err, &apiErr) {
		return APIErrorView{Code: apiErr.Code, Message: apiErr.Message, HTTPStatus: apiErr.StatusCode}
	}
	return APIErrorView{Message: err.Error()}
}

// PrintError writes a failure to w (stderr): a structured {"error":{…}} JSON
// object when format is JSON, otherwise the familiar human "Error: …" line.
func PrintError(w io.Writer, format string, err error) {
	if format == FormatJSON {
		_ = encodeJSON(w, errorPayload{Error: ErrorView(err)})
		return
	}
	fmt.Fprintln(w, "Error: "+FormatError(err))
}
