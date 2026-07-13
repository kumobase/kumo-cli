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

func newVPSRentCmd() *cobra.Command {
	var (
		name, provider, region, plan string
		wait                         bool
		timeout                      time.Duration
	)
	cmd := &cobra.Command{
		Use:   "rent",
		Short: "Rent a new VPS instance",
		Example: `  # List plans for a region first, then rent and wait until it's running
  kumo vps plans --region sg-singapore
  kumo vps rent --name web1 --provider zeabur --region sg-singapore --plan sg-1c-1g --wait`,
		Args: cobra.NoArgs,
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
			v, err := c.VPS().RentServer(cmd.Context(), req, writeOpts("")...)
			if err != nil {
				return mapVPSRentError(err)
			}
			if wait {
				ready, werr := waitForVPSRunning(cmd, c, v.ID, timeout)
				if werr != nil {
					return werr
				}
				v = ready
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
	f.BoolVar(&wait, "wait", false, "wait until the server reaches the running state")
	f.DurationVar(&timeout, "timeout", pollTimeout, "max time to wait when --wait is set")
	return cmd
}

// waitForVPSRunning polls a freshly-rented server until it is running, with
// geometric backoff. There is no RentAndWait in the SDK.
func waitForVPSRunning(cmd *cobra.Command, c *client.Client, id uint, timeout time.Duration) (*types.VPSServerResponse, error) {
	deadline := time.Now().Add(timeout)
	interval := 2 * time.Second
	for {
		v, err := c.VPS().GetServer(cmd.Context(), id)
		if err != nil {
			return nil, err
		}
		if v.Status == "running" {
			return v, nil
		}
		if time.Now().After(deadline) {
			return v, fmt.Errorf("timed out after %s waiting for vps %d to run (status %s)", timeout, id, v.Status)
		}
		select {
		case <-cmd.Context().Done():
			return nil, cmd.Context().Err()
		case <-time.After(interval):
		}
		if interval = time.Duration(float64(interval) * 1.5); interval > 30*time.Second {
			interval = 30 * time.Second
		}
	}
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
			return printResult(cmd, output.ActionResult{
				Resource: "vps", ID: id, Action: "rename", Status: "done",
				Message: fmt.Sprintf("Renamed vps %d to %q", id, newName),
			})
		},
	}
	cmd.Flags().StringVar(&newName, "new-name", "", "new display name (required)")
	return cmd
}

func newVPSCancelCmd() *cobra.Command {
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
			if !flagYes {
				ok, err := confirm(cmd, fmt.Sprintf("Cancel auto-renew on vps %q (id %d)?", v.DisplayName, id))
				if err != nil {
					return err
				}
				if !ok {
					return printAborted(cmd)
				}
			}
			if err := c.VPS().CancelSubscription(cmd.Context(), id, writeOpts("")...); err != nil {
				if client.IsCode(err, codes.AutoRenewAlreadyCancelled) {
					return printResult(cmd, output.ActionResult{
						Resource: "vps", ID: id, Action: "cancel", Status: "noop",
						Message: fmt.Sprintf("Auto-renewal already cancelled; server runs until %s", formatVPSDate(v.ExpiresAt)),
					})
				}
				return err
			}
			return printResult(cmd, output.ActionResult{
				Resource: "vps", ID: id, Action: "cancel", Status: "done",
				Message: fmt.Sprintf("Auto-renewal cancelled; server runs until ExpiresAt: %s", formatVPSDate(v.ExpiresAt)),
			})
		},
	}
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
			if err := c.VPS().Renew(cmd.Context(), id, writeOpts("")...); err != nil {
				return mapVPSRentError(err)
			}
			return printResult(cmd, output.ActionResult{
				Resource: "vps", ID: id, Action: "renew", Status: "done",
				Message: fmt.Sprintf("Renewed vps %d", id),
			})
		},
	}
}
