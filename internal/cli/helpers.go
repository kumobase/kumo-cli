package cli

import (
	"time"

	"github.com/kumobase/kumo-go/client"
)

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
