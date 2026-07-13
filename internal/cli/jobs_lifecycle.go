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

func newJobsRunCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "run <name>",
		Short: "Trigger a job run now",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, s, err := newClient()
			if err != nil {
				return err
			}
			id, _, err := resolveJobRef(cmd.Context(), c, args[0])
			if err != nil {
				return err
			}
			res, err := c.Jobs().RunNow(cmd.Context(), id)
			if err != nil {
				return mapJobError(err, args[0])
			}
			return output.Print(cmd.OutOrStdout(), s.Output, res, func(tw *tabwriter.Writer) {
				fmt.Fprintf(tw, "Started execution %d for job %d — status %s\n", res.ExecutionID, id, res.Status)
			})
		},
	}
}

func newJobsSuspendCmd() *cobra.Command {
	return newJobsToggleCmd("suspend", "Suspend a scheduled job",
		func(ctx context.Context, c *client.Client, id uint) (*types.JobResponse, error) {
			return c.Jobs().Suspend(ctx, id)
		}, "suspended")
}

func newJobsResumeCmd() *cobra.Command {
	return newJobsToggleCmd("resume", "Resume a suspended job",
		func(ctx context.Context, c *client.Client, id uint) (*types.JobResponse, error) {
			return c.Jobs().Resume(ctx, id)
		}, "resumed")
}

func newJobsToggleCmd(use, short string, action func(context.Context, *client.Client, uint) (*types.JobResponse, error), pastTense string) *cobra.Command {
	return &cobra.Command{
		Use:   use + " <name>",
		Short: short,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, _, err := newClient()
			if err != nil {
				return err
			}
			id, _, err := resolveJobRef(cmd.Context(), c, args[0])
			if err != nil {
				return err
			}
			if _, err := action(cmd.Context(), c, id); err != nil {
				return mapJobError(err, args[0])
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Job %d %s\n", id, pastTense)
			return nil
		},
	}
}

func newJobsExecutionsCmd() *cobra.Command {
	var (
		page, pageSize int
		status         string
		from, to       string
	)
	cmd := &cobra.Command{
		Use:   "executions <name> [execution-id]",
		Short: "List a job's executions, or show one execution",
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateExecutionStatus(status); err != nil {
				return err
			}
			if err := validateRFC3339Flag("from", from); err != nil {
				return err
			}
			if err := validateRFC3339Flag("to", to); err != nil {
				return err
			}
			c, s, err := newClient()
			if err != nil {
				return err
			}
			id, _, err := resolveJobRef(cmd.Context(), c, args[0])
			if err != nil {
				return err
			}

			if len(args) == 2 {
				execID, err := parseUintArg(args[1], "execution id")
				if err != nil {
					return err
				}
				ex, err := c.Jobs().GetExecution(cmd.Context(), id, execID)
				if err != nil {
					return mapJobError(err, args[0])
				}
				return output.Print(cmd.OutOrStdout(), s.Output, ex, func(tw *tabwriter.Writer) {
					printJobExecutionDetail(tw, ex)
				})
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
			if from != "" {
				opts = append(opts, client.WithExtraQuery("from", from))
			}
			if to != "" {
				opts = append(opts, client.WithExtraQuery("to", to))
			}
			execs, meta, err := c.Jobs().ListExecutions(cmd.Context(), id, opts...)
			if err != nil {
				return mapJobError(err, args[0])
			}
			return output.PrintList(cmd.OutOrStdout(), s.Output, execs, meta, func(tw *tabwriter.Writer) {
				fmt.Fprintln(tw, "ID\tTRIGGER\tSTATUS\tEXIT\tDURATION\tCREATED")
				for _, ex := range execs {
					fmt.Fprintf(tw, "%d\t%s\t%s\t%s\t%s\t%s\n",
						ex.ID, ex.Trigger, ex.Status, exitCodeLabel(ex.ExitCode),
						durationLabel(ex.DurationMS), ex.CreatedAt.Format(time.RFC3339))
				}
			})
		},
	}
	f := cmd.Flags()
	f.IntVar(&page, "page", 0, "page number (1-based)")
	f.IntVar(&pageSize, "page-size", 0, "items per page (max 100)")
	f.StringVar(&status, "status", "", "filter by status: pending|running|succeeded|failed|timeout")
	f.StringVar(&from, "from", "", "only executions created at/after this RFC3339 time")
	f.StringVar(&to, "to", "", "only executions created at/before this RFC3339 time")
	return cmd
}

// validateExecutionStatus rejects an unknown job-execution status filter.
func validateExecutionStatus(status string) error {
	switch types.JobExecutionStatus(status) {
	case "", types.JobExecutionStatusPending, types.JobExecutionStatusRunning,
		types.JobExecutionStatusSucceeded, types.JobExecutionStatusFailed,
		types.JobExecutionStatusTimeout:
		return nil
	default:
		return usageError{err: fmt.Errorf("invalid --status %q (use pending|running|succeeded|failed|timeout)", status)}
	}
}

func printJobExecutionDetail(tw *tabwriter.Writer, ex *types.JobExecution) {
	fmt.Fprintf(tw, "ID:\t%d\n", ex.ID)
	fmt.Fprintf(tw, "Job id:\t%d\n", ex.JobID)
	fmt.Fprintf(tw, "Trigger:\t%s\n", ex.Trigger)
	fmt.Fprintf(tw, "Status:\t%s\n", ex.Status)
	fmt.Fprintf(tw, "Exit code:\t%s\n", exitCodeLabel(ex.ExitCode))
	fmt.Fprintf(tw, "Duration:\t%s\n", durationLabel(ex.DurationMS))
	fmt.Fprintf(tw, "Started:\t%s\n", formatOptionalTime(ex.PodStartedAt, "-"))
	fmt.Fprintf(tw, "Finished:\t%s\n", formatOptionalTime(ex.PodFinishedAt, "-"))
	if ex.CPUvCPU != "" {
		fmt.Fprintf(tw, "CPU:\t%s vCPU\n", ex.CPUvCPU)
	}
	if ex.MemoryMB > 0 {
		fmt.Fprintf(tw, "Memory:\t%d MB\n", ex.MemoryMB)
	}
	if ex.BilledAmount != nil {
		fmt.Fprintf(tw, "Billed:\t%s\n", *ex.BilledAmount)
	}
	fmt.Fprintf(tw, "Created:\t%s\n", ex.CreatedAt.Format(time.RFC3339))
}

func exitCodeLabel(code *int) string {
	if code == nil {
		return "-"
	}
	return fmt.Sprintf("%d", *code)
}

func durationLabel(ms *int64) string {
	if ms == nil {
		return "-"
	}
	return (time.Duration(*ms) * time.Millisecond).String()
}
