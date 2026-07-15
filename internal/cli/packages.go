package cli

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/kumobase/kumo-go/client"
	"github.com/kumobase/kumo-go/codes"
	"github.com/kumobase/kumo-go/types"

	"github.com/kumobase/kumo-cli/internal/output"
)

// Kumo Packages is a multi-format language-package registry. The CLI covers the
// management API only — browsing and unpublishing. Publishing goes through the
// native tool (npm publish, mvn deploy, twine upload) pointed at Kumo, because
// each ecosystem dictates its own wire protocol.

// packageFormats are the ecosystems a package can belong to. Format is part of a
// package's identity, not a filter: the server enforces uniqueness on
// (organization, format, normalized name), so one org can hold both an npm and a
// PyPI package called "utils". Every call addressing a single package needs one.
var packageFormats = []types.PackageFormat{
	types.PackageFormatNPM,
	types.PackageFormatMaven,
	types.PackageFormatPyPI,
	types.PackageFormatNuGet,
	types.PackageFormatRubyGems,
}

// packageSortColumns are the only columns the list endpoint honours. Anything
// else is silently ignored server-side and the results come back sorted by
// updated_at, so the CLI rejects them rather than return a table the user didn't
// ask for.
var packageSortColumns = []string{"name", "created_at"}

// Local sentinels for failures resolved client-side, so exitCodeFor can classify
// them the same way it classifies their server-side equivalents.
var (
	errPackageNotFound = errors.New("package not found")
	errAmbiguousFormat = errors.New("ambiguous package format")
)

func newPackagesCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "packages",
		Aliases: []string{"package", "pkg", "pkgs"},
		Short:   "Manage Kumo Packages (npm, maven, pypi, nuget, rubygems)",
		Long: "Browse and unpublish packages hosted on Kumo.\n\n" +
			"Publishing is done with the native tool for each ecosystem\n" +
			"(npm publish, mvn deploy, twine upload, …) pointed at Kumo —\n" +
			"the CLI covers the management surface.",
	}
	cmd.AddCommand(
		newPackagesPlansCmd(),
		newPackagesListCmd(),
		newPackagesGetCmd(),
		newPackagesDeleteCmd(),
		newPackagesVersionCmd(),
	)
	return cmd
}

// newPackagesPlansCmd lists the public Kumo Packages billing catalogue. No org
// is required — the catalogue is a public price list.
func newPackagesPlansCmd() *cobra.Command {
	var sort, sortOrder string
	cmd := &cobra.Command{
		Use:   "plans",
		Short: "List Kumo Packages billing plans",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateSortOrder(sortOrder); err != nil {
				return err
			}
			c, s, err := newClient()
			if err != nil {
				return err
			}
			var opts []client.ListOption
			if sort != "" {
				opts = append(opts, client.WithSort(sort, sortOrder))
			}
			plans, err := c.Packages().ListPlans(cmd.Context(), opts...)
			if err != nil {
				return mapPackagesError(err)
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
	cmd.Flags().StringVar(&sortOrder, "sort-order", "asc", "sort direction: asc or desc")
	return cmd
}

func newPackagesListCmd() *cobra.Command {
	var (
		orgSlug          string
		page, pageSize   int
		sortCol, sortDir string
	)
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List packages in an organization",
		Long: "List packages in an organization. Results span every format;\n" +
			"the FORMAT column says which. Sort by name or created_at.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateSortOrder(sortDir); err != nil {
				return err
			}
			if err := validatePackageSort(sortCol); err != nil {
				return err
			}
			c, s, err := newClient()
			if err != nil {
				return err
			}
			org, err := resolveOrgSlug(cmd.Context(), c, orgSlug)
			if err != nil {
				return err
			}
			var opts []client.ListOption
			if page > 0 {
				opts = append(opts, client.WithPage(page))
			}
			if pageSize > 0 {
				opts = append(opts, client.WithPageSize(pageSize))
			}
			if sortCol != "" {
				opts = append(opts, client.WithSort(sortCol, sortDir))
			}
			pkgs, meta, err := c.Packages().Org(org).List(cmd.Context(), opts...)
			if err != nil {
				return mapPackagesError(err)
			}
			return output.PrintList(cmd.OutOrStdout(), s.Output, pkgs, meta, func(tw *tabwriter.Writer) {
				fmt.Fprintln(tw, "NAME\tFORMAT\tLATEST\tVERSIONS\tUPDATED")
				for _, p := range pkgs {
					fmt.Fprintf(tw, "%s\t%s\t%s\t%d\t%s\n",
						p.Name, p.Format, orDash(p.LatestVersion), p.VersionCount,
						p.UpdatedAt.Format("2006-01-02"))
				}
			})
		},
	}
	f := cmd.Flags()
	f.StringVar(&orgSlug, "org", "", "organization slug (defaults to your sole org)")
	f.IntVar(&page, "page", 0, "page number (1-based)")
	f.IntVar(&pageSize, "page-size", 0, "items per page (max 100)")
	f.StringVar(&sortCol, "sort", "", "sort column: name or created_at")
	f.StringVar(&sortDir, "sort-order", "desc", "sort order: asc or desc")
	return cmd
}

