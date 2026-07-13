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

func newVolumeCreateCmd() *cobra.Command {
	var (
		name    string
		tier    string
		size    int
		appName string
		mount   string
		force   bool
		wait    bool
		timeout time.Duration
	)
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a volume",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if name == "" {
				return fmt.Errorf("--name is required")
			}
			if tier == "" {
				return fmt.Errorf("--tier is required")
			}
			if size <= 0 {
				return fmt.Errorf("--size is required and must be > 0")
			}
			if appName != "" && mount == "" {
				return fmt.Errorf("--mount is required when --app is set")
			}

			c, s, err := newClient()
			if err != nil {
				return err
			}

			req := &types.CreateVolumeRequest{
				Name:             name,
				StorageTier:      tier,
				SizeGB:           size,
				MountPath:        mount,
				ForceReconfigure: force,
			}
			if appName != "" {
				req.AppName = appName
			}

			created, err := c.Volumes().Create(cmd.Context(), req, writeOpts("")...)
			if err != nil {
				return mapVolumeCreateError(err)
			}

			final := created
			if wait && types.VolumeStatus(created.Status) == types.VolumeStatusCreating {
				v, perr := pollVolumeUntilReady(cmd, c, created.ID, timeout)
				if perr != nil {
					return perr
				}
				final = v
			}

			return output.Print(cmd.OutOrStdout(), s.Output, final, func(tw *tabwriter.Writer) {
				fmt.Fprintf(tw, "Created volume %q (id %d, status %s)\n", final.Name, final.ID, final.Status)
				if !wait && types.VolumeStatus(final.Status) == types.VolumeStatusCreating {
					fmt.Fprintf(tw, "Run `kumo volume get %s` to poll progress.\n", final.Name)
				}
			})
		},
	}
	f := cmd.Flags()
	f.StringVar(&name, "name", "", "volume name")
	f.StringVar(&tier, "tier", "", "storage tier slug")
	f.IntVar(&size, "size", 0, "size in GB")
	f.StringVar(&appName, "app", "", "attach to this app on create")
	f.StringVar(&mount, "mount", "", "mount path inside the app (required when --app is set)")
	f.BoolVar(&force, "force", false, "auto-reconfigure target app (scale to 1, disable autoscaling)")
	f.BoolVar(&wait, "wait", true, "wait for the volume to leave the creating state")
	f.DurationVar(&timeout, "timeout", pollTimeout, "max time to wait when --wait is set")
	return cmd
}

func pollVolumeUntilReady(cmd *cobra.Command, c *client.Client, id uint, timeout time.Duration) (*types.VolumeResponse, error) {
	deadline := time.Now().Add(timeout)
	const interval = 2 * time.Second
	for {
		v, _, err := c.Volumes().Get(cmd.Context(), id)
		if err != nil {
			return nil, err
		}
		switch types.VolumeStatus(v.Status) {
		// VolumeStatusDetached is deprecated — current servers report an
		// unattached volume as "ready" — but is kept here defensively so the
		// poll still terminates against an older server that emits it.
		case types.VolumeStatusReady, types.VolumeStatusDetached:
			return v, nil
		case types.VolumeStatusFailed:
			msg := "volume creation failed"
			if v.ErrorMessage != nil && *v.ErrorMessage != "" {
				msg = *v.ErrorMessage
			}
			return v, fmt.Errorf("%s", msg)
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("timed out after %s waiting for volume %d", timeout, id)
		}
		select {
		case <-cmd.Context().Done():
			return nil, cmd.Context().Err()
		case <-time.After(interval):
		}
	}
}

func mapVolumeCreateError(err error) error {
	switch {
	case client.IsCode(err, codes.TargetAppAlreadyHasVolume):
		return fmt.Errorf("app already has a volume attached: %w", err)
	case client.IsCode(err, codes.AppMustHaveOneReplica), client.IsCode(err, codes.AutoscalingWithVolume):
		return fmt.Errorf("app must run a single non-autoscaled replica; retry with --force or scale the app down: %w", err)
	case client.IsCode(err, codes.SizeBelowMinimum), client.IsCode(err, codes.SizeAboveMaximum):
		return fmt.Errorf("%w (use a different storage tier)", err)
	}
	return err
}

func newVolumeDeleteCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "delete <name>",
		Short: "Delete a volume",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, _, err := newClient()
			if err != nil {
				return err
			}
			id, v, err := resolveVolumeRef(cmd.Context(), c, args[0])
			if err != nil {
				return err
			}
			if !flagYes {
				ok, err := confirm(cmd, fmt.Sprintf("Delete volume %q (id %d)? This cannot be undone.", v.Name, id))
				if err != nil {
					return err
				}
				if !ok {
					return printAborted(cmd)
				}
			}
			if err := c.Volumes().Delete(cmd.Context(), id, writeOpts("")...); err != nil {
				if client.IsCode(err, codes.VolumeAttached) {
					return fmt.Errorf("volume is attached; run `kumo volume detach %s` first: %w", v.Name, err)
				}
				return mapVolumeBusyError(err, v.Status)
			}
			return printResult(cmd, output.ActionResult{
				Resource: "volume", ID: id, Action: "delete", Status: "done",
				Message: fmt.Sprintf("Volume %d deleted", id),
			})
		},
	}
	return cmd
}
