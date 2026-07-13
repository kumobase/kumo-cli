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

func newVolumeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "volume",
		Aliases: []string{"volumes", "vol"},
		Short:   "Manage persistent volumes",
	}
	cmd.AddCommand(
		newVolumeListCmd(),
		newVolumeGetCmd(),
		newVolumePlansCmd(),
		newVolumeCreateCmd(),
		newVolumeDeleteCmd(),
		newVolumeAttachCmd(),
		newVolumeDetachCmd(),
		newVolumeResizeCmd(),
	)
	return cmd
}

// newVolumePlansCmd lists the storage-tier catalogue so users can discover
// valid --tier slugs for `volume create`. This endpoint is server-paginated,
// so a page footer is shown when more than one page exists.
func newVolumePlansCmd() *cobra.Command {
	var (
		page, pageSize int
		sort           string
	)
	cmd := &cobra.Command{
		Use:   "plans",
		Short: "List available storage tiers (volume plans)",
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
			if sort != "" {
				opts = append(opts, client.WithSort(sort, "asc"))
			}
			plans, meta, err := c.Volumes().ListPlans(cmd.Context(), opts...)
			if err != nil {
				return err
			}
			return output.Print(cmd.OutOrStdout(), s.Output, plans, func(tw *tabwriter.Writer) {
				fmt.Fprintln(tw, "SLUG\tNAME\tMIN GB\tMAX GB\tPRICE/GB-HR")
				for _, p := range plans {
					fmt.Fprintf(tw, "%s\t%s\t%d\t%d\t%s\n",
						p.Slug, p.Name, p.MinSizeGB, p.MaxSizeGB, p.PricePerGBHour)
				}
				if meta != nil && meta.TotalPages > 1 {
					fmt.Fprintf(tw, "\nPage %d/%d (%d items) — use --page to see more\n",
						meta.Page, meta.TotalPages, meta.TotalItems)
				}
			})
		},
	}
	f := cmd.Flags()
	f.IntVar(&page, "page", 0, "page number (1-based)")
	f.IntVar(&pageSize, "page-size", 0, "items per page (max 100)")
	f.StringVar(&sort, "sort", "", "sort column")
	return cmd
}

func newVolumeListCmd() *cobra.Command {
	var (
		page, pageSize   int
		sortCol, sortDir string
		status           string
		appName          string
		attached         string
	)
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List volumes",
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
			if sortCol != "" {
				opts = append(opts, client.WithSort(sortCol, sortDir))
			}
			if status != "" {
				opts = append(opts, client.WithExtraQuery("status", status))
			}
			if attached != "" {
				opts = append(opts, client.WithExtraQuery("attached", attached))
			}
			if appName != "" {
				id, _, _, err := resolveAppRef(cmd.Context(), c, appName)
				if err != nil {
					return err
				}
				opts = append(opts, client.WithExtraQuery("app_id", fmt.Sprintf("%d", id)))
			}
			vols, _, err := c.Volumes().List(cmd.Context(), opts...)
			if err != nil {
				return err
			}
			return output.Print(cmd.OutOrStdout(), s.Output, vols, func(tw *tabwriter.Writer) {
				fmt.Fprintln(tw, "ID\tNAME\tTIER\tSIZE\tSTATUS\tAPP\tMOUNT\tCREATED")
				for _, v := range vols {
					fmt.Fprintf(tw, "%d\t%s\t%s\t%d GB\t%s\t%s\t%s\t%s\n",
						v.ID, v.Name, v.StorageTier.Slug, v.SizeGB, v.Status,
						volumeAppLabel(&v), volumeMountLabel(&v),
						v.CreatedAt.Format("2006-01-02"))
				}
			})
		},
	}
	f := cmd.Flags()
	f.IntVar(&page, "page", 0, "page number (1-based)")
	f.IntVar(&pageSize, "page-size", 0, "items per page (max 100)")
	f.StringVar(&sortCol, "sort", "", "sort column")
	f.StringVar(&sortDir, "sort-order", "desc", "sort order: asc or desc")
	f.StringVar(&status, "status", "", "filter by status")
	f.StringVar(&appName, "app", "", "filter by attached app name")
	f.StringVar(&attached, "attached", "", "filter by attachment state (true|false)")
	return cmd
}

func newVolumeGetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "get <name>",
		Short: "Show volume detail",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, s, err := newClient()
			if err != nil {
				return err
			}
			_, v, err := resolveVolumeRef(cmd.Context(), c, args[0])
			if err != nil {
				return err
			}
			return output.Print(cmd.OutOrStdout(), s.Output, v, func(tw *tabwriter.Writer) {
				printVolumeDetail(tw, v)
			})
		},
	}
}

// resolveVolumeRef looks up a volume by its (per-user unique) name.
func resolveVolumeRef(ctx context.Context, c *client.Client, name string) (uint, *types.VolumeResponse, error) {
	if strings.TrimSpace(name) == "" {
		return 0, nil, fmt.Errorf("volume name is required")
	}
	v, _, err := c.Volumes().GetByName(ctx, name)
	if err != nil {
		if client.IsCode(err, codes.AmbiguousName) {
			return 0, nil, fmt.Errorf("multiple volumes named %q exist; rename one (kumo volume list) to disambiguate", name)
		}
		if client.IsNotFound(err) {
			return 0, nil, fmt.Errorf("no volume named %q", name)
		}
		return 0, nil, err
	}
	return v.ID, v, nil
}

func volumeAppLabel(v *types.VolumeResponse) string {
	if v.AppName != nil && *v.AppName != "" {
		return *v.AppName
	}
	if v.AppID != nil {
		return fmt.Sprintf("#%d", *v.AppID)
	}
	return "-"
}

func volumeMountLabel(v *types.VolumeResponse) string {
	if v.MountPath == "" {
		return "-"
	}
	return v.MountPath
}

func printVolumeDetail(tw *tabwriter.Writer, v *types.VolumeResponse) {
	fmt.Fprintf(tw, "ID:\t%d\n", v.ID)
	fmt.Fprintf(tw, "Name:\t%s\n", v.Name)
	fmt.Fprintf(tw, "Status:\t%s\n", v.Status)
	fmt.Fprintf(tw, "Tier:\t%s (%s)\n", v.StorageTier.Slug, v.StorageTier.Name)
	fmt.Fprintf(tw, "Size:\t%d GB\n", v.SizeGB)
	fmt.Fprintf(tw, "Mount path:\t%s\n", volumeMountLabel(v))
	if v.AppID != nil {
		name := "-"
		if v.AppName != nil {
			name = *v.AppName
		}
		fmt.Fprintf(tw, "App:\t%s (id %d)\n", name, *v.AppID)
	} else {
		fmt.Fprintf(tw, "App:\t-\n")
	}
	if v.ErrorMessage != nil && *v.ErrorMessage != "" {
		fmt.Fprintf(tw, "Error:\t%s\n", *v.ErrorMessage)
	}
	if v.LastResizeError != nil && *v.LastResizeError != "" {
		fmt.Fprintf(tw, "Last resize error:\t%s\n", *v.LastResizeError)
	}
	if v.LastResizeAt != nil {
		fmt.Fprintf(tw, "Last resize at:\t%s\n", v.LastResizeAt.Format(time.RFC3339))
	}
	fmt.Fprintf(tw, "Created:\t%s\n", v.CreatedAt.Format(time.RFC3339))
	fmt.Fprintf(tw, "Updated:\t%s\n", v.UpdatedAt.Format(time.RFC3339))
}

// mapVolumeBusyError translates the transient busy codes into a uniform hint.
func mapVolumeBusyError(err error, status string) error {
	if client.IsCode(err, codes.VolumeResizing) ||
		client.IsCode(err, codes.VolumeCreating) ||
		client.IsCode(err, codes.VolumeDeleting) {
		return fmt.Errorf("volume is busy (status %s); try again shortly: %w", status, err)
	}
	return err
}
