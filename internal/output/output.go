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
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(data)
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
		if apiErr.Code != "" {
			return fmt.Sprintf("%s (%s)", apiErr.Message, apiErr.Code)
		}
		return apiErr.Message
	}
	return err.Error()
}
