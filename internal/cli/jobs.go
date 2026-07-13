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

func newJobsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "jobs",
		Aliases: []string{"job"},
		Short:   "Manage jobs (one-off and scheduled workloads)",
	}
	cmd.AddCommand(
		newJobsListCmd(),
		newJobsGetCmd(),
		newJobsCreateCmd(),
		newJobsUpdateCmd(),
		newJobsDeleteCmd(),
		newJobsRunCmd(),
		newJobsSuspendCmd(),
		newJobsResumeCmd(),
		newJobsExecutionsCmd(),
	)
	return cmd
}

func newJobsListCmd() *cobra.Command {
	var (
		page, pageSize int
		sortCol        string
	)
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List jobs",
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
				opts = append(opts, client.WithSort(sortCol, "desc"))
			}
			jobs, meta, err := c.Jobs().List(cmd.Context(), opts...)
			if err != nil {
				return mapJobError(err, "")
			}
			return output.Print(cmd.OutOrStdout(), s.Output, jobs, func(tw *tabwriter.Writer) {
				fmt.Fprintln(tw, "ID\tNAME\tKIND\tSCHEDULE\tSUSPENDED\tDEPLOYMENT\tLAST RUN")
				for _, j := range jobs {
					fmt.Fprintf(tw, "%d\t%s\t%s\t%s\t%t\t%s\t%s\n",
						j.ID, j.Name, j.Kind, jobSchedule(j.Schedule), j.Suspended,
						j.DeploymentStatus, formatOptionalTime(j.LastExecutionAt, "never"))
				}
				printPageFooter(tw, meta)
			})
		},
	}
	f := cmd.Flags()
	f.IntVar(&page, "page", 0, "page number (1-based)")
	f.IntVar(&pageSize, "page-size", 0, "items per page (max 100)")
	f.StringVar(&sortCol, "sort", "", "sort column")
	return cmd
}

func newJobsGetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "get <name>",
		Short: "Show job detail",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, s, err := newClient()
			if err != nil {
				return err
			}
			_, j, err := resolveJobRef(cmd.Context(), c, args[0])
			if err != nil {
				return err
			}
			return output.Print(cmd.OutOrStdout(), s.Output, j, func(tw *tabwriter.Writer) {
				printJobDetail(tw, j)
			})
		},
	}
}

// resolveJobRef looks up a job by its (per-user unique) name.
func resolveJobRef(ctx context.Context, c *client.Client, name string) (uint, *types.JobResponse, error) {
	if strings.TrimSpace(name) == "" {
		return 0, nil, fmt.Errorf("job name is required")
	}
	j, _, err := c.Jobs().GetByName(ctx, name)
	if err != nil {
		if client.IsCode(err, codes.AmbiguousName) {
			return 0, nil, fmt.Errorf("multiple jobs named %q exist; rename one (kumo jobs list) to disambiguate", name)
		}
		if client.IsNotFound(err) || client.IsCode(err, codes.JobNotFound) {
			return 0, nil, fmt.Errorf("no job named %q", name)
		}
		return 0, nil, mapJobError(err, name)
	}
	return j.ID, j, nil
}

