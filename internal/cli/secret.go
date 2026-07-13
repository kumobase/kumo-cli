package cli

import (
	"fmt"
	"slices"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/kumobase/kumo-go/client"
	"github.com/kumobase/kumo-go/types"

	"github.com/kumobase/kumo-cli/internal/output"
)

// certificateSecretsEnabled gates the TLS "certificate" secret type. While
// false, the type is hidden from `secret create` (no --cert-file/--key-file
// flags, --type certificate rejected) and the app `--tls-secret-id` flag is
// not registered. The implementation below stays compiled and ready; flip
// this to true to expose it.
const certificateSecretsEnabled = false

// enabledSecretTypes returns the secret types a user may select via --type,
// honoring the certificate gate. Used for validation and help text.
func enabledSecretTypes() []types.SecretType {
	ts := []types.SecretType{
		types.SecretTypeRegistry,
		types.SecretTypeEnvVar,
		types.SecretTypeFile,
	}
	if certificateSecretsEnabled {
		ts = append(ts, types.SecretTypeCertificate)
	}
	return ts
}

// enabledSecretTypesString renders enabledSecretTypes for flag help.
func enabledSecretTypesString() string {
	parts := make([]string, 0, 4)
	for _, t := range enabledSecretTypes() {
		parts = append(parts, string(t))
	}
	return strings.Join(parts, ", ")
}

// isEnabledSecretType reports whether t is a user-selectable secret type.
func isEnabledSecretType(t types.SecretType) bool {
	return slices.Contains(enabledSecretTypes(), t)
}

func newSecretCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "secret",
		Aliases: []string{"secrets"},
		Short:   "Manage secrets",
	}
	cmd.AddCommand(
		newSecretListCmd(),
		newSecretGetCmd(),
		newSecretCreateCmd(),
		newSecretUpdateCmd(),
		newSecretDeleteCmd(),
	)
	return cmd
}

func newSecretListCmd() *cobra.Command {
	var (
		typeFilter       string
		search           string
		page, pageSize   int
		sortCol, sortDir string
	)
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List secrets",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			c, s, err := newClient()
			if err != nil {
				return err
			}
			var opts []client.ListOption
			if typeFilter != "" {
				opts = append(opts, client.WithExtraQuery("type", typeFilter))
			}
			if search != "" {
				opts = append(opts, client.WithExtraQuery("search", search))
			}
			if page > 0 {
				opts = append(opts, client.WithPage(page))
			}
			if pageSize > 0 {
				opts = append(opts, client.WithPageSize(pageSize))
			}
			if sortCol != "" {
				opts = append(opts, client.WithSort(sortCol, sortDir))
			}
			secrets, _, err := c.Secrets().List(cmd.Context(), opts...)
			if err != nil {
				return err
			}
			return output.Print(cmd.OutOrStdout(), s.Output, secrets, func(tw *tabwriter.Writer) {
				fmt.Fprintln(tw, "ID\tNAME\tTYPE\tUSED BY\tCREATED")
				for _, sec := range secrets {
					fmt.Fprintf(tw, "%d\t%s\t%s\t%d\t%s\n",
						sec.ID, sec.Name, sec.Type, sec.UsedByCount,
						sec.CreatedAt.Format("2006-01-02"))
				}
			})
		},
	}
	f := cmd.Flags()
	f.StringVar(&typeFilter, "type", "", "filter by secret type ("+enabledSecretTypesString()+")")
	f.StringVar(&search, "search", "", "filter by name substring")
	f.IntVar(&page, "page", 0, "page number (1-based)")
	f.IntVar(&pageSize, "page-size", 0, "items per page (max 100)")
	f.StringVar(&sortCol, "sort", "", "sort column")
	f.StringVar(&sortDir, "sort-order", "desc", "sort order: asc or desc")
	return cmd
}

func newSecretGetCmd() *cobra.Command {
	var reveal bool
	cmd := &cobra.Command{
		Use:   "get <name>",
		Short: "Show secret detail",
		Long: "Show a secret's detail. Sensitive values (registry password, env-var\n" +
			"values, file content) are masked unless --reveal is passed. JSON output\n" +
			"(-o json) always returns the raw payload.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, s, err := newClient()
			if err != nil {
				return err
			}
			_, sec, _, err := resolveSecretRef(cmd.Context(), c, args[0])
			if err != nil {
				return err
			}
			return output.Print(cmd.OutOrStdout(), s.Output, sec, func(tw *tabwriter.Writer) {
				printSecretDetail(tw, sec, reveal)
			})
		},
	}
	cmd.Flags().BoolVar(&reveal, "reveal", false, "show sensitive values in plaintext")
	return cmd
}

// printSecretDetail renders a secret's table view. Sensitive payload fields
// are masked unless reveal is true; non-sensitive metadata always shows.
func printSecretDetail(tw *tabwriter.Writer, sec *types.ResponseGetSecret, reveal bool) {
	fmt.Fprintf(tw, "ID:\t%d\n", sec.ID)
	fmt.Fprintf(tw, "Name:\t%s\n", sec.Name)
	fmt.Fprintf(tw, "Type:\t%s\n", sec.Type)
	fmt.Fprintf(tw, "Used by:\t%d app(s)\n", sec.UsedByCount)
	for _, u := range sec.UsedBy {
		mount := ""
		if u.MountTo != "" {
			mount = " → " + u.MountTo
		}
		fmt.Fprintf(tw, "  - %s (app %d, %s)%s\n", u.AppName, u.AppID, u.UsageType, mount)
	}
	fmt.Fprintf(tw, "Created:\t%s\n", sec.CreatedAt.Format(time.RFC3339))
	fmt.Fprintf(tw, "Updated:\t%s\n", sec.UpdatedAt.Format(time.RFC3339))

	switch sec.Type {
	case types.SecretTypeRegistry:
		if sec.SecretRegistry != nil {
			host := sec.SecretRegistry.RegistryHost
			if host == "" {
				host = "(docker hub)"
			}
			fmt.Fprintf(tw, "Registry host:\t%s\n", host)
			fmt.Fprintf(tw, "Username:\t%s\n", sec.SecretRegistry.Username)
			fmt.Fprintf(tw, "Password:\t%s\n", revealOrMask(sec.SecretRegistry.Password, reveal))
		}
	case types.SecretTypeEnvVar:
		for _, ev := range sec.EnvironmentVariables {
			fmt.Fprintf(tw, "Env:\t%s=%s\n", ev.Key, revealOrMask(ev.Value, reveal))
		}
	case types.SecretTypeFile:
		fmt.Fprintf(tw, "File content:\t%s\n", revealOrMask(sec.FileContent, reveal))
	case types.SecretTypeCertificate:
		if sec.CertificateContent != nil {
			fmt.Fprintf(tw, "Certificate:\t%s\n", revealOrMask(sec.CertificateContent.Certificate, reveal))
			fmt.Fprintf(tw, "Private key:\t%s\n", revealOrMask(sec.CertificateContent.PrivateKey, reveal))
		}
	}
}

// revealOrMask returns the value verbatim when reveal is true, otherwise a
// fixed mask. An empty value renders as "(empty)" regardless.
func revealOrMask(v string, reveal bool) string {
	if v == "" {
		return "(empty)"
	}
	if reveal {
		return v
	}
	return maskValue(v)
}
