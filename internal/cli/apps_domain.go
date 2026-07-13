package cli

import (
	"fmt"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/kumobase/kumo-go/types"

	"github.com/kumobase/kumo-cli/internal/output"
)

func newAppsDomainCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "domain",
		Short: "Manage an app's custom domain",
	}
	cmd.AddCommand(
		newAppsDomainAddCmd(),
		newAppsDomainGetCmd(),
		newAppsDomainRemoveCmd(),
		newAppsDomainVerifyCmd(),
	)
	return cmd
}

func newAppsDomainAddCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "add <name> <domain>",
		Short: "Attach a custom domain to an app",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, s, err := newClient()
			if err != nil {
				return err
			}
			id, _, _, err := resolveAppRef(cmd.Context(), c, args[0])
			if err != nil {
				return err
			}
			info, err := c.Apps().AddCustomDomain(cmd.Context(), id, args[1])
			if err != nil {
				return err
			}
			return printDomain(cmd, s.Output, info)
		},
	}
}

func newAppsDomainGetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "get <name>",
		Short: "Show the custom domain attached to an app",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, s, err := newClient()
			if err != nil {
				return err
			}
			id, _, _, err := resolveAppRef(cmd.Context(), c, args[0])
			if err != nil {
				return err
			}
			info, err := c.Apps().GetCustomDomain(cmd.Context(), id)
			if err != nil {
				return err
			}
			return printDomain(cmd, s.Output, info)
		},
	}
}

func newAppsDomainRemoveCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "remove <name>",
		Short: "Detach the custom domain from an app",
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
			if err := c.Apps().DeleteCustomDomain(cmd.Context(), id); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Custom domain detached from app %d\n", id)
			return nil
		},
	}
}

func newAppsDomainVerifyCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "verify <name>",
		Short: "Trigger DNS verification for the app's custom domain",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, s, err := newClient()
			if err != nil {
				return err
			}
			id, _, _, err := resolveAppRef(cmd.Context(), c, args[0])
			if err != nil {
				return err
			}
			info, err := c.Apps().VerifyCustomDomain(cmd.Context(), id)
			if err != nil {
				return err
			}
			return printDomain(cmd, s.Output, info)
		},
	}
}

func printDomain(cmd *cobra.Command, format string, info *types.CustomDomainInfo) error {
	return output.Print(cmd.OutOrStdout(), format, info, func(tw *tabwriter.Writer) {
		fmt.Fprintf(tw, "Domain:\t%s\n", info.Domain)
		fmt.Fprintf(tw, "Verification:\t%s\n", info.VerificationStatus)
	})
}
