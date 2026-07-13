package cli

import (
	"fmt"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/kumobase/kumo-go/client"
	"github.com/kumobase/kumo-go/codes"

	"github.com/kumobase/kumo-cli/internal/output"
)

func newSourceCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "source",
		Aliases: []string{"sources", "connections", "connection"},
		Short:   "Inspect git source connections",
		Long: "List git source connections (GitHub/GitLab) and the repositories they can\n" +
			"access, used for git-build apps and CI runners.\n\n" +
			"Connecting a new source is a browser sign-in flow done in the Kumo\n" +
			"dashboard; the CLI can list, inspect, and disconnect existing ones.",
	}
	cmd.AddCommand(newSourceListCmd(), newSourceReposCmd(), newSourceDisconnectCmd())
	return cmd
}

func newSourceListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List git source connections",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			c, s, err := newClient()
			if err != nil {
				return err
			}
			conns, err := c.SourceConnections().List(cmd.Context())
			if err != nil {
				return mapSourceError(err, 0)
			}
			return output.Print(cmd.OutOrStdout(), s.Output, conns, func(tw *tabwriter.Writer) {
				fmt.Fprintln(tw, "ID\tPROVIDER\tACCOUNT\tKIND\tSTATUS\tREPOS")
				for _, cn := range conns {
					repos := orDash(cn.RepoSelection)
					if cn.RepoSelection == "selected" {
						repos = fmt.Sprintf("selected (%d)", cn.RepoCount)
					}
					fmt.Fprintf(tw, "%d\t%s\t%s\t%s\t%s\t%s\n",
						cn.ID, cn.Provider, cn.AccountLogin, cn.AppKind, cn.Status, repos)
				}
			})
		},
	}
}

func newSourceReposCmd() *cobra.Command {
	var page, pageSize int
	cmd := &cobra.Command{
		Use:   "repos <id>",
		Short: "List repositories a source connection can access",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := parseUintArg(args[0], "source connection id")
			if err != nil {
				return err
			}
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
			repos, meta, err := c.SourceConnections().ListRepos(cmd.Context(), id, opts...)
			if err != nil {
				return mapSourceError(err, id)
			}
			return output.Print(cmd.OutOrStdout(), s.Output, repos, func(tw *tabwriter.Writer) {
				fmt.Fprintln(tw, "REPO\tPRIVATE\tDEFAULT BRANCH")
				for _, r := range repos {
					fmt.Fprintf(tw, "%s\t%t\t%s\n", r.FullName, r.Private, r.DefaultBranch)
				}
				printPageFooter(tw, meta)
			})
		},
	}
	f := cmd.Flags()
	f.IntVar(&page, "page", 0, "page number (1-based)")
	f.IntVar(&pageSize, "page-size", 0, "items per page (max 100)")
	return cmd
}

func newSourceDisconnectCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "disconnect <id>",
		Short: "Disconnect a git source connection",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := parseUintArg(args[0], "source connection id")
			if err != nil {
				return err
			}
			c, _, err := newClient()
			if err != nil {
				return err
			}
			if !flagYes {
				ok, err := confirm(cmd, fmt.Sprintf("Disconnect source connection %d? Git-build apps using it will stop building.", id))
				if err != nil {
					return err
				}
				if !ok {
					fmt.Fprintln(cmd.OutOrStdout(), "Aborted.")
					return nil
				}
			}
			if _, err := c.SourceConnections().Disconnect(cmd.Context(), id); err != nil {
				return mapSourceError(err, id)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Source connection %d disconnected\n", id)
			return nil
		},
	}
	return cmd
}

// mapSourceError translates source-connection error codes into friendly
// messages.
func mapSourceError(err error, id uint) error {
	switch {
	case err == nil:
		return nil
	case client.IsCode(err, codes.SourceConnectionNotFound) || client.IsNotFound(err):
		return fmt.Errorf("no source connection with id %d", id)
	case client.IsCode(err, codes.BuildConnectionInUse):
		return fmt.Errorf("source connection %d is still used by a git-build app; delete or redeploy those apps first: %w", id, err)
	case client.IsCode(err, codes.SourceConnectionForbidden):
		return fmt.Errorf("not authorized for this source connection: %w", err)
	case client.IsCode(err, codes.SourceConnectionSuspended):
		return fmt.Errorf("this source connection is suspended: %w", err)
	case client.IsCode(err, codes.SourceProviderError):
		return fmt.Errorf("the git provider returned an error; try again shortly: %w", err)
	}
	return err
}
