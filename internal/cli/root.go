// Package cli wires up the kumo command tree.
package cli

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/kumobase/kumo-go/client"
	"github.com/kumobase/kumo-go/codes"
	"github.com/kumobase/kumo-go/types"

	"github.com/kumobase/kumo-cli/internal/config"
	"github.com/kumobase/kumo-cli/internal/kumoclient"
	"github.com/kumobase/kumo-cli/internal/output"
	"github.com/kumobase/kumo-cli/internal/version"
)

// Persistent flags shared by every command.
var (
	flagProfile string
	flagBaseURL string
	flagOutput  string
)

// NewRootCmd builds the root cobra command and registers all subcommands.
func NewRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:           "kumo",
		Short:         "Kumo platform command-line interface",
		Long:          "kumo is the command-line interface for the Kumo platform (https://kumo.run).",
		SilenceUsage:  true,
		SilenceErrors: true,
		Version:       version.Version,
	}

	pf := root.PersistentFlags()
	pf.StringVar(&flagProfile, "profile", "", `configuration profile to use (default "default")`)
	pf.StringVar(&flagBaseURL, "base-url", "", "Kumo API base URL override")
	pf.StringVarP(&flagOutput, "output", "o", "", "output format: table or json")

	root.AddCommand(newAuthCmd())
	root.AddCommand(newAppsCmd())
	root.AddCommand(newSecretCmd())
	root.AddCommand(newAPIKeyCmd())
	root.AddCommand(newRegistryCmd())
	root.AddCommand(newVolumeCmd())
	root.AddCommand(newVPSCmd())
	root.AddCommand(newBillingCmd())
	root.AddCommand(newRunnersCmd())
	root.AddCommand(newSourceCmd())
	root.AddCommand(newJobsCmd())
	return root
}

// Execute runs the root command and maps errors to a non-zero exit code,
// unwrapping SDK API errors for a friendlier message.
func Execute() {
	if err := NewRootCmd().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "Error: "+output.FormatError(err))
		os.Exit(1)
	}
}

// loadSettings loads the config store and resolves it against the persistent
// flags and environment.
func loadSettings() (*config.Store, config.Settings, error) {
	store, err := config.Load()
	if err != nil {
		return nil, config.Settings{}, err
	}
	s := store.Resolve(config.Overrides{
		Profile: flagProfile,
		BaseURL: flagBaseURL,
		Output:  flagOutput,
	})
	if !output.Valid(s.Output) {
		return nil, config.Settings{}, fmt.Errorf("invalid output format %q (use table or json)", s.Output)
	}
	return store, s, nil
}

// newClient resolves settings and builds an authenticated SDK client.
func newClient() (*client.Client, config.Settings, error) {
	_, s, err := loadSettings()
	if err != nil {
		return nil, s, err
	}
	c, err := kumoclient.New(s)
	return c, s, err
}

// resolveProfileName returns the profile name selected by flag, env, or the
// store's current profile.
func resolveProfileName(store *config.Store) string {
	if flagProfile != "" {
		return flagProfile
	}
	if env := os.Getenv(config.EnvProfile); env != "" {
		return env
	}
	return store.CurrentProfile()
}

// resolveAppRef looks up an app by its (per-user unique) name. The returned
// ETag can be threaded into a subsequent IfMatch update.
func resolveAppRef(ctx context.Context, c *client.Client, name string) (uint, *types.AppByIdResponse, string, error) {
	if strings.TrimSpace(name) == "" {
		return 0, nil, "", fmt.Errorf("app name is required")
	}
	app, etag, err := c.Apps().GetByName(ctx, name)
	if err != nil {
		if client.IsCode(err, codes.AmbiguousName) {
			return 0, nil, "", fmt.Errorf("multiple apps named %q exist; rename one (kumo apps list) to disambiguate", name)
		}
		if client.IsNotFound(err) {
			return 0, nil, "", fmt.Errorf("no app named %q", name)
		}
		return 0, nil, "", err
	}
	return app.Id, app, etag, nil
}

// resolveSecretRef looks up a secret by its (per-user unique) name.
func resolveSecretRef(ctx context.Context, c *client.Client, name string) (uint, *types.ResponseGetSecret, string, error) {
	if strings.TrimSpace(name) == "" {
		return 0, nil, "", fmt.Errorf("secret name is required")
	}
	sec, etag, err := c.Secrets().GetByName(ctx, name)
	if err != nil {
		if client.IsCode(err, codes.AmbiguousName) {
			return 0, nil, "", fmt.Errorf("multiple secrets named %q exist; rename one (kumo secret list) to disambiguate", name)
		}
		if client.IsNotFound(err) {
			return 0, nil, "", fmt.Errorf("no secret named %q", name)
		}
		return 0, nil, "", err
	}
	return sec.ID, sec, etag, nil
}

// confirm prompts the user for a yes/no answer on stdin. When stdin is not a
// terminal it returns an error instructing the caller to pass --yes.
func confirm(cmd *cobra.Command, prompt string) (bool, error) {
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		return false, fmt.Errorf("refusing to proceed without confirmation; re-run with --yes")
	}
	fmt.Fprintf(cmd.OutOrStdout(), "%s [y/N]: ", prompt)
	line, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil && line == "" {
		return false, err
	}
	switch strings.ToLower(strings.TrimSpace(line)) {
	case "y", "yes":
		return true, nil
	default:
		return false, nil
	}
}

// maskKey redacts an API key for display, keeping a short suffix for
// recognisability.
func maskKey(key string) string {
	if key == "" {
		return "(none)"
	}
	if len(key) <= 8 {
		return "****"
	}
	return key[:8] + "…" + key[len(key)-4:]
}
