package cli

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/kumobase/kumo-go/client"
	"github.com/kumobase/kumo-go/codes"
	"github.com/kumobase/kumo-go/types"

	"github.com/kumobase/kumo-cli/internal/output"
)

func newVolumeAttachCmd() *cobra.Command {
	var (
		appName string
		mount   string
		force   bool
	)
	cmd := &cobra.Command{
		Use:   "attach <name>",
		Short: "Attach a volume to an app",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if appName == "" {
				return fmt.Errorf("--app is required")
			}
			if mount == "" {
				return fmt.Errorf("--mount is required")
			}
			c, _, err := newClient()
			if err != nil {
				return err
			}
			id, v, err := resolveVolumeRef(cmd.Context(), c, args[0])
			if err != nil {
				return err
			}
			req := &types.AttachVolumeRequest{
				AppName:          appName,
				MountPath:        mount,
				ForceReconfigure: force,
			}
			res, err := c.Volumes().Attach(cmd.Context(), id, req, writeOpts("")...)
			if err != nil {
				return mapVolumeAttachError(err, v.Status)
			}
			return printResult(cmd, output.ActionResult{
				Resource: "volume", ID: res.ID, Action: "attach", Status: "done",
				Message: fmt.Sprintf("Volume %d attached to %s at %s", res.ID, appName, mount),
			})
		},
	}
	f := cmd.Flags()
	f.StringVar(&appName, "app", "", "target app name")
	f.StringVar(&mount, "mount", "", "mount path inside the app")
	f.BoolVar(&force, "force", false, "auto-reconfigure target app (scale to 1, disable autoscaling)")
	return cmd
}

func mapVolumeAttachError(err error, status string) error {
	switch {
	case client.IsCode(err, codes.TargetAppAlreadyHasVolume):
		return fmt.Errorf("app already has a volume attached: %w", err)
	case client.IsCode(err, codes.AppMustHaveOneReplica), client.IsCode(err, codes.AutoscalingWithVolume):
		return fmt.Errorf("app must run a single non-autoscaled replica; retry with --force or scale the app down: %w", err)
	}
	return mapVolumeBusyError(err, status)
}

func newVolumeDetachCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "detach <name>",
		Short: "Detach a volume from its app",
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
				ok, err := confirm(cmd, fmt.Sprintf("Detach volume %q from its app? The app loses access to its data.", v.Name))
				if err != nil {
					return err
				}
				if !ok {
					return printAborted(cmd)
				}
			}
			if _, err := c.Volumes().Detach(cmd.Context(), id, writeOpts("")...); err != nil {
				return mapVolumeBusyError(err, v.Status)
			}
			return printResult(cmd, output.ActionResult{
				Resource: "volume", ID: id, Action: "detach", Status: "done",
				Message: fmt.Sprintf("Volume %d detached", id),
			})
		},
	}
	return cmd
}

func newVolumeResizeCmd() *cobra.Command {
	var (
		size    int
		force   bool
		wait    bool
		timeout time.Duration
	)
	cmd := &cobra.Command{
		Use:   "resize <name>",
		Short: "Resize a volume (online expand)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if size <= 0 {
				return fmt.Errorf("--size is required and must be > 0")
			}
			c, _, err := newClient()
			if err != nil {
				return err
			}
			id, v, err := resolveVolumeRef(cmd.Context(), c, args[0])
			if err != nil {
				return err
			}
			if !force && v.AppID != nil {
				if err := resizePreflight(cmd, c, *v.AppID); err != nil {
					return err
				}
			}
			req := &types.ResizeVolumeRequest{SizeGB: size}
			if wait {
				res, err := c.Volumes().ResizeAndWait(cmd.Context(), id, req, pollOpts(timeout)...)
				if err != nil {
					return mapVolumeResizeError(err, v.Status, v.Name)
				}
				return printResult(cmd, output.ActionResult{
					Resource: "volume", ID: res.ID, Action: "resize", Status: string(res.Status),
					Message: fmt.Sprintf("Volume %d resized to %d GB (status %s)", res.ID, res.SizeGB, res.Status),
				})
			}
			if _, err := c.Volumes().Resize(cmd.Context(), id, req, writeOpts("")...); err != nil {
				return mapVolumeResizeError(err, v.Status, v.Name)
			}
			return printResult(cmd, output.ActionResult{
				Resource: "volume", ID: id, Action: "resize", Status: "queued",
				Message: fmt.Sprintf("Resize queued for volume %d; run `kumo volume get %s` to poll progress", id, v.Name),
			})
		},
	}
	f := cmd.Flags()
	f.IntVar(&size, "size", 0, "new size in GB (must be larger than current)")
	f.BoolVar(&force, "force", false, "auto-reconfigure target app (scale to 1, disable autoscaling)")
	f.BoolVar(&wait, "wait", true, "wait for the resize to complete")
	f.DurationVar(&timeout, "timeout", pollTimeout, "max time to wait when --wait is set")
	return cmd
}

// resizePreflight short-circuits the resize when the attached app is not in a
// resize-safe shape (single replica, no autoscaling).
func resizePreflight(cmd *cobra.Command, c *client.Client, appID uint) error {
	app, _, err := c.Apps().Get(cmd.Context(), appID)
	if err != nil {
		// Pre-flight is best-effort; let the backend reject if we can't read it.
		return nil
	}
	if app.Replicas > 1 {
		return fmt.Errorf("app %q has %d replicas; resize requires a single replica — retry with --force or `kumo apps update %s --replicas 1`", app.Name, app.Replicas, app.Name)
	}
	if app.Autoscaling != nil && app.Autoscaling.Enabled {
		return fmt.Errorf("app %q has autoscaling enabled; resize requires it disabled — retry with --force or update the app", app.Name)
	}
	return nil
}

func mapVolumeResizeError(err error, status, name string) error {
	switch {
	case client.IsCode(err, codes.VolumeNotAttached):
		return fmt.Errorf("resize requires the volume to be attached; run `kumo volume attach %s --app <app>` first: %w", name, err)
	case client.IsCode(err, codes.AppMustHaveOneReplica), client.IsCode(err, codes.AutoscalingWithVolume):
		return fmt.Errorf("app must run a single non-autoscaled replica; retry with --force or scale the app down: %w", err)
	case client.IsCode(err, codes.CannotShrinkVolume):
		return err
	case client.IsCode(err, codes.SizeBelowMinimum), client.IsCode(err, codes.SizeAboveMaximum):
		return fmt.Errorf("%w (use a different storage tier)", err)
	}
	return mapVolumeBusyError(err, status)
}
