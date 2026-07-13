package cli

import (
	"fmt"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/kumobase/kumo-go/client"
	"github.com/kumobase/kumo-go/types"

	"github.com/kumobase/kumo-cli/internal/output"
)

func newAppsOperationsCmd() *cobra.Command {
	var (
		page, pageSize int
	)
	cmd := &cobra.Command{
		Use:   "operations <name> [operation-id]",
		Short: "List operations for an app, or show one operation",
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, s, err := newClient()
			if err != nil {
				return err
			}
			id, _, _, err := resolveAppRef(cmd.Context(), c, args[0])
			if err != nil {
				return err
			}

			if len(args) == 2 {
				op, err := c.Apps().GetOperation(cmd.Context(), id, args[1])
				if err != nil {
					return err
				}
				return output.Print(cmd.OutOrStdout(), s.Output, op, func(tw *tabwriter.Writer) {
					printOperation(tw, op)
				})
			}

			var opts []client.ListOption
			if page > 0 {
				opts = append(opts, client.WithPage(page))
			}
			if pageSize > 0 {
				opts = append(opts, client.WithPageSize(pageSize))
			}
			ops, meta, err := c.Apps().ListOperations(cmd.Context(), id, opts...)
			if err != nil {
				return err
			}
			return output.PrintList(cmd.OutOrStdout(), s.Output, ops, meta, func(tw *tabwriter.Writer) {
				fmt.Fprintln(tw, "OPERATION ID\tACTION\tSTATUS\tQUEUED")
				for _, op := range ops {
					fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n",
						op.OperationID, op.ActionType, op.Status, op.QueuedAt.Format(time.RFC3339))
				}
			})
		},
	}
	f := cmd.Flags()
	f.IntVar(&page, "page", 0, "page number (1-based)")
	f.IntVar(&pageSize, "page-size", 0, "items per page (max 100)")
	return cmd
}

func printOperation(tw *tabwriter.Writer, op *types.AppOperation) {
	fmt.Fprintf(tw, "Operation ID:\t%s\n", op.OperationID)
	fmt.Fprintf(tw, "App ID:\t%d\n", op.AppID)
	fmt.Fprintf(tw, "Action:\t%s\n", op.ActionType)
	fmt.Fprintf(tw, "Status:\t%s\n", op.Status)
	if op.ErrorCode != nil && *op.ErrorCode != "" {
		fmt.Fprintf(tw, "Error code:\t%s\n", *op.ErrorCode)
	}
	if op.ErrorMsg != nil && *op.ErrorMsg != "" {
		fmt.Fprintf(tw, "Error:\t%s\n", *op.ErrorMsg)
	}
	fmt.Fprintf(tw, "Queued:\t%s\n", op.QueuedAt.Format(time.RFC3339))
	if op.StartedAt != nil {
		fmt.Fprintf(tw, "Started:\t%s\n", op.StartedAt.Format(time.RFC3339))
	}
	if op.CompletedAt != nil {
		fmt.Fprintf(tw, "Completed:\t%s\n", op.CompletedAt.Format(time.RFC3339))
	}
}