func printJobDetail(tw *tabwriter.Writer, j *types.JobResponse) {
	fmt.Fprintf(tw, "ID:\t%d\n", j.ID)
	fmt.Fprintf(tw, "Name:\t%s\n", j.Name)
	fmt.Fprintf(tw, "Kind:\t%s\n", j.Kind)
	if j.AppName != nil && *j.AppName != "" {
		fmt.Fprintf(tw, "App:\t%s\n", *j.AppName)
	}
	if j.Image != "" {
		fmt.Fprintf(tw, "Image:\t%s\n", j.Image)
	}
	if len(j.Command) > 0 {
		fmt.Fprintf(tw, "Command:\t%s\n", strings.Join(j.Command, " "))
	}
	if len(j.Args) > 0 {
		fmt.Fprintf(tw, "Args:\t%s\n", strings.Join(j.Args, " "))
	}
	fmt.Fprintf(tw, "Schedule:\t%s\n", jobSchedule(j.Schedule))
	if j.Schedule != "" {
		fmt.Fprintf(tw, "Timezone:\t%s\n", j.Timezone)
		fmt.Fprintf(tw, "Concurrency:\t%s\n", j.ConcurrencyPolicy)
	}
	fmt.Fprintf(tw, "Suspended:\t%t\n", j.Suspended)
	fmt.Fprintf(tw, "Deployment:\t%s\n", j.DeploymentStatus)
	fmt.Fprintf(tw, "Plan:\t%s (%s)\n", j.ResourceTemplate.Slug, j.ResourceTemplate.Name)
	fmt.Fprintf(tw, "Last run:\t%s\n", formatOptionalTime(j.LastExecutionAt, "never"))
	for _, t := range j.NextRunTimes {
		fmt.Fprintf(tw, "Next run:\t%s\n", t.Format(time.RFC3339))
	}
}

// jobSchedule renders the schedule column: the cron expression, or "manual"
// for a one-off job (no schedule).
func jobSchedule(schedule string) string {
	if schedule == "" {
		return "manual"
	}
	return schedule
}

// mapJobError translates the job error surface into friendly messages. name is
// used only for not-found phrasing and may be empty.
func mapJobError(err error, name string) error {
	switch {
	case err == nil:
		return nil
	case client.IsCode(err, codes.JobNotFound) || client.IsNotFound(err):
		if name != "" {
			return fmt.Errorf("no job named %q", name)
		}
		return fmt.Errorf("job not found: %w", err)
	case client.IsCode(err, codes.JobExecutionNotFound):
		return fmt.Errorf("no such job execution: %w", err)
	case client.IsCode(err, codes.JobAppRequired):
		return fmt.Errorf("an app-attached job requires --app: %w", err)
	case client.IsCode(err, codes.JobAppNotFound):
		return fmt.Errorf("the referenced app does not exist: %w", err)
	case client.IsCode(err, codes.JobImageRequired):
		return fmt.Errorf("a standalone job requires --image: %w", err)
	case client.IsCode(err, codes.JobImageNotFound), client.IsCode(err, codes.JobImageUnauthorized), client.IsCode(err, codes.JobImageRegistryUnreachable):
		return fmt.Errorf("the job image could not be pulled; check the reference and registry credentials: %w", err)
	case client.IsCode(err, codes.JobScheduleInvalid):
		return fmt.Errorf("invalid --schedule cron expression: %w", err)
	case client.IsCode(err, codes.JobScheduleTooFrequent):
		return fmt.Errorf("--schedule runs too frequently for your plan: %w", err)
	case client.IsCode(err, codes.JobTimezoneInvalid):
		return fmt.Errorf("invalid --timezone (use an IANA name like UTC or Asia/Jakarta): %w", err)
	case client.IsCode(err, codes.JobKindInvalid), client.IsCode(err, codes.JobKindUnsupported):
		return fmt.Errorf("invalid --kind (use standalone or app-attached): %w", err)
	case client.IsCode(err, codes.JobConcurrencyUnsupported):
		return fmt.Errorf("--concurrency-policy only applies to scheduled jobs: %w", err)
	case client.IsCode(err, codes.JobInvalidPricingSlug):
		return fmt.Errorf("invalid --pricing-slug: %w", err)
	case client.IsCode(err, codes.JobQuotaExceeded):
		return fmt.Errorf("job quota exceeded for your plan: %w", err)
	case client.IsCode(err, codes.JobInsufficientBalance):
		return fmt.Errorf("insufficient balance to run this job; top up in the dashboard: %w", err)
	case client.IsCode(err, codes.JobAlreadySuspended):
		return fmt.Errorf("job is already suspended: %w", err)
	case client.IsCode(err, codes.JobNotSuspended):
		return fmt.Errorf("job is not suspended: %w", err)
	}
	return err
}
