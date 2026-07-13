package cli

import (
	"fmt"
	"strconv"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/kumobase/kumo-go/client"
	"github.com/kumobase/kumo-go/codes"

	"github.com/kumobase/kumo-cli/internal/output"
)

func newRunnersCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "runners",
		Aliases: []string{"runner"},
		Short:   "Inspect VM-backed CI runner jobs",
		Long: "View the status and history of your CI runner jobs.\n\n" +
			"There is nothing to provision here: connect the Kumo GitHub App (see\n" +
			"`kumo source`) and add a `kumo-*` runner size label to your workflow's\n" +
			"runs-on. Capacity and placement are managed by Kumo.",
	}
	cmd.AddCommand(newRunnersListCmd(), newRunnersGetCmd())
	return cmd
}

func newRunnersListCmd() *cobra.Command {
	var (
		page, pageSize  int
		state, provider string
	)
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List CI runner jobs",
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
			if state != "" {
				opts = append(opts, client.WithExtraQuery("state", state))
			}
			if provider != "" {
				opts = append(opts, client.WithExtraQuery("provider", provider))
			}
			jobs, meta, err := c.Runners().ListJobs(cmd.Context(), opts...)
			if err != nil {
				return mapRunnerError(err, 0)
			}
			return output.Print(cmd.OutOrStdout(), s.Output, jobs, func(tw *tabwriter.Writer) {
				fmt.Fprintln(tw, "ID\tPROVIDER\tREPO\tSPEC\tSTATE\tCONCLUSION\tQUEUED")
				for _, j := range jobs {
					fmt.Fprintf(tw, "%d\t%s\t%s\t%s\t%s\t%s\t%s\n",
						j.ID, j.Provider, j.RepoFullName, j.SpecLabel, j.State,
						orDash(j.Conclusion), j.QueuedAt.Format(time.RFC3339))
				}
				printPageFooter(tw, meta)
			})
		},
	}
	f := cmd.Flags()
	f.IntVar(&page, "page", 0, "page number (1-based)")
	f.IntVar(&pageSize, "page-size", 0, "items per page (max 100)")
	f.StringVar(&state, "state", "", "filter by state (e.g. running, completed, failed)")
	f.StringVar(&provider, "provider", "", "filter by provider (github|gitlab)")
	return cmd
}

func newRunnersGetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "get <id>",
		Short: "Show runner job detail",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := parseUintArg(args[0], "runner job id")
			if err != nil {
				return err
			}
			c, s, err := newClient()
			if err != nil {
				return err
			}
			j, err := c.Runners().GetJob(cmd.Context(), id)
			if err != nil {
				return mapRunnerError(err, id)
			}
			return output.Print(cmd.OutOrStdout(), s.Output, j, func(tw *tabwriter.Writer) {
				fmt.Fprintf(tw, "ID:\t%d\n", j.ID)
				fmt.Fprintf(tw, "Provider:\t%s\n", j.Provider)
				fmt.Fprintf(tw, "Repo:\t%s\n", j.RepoFullName)
				fmt.Fprintf(tw, "Spec label:\t%s\n", j.SpecLabel)
				fmt.Fprintf(tw, "State:\t%s\n", j.State)
				fmt.Fprintf(tw, "Conclusion:\t%s\n", orDash(j.Conclusion))
				if j.GitLabJobID != nil {
					fmt.Fprintf(tw, "GitLab job id:\t%d\n", *j.GitLabJobID)
				} else {
					fmt.Fprintf(tw, "GitHub job id:\t%d\n", j.GithubJobID)
					fmt.Fprintf(tw, "Run id:\t%d\n", j.RunID)
				}
				if j.WebURL != "" {
					fmt.Fprintf(tw, "URL:\t%s\n", j.WebURL)
				}
				fmt.Fprintf(tw, "Queued:\t%s\n", j.QueuedAt.Format(time.RFC3339))
				fmt.Fprintf(tw, "Started:\t%s\n", formatOptionalTime(j.StartedAt, "-"))
				fmt.Fprintf(tw, "Finished:\t%s\n", formatOptionalTime(j.FinishedAt, "-"))
			})
		},
	}
}

// mapRunnerError translates runner error codes into friendly messages.
func mapRunnerError(err error, id uint) error {
	switch {
	case err == nil:
		return nil
	case client.IsCode(err, codes.RunnerJobNotFound) || client.IsNotFound(err):
		return fmt.Errorf("no runner job with id %d", id)
	case client.IsCode(err, codes.RunnerUnauthorized):
		return fmt.Errorf("not authorized to view this runner job: %w", err)
	case client.IsCode(err, codes.RunnerInvalidID):
		return fmt.Errorf("invalid runner job id: %w", err)
	}
	return err
}

// parseUintArg parses a positional numeric id argument with a friendly error.
func parseUintArg(s, label string) (uint, error) {
	n, err := strconv.ParseUint(s, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid %s %q: must be a number", label, s)
	}
	return uint(n), nil
}

// orDash returns the string or "-" when empty.
func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}
