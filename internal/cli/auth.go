package cli

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/kumobase/kumo-cli/internal/config"
	"github.com/kumobase/kumo-cli/internal/kumoclient"
	"github.com/kumobase/kumo-cli/internal/output"
)

func newAuthCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "auth",
		Short: "Manage authentication and credentials",
	}
	cmd.AddCommand(newAuthLoginCmd(), newAuthLogoutCmd(), newAuthWhoamiCmd())
	return cmd
}

func newAuthLoginCmd() *cobra.Command {
	var apiKey string
	cmd := &cobra.Command{
		Use:   "login",
		Short: "Authenticate with a Kumo API key",
		Long: "Store a Kumo API key (kumo_sk_...) for the active profile.\n\n" +
			"The key may be provided via --api-key, piped on stdin, or entered at the\n" +
			"interactive prompt. It is validated against the API before being saved.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			store, err := config.Load()
			if err != nil {
				return err
			}
			profile := resolveProfileName(store)

			key := strings.TrimSpace(apiKey)
			if key == "" {
				key, err = readAPIKey(cmd)
				if err != nil {
					return err
				}
			}
			if key == "" {
				return fmt.Errorf("no API key provided")
			}

			// Validate the key against the resolved base URL before saving.
			settings := store.Resolve(config.Overrides{Profile: flagProfile, BaseURL: flagBaseURL})
			settings.APIKey = key
			c, err := kumoclient.New(settings)
			if err != nil {
				return err
			}
			profileData, err := c.Profile().Get(cmd.Context())
			if err != nil {
				return fmt.Errorf("could not validate API key: %w", err)
			}

			store.SetCredential(profile, config.Credential{
				Type:   config.CredentialTypeAPIKey,
				APIKey: key,
			})
			if flagBaseURL != "" {
				p, _ := store.GetProfile(profile)
				p.BaseURL = flagBaseURL
				store.SetProfile(profile, p)
			}
			store.SetCurrentProfile(profile)
			if err := store.Save(); err != nil {
				return err
			}

			return printResult(cmd, output.ActionResult{
				Resource: "auth", Action: "login", Status: "done",
				Message: fmt.Sprintf("Logged in as %s <%s> (profile %q)",
					profileData.FullName, profileData.Email, profile),
			})
		},
	}
	cmd.Flags().StringVar(&apiKey, "api-key", "", "API key (kumo_sk_...); if omitted, read from stdin or prompt")
	return cmd
}

func newAuthLogoutCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "logout",
		Short: "Remove stored credentials for the active profile",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			store, err := config.Load()
			if err != nil {
				return err
			}
			profile := resolveProfileName(store)
			if _, ok := store.Credential(profile); !ok {
				return printResult(cmd, output.ActionResult{
					Resource: "auth", Action: "logout", Status: "noop",
					Message: fmt.Sprintf("No credentials stored for profile %q", profile),
				})
			}
			store.RemoveCredential(profile)
			if err := store.Save(); err != nil {
				return err
			}
			return printResult(cmd, output.ActionResult{
				Resource: "auth", Action: "logout", Status: "done",
				Message: fmt.Sprintf("Logged out of profile %q (local credentials removed; the API key is not revoked)", profile),
			})
		},
	}
}

func newAuthWhoamiCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "whoami",
		Short: "Show the active profile and authenticated identity",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			c, s, err := newClient()
			if err != nil {
				return err
			}
			p, err := c.Profile().Get(cmd.Context())
			if err != nil {
				return err
			}
			return output.Print(cmd.OutOrStdout(), s.Output, p, func(tw *tabwriter.Writer) {
				fmt.Fprintf(tw, "Profile:\t%s\n", s.Profile)
				fmt.Fprintf(tw, "Base URL:\t%s\n", s.BaseURL)
				fmt.Fprintf(tw, "API key:\t%s\n", maskKey(s.APIKey))
				fmt.Fprintf(tw, "Name:\t%s\n", p.FullName)
				fmt.Fprintf(tw, "Email:\t%s\n", p.Email)
				fmt.Fprintf(tw, "Verified:\t%t\n", p.IsVerified)
			})
		},
	}
}

// readAPIKey reads an API key from stdin: a single line when piped, or a
// hidden interactive prompt when attached to a terminal.
func readAPIKey(cmd *cobra.Command) (string, error) {
	fd := int(os.Stdin.Fd())
	if !term.IsTerminal(fd) {
		line, err := bufio.NewReader(os.Stdin).ReadString('\n')
		if err != nil && line == "" {
			return "", fmt.Errorf("read API key from stdin: %w", err)
		}
		return strings.TrimSpace(line), nil
	}
	fmt.Fprint(cmd.OutOrStdout(), "Kumo API key: ")
	b, err := term.ReadPassword(fd)
	fmt.Fprintln(cmd.OutOrStdout())
	if err != nil {
		return "", fmt.Errorf("read API key: %w", err)
	}
	return strings.TrimSpace(string(b)), nil
}
