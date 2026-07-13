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

func newVPSCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "vps",
		Aliases: []string{"vpses"},
		Short:   "Manage Kumo VPS instances",
	}
	cmd.AddCommand(
		newVPSListCmd(),
		newVPSGetCmd(),
		newVPSRegionsCmd(),
		newVPSPlansCmd(),
		newVPSRentCmd(),
		newVPSRenameCmd(),
		newVPSCancelCmd(),
		newVPSRenewCmd(),
		newVPSStartCmd(),
		newVPSStopCmd(),
		newVPSRebootCmd(),
		newVPSReinstallCmd(),
		newVPSPasswordCmd(),
		newVPSResetPasswordCmd(),
		newVPSSSHCmd(),
	)
	return cmd
}

func newVPSListCmd() *cobra.Command {
	var (
		status, region, provider string
		page, pageSize           int
	)
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List VPS instances",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			c, s, err := newClient()
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
			if status != "" {
				opts = append(opts, client.WithExtraQuery("status", status))
			}
			if region != "" {
				opts = append(opts, client.WithExtraQuery("region", region))
			}
			if provider != "" {
				opts = append(opts, client.WithExtraQuery("provider_name", provider))
			}
			servers, _, err := c.VPS().ListServers(cmd.Context(), opts...)
			if err != nil {
				return err
			}
			return output.Print(cmd.OutOrStdout(), s.Output, servers, func(tw *tabwriter.Writer) {
				fmt.Fprintln(tw, "ID\tNAME\tSTATUS\tPROVIDER\tREGION\tIP\tEXPIRES")
				for _, v := range servers {
					fmt.Fprintf(tw, "%d\t%s\t%s\t%s\t%s\t%s\t%s\n",
						v.ID, vpsDisplayName(&v), v.Status, v.DisplayProvider,
						v.RegionID, valOr(v.IPAddress, "-"),
						formatVPSDate(v.ExpiresAt))
				}
			})
		},
	}
	f := cmd.Flags()
	f.StringVar(&status, "status", "", "filter by status (provisioning, running, stopped, expired)")
	f.StringVar(&region, "region", "", "filter by region id")
	f.StringVar(&provider, "provider", "", "filter by provider name")
	f.IntVar(&page, "page", 0, "page number (1-based)")
	f.IntVar(&pageSize, "page-size", 0, "items per page (max 100)")
	return cmd
}

func newVPSGetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "get <name>",
		Short: "Show VPS instance detail",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, s, err := newClient()
			if err != nil {
				return err
			}
			_, v, err := resolveVPSRef(cmd.Context(), c, args[0])
			if err != nil {
				return err
			}
			return output.Print(cmd.OutOrStdout(), s.Output, v, func(tw *tabwriter.Writer) {
				printVPSDetail(tw, v)
			})
		},
	}
}

func newVPSRegionsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "regions",
		Short: "List VPS regions",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			c, s, err := newClient()
			if err != nil {
				return err
			}
			regions, err := c.VPS().ListRegions(cmd.Context())
			if err != nil {
				return err
			}
			return output.Print(cmd.OutOrStdout(), s.Output, regions, func(tw *tabwriter.Writer) {
				fmt.Fprintln(tw, "ID\tNAME")
				for _, r := range regions {
					fmt.Fprintf(tw, "%s\t%s\n", r.ID, r.Name)
				}
			})
		},
	}
}

func newVPSPlansCmd() *cobra.Command {
	var (
		region, provider, sort string
	)
	cmd := &cobra.Command{
		Use:   "plans",
		Short: "List VPS plans for a region",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if region == "" {
				return fmt.Errorf("--region is required")
			}
			c, s, err := newClient()
			if err != nil {
				return err
			}
			opts := []client.ListOption{client.WithExtraQuery("region", region)}
			if provider != "" {
				opts = append(opts, client.WithExtraQuery("provider_name", provider))
			}
			if sort != "" {
				opts = append(opts, client.WithSort(sort, "asc"))
			}
			plans, err := c.VPS().ListPlans(cmd.Context(), opts...)
			if err != nil {
				return mapVPSCatalogueError(err)
			}
			return output.Print(cmd.OutOrStdout(), s.Output, plans, func(tw *tabwriter.Writer) {
				fmt.Fprintln(tw, "PROVIDER\tPLAN ID\tNAME\tCPU\tMEMORY\tDISK\tPRICE")
				for _, p := range plans {
					fmt.Fprintf(tw, "%s\t%s\t%s\t%d\t%d\t%d\t%s\n",
						p.ProviderName, p.ExternalPlanID, p.Name,
						p.CPU, p.Memory, p.Disk, p.SellingPrice)
				}
			})
		},
	}
	f := cmd.Flags()
	f.StringVar(&region, "region", "", "region id (required)")
	f.StringVar(&provider, "provider", "", "filter by provider name")
	f.StringVar(&sort, "sort", "", "sort column")
	return cmd
}

// resolveVPSRef looks up a VPS instance by its display_name.
func resolveVPSRef(ctx context.Context, c *client.Client, name string) (uint, *types.VPSServerResponse, error) {
	if strings.TrimSpace(name) == "" {
		return 0, nil, fmt.Errorf("vps name is required")
	}
	v, err := c.VPS().GetServerByName(ctx, name)
	if err != nil {
		if client.IsCode(err, codes.AmbiguousName) {
			return 0, nil, fmt.Errorf("multiple vps instances named %q exist; rename one (kumo vps list) to disambiguate", name)
		}
		if client.IsCode(err, codes.InstanceNotFound) || client.IsNotFound(err) {
			return 0, nil, fmt.Errorf("no vps named %q", name)
		}
		return 0, nil, err
	}
	return v.ID, v, nil
}

