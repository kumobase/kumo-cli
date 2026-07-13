package cli

import (
	"context"
	"errors"
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

func newRegistryCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "registry",
		Aliases: []string{"registries"},
		Short:   "Manage the Kumo container registry",
	}
	cmd.AddCommand(
		newRegistryOrgCmd(),
		newRegistryRepoCmd(),
		newRegistryImageCmd(),
		newRegistryPlansCmd(),
		newRegistryLoginCmd(),
		newRegistryLogoutCmd(),
	)
	return cmd
}

// newRegistryPlansCmd lists the public container-registry billing catalogue.
// No org is required — the catalogue is a public price list.
func newRegistryPlansCmd() *cobra.Command {
	var sort string
	cmd := &cobra.Command{
		Use:   "plans",
		Short: "List container-registry billing plans",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			c, s, err := newClient()
			if err != nil {
				return err
			}
			var opts []client.ListOption
			if sort != "" {
				opts = append(opts, client.WithSort(sort, "asc"))
			}
			plans, err := c.Registry().ListPlans(cmd.Context(), opts...)
			if err != nil {
				return mapRegistryError(err)
			}
			return output.Print(cmd.OutOrStdout(), s.Output, plans, func(tw *tabwriter.Writer) {
				fmt.Fprintln(tw, "NAME\tUNIT\tPRICE/UNIT\tCURRENCY\tCHARGE MODEL\tBILLING PERIOD")
				for _, p := range plans {
					fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n",
						p.Name, p.Unit, p.PricePerUnit, p.Currency, p.ChargeModel, p.BillingPeriod)
				}
			})
		},
	}
	cmd.Flags().StringVar(&sort, "sort", "", "sort column")
	return cmd
}

func newRegistryOrgCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "org",
		Aliases: []string{"orgs", "organization", "organizations"},
		Short:   "Manage registry organizations (namespaces)",
	}
	cmd.AddCommand(
		newRegistryOrgListCmd(),
		newRegistryOrgGetCmd(),
		newRegistryOrgCreateCmd(),
		newRegistryOrgUpdateCmd(),
		newRegistryOrgDeleteCmd(),
	)
	return cmd
}

func newRegistryOrgListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List registry organizations",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			c, s, err := newClient()
			if err != nil {
				return err
			}
			orgs, err := c.Registry().Orgs().List(cmd.Context())
			if err != nil {
				return mapRegistryError(err)
			}
			return output.Print(cmd.OutOrStdout(), s.Output, orgs, func(tw *tabwriter.Writer) {
				fmt.Fprintln(tw, "SLUG\tDISPLAY NAME\tDEFAULT\tAUTO-CREATE REPOS\tSUSPENDED\tCREATED")
				for _, o := range orgs {
					fmt.Fprintf(tw, "%s\t%s\t%t\t%t\t%s\t%s\n",
						o.Slug, o.DisplayName, o.IsDefault, o.RegistryAutoCreateRepos,
						formatOptionalTime(o.RegistrySuspendedAt, "no"),
						o.CreatedAt.Format("2006-01-02"))
				}
			})
		},
	}
}

func newRegistryOrgGetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "get <slug>",
		Short: "Show registry organization detail",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, s, err := newClient()
			if err != nil {
				return err
			}
			o, _, err := c.Registry().Orgs().Get(cmd.Context(), args[0])
			if err != nil {
				return mapRegistryOrgGetError(err, args[0])
			}
			return output.Print(cmd.OutOrStdout(), s.Output, o, func(tw *tabwriter.Writer) {
				printRegistryOrgDetail(tw, o)
			})
		},
	}
}

func newRegistryOrgCreateCmd() *cobra.Command {
	var displayName string
	cmd := &cobra.Command{
		Use:   "create <slug>",
		Short: "Create a registry organization",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, s, err := newClient()
			if err != nil {
				return err
			}
			req := &types.CreateOrganizationRequest{Slug: args[0], DisplayName: displayName}
			o, err := c.Registry().Orgs().Create(cmd.Context(), req)
			if err != nil {
				return mapRegistryError(err)
			}
			return output.Print(cmd.OutOrStdout(), s.Output, o, func(tw *tabwriter.Writer) {
				fmt.Fprintf(tw, "Created org %q (display name %q)\n", o.Slug, o.DisplayName)
			})
		},
	}
	cmd.Flags().StringVar(&displayName, "display-name", "", "human-readable display name")
	return cmd
}

