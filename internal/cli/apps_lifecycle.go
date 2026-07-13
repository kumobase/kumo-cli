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

func newAppsStartCmd() *cobra.Command {
	var (
		wait    bool
		timeout time.Duration
	)
	cmd := &cobra.Command{
		Use:   "start <name>",
		Short: "Start (un-suspend) an application",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, _, err := newClient()
			if err != nil {
				return err
			}
			id, _, _, err := resolveAppRef(cmd.Context(), c, args[0])
			if err != nil {
				return err
			}
			since := time.Now()
			if err := c.Apps().Start(cmd.Context(), id); err != nil {
				return err
			}
			return finishLifecycle(cmd, c, id, types.AppOperationActionStart, "started", since, wait, timeout)
		},
	}
	addWaitFlags(cmd, &wait, &timeout, false)
	return cmd
}

func newAppsStopCmd() *cobra.Command {
	var (
		wait    bool
		timeout time.Duration
	)
	cmd := &cobra.Command{
		Use:   "stop <name>",
		Short: "Stop (suspend) an application",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, _, err := newClient()
			if err != nil {
				return err
			}
			id, _, _, err := resolveAppRef(cmd.Context(), c, args[0])
			if err != nil {
				return err
			}
			since := time.Now()
			if err := c.Apps().Stop(cmd.Context(), id); err != nil {
				if client.IsCode(err, codes.AppAlreadyStopped) {
					return printResult(cmd, output.ActionResult{
						Resource: "app", ID: id, Action: "stop", Status: "noop",
						Message: fmt.Sprintf("App %d is already stopped", id),
					})
				}
				return err
			}
			return finishLifecycle(cmd, c, id, types.AppOperationActionStop, "stopped", since, wait, timeout)
		},
	}
	addWaitFlags(cmd, &wait, &timeout, false)
	return cmd
}

// finishLifecycle reports the queued action and, when wait is set, blocks
// until the corresponding operation completes.
func finishLifecycle(cmd *cobra.Command, c *client.Client, id uint, action types.AppOperationActionType, pastTense string, since time.Time, wait bool, timeout time.Duration) error {
	if !wait {
		return printResult(cmd, output.ActionResult{
			Resource: "app", ID: id, Action: string(action), Status: "queued",
			Message: fmt.Sprintf("%s queued for app %d", capitalize(string(action)), id),
		})
	}
	op, err := waitForOperation(cmd.Context(), c, id, action, since, timeout)
	if err != nil {
		return err
	}
	res := output.ActionResult{
		Resource: "app", ID: id, Action: string(action), Status: "done",
		Message: fmt.Sprintf("App %d %s", id, pastTense),
	}
	if op != nil {
		res.OperationID = op.OperationID
	}
	return printResult(cmd, res)
}

func addWaitFlags(cmd *cobra.Command, wait *bool, timeout *time.Duration, defaultWait bool) {
	f := cmd.Flags()
	f.BoolVar(wait, "wait", defaultWait, "wait for the operation to complete")
	f.DurationVar(timeout, "timeout", pollTimeout, "max time to wait when --wait is set")
}

func capitalize(s string) string {
	if s == "" {
		return s
	}
	b := []byte(s)
	if b[0] >= 'a' && b[0] <= 'z' {
		b[0] -= 'a' - 'A'
	}
	return string(b)
}
