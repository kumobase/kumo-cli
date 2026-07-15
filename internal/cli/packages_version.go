package cli

import (
	"fmt"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/kumobase/kumo-go/client"
	"github.com/kumobase/kumo-go/codes"
	"github.com/kumobase/kumo-go/types"

	"github.com/kumobase/kumo-cli/internal/output"
)

func newPackagesVersionCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "version",
		Aliases: []string{"versions", "ver"},
		Short:   "Inspect and unpublish individual package versions",
	}
	cmd.AddCommand(
		newPackagesVersionGetCmd(),
		newPackagesVersionDeleteCmd(),
	)
	return cmd
}

func newPackagesVersionGetCmd() *cobra.Command {
	var orgSlug, format string
	cmd := &cobra.Command{
		Use:   "get <name> <version>",
		Short: "Show a single published version",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, s, err := newClient()
			if err != nil {
				return err
			}
			org, err := resolveOrgSlug(cmd.Context(), c, orgSlug)
			if err != nil {
				return err
			}
			name, version := args[0], args[1]
			fm, err := resolvePackageFormat(cmd.Context(), c, org, name, format)
			if err != nil {
				return err
			}
			v, err := c.Packages().Org(org).GetVersion(cmd.Context(), fm, name, version)
			if err != nil {
				return mapPackageVersionError(err, name, version)
			}
			return output.Print(cmd.OutOrStdout(), s.Output, v, func(tw *tabwriter.Writer) {
				printPackageVersionDetail(tw, org, fm, name, v)
			})
		},
	}
	addPackageFormatFlags(cmd, &orgSlug, &format)
	return cmd
}

func newPackagesVersionDeleteCmd() *cobra.Command {
	var orgSlug, format string
	cmd := &cobra.Command{
		Use:   "delete <name> <version>",
		Short: "Unpublish a single package version",
		Long: "Unpublish one version of a package.\n\n" +
			"The server soft-deletes and schedules a garbage-collection purge, so\n" +
			"this returns once deletion is scheduled — not once storage is\n" +
			"reclaimed. Deletes are not deduplicated by --idempotency-key.",
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, _, err := newClient()
			if err != nil {
				return err
			}
			org, err := resolveOrgSlug(cmd.Context(), c, orgSlug)
			if err != nil {
				return err
			}
			name, version := args[0], args[1]
			fm, err := resolvePackageFormat(cmd.Context(), c, org, name, format)
			if err != nil {
				return err
			}
			if !flagYes {
				ok, err := confirm(cmd, fmt.Sprintf("Unpublish %s %s@%s in org %q? This cannot be undone.", fm, name, version, org))
				if err != nil {
					return err
				}
				if !ok {
					return printAborted(cmd)
				}
			}
			if err := c.Packages().Org(org).DeleteVersion(cmd.Context(), fm, name, version, writeOpts("")...); err != nil {
				return mapPackageVersionError(err, name, version)
			}
			return printResult(cmd, output.ActionResult{
				Resource: "package-version", Action: "delete", Status: "scheduled",
				Message: fmt.Sprintf("Package %s %s@%s unpublish scheduled", fm, name, version),
			})
		},
	}
	addPackageFormatFlags(cmd, &orgSlug, &format)
	return cmd
}

func printPackageVersionDetail(tw *tabwriter.Writer, org string, fm types.PackageFormat, name string, v *types.PackageVersionResponse) {
	fmt.Fprintf(tw, "Name:\t%s\n", name)
	fmt.Fprintf(tw, "Format:\t%s\n", fm)
	fmt.Fprintf(tw, "Org:\t%s\n", org)
	fmt.Fprintf(tw, "Version:\t%s\n", v.Version)
	fmt.Fprintf(tw, "Size:\t%s\n", humanBytes(v.SizeBytes))
	fmt.Fprintf(tw, "Published:\t%s\n", v.PublishedAt.Format(time.RFC3339))
	fmt.Fprintf(tw, "Deprecated:\t%s\n", derefOr(v.Deprecated, "-"))
	// Shasum and Integrity are npm dist metadata; empty for every other format.
	fmt.Fprintf(tw, "Shasum:\t%s\n", orDash(v.Shasum))
	fmt.Fprintf(tw, "Integrity:\t%s\n", orDash(v.Integrity))
}

func mapPackageVersionError(err error, name, version string) error {
	if client.IsCode(err, codes.PackageVersionNotFound) {
		return friendlyf(err, "package %q has no version %q", name, version)
	}
	return mapPackageRefError(err, name)
}
