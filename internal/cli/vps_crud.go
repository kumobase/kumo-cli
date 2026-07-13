package cli

import (
	"fmt"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/kumobase/kumo-go/client"
	"github.com/kumobase/kumo-go/codes"
	"github.com/kumobase/kumo-go/types"

	"github.com/kumobase/kumo-cli/internal/output"
)

func newVPSRentCmd() *cobra.Command {
	var (
		name, provider, region, plan string
	)
	cmd := &cobra.Command{
		Use:   "rent",
		Short: "Rent a new VPS instance",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if name == "" {
				return fmt.Errorf("--name is required")
			}
			if provider == "" {
				return fmt.Errorf("--provider is required")
			}
			if region == "" {
				return fmt.Errorf("--region is required")
			}
			if plan == "" {
				return fmt.Errorf("--plan is required")
			}
			c, s, err := newClient()
			if err != nil {
				return err
			}
			req := &types.RentServerRequest{
				Provider: provider,
				Region:   region,
				Plan:     plan,
				Name:     name,
			}
			v, err := c.VPS().RentServer(cmd.Context(), req)
			if err != nil {
				return mapVPSRentError(err)
			}
			return output.Print(cmd.OutOrStdout(), s.Output, v, func(tw *tabwriter.Writer) {
				fmt.Fprintf(tw, "Rented vps %q (id %d, status %s)\n", vpsDisplayName(v), v.ID, v.Status)
			})
		},
	}
	f := cmd.Flags()
	f.StringVar(&name, "name", "", "display name (required)")
	f.StringVar(&provider, "provider", "", "provider name (required)")
	f.StringVar(&region, "region", "", "region id (required)")
	f.StringVar(&plan, "plan", "", "plan id (required)")
	return cmd
}

func newVPSRenameCmd() *cobra.Command {
	var newName string
	cmd := &cobra.Command{
		Use:   "rename <name>",
		Short: "Rename a VPS instance",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if newName == "" {
				return fmt.Errorf("--new-name is required")
			}
			c, _, err := newClient()
			if err != nil {
				return err
			}
			id, _, err := resolveVPSRef(cmd.Context(), c, args[0])
			if err != nil {
				return err
			}
			if err := c.VPS().UpdateServerName(cmd.Context(), id, newName); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Renamed vps %d to %q\n", id, newName)
			return nil
		},
	}
	cmd.Flags().StringVar(&newName, "new-name", "", "new display name (required)")
	return cmd
}

func newVPSCancelCmd() *cobra.Command {
	var yes bool
	cmd := &cobra.Command{
		Use:   "cancel <name>",
		Short: "Cancel auto-renewal; server runs until ExpiresAt",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, _, err := newClient()
			if err != nil {
				return err
			}
			id, v, err := resolveVPSRef(cmd.Context(), c, args[0])
			if err != nil {
				return err
			}
			if !yes {
				ok, err := confirm(cmd, fmt.Sprintf("Cancel auto-renew on vps %q (id %d)?", v.DisplayName, id))
				if err != nil {
					return err
				}
				if !ok {
					fmt.Fprintln(cmd.OutOrStdout(), "Aborted.")
					return nil
				}
			}
			if err := c.VPS().CancelSubscription(cmd.Context(), id); err != nil {
				if client.IsCode(err, codes.AutoRenewAlreadyCancelled) {
					fmt.Fprintf(cmd.OutOrStdout(), "Auto-renewal already cancelled; server runs until %s\n", formatVPSDate(v.ExpiresAt))
					return nil
				}
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Auto-renewal cancelled; server runs until ExpiresAt: %s\n", formatVPSDate(v.ExpiresAt))
			return nil
		},
	}
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "skip the confirmation prompt")
	return cmd
}

func newVPSRenewCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "renew <name>",
		Short: "Renew (extend) a VPS subscription for one more period",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, _, err := newClient()
			if err != nil {
				return err
			}
			id, _, err := resolveVPSRef(cmd.Context(), c, args[0])
			if err != nil {
				return err
			}
			if err := c.VPS().Renew(cmd.Context(), id); err != nil {
				return mapVPSRentError(err)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Renewed vps %d\n", id)
			return nil
		},
	}
}
