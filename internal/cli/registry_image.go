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

func newRegistryImageCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "image",
		Aliases: []string{"images", "manifest", "manifests"},
		Short:   "Inspect pushed images (manifests)",
	}
	cmd.AddCommand(newRegistryImageListCmd(), newRegistryImageGetCmd(), newRegistryImageDeleteCmd())
	return cmd
}

func newRegistryImageListCmd() *cobra.Command {
	var (
		orgSlug          string
		tag              string
		platform         string
		page, pageSize   int
		sortCol, sortDir string
	)
	cmd := &cobra.Command{
		Use:   "list <repo>",
		Short: "List images in a repository",
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
			var opts []client.ListOption
			if tag != "" {
				opts = append(opts, client.WithExtraQuery("tag", tag))
			}
			if platform != "" {
				opts = append(opts, client.WithExtraQuery("platform", platform))
			}
			if page > 0 {
				opts = append(opts, client.WithPage(page))
			}
			if pageSize > 0 {
				opts = append(opts, client.WithPageSize(pageSize))
			}
			if sortCol != "" {
				opts = append(opts, client.WithSort(sortCol, sortDir))
			}
			ms, meta, err := c.Registry().Repos(org).ListManifests(cmd.Context(), args[0], opts...)
			if err != nil {
				return mapRegistryRepoError(err, args[0])
			}
			return output.PrintList(cmd.OutOrStdout(), s.Output, ms, meta, func(tw *tabwriter.Writer) {
				fmt.Fprintln(tw, "TAG\tDIGEST\tSIZE\tARCH/OS\tPUSHED")
				for _, m := range ms {
					fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n",
						derefOr(m.Tag, "-"),
						shortDigest(m.Digest),
						manifestSizeLabel(&m),
						manifestArchOSLabel(&m),
						m.PushedAt.Format(time.RFC3339))
				}
			})
		},
	}
	f := cmd.Flags()
	f.StringVar(&orgSlug, "org", "", "organization slug (defaults to your sole org)")
	f.StringVar(&tag, "tag", "", "filter by tag")
	f.StringVar(&platform, "platform", "", "filter by platform (e.g. linux/amd64)")
	f.IntVar(&page, "page", 0, "page number (1-based)")
	f.IntVar(&pageSize, "page-size", 0, "items per page (max 100)")
	f.StringVar(&sortCol, "sort", "", "sort column")
	f.StringVar(&sortDir, "sort-order", "desc", "sort order: asc or desc")
	return cmd
}

func newRegistryImageGetCmd() *cobra.Command {
	var orgSlug string
	cmd := &cobra.Command{
		Use:   "get <repo> <digest-or-tag>",
		Short: "Show image manifest detail",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, s, err := newClient()
			if err != nil {
				return err
			}
			org, err := resolveOrgSlug(cmd.Context(), c, orgSlug)
			if err != nil {
				return err
			}
			m, err := c.Registry().Repos(org).GetManifest(cmd.Context(), args[0], args[1])
			if err != nil {
				return mapRegistryManifestError(err, args[1], args[0])
			}
			return output.Print(cmd.OutOrStdout(), s.Output, m, func(tw *tabwriter.Writer) {
				printManifestDetail(tw, m)
			})
		},
	}
	cmd.Flags().StringVar(&orgSlug, "org", "", "organization slug (defaults to your sole org)")
	return cmd
}

func newRegistryImageDeleteCmd() *cobra.Command {
	var (
		orgSlug string
	)
	cmd := &cobra.Command{
		Use:   "delete <repo> <digest-or-tag>",
		Short: "Delete an image manifest by digest",
		Long: "Delete an image manifest. Deletion is by digest, so every tag pointing at\n" +
			"that digest is removed. A tag may be given instead of a digest; it is\n" +
			"resolved to its digest first.",
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, _, err := newClient()
			if err != nil {
				return err
			}
			org, err := resolveOrgSlug(cmd.Context(), c, orgSlug)
			if err != nil {
				return err
			}
			repo, ref := args[0], args[1]

			// DeleteManifest addresses images by digest. When a tag is given,
			// resolve it to the concrete digest so the confirmation prompt can
			// show exactly what will be removed (all tags at that digest).
			digest := ref
			if !strings.HasPrefix(ref, "sha256:") {
				m, err := c.Registry().Repos(org).GetManifest(cmd.Context(), repo, ref)
				if err != nil {
					return mapRegistryManifestError(err, ref, repo)
				}
				digest = m.Digest
			}

			if !flagYes {
				ok, err := confirm(cmd, fmt.Sprintf(
					"Delete image %s from %s/%s? This removes ALL tags at that digest and cannot be undone.",
					digest, org, repo))
				if err != nil {
					return err
				}
				if !ok {
					return printAborted(cmd)
				}
			}
			if err := c.Registry().Repos(org).DeleteManifest(cmd.Context(), repo, digest, writeOpts("")...); err != nil {
				return mapRegistryManifestError(err, digest, repo)
			}
			return printResult(cmd, output.ActionResult{
				Resource: "registry-image", Action: "delete", Status: "done",
				Message: fmt.Sprintf("Image %s deleted from %s/%s", digest, org, repo),
			})
		},
	}
	f := cmd.Flags()
	f.StringVar(&orgSlug, "org", "", "organization slug (defaults to your sole org)")
	return cmd
}

