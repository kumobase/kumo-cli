package cli

import (
	"fmt"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/kumobase/kumo-go/client"
	"github.com/kumobase/kumo-go/codes"
	"github.com/kumobase/kumo-go/types"

	"github.com/kumobase/kumo-cli/internal/output"
)

func newAppsBuildsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "builds",
		Aliases: []string{"build"},
		Short:   "Inspect and control git-build app builds",
	}
	cmd.AddCommand(
		newAppsBuildsListCmd(),
		newAppsBuildsGetCmd(),
		newAppsBuildsLogsCmd(),
		newAppsBuildsRebuildCmd(),
		newAppsBuildsCancelCmd(),
	)
	return cmd
}

func newAppsBuildsListCmd() *cobra.Command {
	var page, pageSize int
	cmd := &cobra.Command{
		Use:   "list <app>",
		Short: "List builds for a git-build app",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, s, err := newClient()
			if err != nil {
				return err
			}
			appID, _, _, err := resolveAppRef(cmd.Context(), c, args[0])
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
			builds, meta, err := c.Builds().List(cmd.Context(), appID, opts...)
			if err != nil {
				return mapBuildError(err, appID)
			}
			return output.Print(cmd.OutOrStdout(), s.Output, builds, func(tw *tabwriter.Writer) {
				fmt.Fprintln(tw, "ID\tCOMMIT\tREF\tSTATUS\tCREATED\tFINISHED")
				for _, b := range builds {
					fmt.Fprintf(tw, "%d\t%s\t%s\t%s\t%s\t%s\n",
						b.ID, shortSHA(b.CommitSHA), orDash(b.Ref), b.Status,
						b.CreatedAt.Format(time.RFC3339), formatOptionalTime(b.FinishedAt, "-"))
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

func newAppsBuildsGetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "get <app> <build-id>",
		Short: "Show build detail",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, s, err := newClient()
			if err != nil {
				return err
			}
			appID, buildID, err := resolveAppAndBuild(cmd, c, args[0], args[1])
			if err != nil {
				return err
			}
			b, err := c.Builds().Get(cmd.Context(), appID, buildID)
			if err != nil {
				return mapBuildError(err, appID)
			}
			return output.Print(cmd.OutOrStdout(), s.Output, b, func(tw *tabwriter.Writer) {
				fmt.Fprintf(tw, "ID:\t%d\n", b.ID)
				fmt.Fprintf(tw, "App id:\t%d\n", b.AppID)
				fmt.Fprintf(tw, "Status:\t%s\n", b.Status)
				fmt.Fprintf(tw, "Commit:\t%s\n", b.CommitSHA)
				fmt.Fprintf(tw, "Ref:\t%s\n", orDash(b.Ref))
				if b.ImageDigest != "" {
					fmt.Fprintf(tw, "Image digest:\t%s\n", b.ImageDigest)
				}
				if b.Error != "" {
					fmt.Fprintf(tw, "Error:\t%s\n", b.Error)
				}
				fmt.Fprintf(tw, "Created:\t%s\n", b.CreatedAt.Format(time.RFC3339))
				fmt.Fprintf(tw, "Started:\t%s\n", formatOptionalTime(b.StartedAt, "-"))
				fmt.Fprintf(tw, "Finished:\t%s\n", formatOptionalTime(b.FinishedAt, "-"))
			})
		},
	}
}

func newAppsBuildsLogsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "logs <app> <build-id>",
		Short: "Print a fresh presigned URL to the build log",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, _, err := newClient()
			if err != nil {
				return err
			}
			appID, buildID, err := resolveAppAndBuild(cmd, c, args[0], args[1])
			if err != nil {
				return err
			}
			url, err := c.Builds().GetLogURL(cmd.Context(), appID, buildID)
			if err != nil {
				return mapBuildError(err, appID)
			}
			fmt.Fprintln(cmd.OutOrStdout(), url)
			return nil
		},
	}
}

func newAppsBuildsRebuildCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "rebuild <app>",
		Short: "Trigger a new build from the app's source",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, _, err := newClient()
			if err != nil {
				return err
			}
			appID, _, _, err := resolveAppRef(cmd.Context(), c, args[0])
			if err != nil {
				return err
			}
			if !flagYes {
				ok, err := confirm(cmd, fmt.Sprintf("Rebuild app %q from its latest source?", args[0]))
				if err != nil {
					return err
				}
				if !ok {
					return printAborted(cmd)
				}
			}
			b, err := c.Builds().Rebuild(cmd.Context(), appID, writeOpts("")...)
			if err != nil {
				return mapBuildError(err, appID)
			}
			return printResult(cmd, output.ActionResult{
				Resource: "build", ID: b.ID, Action: "rebuild", Status: string(b.Status),
				Message: fmt.Sprintf("Build %d queued (status %s)", b.ID, b.Status),
			})
		},
	}
	return cmd
}

func newAppsBuildsCancelCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "cancel <app> <build-id>",
		Short: "Cancel a running build",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, _, err := newClient()
			if err != nil {
				return err
			}
			appID, buildID, err := resolveAppAndBuild(cmd, c, args[0], args[1])
			if err != nil {
				return err
			}
			if !flagYes {
				ok, err := confirm(cmd, fmt.Sprintf("Cancel build %d for app %q?", buildID, args[0]))
				if err != nil {
					return err
				}
				if !ok {
					return printAborted(cmd)
				}
			}
			b, err := c.Builds().Cancel(cmd.Context(), appID, buildID)
			if err != nil {
				return mapBuildError(err, appID)
			}
			return printResult(cmd, output.ActionResult{
				Resource: "build", ID: b.ID, Action: "cancel", Status: string(b.Status),
				Message: fmt.Sprintf("Build %d %s", b.ID, b.Status),
			})
		},
	}
}

// newAppsBuildersCmd lists the build engines and language presets available for
// git-build apps (feeds `apps create --git … --language`).
func newAppsBuildersCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "builders",
		Short: "List build engines and language presets for git-build apps",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			c, s, err := newClient()
			if err != nil {
				return err
			}
			b, err := c.Builds().ListBuilders(cmd.Context())
			if err != nil {
				return err
			}
			return output.Print(cmd.OutOrStdout(), s.Output, b, func(tw *tabwriter.Writer) {
				fmt.Fprintln(tw, "BUILDER\tLABEL\tDEFAULT")
				for _, bl := range b.Builders {
					fmt.Fprintf(tw, "%s\t%s\t%t\n", bl.Kind, bl.Label, bl.Default)
				}
				fmt.Fprintln(tw, "\nLANGUAGES")
				for _, l := range b.Languages {
					fmt.Fprintf(tw, "%s\n", l.Value)
				}
			})
		},
	}
}

// resolveAppAndBuild resolves an app name and parses a build id in one step.
func resolveAppAndBuild(cmd *cobra.Command, c *client.Client, appName, buildArg string) (uint, uint, error) {
	appID, _, _, err := resolveAppRef(cmd.Context(), c, appName)
	if err != nil {
		return 0, 0, err
	}
	buildID, err := parseUintArg(buildArg, "build id")
	if err != nil {
		return 0, 0, err
	}
	return appID, buildID, nil
}

// mapBuildError translates build error codes into friendly messages.
func mapBuildError(err error, appID uint) error {
	switch {
	case err == nil:
		return nil
	case client.IsCode(err, codes.BuildNotFound) || client.IsNotFound(err):
		return fmt.Errorf("no such build for app %d", appID)
	case client.IsCode(err, codes.BuildAlreadyRunning):
		return fmt.Errorf("a build is already running for this app: %w", err)
	case client.IsCode(err, codes.BuildLogNotAvailable):
		return fmt.Errorf("build logs are not available (yet) for this build: %w", err)
	case client.IsCode(err, codes.BuildConnectionRequired):
		return fmt.Errorf("this app has no git source connection; it is not a git-build app: %w", err)
	case client.IsCode(err, codes.BuildSourceUnavailable):
		return fmt.Errorf("the git source is unavailable; check the connection and repo: %w", err)
	case client.IsCode(err, codes.BuildAppImageImmutable):
		return fmt.Errorf("this app's image is immutable and cannot be rebuilt: %w", err)
	case client.IsCode(err, codes.BuildNeedsBranch):
		return fmt.Errorf("a branch is required to build; pass --branch: %w", err)
	case client.IsCode(err, codes.BuildInvalidTagPattern):
		return fmt.Errorf("invalid --tag-pattern glob: %w", err)
	case client.IsCode(err, codes.BuildInvalidDockerfilePath):
		return fmt.Errorf("invalid --dockerfile-path (must be a clean relative path): %w", err)
	case client.IsCode(err, codes.BuildNoDockerfile):
		return fmt.Errorf("no Dockerfile found at the given path in the repo: %w", err)
	case client.IsCode(err, codes.BuildNoRailpackPlan):
		return fmt.Errorf("Railpack could not detect how to build this repo; try --language dockerfile: %w", err)
	case client.IsCode(err, codes.BuildProviderError):
		return fmt.Errorf("the git provider returned an error; try again shortly: %w", err)
	}
	return err
}

