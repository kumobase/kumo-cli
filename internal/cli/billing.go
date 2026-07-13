package cli

import (
	"fmt"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/kumobase/kumo-go/client"
	"github.com/kumobase/kumo-go/codes"
	"github.com/kumobase/kumo-go/types"

	"github.com/kumobase/kumo-cli/internal/output"
)

func newBillingCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "billing",
		Aliases: []string{"bill"},
		Short:   "Inspect usage charges and billing",
		Long: "View billing summary, charges, and cost breakdown for the active user.\n\n" +
			"This is a read-only surface. Top-ups and voucher redemption are done in the\n" +
			"Kumo dashboard.",
	}
	cmd.AddCommand(newBillingSummaryCmd(), newBillingChargesCmd(), newBillingBreakdownCmd())
	return cmd
}

func newBillingSummaryCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "summary",
		Short: "Show the current billing period summary",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			c, s, err := newClient()
			if err != nil {
				return err
			}
			sum, err := c.Billing().GetSummary(cmd.Context())
			if err != nil {
				return mapBillingError(err)
			}
			return output.Print(cmd.OutOrStdout(), s.Output, sum, func(tw *tabwriter.Writer) {
				cp := sum.CurrentPeriod
				fmt.Fprintf(tw, "Currency:\t%s\n", sum.Currency)
				fmt.Fprintf(tw, "Period:\t%s → %s\n", cp.Start.Format("2006-01-02"), cp.End.Format("2006-01-02"))
				fmt.Fprintf(tw, "Charged so far:\t%s\n", cp.TotalCharged)
				fmt.Fprintf(tw, "Accruing (unbilled):\t%s\n", cp.AccruingTotal)
				fmt.Fprintf(tw, "Previous period total:\t%s\n", sum.PreviousPeriodTotal)
			})
		},
	}
}

func newBillingChargesCmd() *cobra.Command {
	var (
		page, pageSize      int
		sort                string
		from, to            string
		productType, status string
		groupBy             string
		grouped             bool
	)
	cmd := &cobra.Command{
		Use:   "charges",
		Short: "List billing charges",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			c, s, err := newClient()
			if err != nil {
				return err
			}
			opts := billingChargeOpts(page, pageSize, sort, from, to, productType, status, groupBy)

			if grouped {
				groups, meta, err := c.Billing().ListGroupedCharges(cmd.Context(), opts...)
				if err != nil {
					return mapBillingError(err)
				}
				return output.Print(cmd.OutOrStdout(), s.Output, groups, func(tw *tabwriter.Writer) {
					fmt.Fprintln(tw, "GROUP\tTOTAL\tCURRENCY\tCHARGES")
					for _, g := range groups {
						fmt.Fprintf(tw, "%s\t%s\t%s\t%d\n", g.GroupKey, g.TotalAmount, g.Currency, g.ChargeCount)
					}
					printPageFooter(tw, meta)
				})
			}

			charges, meta, err := c.Billing().ListCharges(cmd.Context(), opts...)
			if err != nil {
				return mapBillingError(err)
			}
			return output.Print(cmd.OutOrStdout(), s.Output, charges, func(tw *tabwriter.Writer) {
				fmt.Fprintln(tw, "ID\tPRODUCT\tPLAN\tAMOUNT\tCURRENCY\tTYPE\tSTATUS\tPERIOD")
				for _, ch := range charges {
					fmt.Fprintf(tw, "%d\t%s\t%s\t%s\t%s\t%s\t%s\t%s → %s\n",
						ch.ID, ch.ProductType, ch.PlanName, ch.Amount, ch.Currency,
						ch.ChargeType, ch.Status,
						ch.PeriodStart.Format("2006-01-02"), ch.PeriodEnd.Format("2006-01-02"))
				}
				printPageFooter(tw, meta)
			})
		},
	}
	f := cmd.Flags()
	f.IntVar(&page, "page", 0, "page number (1-based)")
	f.IntVar(&pageSize, "page-size", 0, "items per page (max 100)")
	f.StringVar(&sort, "sort", "", "sort column")
	f.StringVar(&from, "from", "", "start date (YYYY-MM-DD)")
	f.StringVar(&to, "to", "", "end date (YYYY-MM-DD)")
	f.StringVar(&productType, "product-type", "", "filter by product type")
	f.StringVar(&status, "status", "", "filter by charge status")
	f.StringVar(&groupBy, "group-by", "", "group key when --group is set (e.g. date, subscription)")
	f.BoolVar(&grouped, "group", false, "return grouped totals instead of individual charges")
	return cmd
}