// mapRegistryManifestError translates manifest-not-found into a friendly
// message, delegating other codes to the repo/registry mappers.
func mapRegistryManifestError(err error, ref, repo string) error {
	if client.IsCode(err, codes.RegistryManifestNotFound) || client.IsNotFound(err) {
		return fmt.Errorf("no manifest %q in repo %q", ref, repo)
	}
	return mapRegistryRepoError(err, repo)
}

func manifestArchOSLabel(m *types.ManifestResponse) string {
	if m.Platform != nil && *m.Platform != "" {
		return *m.Platform
	}
	if m.Architecture != nil && m.OS != nil {
		return *m.OS + "/" + *m.Architecture
	}
	if m.Kind == "index" {
		return "multi-arch"
	}
	return "-"
}

func manifestSizeLabel(m *types.ManifestResponse) string {
	if m.ImageSizeBytes > 0 {
		return humanBytes(m.ImageSizeBytes)
	}
	if m.SizeBytes > 0 {
		return humanBytes(m.SizeBytes)
	}
	return "-"
}

func printManifestDetail(tw *tabwriter.Writer, m *types.ManifestResponse) {
	fmt.Fprintf(tw, "Digest:\t%s\n", m.Digest)
	fmt.Fprintf(tw, "Tag:\t%s\n", derefOr(m.Tag, "-"))
	fmt.Fprintf(tw, "Kind:\t%s\n", m.Kind)
	fmt.Fprintf(tw, "Media type:\t%s\n", m.MediaType)
	fmt.Fprintf(tw, "Manifest size:\t%s\n", humanBytes(m.SizeBytes))
	if m.ImageSizeBytes > 0 {
		fmt.Fprintf(tw, "Image size:\t%s\n", humanBytes(m.ImageSizeBytes))
	}
	fmt.Fprintf(tw, "Pushed:\t%s\n", m.PushedAt.Format(time.RFC3339))
	if m.HydratedAt != nil {
		fmt.Fprintf(tw, "Hydrated:\t%s\n", m.HydratedAt.Format(time.RFC3339))
	}
	if m.HydrationError != nil && *m.HydrationError != "" {
		fmt.Fprintf(tw, "Hydration error:\t%s\n", *m.HydrationError)
	}
	if m.Kind == "image" {
		fmt.Fprintf(tw, "Arch/OS:\t%s\n", manifestArchOSLabel(m))
		if m.Variant != nil && *m.Variant != "" {
			fmt.Fprintf(tw, "Variant:\t%s\n", *m.Variant)
		}
		if m.ConfigDigest != nil {
			fmt.Fprintf(tw, "Config digest:\t%s\n", *m.ConfigDigest)
		}
		if m.LayerCount != nil {
			fmt.Fprintf(tw, "Layers:\t%d\n", *m.LayerCount)
		}
		if m.ImageCreatedAt != nil {
			fmt.Fprintf(tw, "Image created:\t%s\n", m.ImageCreatedAt.Format(time.RFC3339))
		}
	}
	if len(m.Platforms) > 0 {
		fmt.Fprintln(tw, "Platforms:")
		for _, p := range m.Platforms {
			label := p.Platform
			if label == "" {
				label = p.OS + "/" + p.Architecture
			}
			extra := ""
			if p.ArtifactType != "" {
				extra = " [" + p.ArtifactType + "]"
			}
			fmt.Fprintf(tw, "  - %s\t%s\t%s%s\n",
				label, shortDigest(p.Digest), humanBytes(p.ImageSizeBytes), extra)
		}
	}
}

func humanBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for x := n / unit; x >= unit; x /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(n)/float64(div), "KMGTPE"[exp])
}