func newPackagesGetCmd() *cobra.Command {
	var orgSlug, format string
	cmd := &cobra.Command{
		Use:   "get <name>",
		Short: "Show package detail and its versions",
		Long: "Show a package and every published version.\n\n" +
			"Scoped npm names (@acme/utils) and Maven coordinates (com.acme:lib)\n" +
			"are passed through as-is. Without --format the package is looked up\n" +
			"by name; pass --format when the name exists in several ecosystems.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, s, err := newClient()
			if err != nil {
				return err
			}
			org, err := resolveOrgSlug(cmd.Context(), c, orgSlug)
			if err != nil {
				return err
			}
			name := args[0]
			fm, err := resolvePackageFormat(cmd.Context(), c, org, name, format)
			if err != nil {
				return err
			}
			d, _, err := c.Packages().Org(org).Get(cmd.Context(), fm, name)
			if err != nil {
				return mapPackageRefError(err, name)
			}
			return output.Print(cmd.OutOrStdout(), s.Output, d, func(tw *tabwriter.Writer) {
				printPackageDetail(tw, org, d)
			})
		},
	}
	addPackageFormatFlags(cmd, &orgSlug, &format)
	return cmd
}

func newPackagesDeleteCmd() *cobra.Command {
	var orgSlug, format string
	cmd := &cobra.Command{
		Use:   "delete <name>",
		Short: "Unpublish a package and all its versions",
		Long: "Unpublish a package and every version of it.\n\n" +
			"The server soft-deletes and schedules a garbage-collection purge, so\n" +
			"this returns once deletion is scheduled — not once storage is\n" +
			"reclaimed. Deletes are not deduplicated by --idempotency-key.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, _, err := newClient()
			if err != nil {
				return err
			}
			org, err := resolveOrgSlug(cmd.Context(), c, orgSlug)
			if err != nil {
				return err
			}
			name := args[0]
			fm, err := resolvePackageFormat(cmd.Context(), c, org, name, format)
			if err != nil {
				return err
			}
			if !flagYes {
				ok, err := confirm(cmd, fmt.Sprintf("Unpublish %s package %q in org %q? This cannot be undone.", fm, name, org))
				if err != nil {
					return err
				}
				if !ok {
					return printAborted(cmd)
				}
			}
			if err := c.Packages().Org(org).Delete(cmd.Context(), fm, name, writeOpts("")...); err != nil {
				return mapPackageRefError(err, name)
			}
			return printResult(cmd, output.ActionResult{
				Resource: "package", Action: "delete", Status: "scheduled",
				Message: fmt.Sprintf("Package %s %q unpublish scheduled", fm, name),
			})
		},
	}
	addPackageFormatFlags(cmd, &orgSlug, &format)
	return cmd
}

// addPackageFormatFlags binds the --org/--format pair shared by every command
// that addresses a single package.
func addPackageFormatFlags(cmd *cobra.Command, orgSlug, format *string) {
	f := cmd.Flags()
	f.StringVar(orgSlug, "org", "", "organization slug (defaults to your sole org)")
	f.StringVar(format, "format", "", "package format: "+packageFormatList()+" (inferred from the name when unambiguous)")
}

func packageFormatList() string {
	names := make([]string, len(packageFormats))
	for i, f := range packageFormats {
		names[i] = string(f)
	}
	return strings.Join(names, ", ")
}

// parsePackageFormat validates a --format value locally so an unknown format
// never round-trips to a PACKAGE_INVALID_FORMAT.
func parsePackageFormat(s string) (types.PackageFormat, error) {
	got := types.PackageFormat(strings.ToLower(strings.TrimSpace(s)))
	for _, f := range packageFormats {
		if got == f {
			return f, nil
		}
	}
	return "", usageError{err: fmt.Errorf("invalid --format %q (use one of: %s)", s, packageFormatList())}
}

// validatePackageSort rejects sort columns the server does not honour.
func validatePackageSort(col string) error {
	if col == "" || slices.Contains(packageSortColumns, col) {
		return nil
	}
	return usageError{err: fmt.Errorf("invalid --sort %q (use one of: %s)",
		col, strings.Join(packageSortColumns, ", "))}
}