func vpsDisplayName(v *types.VPSServerResponse) string {
	if v.DisplayName != "" {
		return v.DisplayName
	}
	return fmt.Sprintf("(id %d)", v.ID)
}

func valOr(s, fallback string) string {
	if s == "" {
		return fallback
	}
	return s
}

// formatVPSDate renders an RFC3339 timestamp as YYYY-MM-DD; falls back to the
// raw string when parsing fails or returns "-" when empty.
func formatVPSDate(ts string) string {
	if ts == "" {
		return "-"
	}
	if t, err := time.Parse(time.RFC3339, ts); err == nil {
		return t.Format("2006-01-02")
	}
	return ts
}

// formatVPSTime renders an RFC3339 timestamp as RFC3339; falls back to the
// raw string when parsing fails or returns "-" when empty.
func formatVPSTime(ts string) string {
	if ts == "" {
		return "-"
	}
	if t, err := time.Parse(time.RFC3339, ts); err == nil {
		return t.Format(time.RFC3339)
	}
	return ts
}

func printVPSDetail(tw *tabwriter.Writer, v *types.VPSServerResponse) {
	fmt.Fprintf(tw, "ID:\t%d\n", v.ID)
	fmt.Fprintf(tw, "Name:\t%s\n", vpsDisplayName(v))
	fmt.Fprintf(tw, "Status:\t%s\n", v.Status)
	fmt.Fprintf(tw, "Provider:\t%s\n", v.DisplayProvider)
	fmt.Fprintf(tw, "Region:\t%s\n", v.RegionID)
	if v.OS != "" {
		fmt.Fprintf(tw, "OS:\t%s\n", v.OS)
	}
	fmt.Fprintf(tw, "IP:\t%s\n", valOr(v.IPAddress, "-"))
	fmt.Fprintf(tw, "SSH port:\t%d\n", v.SSHPort)
	fmt.Fprintf(tw, "Auto-renew:\t%t\n", v.AutoRenew)
	fmt.Fprintf(tw, "Expires:\t%s\n", formatVPSTime(v.ExpiresAt))
	fmt.Fprintf(tw, "SSH setup:\t%t\n", v.SSHSetupCompleted)
	if v.ActionStatus != "" {
		fmt.Fprintf(tw, "Action status:\t%s (%s)\n", v.ActionStatus, formatVPSTime(v.ActionStatusUpdatedAt))
	} else {
		fmt.Fprintf(tw, "Action status:\tidle\n")
	}
	if v.ActionError != "" {
		fmt.Fprintf(tw, "Action error:\t%s\n", v.ActionError)
	}
	if v.Plan != nil {
		fmt.Fprintf(tw, "Plan:\t%s (cpu=%d, mem=%d, disk=%d, price=%s)\n",
			v.Plan.Name, v.Plan.CPU, v.Plan.Memory, v.Plan.Disk, v.Plan.SellingPrice)
	}
	fmt.Fprintf(tw, "Created:\t%s\n", formatVPSTime(v.CreatedAt))
}

// mapVPSCatalogueError maps region/plan catalogue errors to friendly messages.
func mapVPSCatalogueError(err error) error {
	switch {
	case err == nil:
		return nil
	case client.IsCode(err, codes.MissingRegion):
		return fmt.Errorf("--region is required")
	case client.IsCode(err, codes.InvalidRegion):
		return fmt.Errorf("invalid region; list available regions with `kumo vps regions`: %w", err)
	}
	return err
}

// mapVPSActionError maps action-time errors to friendly messages.
func mapVPSActionError(err error, name, status string) error {
	switch {
	case err == nil:
		return nil
	case client.IsCode(err, codes.InstanceExpired):
		return fmt.Errorf("server expired; renew with `kumo vps renew %s`: %w", name, err)
	case client.IsCode(err, codes.InstanceNotRunning):
		return fmt.Errorf("server is not running (status: %s): %w", status, err)
	case client.IsCode(err, codes.ActionInProgress):
		return fmt.Errorf("another action is in progress; wait for it to finish: %w", err)
	case client.IsCode(err, codes.SSHNotReady):
		return fmt.Errorf("SSH not yet provisioned: %w", err)
	}
	return err
}

// mapVPSRentError maps rent/renew-time errors to friendly messages.
func mapVPSRentError(err error) error {
	switch {
	case err == nil:
		return nil
	case client.IsCode(err, codes.InsufficientBalance):
		return fmt.Errorf("insufficient balance; top up in the dashboard: %w", err)
	case client.IsCode(err, codes.ProviderDisabled):
		return fmt.Errorf("provider disabled; see `kumo vps plans --region <region>`: %w", err)
	case client.IsCode(err, codes.PlanDisabled):
		return fmt.Errorf("plan disabled; see `kumo vps plans --region <region>`: %w", err)
	case client.IsCode(err, codes.MissingRegion):
		return fmt.Errorf("--region is required")
	case client.IsCode(err, codes.InvalidRegion):
		return fmt.Errorf("invalid region; list available regions with `kumo vps regions`: %w", err)
	}
	return err
}
