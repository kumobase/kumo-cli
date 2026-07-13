package cli

import (
	"context"
	"fmt"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/kumobase/kumo-go/client"
	"github.com/kumobase/kumo-go/codes"
	"github.com/kumobase/kumo-go/types"

	"github.com/kumobase/kumo-cli/internal/output"
)

// API-key write endpoints (create/update/delete) are session-only on the
// backend — calls authenticated with a stored API key are rejected with
// codes.APIKeySessionRequired. The CLI today only carries an API key, so
// only the read paths (list, get) are exposed here. Use the dashboard to
// manage keys until a JWT login flow lands.

func newAPIKeyCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "apikey",
		Aliases: []string{"apikeys", "api-key", "api-keys"},
		Short:   "Inspect Kumo API keys",
		Long: "List and inspect API keys for the active user.\n\n" +
			"Write operations (create/update/delete) are session-only on the\n" +
			"backend and not available via the CLI today — use the dashboard.",
	}
	cmd.AddCommand(newAPIKeyListCmd(), newAPIKeyGetCmd())
	return cmd
}

func newAPIKeyListCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List API keys",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			c, s, err := newClient()
			if err != nil {
				return err
			}
			keys, err := c.APIKeys().List(cmd.Context())
			if err != nil {
				return mapAPIKeySessionError(err)
			}
			return output.PrintList(cmd.OutOrStdout(), s.Output, keys, nil, func(tw *tabwriter.Writer) {
				fmt.Fprintln(tw, "ID\tNAME\tKIND\tPREFIX\tSCOPES\tEXPIRES\tLAST USED")
				for _, k := range keys {
					fmt.Fprintf(tw, "%d\t%s\t%s\t%s\t%s\t%s\t%s\n",
						k.ID, k.Name, apiKeyKind(&k), k.KeyPrefix,
						apiKeyScopesLabel(&k),
						formatOptionalTime(k.ExpiresAt, "never"),
						formatOptionalTime(k.LastUsedAt, "never"))
				}
			})
		},
	}
	return cmd
}

func newAPIKeyGetCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get <name>",
		Short: "Show API key detail",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, s, err := newClient()
			if err != nil {
				return err
			}
			_, k, err := resolveAPIKeyRef(cmd.Context(), c, args[0])
			if err != nil {
				return err
			}
			return output.Print(cmd.OutOrStdout(), s.Output, k, func(tw *tabwriter.Writer) {
				printAPIKeyDetail(tw, k)
			})
		},
	}
	return cmd
}

// resolveAPIKeyRef looks up an API key by its (per-user unique) name.
func resolveAPIKeyRef(ctx context.Context, c *client.Client, name string) (uint, *types.APIKeyResponse, error) {
	if strings.TrimSpace(name) == "" {
		return 0, nil, fmt.Errorf("api key name is required")
	}
	k, err := c.APIKeys().GetByName(ctx, name)
	if err != nil {
		if client.IsCode(err, codes.AmbiguousName) {
			return 0, nil, fmt.Errorf("multiple api keys named %q exist; rename one (kumo apikey list) to disambiguate", name)
		}
		if client.IsNotFound(err) {
			return 0, nil, fmt.Errorf("no api key named %q", name)
		}
		if mapped := mapAPIKeySessionError(err); mapped != err {
			return 0, nil, mapped
		}
		return 0, nil, err
	}
	return k.ID, k, nil
}

// apiKeyKind returns "registry" for registry-scoped keys, "personal" otherwise.
func apiKeyKind(k *types.APIKeyResponse) string {
	if k.RegistryOrgSlug != nil && *k.RegistryOrgSlug != "" {
		return "registry"
	}
	return "personal"
}

