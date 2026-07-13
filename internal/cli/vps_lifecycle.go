package cli

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/kumobase/kumo-go/client"
)

func newVPSStartCmd() *cobra.Command {
	return newVPSPowerCmd("start", "Power on a VPS instance", "powered on",
		func(ctx context.Context, c *client.Client, id uint, timeout time.Duration) error {
			_, err := c.VPS().PowerOnAndWait(ctx, id, pollOpts(timeout)...)
			return err
		},
		func(ctx context.Context, c *client.Client, id uint) error {
			return c.VPS().PowerOn(ctx, id)
		})
}

func newVPSStopCmd() *cobra.Command {
	return newVPSPowerCmd("stop", "Power off a VPS instance", "powered off",
		func(ctx context.Context, c *client.Client, id uint, timeout time.Duration) error {
			_, err := c.VPS().PowerOffAndWait(ctx, id, pollOpts(timeout)...)
			return err
		},
		func(ctx context.Context, c *client.Client, id uint) error {
			return c.VPS().PowerOff(ctx, id)
		})
}

func newVPSRebootCmd() *cobra.Command {
	return newVPSPowerCmd("reboot", "Reboot a VPS instance", "rebooted",
		func(ctx context.Context, c *client.Client, id uint, timeout time.Duration) error {
			_, err := c.VPS().RebootAndWait(ctx, id, pollOpts(timeout)...)
			return err
		},
		func(ctx context.Context, c *client.Client, id uint) error {
			return c.VPS().Reboot(ctx, id)
		})
}

func newVPSReinstallCmd() *cobra.Command {
	var (
		wait    bool
		timeout time.Duration
	)
	cmd := &cobra.Command{
		Use:   "reinstall <name>",
		Short: "Reinstall (wipe and re-image) a VPS instance",
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
				ok, err := confirm(cmd, fmt.Sprintf("Reinstall vps %q (id %d)? This wipes all data and cannot be undone.", v.DisplayName, id))
				if err != nil {
					return err
				}
				if !ok {
					fmt.Fprintln(cmd.OutOrStdout(), "Aborted.")
					return nil
				}
			}
			if !wait {
				if err := c.VPS().Reinstall(cmd.Context(), id); err != nil {
					return mapVPSActionError(err, args[0], v.Status)
				}
				fmt.Fprintf(cmd.OutOrStdout(), "Action queued; poll `kumo vps get %s`\n", args[0])
				return nil
			}
			if _, err := c.VPS().ReinstallAndWait(cmd.Context(), id, pollOpts(timeout)...); err != nil {
				return mapVPSActionError(err, args[0], v.Status)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "VPS %d reinstalled\n", id)
			return nil
		},
	}
	f := cmd.Flags()
	f.BoolVar(&wait, "wait", true, "wait for the action to complete")
	f.DurationVar(&timeout, "timeout", pollTimeout, "max time to wait when --wait is set")
	return cmd
}

func newVPSPowerCmd(use, short, pastTense string,
	withWait func(ctx context.Context, c *client.Client, id uint, timeout time.Duration) error,
	noWait func(ctx context.Context, c *client.Client, id uint) error,
) *cobra.Command {
	var (
		wait    bool
		timeout time.Duration
	)
	cmd := &cobra.Command{
		Use:   use + " <name>",
		Short: short,
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
			if !wait {
				if err := noWait(cmd.Context(), c, id); err != nil {
					return mapVPSActionError(err, args[0], v.Status)
				}
				fmt.Fprintf(cmd.OutOrStdout(), "Action queued; poll `kumo vps get %s`\n", args[0])
				return nil
			}
			if err := withWait(cmd.Context(), c, id, timeout); err != nil {
				return mapVPSActionError(err, args[0], v.Status)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "VPS %d %s\n", id, pastTense)
			return nil
		},
	}
	f := cmd.Flags()
	f.BoolVar(&wait, "wait", true, "wait for the action to complete")
	f.DurationVar(&timeout, "timeout", pollTimeout, "max time to wait when --wait is set")
	return cmd
}
