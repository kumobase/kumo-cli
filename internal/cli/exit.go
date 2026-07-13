package cli

import (
	"errors"
	"fmt"

	"github.com/kumobase/kumo-go/client"
	"github.com/kumobase/kumo-go/codes"

	"github.com/kumobase/kumo-cli/internal/kumoclient"
)

// usageError tags a flag-parse error so Execute maps it to exit code 2 and
// leaves the message unadorned. Set via root.SetFlagErrorFunc.
type usageError struct{ err error }

func (u usageError) Error() string { return u.err.Error() }
func (u usageError) Unwrap() error { return u.err }

func isUsageError(err error) bool {
	var u usageError
	return errors.As(err, &u)
}

// friendlyError attaches a human-readable message to a cause while keeping the
// cause in the error chain. Its Error() returns only the friendly text (so
// tables stay clean), while errors.Is/As still reach the cause — letting
// exitCodeFor classify it and FormatError show the friendly message.
type friendlyError struct {
	msg   string
	cause error
}

func (e *friendlyError) Error() string { return e.msg }
func (e *friendlyError) Unwrap() error { return e.cause }

// friendlyf wraps cause with a formatted human message.
func friendlyf(cause error, format string, a ...any) error {
	return &friendlyError{msg: fmt.Sprintf(format, a...), cause: cause}
}

// exitCodeFor maps an error to a stable process exit status so agents can
// branch on failure class without parsing messages:
//
//	0 success · 1 generic · 2 usage · 3 auth · 4 not-found ·
//	5 conflict · 6 validation · 7 etag-mismatch
func exitCodeFor(err error) int {
	switch {
	case err == nil:
		return 0
	case isUsageError(err):
		return 2
	case errors.Is(err, kumoclient.ErrNotLoggedIn), client.IsCode(err, codes.Unauthorized):
		return 3
	case client.IsNotFound(err):
		return 4
	case client.IsConflict(err), client.IsCode(err, codes.AmbiguousName):
		return 5
	case errors.Is(err, client.ErrValidationFailed),
		client.IsCode(err, codes.InvalidResourceName),
		errors.Is(err, client.ErrInvalidFilterCombination):
		return 6
	case errors.Is(err, client.ErrETagMismatch):
		return 7
	default:
		return 1
	}
}