// gitBuildParams carries the flag values for the `apps create --git` path.
type gitBuildParams struct {
	name, image, pricingSlug       string
	port                           uint16
	exposed                        bool
	replicas                       int
	envs, secretVars, secretMounts []string
	tlsSecret                      string
	skipSecretChecks               bool
	connID                         uint
	connSet                        bool
	repo, branch, tagPattern       string
	language, dockerfilePath       string
	outputDir, buildCommand        string
	autoscaling                    *types.AutoscalingConfig
	healthCheck                    *types.HealthCheck
}

// runGitBuildCreate creates a git-build app via the Builds service. It is the
// git-source alternative to the registry-image `apps create` path.
func runGitBuildCreate(cmd *cobra.Command, p gitBuildParams) error {
	if p.image != "" {
		return fmt.Errorf("--image and --git/--repo are mutually exclusive")
	}
	if p.name == "" {
		return fmt.Errorf("--name is required")
	}
	if p.repo == "" {
		return fmt.Errorf("--repo (owner/repo) is required for a git-build app")
	}
	if p.pricingSlug == "" {
		return fmt.Errorf("--pricing-slug is required (see 'kumo apps plans')")
	}
	if p.branch != "" && p.tagPattern != "" {
		return fmt.Errorf("--branch and --tag-pattern are alternatives; set only one")
	}
	if p.dockerfilePath != "" && p.language != "dockerfile" {
		return fmt.Errorf("--dockerfile-path only applies with --language dockerfile")
	}
	if (p.outputDir != "" || p.buildCommand != "") && p.language != "static" {
		return fmt.Errorf("--output-dir/--build-command only apply with --language static")
	}

	c, s, err := newClient()
	if err != nil {
		return err
	}

	// Resolve the source connection: explicit --git, or the sole connection.
	connID := p.connID
	if !p.connSet {
		conns, err := c.SourceConnections().List(cmd.Context())
		if err != nil {
			return mapSourceError(err, 0)
		}
		switch len(conns) {
		case 0:
			return fmt.Errorf("no source connections found; connect one in the dashboard, then pass --git <id>")
		case 1:
			connID = conns[0].ID
		default:
			return fmt.Errorf("multiple source connections available; specify --git <id> (see 'kumo source list')")
		}
	}

	req := &types.CreateGitBuildAppRequest{
		Name:           p.name,
		Port:           p.port,
		IsExposed:      p.exposed,
		Replicas:       p.replicas,
		RepoFullName:   p.repo,
		Branch:         p.branch,
		TagPattern:     p.tagPattern,
		Language:       p.language,
		DockerfilePath: p.dockerfilePath,
		OutputDir:      p.outputDir,
		BuildCommand:   p.buildCommand,
		PricingSlug:    p.pricingSlug,
		Autoscaling:    p.autoscaling,
		HealthCheck:    p.healthCheck,
	}
	ev, err := parseEnvFlags(p.envs)
	if err != nil {
		return err
	}
	req.EnvironmentVariables = ev
	if p.secretVars != nil {
		sv, err := parseSecretVarFlags(p.secretVars)
		if err != nil {
			return err
		}
		req.SecretVars = sv
	}
	if p.secretMounts != nil {
		sm, err := parseSecretFileMountFlags(p.secretMounts)
		if err != nil {
			return err
		}
		req.SecretFileMounts = sm
	}
	if certificateSecretsEnabled && p.tlsSecret != "" {
		id, _, _, err := resolveSecretRef(cmd.Context(), c, p.tlsSecret)
		if err != nil {
			return err
		}
		req.TLSSecretId = &id
	}

	if !p.skipSecretChecks {
		if err := validateAppSecrets(cmd.Context(), c,
			collectAppSecretRefs("", p.tlsSecret, req.SecretVars, req.SecretFileMounts)); err != nil {
			return err
		}
	}

	res, err := c.Builds().CreateGitBuildApp(cmd.Context(), connID, req, writeOpts("")...)
	if err != nil {
		return mapBuildError(err, 0)
	}
	return output.Print(cmd.OutOrStdout(), s.Output, res, func(tw *tabwriter.Writer) {
		fmt.Fprintf(tw, "Created git-build app %q (id %d); first build queued — deployment %s\n",
			res.Name, res.ID, res.DeploymentStatus)
	})
}

// shortSHA truncates a git commit SHA to 12 chars for display.
func shortSHA(sha string) string {
	if len(sha) > 12 {
		return sha[:12]
	}
	if sha == "" {
		return "-"
	}
	return sha
}