func newRegistryOrgUpdateCmd() *cobra.Command {
	var (
		displayName string
		autoCreate  bool
	)
	cmd := &cobra.Command{
		Use:   "update <slug>",
		Short: "Update a registry organization",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, s, err := newClient()
			if err != nil {
				return err
			}
			_, etag, err := c.Registry().Orgs().Get(cmd.Context(), args[0])
			if err != nil {
				return mapRegistryOrgGetError(err, args[0])
			}
			req := &types.UpdateOrganizationRequest{}
			f := cmd.Flags()
			if f.Changed("display-name") {
				req.DisplayName = &displayName
			}
			if f.Changed("auto-create-repos") {
				req.RegistryAutoCreateRepos = &autoCreate
			}
			o, err := c.Registry().Orgs().Update(cmd.Context(), args[0], req, writeOpts(etag)...)
			if err != nil {
				if errors.Is(err, client.ErrETagMismatch) {
					return fmt.Errorf("org changed since it was read; re-run the update: %w", err)
				}
				return mapRegistryError(err)
			}
			return output.Print(cmd.OutOrStdout(), s.Output, o, func(tw *tabwriter.Writer) {
				fmt.Fprintf(tw, "Updated org %q\n", o.Slug)
			})
		},
	}
	f := cmd.Flags()
	f.StringVar(&displayName, "display-name", "", "new display name")
	f.BoolVar(&autoCreate, "auto-create-repos", false, "auto-create repos on first push")
	return cmd
}

func newRegistryOrgDeleteCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "delete <slug>",
		Short: "Delete a registry organization",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, _, err := newClient()
			if err != nil {
				return err
			}
			slug := args[0]
			// The platform-provisioned default org is undeletable; refuse up
			// front (offline-friendly) rather than round-tripping to a 409.
			o, _, err := c.Registry().Orgs().Get(cmd.Context(), slug)
			if err != nil {
				return mapRegistryOrgGetError(err, slug)
			}
			if o.IsDefault {
				return fmt.Errorf("org %q is the default organization and cannot be deleted", slug)
			}
			if !flagYes {
				ok, err := confirm(cmd, fmt.Sprintf("Delete org %q? This cannot be undone.", slug))
				if err != nil {
					return err
				}
				if !ok {
					fmt.Fprintln(cmd.OutOrStdout(), "Aborted.")
					return nil
				}
			}
			if err := c.Registry().Orgs().Delete(cmd.Context(), slug); err != nil {
				if client.IsCode(err, codes.OrgCannotDeleteDefault) {
					return fmt.Errorf("org %q is the default organization and cannot be deleted", slug)
				}
				if client.IsCode(err, codes.OrgHasRepos) {
					return fmt.Errorf("org %q has repositories; delete them first: %w", slug, err)
				}
				return mapRegistryError(err)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Org %q deleted\n", slug)
			return nil
		},
	}
	return cmd
}

func printRegistryOrgDetail(tw *tabwriter.Writer, o *types.OrganizationResponse) {
	fmt.Fprintf(tw, "ID:\t%d\n", o.ID)
	fmt.Fprintf(tw, "Slug:\t%s\n", o.Slug)
	fmt.Fprintf(tw, "Display name:\t%s\n", o.DisplayName)
	fmt.Fprintf(tw, "Default:\t%t\n", o.IsDefault)
	fmt.Fprintf(tw, "Owner user id:\t%d\n", o.OwnerUserID)
	fmt.Fprintf(tw, "Auto-create repos:\t%t\n", o.RegistryAutoCreateRepos)
	fmt.Fprintf(tw, "Suspended:\t%s\n", formatOptionalTime(o.RegistrySuspendedAt, "no"))
	fmt.Fprintf(tw, "Created:\t%s\n", o.CreatedAt.Format(time.RFC3339))
	fmt.Fprintf(tw, "Updated:\t%s\n", o.UpdatedAt.Format(time.RFC3339))
}

// resolveOrgSlug returns explicit when set; otherwise the user's sole org or
// an error suggesting --org when zero or multiple orgs exist.
func resolveOrgSlug(ctx context.Context, c *client.Client, explicit string) (string, error) {
	if explicit != "" {
		return explicit, nil
	}
	orgs, err := c.Registry().Orgs().List(ctx)
	if err != nil {
		return "", mapRegistryError(err)
	}
	switch len(orgs) {
	case 0:
		return "", fmt.Errorf("no registry orgs found; create one with `kumo registry org create <slug>` or specify --org")
	case 1:
		return orgs[0].Slug, nil
	default:
		return "", fmt.Errorf("multiple registry orgs available; specify --org")
	}
}

// mapRegistryError translates common registry/org error codes into friendly
// messages while preserving the underlying error chain.
func mapRegistryError(err error) error {
	switch {
	case err == nil:
		return nil
	case client.IsCode(err, codes.RegistrySuspended):
		return fmt.Errorf("registry access is suspended for this org; check billing in the dashboard: %w", err)
	case client.IsCode(err, codes.AmbiguousName):
		return fmt.Errorf("ambiguous name; pass the exact slug/name to disambiguate: %w", err)
	}
	return err
}

func mapRegistryOrgGetError(err error, slug string) error {
	if client.IsCode(err, codes.OrgNotFound) || client.IsNotFound(err) {
		return fmt.Errorf("no org named %q", slug)
	}
	return mapRegistryError(err)
}

// shortDigest returns the first 12 hex chars of an OCI digest (after the
// "sha256:" prefix). Returns the input unchanged when it doesn't look like a
// sha256 digest.
func shortDigest(d string) string {
	trimmed := strings.TrimPrefix(d, "sha256:")
	if len(trimmed) >= 12 {
		return trimmed[:12]
	}
	return trimmed
}
