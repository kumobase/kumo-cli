package cli

import (
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

func newRegistryRepoCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "repo",
		Aliases: []string{"repos", "repository", "repositories"},
		Short:   "Manage registry repositories",
	}
	cmd.AddCommand(
		newRegistryRepoListCmd(),
		newRegistryRepoGetCmd(),
		newRegistryRepoCreateCmd(),
		newRegistryRepoUpdateCmd(),
		newRegistryRepoDeleteCmd(),
	)
	return cmd
}

func newRegistryRepoListCmd() *cobra.Command {
	var (
		orgSlug          string
		page, pageSize   int
		sortCol, sortDir string
	)
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List repositories in an organization",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			c, s, err := newClient()
			if err != nil {
				return err
			}
			org, err := resolveOrgSlug(cmd.Context(), c, orgSlug)
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
			repos, meta, err := c.Registry().Repos(org).List(cmd.Context(), opts...)
			if err != nil {
				return mapRegistryError(err)
			}
			return output.PrintList(cmd.OutOrStdout(), s.Output, repos, meta, func(tw *tabwriter.Writer) {
				fmt.Fprintln(tw, "NAME\tTAG MUTABILITY\tCREATED")
				for _, r := range repos {
					fmt.Fprintf(tw, "%s\t%s\t%s\n",
						r.Name, r.TagMutability,
						r.CreatedAt.Format("2006-01-02"))
				}
			})
		},
	}
	f := cmd.Flags()
	f.StringVar(&orgSlug, "org", "", "organization slug (defaults to your sole org)")
	f.IntVar(&page, "page", 0, "page number (1-based)")
	f.IntVar(&pageSize, "page-size", 0, "items per page (max 100)")
	f.StringVar(&sortCol, "sort", "", "sort column")
	f.StringVar(&sortDir, "sort-order", "desc", "sort order: asc or desc")
	return cmd
}

func newRegistryRepoGetCmd() *cobra.Command {
	var orgSlug string
	cmd := &cobra.Command{
		Use:   "get <name>",
		Short: "Show repository detail",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, s, err := newClient()
			if err != nil {
				return err
			}
			org, err := resolveOrgSlug(cmd.Context(), c, orgSlug)
			if err != nil {
				return err
			}
			r, err := c.Registry().Repos(org).Get(cmd.Context(), args[0])
			if err != nil {
				return mapRegistryRepoError(err, args[0])
			}
			return output.Print(cmd.OutOrStdout(), s.Output, r, func(tw *tabwriter.Writer) {
				printRegistryRepoDetail(tw, org, r)
			})
		},
	}
	cmd.Flags().StringVar(&orgSlug, "org", "", "organization slug (defaults to your sole org)")
	return cmd
}

func newRegistryRepoCreateCmd() *cobra.Command {
	var (
		orgSlug       string
		tagMutability string
	)
	cmd := &cobra.Command{
		Use:   "create <name>",
		Short: "Create a repository",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, s, err := newClient()
			if err != nil {
				return err
			}
			org, err := resolveOrgSlug(cmd.Context(), c, orgSlug)
			if err != nil {
				return err
			}
			req := &types.CreateRepositoryRequest{Name: args[0]}
			f := cmd.Flags()
			if f.Changed("tag-mutability") {
				tm := strings.ToUpper(tagMutability)
				if tm != string(types.TagMutabilityMutable) && tm != string(types.TagMutabilityImmutable) {
					return fmt.Errorf("invalid --tag-mutability %q (use MUTABLE or IMMUTABLE)", tagMutability)
				}
				req.TagMutability = types.TagMutability(tm)
			}
			r, err := c.Registry().Repos(org).Create(cmd.Context(), req)
			if err != nil {
				return mapRegistryError(err)
			}
			return output.Print(cmd.OutOrStdout(), s.Output, r, func(tw *tabwriter.Writer) {
				fmt.Fprintf(tw, "Created repo %s/%s\n", org, r.Name)
			})
		},
	}
	f := cmd.Flags()
	f.StringVar(&orgSlug, "org", "", "organization slug (defaults to your sole org)")
	f.StringVar(&tagMutability, "tag-mutability", "", "tag mutability: MUTABLE or IMMUTABLE")
	return cmd
}