// resolvePackageFormat returns explicit when set — with no network call —
// otherwise infers the format by finding the name in the org's package list.
// Ambiguity is a real possibility, not a corner case: uniqueness is per
// (org, format, name), so the same name can exist under several formats.
func resolvePackageFormat(ctx context.Context, c *client.Client, org, name, explicit string) (types.PackageFormat, error) {
	if explicit != "" {
		return parsePackageFormat(explicit)
	}
	pkgs, err := listAllPackages(ctx, c, org)
	if err != nil {
		return "", err
	}
	var found []string
	for _, p := range pkgs {
		if p.Name == name {
			found = append(found, p.Format)
		}
	}
	switch len(found) {
	case 0:
		return "", friendlyf(errPackageNotFound, "no package named %q in org %q", name, org)
	case 1:
		return types.PackageFormat(found[0]), nil
	default:
		sort.Strings(found)
		return "", friendlyf(errAmbiguousFormat,
			"package %q exists as %s in org %q; specify --format",
			name, strings.Join(found, ", "), org)
	}
}

// listAllPackages walks every page of the org's package list. The list route is
// paginated and clamped to 100 server-side, so format inference has to page or
// it would silently miss packages in a large org.
func listAllPackages(ctx context.Context, c *client.Client, org string) ([]types.PackageResponse, error) {
	var all []types.PackageResponse
	for page := 1; ; page++ {
		pkgs, meta, err := c.Packages().Org(org).List(ctx,
			client.WithPage(page), client.WithPageSize(100))
		if err != nil {
			return nil, mapPackagesError(err)
		}
		all = append(all, pkgs...)
		// Stop on the last page, and defensively on an empty one so a server
		// that under-reports TotalPages can't spin this loop forever.
		if len(pkgs) == 0 || meta == nil || page >= meta.TotalPages {
			return all, nil
		}
	}
}

func printPackageDetail(tw *tabwriter.Writer, org string, d *types.PackageDetailResponse) {
	p := d.Package
	fmt.Fprintf(tw, "ID:\t%d\n", p.ID)
	fmt.Fprintf(tw, "Name:\t%s\n", p.Name)
	fmt.Fprintf(tw, "Format:\t%s\n", p.Format)
	fmt.Fprintf(tw, "Org:\t%s\n", org)
	fmt.Fprintf(tw, "Latest version:\t%s\n", orDash(p.LatestVersion))
	fmt.Fprintf(tw, "Versions:\t%d\n", p.VersionCount)
	fmt.Fprintf(tw, "Created:\t%s\n", p.CreatedAt.Format(time.RFC3339))
	fmt.Fprintf(tw, "Updated:\t%s\n", p.UpdatedAt.Format(time.RFC3339))

	// DistTags is npm-only — an empty map for every other format.
	if len(d.DistTags) > 0 {
		tags := make([]string, 0, len(d.DistTags))
		for t := range d.DistTags {
			tags = append(tags, t)
		}
		sort.Strings(tags)
		fmt.Fprintln(tw, "\nDIST TAG\tVERSION")
		for _, t := range tags {
			fmt.Fprintf(tw, "%s\t%s\n", t, d.DistTags[t])
		}
	}

	if len(d.Versions) > 0 {
		fmt.Fprintln(tw, "\nVERSION\tSIZE\tPUBLISHED\tDEPRECATED")
		for _, v := range d.Versions {
			fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n",
				v.Version, humanBytes(v.SizeBytes),
				v.PublishedAt.Format("2006-01-02"),
				derefOr(v.Deprecated, "-"))
		}
	}
}

// mapPackagesError translates Kumo Packages error codes into actionable
// messages while preserving the chain so exitCodeFor still classifies them.
func mapPackagesError(err error) error {
	switch {
	case err == nil:
		return nil
	case client.IsCode(err, codes.PackageOrgSuspended):
		return fmt.Errorf("packages access is suspended for this org; check billing in the dashboard: %w", err)
	case client.IsCode(err, codes.PackageUnpublishForbidden):
		return fmt.Errorf("this version cannot be unpublished; the unpublish window has passed or a policy forbids it: %w", err)
	case client.IsCode(err, codes.PackageTagProtected):
		return fmt.Errorf("that dist-tag is protected and cannot be changed or removed: %w", err)
	case client.IsCode(err, codes.PackageForbidden):
		return fmt.Errorf("your credentials do not grant packages access to this org: %w", err)
	case client.IsCode(err, codes.AmbiguousName):
		return fmt.Errorf("ambiguous name; pass the exact package name to disambiguate: %w", err)
	}
	return err
}

func mapPackageRefError(err error, name string) error {
	if client.IsCode(err, codes.PackageNotFound) {
		return friendlyf(err, "no package named %q", name)
	}
	return mapPackagesError(err)
}
