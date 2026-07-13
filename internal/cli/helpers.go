package cli

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/kumobase/kumo-go/client"

	"github.com/kumobase/kumo-cli/internal/output"
)

// outputFormat returns the output format resolved once by the root
// PersistentPreRunE, falling back to table when unset (e.g. very early errors).
func outputFormat() string {
	if resolvedOK {
		return resolved.Output
	}
	return output.FormatTable
}

// printResult renders a mutation/lifecycle outcome to the command's stdout in
// the resolved format (JSON success envelope or human line), honouring --quiet.
func printResult(cmd *cobra.Command, r output.ActionResult) error {
	return output.PrintResult(cmd.OutOrStdout(), outputFormat(), flagQuiet, r)
}

// printAborted renders a declined-confirmation result and returns nil (an abort
// is a user choice, exit 0).
func printAborted(cmd *cobra.Command) error {
	return output.PrintAborted(cmd.OutOrStdout(), outputFormat())
}

// validateSortOrder rejects a --sort-order value that is not asc or desc. An
// empty value is accepted (the caller's default applies).
func validateSortOrder(order string) error {
	switch order {
	case "", "asc", "desc":
		return nil
	default:
		return usageError{err: fmt.Errorf("invalid --sort-order %q (use asc or desc)", order)}
	}
}

// validateRFC3339Flag rejects a non-empty flag value that is not a valid
// RFC3339 timestamp, naming the flag in the error.
func validateRFC3339Flag(flag, value string) error {
	if value == "" {
		return nil
	}
	if _, err := time.Parse(time.RFC3339, value); err != nil {
		return usageError{err: fmt.Errorf("invalid --%s %q (use an RFC3339 time like 2024-01-02T15:04:05Z)", flag, value)}
	}
	return nil
}

// writeOpts assembles the WriteOptions for a mutating call: an optional IfMatch
// ETag (pass "" when none) plus the global --idempotency-key when set. Attach
// these only to a command's primary write, never to pre-flight reads.
func writeOpts(etag string) []client.WriteOption {
	var o []client.WriteOption
	if etag != "" {
		o = append(o, client.IfMatch(etag))
	}
	if flagIdemKey != "" {
		o = append(o, client.WithIdempotencyKey(flagIdemKey))
	}
	return o
}

// pollOpts returns the poll options for a wait/AndWait call: a max-wait bound
// plus geometric backoff so long operations don't hammer the status endpoint.
func pollOpts(timeout time.Duration) []client.PollOption {
	return []client.PollOption{
		client.WithPollMaxWait(timeout),
		client.WithPollBackoff(1.5),
	}
}
