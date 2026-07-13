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

// Print renders data as JSON, or invokes table to render a human table,
// depending on format.
func Print(w io.Writer, format string, data any, table TableFunc) error {
	switch format {
	case FormatJSON:
		return encodeJSON(w, Envelope{OK: true, Data: data})
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

// Envelope is the machine-readable wrapper for every -o json response. Agents
// branch on OK; exactly one of Data / Error is populated.
type Envelope struct {
	OK    bool          `json:"ok"`
	Data  any           `json:"data,omitempty"`
	Error *APIErrorView `json:"error,omitempty"`
}

// APIErrorView is the stable JSON error shape. Code is the server's wire code
// (empty for local/client errors); Message may evolve and should not be parsed.
type APIErrorView struct {
	Code       string `json:"code,omitempty"`
	Message    string `json:"message"`
	HTTPStatus int    `json:"http_status,omitempty"`
}

// ActionResult is the canonical -o json payload for a mutation/lifecycle
// outcome. Message carries the human table line and is never serialized.
type ActionResult struct {
	Resource    string `json:"resource"`
	ID          uint   `json:"id,omitempty"`
	Action      string `json:"action"`
	Status      string `json:"status"`
	OperationID *uint  `json:"operation_id,omitempty"`
	Message     string `json:"-"`
}

// listPayload wraps list results so pagination metadata reaches JSON output.
type listPayload struct {
	Items any         `json:"items"`
	Meta  *types.Meta `json:"meta,omitempty"`
}

func encodeJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

// PrintResult renders a mutation/lifecycle outcome. In JSON it emits a success
// envelope; in table mode it writes the human line unless quiet is set.
func PrintResult(w io.Writer, format string, quiet bool, r ActionResult) error {
	if format == FormatJSON {
		return encodeJSON(w, Envelope{OK: true, Data: r})
	}
	if quiet || r.Message == "" {
		return nil
	}
	_, err := fmt.Fprintln(w, r.Message)
	return err
}

// PrintList renders a list result with optional pagination metadata in both
// output formats. items is the slice to render; meta may be nil.
func PrintList(w io.Writer, format string, items any, meta *types.Meta, table TableFunc) error {
	switch format {
	case FormatJSON:
		return encodeJSON(w, Envelope{OK: true, Data: listPayload{Items: items, Meta: meta}})
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

// ErrorView unwraps err into the stable JSON error shape.
func ErrorView(err error) APIErrorView {
	var apiErr *client.APIError
	if errors.As(err, &apiErr) {
		return APIErrorView{Code: apiErr.Code, Message: apiErr.Message, HTTPStatus: apiErr.StatusCode}
	}
	return APIErrorView{Message: err.Error()}
}

// PrintError writes a failure to w: a JSON error envelope when format is JSON,
// otherwise the familiar human "Error: …" line.
func PrintError(w io.Writer, format string, err error) {
	if format == FormatJSON {
		view := ErrorView(err)
		_ = encodeJSON(w, Envelope{OK: false, Error: &view})
		return
	}
	fmt.Fprintln(w, "Error: "+FormatError(err))
}
