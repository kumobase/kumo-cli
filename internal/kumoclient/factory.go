// Package kumoclient builds a configured kumo-go SDK client from resolved
// CLI settings.
package kumoclient

import (
	"errors"
	"fmt"

	"github.com/kumobase/kumo-go/client"

	"github.com/kumobase/kumo-cli/internal/config"
	"github.com/kumobase/kumo-cli/internal/version"
)

// ErrNotLoggedIn is returned when no API key is available for the active
// profile. Callers can branch on it to print onboarding guidance.
var ErrNotLoggedIn = errors.New("not logged in")

// New constructs an authenticated *client.Client from resolved settings.
func New(s config.Settings) (*client.Client, error) {
	if s.APIKey == "" {
		return nil, fmt.Errorf("%w: no API key for profile %q — run `kumo auth login`", ErrNotLoggedIn, s.Profile)
	}
	c, err := client.New(s.BaseURL,
		client.WithAPIKey(s.APIKey),
		client.WithUserAgent("kumo-cli/"+version.Version),
	)
	if err != nil {
		return nil, fmt.Errorf("initialise kumo client: %w", err)
	}
	return c, nil
}