func newBillingBreakdownCmd() *cobra.Command {
	var from, to, granularity, groupBy string
	cmd := &cobra.Command{
		Use:   "breakdown",
		Short: "Show a time-bucketed cost breakdown",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			c, s, err := newClient()
			if err != nil {
				return err
			}
			var opts []client.ListOption
			for _, kv := range [][2]string{
				{"from", from}, {"to", to}, {"granularity", granularity}, {"group_by", groupBy},
			} {
				if kv[1] != "" {
					opts = append(opts, client.WithExtraQuery(kv[0], kv[1]))
				}
			}
			bd, err := c.Billing().GetBreakdown(cmd.Context(), opts...)
			if err != nil {
				return mapBillingError(err)
			}
			return output.Print(cmd.OutOrStdout(), s.Output, bd, func(tw *tabwriter.Writer) {
				fmt.Fprintf(tw, "Currency:\t%s\n", bd.Currency)
				fmt.Fprintf(tw, "Range:\t%s → %s (%s, by %s)\n", bd.From, bd.To, bd.Granularity, bd.GroupBy)
				fmt.Fprintf(tw, "Total:\t%s\n", bd.Totals.Amount)
				fmt.Fprintln(tw, "PERIOD\tAMOUNT")
				for _, b := range bd.Buckets {
					fmt.Fprintf(tw, "%s\t%s\n", b.PeriodStart.Format("2006-01-02"), b.Amount)
				}
			})
		},
	}
	f := cmd.Flags()
	f.StringVar(&from, "from", "", "start date (YYYY-MM-DD)")
	f.StringVar(&to, "to", "", "end date (YYYY-MM-DD)")
	f.StringVar(&granularity, "granularity", "", "time bucket: daily or monthly")
	f.StringVar(&groupBy, "group-by", "", "group dimension: product_type, charge_model, subscription, none")
	return cmd
}

// billingChargeOpts builds the shared list options for the charges endpoints.
func billingChargeOpts(page, pageSize int, sort, from, to, productType, status, groupBy string) []client.ListOption {
	var opts []client.ListOption
	if page > 0 {
		opts = append(opts, client.WithPage(page))
	}
	if pageSize > 0 {
		opts = append(opts, client.WithPageSize(pageSize))
	}
	if sort != "" {
		opts = append(opts, client.WithSort(sort, "desc"))
	}
	for _, kv := range [][2]string{
		{"from", from}, {"to", to}, {"product_type", productType},
		{"status", status}, {"group_by", groupBy},
	} {
		if kv[1] != "" {
			opts = append(opts, client.WithExtraQuery(kv[0], kv[1]))
		}
	}
	return opts
}

// mapBillingError translates billing read-endpoint error codes into friendly
// messages while preserving the underlying error chain.
func mapBillingError(err error) error {
	switch {
	case err == nil:
		return nil
	case client.IsCode(err, codes.BillingInvalidFilterCombination):
		return fmt.Errorf("invalid filter combination; check --group-by against --group: %w", err)
	case client.IsCode(err, codes.BillingInvalidDateRange):
		return fmt.Errorf("invalid date range; --from must be on or before --to (YYYY-MM-DD): %w", err)
	case client.IsCode(err, codes.BillingInvalidEnumValue):
		return fmt.Errorf("invalid filter value; check --granularity/--group-by/--status: %w", err)
	case client.IsCode(err, codes.BillingBreakdownFailed):
		return fmt.Errorf("the billing breakdown could not be computed; try a smaller range: %w", err)
	case client.IsCode(err, codes.BillingInternalError):
		return fmt.Errorf("billing service error; try again shortly: %w", err)
	}
	return err
}

// printPageFooter renders a pagination hint when more than one page exists.
// Shared by paginated list commands across products.
func printPageFooter(tw *tabwriter.Writer, meta *types.Meta) {
	if meta != nil && meta.TotalPages > 1 {
		fmt.Fprintf(tw, "\nPage %d/%d (%d items) — use --page to see more\n",
			meta.Page, meta.TotalPages, meta.TotalItems)
	}
}