// apiKeyScopesLabel renders the scopes column. Keys created under the unified
// permission model carry Grants and are summarized from those; otherwise it
// falls back to the legacy view — registry permissions (pull/push/delete) for
// registry keys, read/write scopes for personal keys; "-" when none apply.
func apiKeyScopesLabel(k *types.APIKeyResponse) string {
	if len(k.Grants) > 0 {
		return apiKeyGrantsLabel(k.Grants)
	}
	if apiKeyKind(k) == "registry" {
		if len(k.RegistryPermissions) == 0 {
			return "-"
		}
		return strings.Join(k.RegistryPermissions, ",")
	}
	if len(k.Scopes) == 0 {
		return "-"
	}
	return strings.Join(k.Scopes, ",")
}

// apiKeyGrantsLabel renders a compact one-line summary of the unified grants
// for the list column, e.g. "registry:pull,push; control_plane:read".
func apiKeyGrantsLabel(grants []types.Grant) string {
	parts := make([]string, 0, len(grants))
	for _, g := range grants {
		parts = append(parts, fmt.Sprintf("%s:%s", g.Domain, strings.Join(g.Actions, ",")))
	}
	return strings.Join(parts, "; ")
}

// printAPIKeyDetail renders the full single-key view.
func printAPIKeyDetail(tw *tabwriter.Writer, k *types.APIKeyResponse) {
	fmt.Fprintf(tw, "ID:\t%d\n", k.ID)
	fmt.Fprintf(tw, "Name:\t%s\n", k.Name)
	fmt.Fprintf(tw, "Kind:\t%s\n", apiKeyKind(k))
	fmt.Fprintf(tw, "Prefix:\t%s\n", k.KeyPrefix)
	fmt.Fprintf(tw, "Created:\t%s\n", k.CreatedAt.Format(time.RFC3339))
	fmt.Fprintf(tw, "Expires:\t%s\n", formatOptionalTime(k.ExpiresAt, "never"))
	fmt.Fprintf(tw, "Last used:\t%s\n", formatOptionalTime(k.LastUsedAt, "never"))
	// Unified-model keys carry Grants (and optional Conditions); legacy keys
	// carry only Scopes / registry_* fields. Render whichever the key has.
	if len(k.Grants) > 0 {
		for _, g := range k.Grants {
			orgs := "all orgs"
			if len(g.Orgs) > 0 {
				orgs = strings.Join(g.Orgs, ",")
			}
			fmt.Fprintf(tw, "Grant:\t%s [%s] (%s)\n", g.Domain, strings.Join(g.Actions, ","), orgs)
		}
		if k.Conditions != nil && len(k.Conditions.IPAllowlist) > 0 {
			fmt.Fprintf(tw, "IP allowlist:\t%s\n", strings.Join(k.Conditions.IPAllowlist, ","))
		}
	} else if apiKeyKind(k) == "registry" {
		fmt.Fprintf(tw, "Registry org:\t%s\n", deref(k.RegistryOrgSlug))
		fmt.Fprintf(tw, "Registry repo:\t%s\n", derefOr(k.RegistryRepoName, "(org-wide)"))
		fmt.Fprintf(tw, "Permissions:\t%s\n", strings.Join(k.RegistryPermissions, ","))
	} else {
		fmt.Fprintf(tw, "Scopes:\t%s\n", strings.Join(k.Scopes, ","))
	}
}

// mapAPIKeySessionError translates the sessionOnly 403 into a user-facing
// message pointing at the dashboard.
func mapAPIKeySessionError(err error) error {
	if client.IsCode(err, codes.APIKeySessionRequired) {
		return fmt.Errorf("api-key management requires a session login; use the Kumo dashboard to manage keys: %w", err)
	}
	return err
}

// formatOptionalTime renders *time.Time or returns the placeholder when nil.
func formatOptionalTime(t *time.Time, ifNil string) string {
	if t == nil {
		return ifNil
	}
	return t.Format(time.RFC3339)
}

// deref returns the string pointed to or "" when nil.
func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// derefOr returns the string pointed to or the fallback when nil/empty.
func derefOr(s *string, fallback string) string {
	if s == nil || *s == "" {
		return fallback
	}
	return *s
}
