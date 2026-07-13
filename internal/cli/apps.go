package cli

import (
	"context"
	"fmt"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/kumobase/kumo-go/client"
	"github.com/kumobase/kumo-go/types"

	"github.com/kumobase/kumo-cli/internal/output"
)

func newAppsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "apps",
		Aliases: []string{"app"},
		Short:   "Manage applications",
		Long: "Manage applications: deploy from a registry image or git source, update,\n" +
			"scale, and manage custom domains and builds.\n\n" +
			"Note: runtime application logs and metrics are not yet available via the\n" +
			"CLI (the SDK exposes no endpoint for them); view them in the Kumo\n" +
			"dashboard. `kumo apps builds logs` prints build logs for git-build apps.",
	}
	cmd.AddCommand(
		newAppsListCmd(),
		newAppsGetCmd(),
		newAppsPlansCmd(),
		newAppsCreateCmd(),
		newAppsUpdateCmd(),
		newAppsDeleteCmd(),
		newAppsStartCmd(),
		newAppsStopCmd(),
		newAppsOperationsCmd(),
		newAppsDomainCmd(),
		newAppsBuildsCmd(),
		newAppsBuildersCmd(),
	)
	return cmd
}

func newAppsListCmd() *cobra.Command {
	var (
		page, pageSize   int
		sortCol, sortDir string
	)
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List applications",
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
			apps, meta, err := c.Apps().List(cmd.Context(), opts...)
			if err != nil {
				return err
			}
			return output.PrintList(cmd.OutOrStdout(), s.Output, apps, meta, func(tw *tabwriter.Writer) {
				fmt.Fprintln(tw, "ID\tNAME\tSTATUS\tREPLICAS\tEXPOSED\tSUBDOMAIN\tCREATED")
				for _, a := range apps {
					fmt.Fprintf(tw, "%d\t%s\t%s\t%d/%d\t%t\t%s\t%s\n",
						a.Id, a.Name, appStatus(a.AppStatus, a.IsSuspended),
						a.ReadyReplicas, a.DesiredReplicas, a.IsExposed,
						a.GeneratedSubDomain, a.CreatedAt.Format("2006-01-02"))
				}
			})
		},
	}
	f := cmd.Flags()
	f.IntVar(&page, "page", 0, "page number (1-based)")
	f.IntVar(&pageSize, "page-size", 0, "items per page (max 100)")
	f.StringVar(&sortCol, "sort", "", "sort column")
	f.StringVar(&sortDir, "sort-order", "desc", "sort order: asc or desc")
	return cmd
}

func newAppsGetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "get <name>",
		Short: "Show application detail",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, s, err := newClient()
			if err != nil {
				return err
			}
			_, app, _, err := resolveAppRef(cmd.Context(), c, args[0])
			if err != nil {
				return err
			}
			return output.Print(cmd.OutOrStdout(), s.Output, app, func(tw *tabwriter.Writer) {
				fmt.Fprintf(tw, "ID:\t%d\n", app.Id)
				fmt.Fprintf(tw, "Name:\t%s\n", app.Name)
				fmt.Fprintf(tw, "Image:\t%s\n", app.Image)
				fmt.Fprintf(tw, "Status:\t%s\n", appStatus(app.AppStatus, app.IsSuspended))
				if app.StatusMessage != "" {
					fmt.Fprintf(tw, "Status message:\t%s\n", app.StatusMessage)
				}
				fmt.Fprintf(tw, "Replicas:\t%d/%d ready\n", app.ReadyReplicas, app.DesiredReplicas)
				fmt.Fprintf(tw, "Instances:\t%d running, %d pending, %d failed\n", app.RunningInstances, app.PendingInstances, app.FailedInstances)
				fmt.Fprintf(tw, "Port:\t%d\n", app.Port)
				fmt.Fprintf(tw, "Exposed:\t%t\n", app.IsExposed)
				fmt.Fprintf(tw, "Subdomain:\t%s\n", app.GeneratedSubDomain)
				if app.CustomDomain != nil {
					fmt.Fprintf(tw, "Custom domain:\t%s (%s)\n", app.CustomDomain.Domain, app.CustomDomain.VerificationStatus)
				}
				fmt.Fprintf(tw, "Internal DNS:\t%s\n", app.InternalDNS)
				fmt.Fprintf(tw, "Created:\t%s\n", app.CreatedAt.Format(time.RFC3339))
				fmt.Fprintf(tw, "Updated:\t%s\n", app.UpdatedAt.Format(time.RFC3339))
			})
		},
	}
}