func newRegistryRepoUpdateCmd() *cobra.Command {
	var (
		orgSlug       string
		tagMutability string
	)
	cmd := &cobra.Command{
		Use:   "update <name>",
		Short: "Update a repository",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, s, err := newClient()
			if err != nil {
				return err
			}
			org, err := resolveOrgSlug(cmd.Context(), c, orgSlug)
			if err != nil {
				return err
			}
			req := &types.UpdateRepositoryRequest{}
			f := cmd.Flags()
			if f.Changed("tag-mutability") {
				tm := strings.ToUpper(tagMutability)
				if tm != string(types.TagMutabilityMutable) && tm != string(types.TagMutabilityImmutable) {
					return fmt.Errorf("invalid --tag-mutability %q (use MUTABLE or IMMUTABLE)", tagMutability)
				}
				v := types.TagMutability(tm)
				req.TagMutability = &v
			}
			r, err := c.Registry().Repos(org).Update(cmd.Context(), args[0], req)
			if err != nil {
				return mapRegistryRepoError(err, args[0])
			}
			return output.Print(cmd.OutOrStdout(), s.Output, r, func(tw *tabwriter.Writer) {
				fmt.Fprintf(tw, "Updated repo %s/%s\n", org, r.Name)
			})
		},
	}
	f := cmd.Flags()
	f.StringVar(&orgSlug, "org", "", "organization slug (defaults to your sole org)")
	f.StringVar(&tagMutability, "tag-mutability", "", "tag mutability: MUTABLE or IMMUTABLE")
	return cmd
}

func newRegistryRepoDeleteCmd() *cobra.Command {
	var (
		orgSlug string
	)
	cmd := &cobra.Command{
		Use:   "delete <name>",
		Short: "Delete a repository",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, _, err := newClient()
			if err != nil {
				return err
			}
			org, err := resolveOrgSlug(cmd.Context(), c, orgSlug)
			if err != nil {
				return err
			}
			name := args[0]
			if !flagYes {
				ok, err := confirm(cmd, fmt.Sprintf("Delete repo %s/%s? This cannot be undone.", org, name))
				if err != nil {
					return err
				}
				if !ok {
					return printAborted(cmd)
				}
			}
			if err := c.Registry().Repos(org).Delete(cmd.Context(), name); err != nil {
				return mapRegistryRepoError(err, name)
			}
			return printResult(cmd, output.ActionResult{
				Resource: "registry-repo", Action: "delete", Status: "done",
				Message: fmt.Sprintf("Repo %s/%s deleted", org, name),
			})
		},
	}
	f := cmd.Flags()
	f.StringVar(&orgSlug, "org", "", "organization slug (defaults to your sole org)")
	return cmd
}

func mapRegistryRepoError(err error, name string) error {
	if client.IsCode(err, codes.RegistryRepositoryNotFound) || client.IsNotFound(err) {
		return fmt.Errorf("no repo named %q", name)
	}
	return mapRegistryError(err)
}

func printRegistryRepoDetail(tw *tabwriter.Writer, org string, r *types.RepositoryResponse) {
	fmt.Fprintf(tw, "ID:\t%d\n", r.ID)
	fmt.Fprintf(tw, "Name:\t%s\n", r.Name)
	fmt.Fprintf(tw, "Org:\t%s\n", org)
	fmt.Fprintf(tw, "Tag mutability:\t%s\n", r.TagMutability)
	fmt.Fprintf(tw, "Created:\t%s\n", r.CreatedAt.Format(time.RFC3339))
	fmt.Fprintf(tw, "Updated:\t%s\n", r.UpdatedAt.Format(time.RFC3339))
}
