// Package cli wires up the kumo command tree.
package cli

import (
	"bufio"
	"context"
	"fmt"
	"io"
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
	flagYes     bool
	flagQuiet   bool
	flagIdemKey string
)

// resolved holds the settings resolved once by the root PersistentPreRunE so
// the error path in Execute can render in the selected output format.
var (
	resolved   config.Settings
	resolvedOK bool
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
		// Resolve settings once so Execute's error path can render in the
		// selected output format (JSON envelope vs. human line). Skipped for
		// commands that need no config (help/version/completion).
		PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
			if isConfiglessCmd(cmd) {
				return nil
			}
			_, s, err := loadSettings()
			if err != nil {
				return err
			}
			resolved, resolvedOK = s, true
			return nil
		},
	}

	// Tag flag-parse failures so Execute can map them to exit code 2.
	root.SetFlagErrorFunc(func(_ *cobra.Command, e error) error {
		return usageError{err: e}
	})

	pf := root.PersistentFlags()
	pf.StringVar(&flagProfile, "profile", "", `configuration profile to use (default "default")`)
	pf.StringVar(&flagBaseURL, "base-url", "", "Kumo API base URL override")
	pf.StringVarP(&flagOutput, "output", "o", "", "output format: table or json")
	pf.BoolVarP(&flagYes, "yes", "y", false, "skip confirmation prompts")
	pf.BoolVarP(&flagQuiet, "quiet", "q", false, "suppress non-essential (progress) output")
	pf.StringVar(&flagIdemKey, "idempotency-key", "", "idempotency key for the write request (advanced)")

	root.AddCommand(newAuthCmd())
	root.AddCommand(newAppsCmd())
	root.AddCommand(newSecretCmd())
	root.AddCommand(newAPIKeyCmd())
	root.AddCommand(newRegistryCmd())
	root.AddCommand(newPackagesCmd())
	root.AddCommand(newVolumeCmd())
	root.AddCommand(newVPSCmd())
	root.AddCommand(newBillingCmd())
	root.AddCommand(newRunnersCmd())
	root.AddCommand(newSourceCmd())
	root.AddCommand(newJobsCmd())
	root.AddCommand(newIntrospectCmd())
	return root
}

// Execute runs the root command, renders any error in the selected output
// format (JSON error envelope or human line), and exits with a class-specific
// status code so callers — including AI agents — can branch on failure type.
func Execute() {
	root := NewRootCmd()
	if err := root.Execute(); err != nil {
		os.Exit(renderExecError(root.ErrOrStderr(), err))
	}
}

// renderExecError writes err to w in the resolved output format (a JSON error
// envelope under -o json, otherwise the human "Error: …" line) and returns the
// process exit code for its failure class. Shared by Execute and the tests so
// the error contract is exercised the same way in both.
func renderExecError(w io.Writer, err error) int {
	format := output.FormatTable
	if resolvedOK {
		format = resolved.Output
	}
	output.PrintError(w, format, err)
	return exitCodeFor(err)
}

// isConfiglessCmd reports whether cmd runs without resolved settings (help,
// version, and the completion machinery), so PersistentPreRunE can skip config
// loading for them.
func isConfiglessCmd(cmd *cobra.Command) bool {
	for c := cmd; c != nil; c = c.Parent() {
		switch c.Name() {
		case "help", "completion", cobra.ShellCompRequestCmd, cobra.ShellCompNoDescRequestCmd:
			return true
		}
	}
	return false
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
			return 0, nil, "", friendlyf(err, "multiple apps named %q exist; rename one (kumo apps list) to disambiguate", name)
		}
		if client.IsNotFound(err) {
			return 0, nil, "", friendlyf(err, "no app named %q", name)
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
	if id, ok := numericID(name); ok {
		sec, etag, err := c.Secrets().Get(ctx, id)
		if err != nil {
			if client.IsNotFound(err) {
				return 0, nil, "", friendlyf(err, "no secret with id %d", id)
			}
			return 0, nil, "", err
		}
		return sec.ID, sec, etag, nil
	}
	sec, etag, err := c.Secrets().GetByName(ctx, name)
	if err != nil {
		if client.IsCode(err, codes.AmbiguousName) {
			return 0, nil, "", friendlyf(err, "multiple secrets named %q exist; use the numeric id (kumo secret list) to disambiguate", name)
		}
		if client.IsNotFound(err) {
			return 0, nil, "", friendlyf(err, "no secret named %q", name)
		}
		return 0, nil, "", err
	}
	return sec.ID, sec, etag, nil
}

// confirm prompts the user for a yes/no answer on stdin. When stdin is not a
// terminal it returns an error instructing the caller to pass --yes.
func confirm(cmd *cobra.Command, prompt string) (bool, error) {
	in := cmd.InOrStdin()
	// Refuse only when connected to a real non-interactive terminal; a test or
	// caller that injects its own reader (not os.Stdin) is answered from it.
	if f, ok := in.(*os.File); ok && !term.IsTerminal(int(f.Fd())) {
		return false, fmt.Errorf("refusing to proceed without confirmation; re-run with --yes")
	}
	fmt.Fprintf(cmd.OutOrStdout(), "%s [y/N]: ", prompt)
	line, err := bufio.NewReader(in).ReadString('\n')
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