// appStatus renders a display status, treating suspension as "stopped".
func appStatus(status string, suspended bool) string {
	if suspended {
		return types.AppStatusStopped
	}
	if status == "" {
		return types.AppStatusUnknown
	}
	return status
}

// newAppsPlansCmd lists the public app plan catalogue (CPU/memory tiers with
// their price) so users can pick a --pricing-slug. Mirrors `vps plans`.
func newAppsPlansCmd() *cobra.Command {
	var sort string
	cmd := &cobra.Command{
		Use:   "plans",
		Short: "List available application plans",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			c, s, err := newClient()
			if err != nil {
				return err
			}
			var opts []client.ListOption
			if sort != "" {
				opts = append(opts, client.WithSort(sort, "asc"))
			}
			plans, err := c.Apps().ListPlans(cmd.Context(), opts...)
			if err != nil {
				return err
			}
			return output.Print(cmd.OutOrStdout(), s.Output, plans, func(tw *tabwriter.Writer) {
				fmt.Fprintln(tw, "SLUG\tNAME\tCPU (vCPU)\tMEMORY (MB)\tPRICE/HR\tPRICE/MO")
				for _, p := range plans {
					fmt.Fprintf(tw, "%s\t%s\t%s→%s\t%d→%d\t%s\t%s\n",
						p.Slug, p.Name,
						p.CPURequestvCPU, p.CPULimitvCPU,
						p.MemoryRequestMB, p.MemoryLimitMB,
						p.PriceHour, p.PriceMonth)
				}
			})
		},
	}
	cmd.Flags().StringVar(&sort, "sort", "", "sort column")
	return cmd
}

// pollTimeout is the default --timeout for wait flows.
const pollTimeout = 10 * time.Minute

// waitForOperation polls an app's operation history until the most recent
// operation of the given action type, queued at or after since, reaches a
// terminal state. It does not rely on server-side ordering.
func waitForOperation(ctx context.Context, c *client.Client, appID uint, action types.AppOperationActionType, since time.Time, timeout time.Duration) (*types.AppOperation, error) {
	deadline := time.Now().Add(timeout)
	// Geometric backoff (matches the SDK's *AndWait behaviour) so a long
	// operation doesn't hammer the status endpoint.
	const (
		initialInterval = 2 * time.Second
		maxInterval     = 30 * time.Second
		backoffFactor   = 1.5
	)
	interval := initialInterval
	for {
		ops, _, err := c.Apps().ListOperations(ctx, appID)
		if err != nil {
			return nil, err
		}
		if op := latestOperation(ops, action, since); op != nil {
			switch op.Status {
			case types.AppOperationStatusSucceeded:
				return op, nil
			case types.AppOperationStatusFailed:
				return op, fmt.Errorf("operation %s failed: %s", op.OperationID, operationError(op))
			case types.AppOperationStatusCancelled:
				return op, fmt.Errorf("operation %s was cancelled", op.OperationID)
			}
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("timed out after %s waiting for %s to complete", timeout, action)
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(interval):
		}
		if interval = time.Duration(float64(interval) * backoffFactor); interval > maxInterval {
			interval = maxInterval
		}
	}
}

// latestOperation returns the most recently queued operation matching action
// and queued at or after since, or nil.
func latestOperation(ops []types.AppOperation, action types.AppOperationActionType, since time.Time) *types.AppOperation {
	var latest *types.AppOperation
	for i := range ops {
		op := &ops[i]
		if op.ActionType != action || op.QueuedAt.Before(since) {
			continue
		}
		if latest == nil || op.QueuedAt.After(latest.QueuedAt) {
			latest = op
		}
	}
	return latest
}

func operationError(op *types.AppOperation) string {
	if op.ErrorMsg != nil && *op.ErrorMsg != "" {
		if op.ErrorCode != nil && *op.ErrorCode != "" {
			return fmt.Sprintf("%s (%s)", *op.ErrorMsg, *op.ErrorCode)
		}
		return *op.ErrorMsg
	}
	if op.ErrorCode != nil {
		return *op.ErrorCode
	}
	return "unknown error"
}
