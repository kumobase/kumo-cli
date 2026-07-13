package cli

import (
	"fmt"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/kumobase/kumo-go/types"

	"github.com/kumobase/kumo-cli/internal/output"
)

func newJobsCreateCmd() *cobra.Command {
	var (
		name              string
		kind              string
		image             string
		appName           string
		pricingSlug       string
		command           []string
		jobArgs           []string
		envs              []string
		secretEnvs        []string
		secretMounts      []string
		schedule          string
		timezone          string
		concurrencyPolicy string
		activeDeadline    int
		backoffLimit      int
	)
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a job",
		Long: "Create a one-off or scheduled job.\n\n" +
			"A standalone job runs its own --image. An app-attached job reuses an\n" +
			"existing app's image (--app). Omit --schedule for a manual (one-off) job;\n" +
			"give a cron expression for a scheduled job.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			k, err := parseJobKind(kind)
			if err != nil {
				return err
			}
			if name == "" {
				return fmt.Errorf("--name is required")
			}
			if pricingSlug == "" {
				return fmt.Errorf("--pricing-slug is required (see plan slugs in the dashboard)")
			}
			if concurrencyPolicy != "" {
				if _, err := parseConcurrencyPolicy(concurrencyPolicy); err != nil {
					return err
				}
			}

			req := &types.CreateJobRequest{
				Name:                  name,
				Kind:                  k,
				PricingSlug:           pricingSlug,
				Command:               command,
				Args:                  jobArgs,
				Schedule:              schedule,
				Timezone:              timezone,
				ConcurrencyPolicy:     types.JobConcurrencyPolicy(concurrencyPolicy),
				ActiveDeadlineSeconds: activeDeadline,
				BackoffLimit:          backoffLimit,
			}
			ev, err := parseEnvFlags(envs)
			if err != nil {
				return err
			}
			req.Env = ev
			refs, err := parseJobSecretFlags(secretEnvs, secretMounts)
			if err != nil {
				return err
			}
			req.SecretRefs = refs

			c, s, err := newClient()
			if err != nil {
				return err
			}

			switch k {
			case types.JobKindStandalone:
				if image == "" {
					return fmt.Errorf("a standalone job requires --image")
				}
				req.Image = image
			case types.JobKindAppAttached:
				if appName == "" {
					return fmt.Errorf("an app-attached job requires --app")
				}
				id, _, _, err := resolveAppRef(cmd.Context(), c, appName)
				if err != nil {
					return err
				}
				req.AppID = &id
			}

			res, err := c.Jobs().Create(cmd.Context(), req)
			if err != nil {
				return mapJobError(err, name)
			}
			return output.Print(cmd.OutOrStdout(), s.Output, res, func(tw *tabwriter.Writer) {
				fmt.Fprintf(tw, "Created job %q (id %d); deployment %s\n", res.Name, res.ID, res.DeploymentStatus)
			})
		},
	}
	f := cmd.Flags()
	f.StringVar(&name, "name", "", "job name")
	f.StringVar(&kind, "kind", "standalone", "job kind: standalone or app-attached")
	f.StringVar(&image, "image", "", "container image (standalone jobs)")
	f.StringVar(&appName, "app", "", "app name to attach to (app-attached jobs)")
	f.StringVar(&pricingSlug, "pricing-slug", "", "resource plan slug")
	f.StringArrayVar(&command, "command", nil, "override the image entrypoint (repeatable)")
	f.StringArrayVar(&jobArgs, "arg", nil, "argument passed to the command (repeatable)")
	f.StringArrayVar(&envs, "env", nil, "environment variable KEY=VALUE (repeatable)")
	f.StringArrayVar(&secretEnvs, "secret-env", nil, "env var from a secret: SECRET_NAME:KEY:ENV_NAME (repeatable)")
	f.StringArrayVar(&secretMounts, "secret-mount", nil, "mount a file secret: SECRET_NAME:/mount/path (repeatable)")
	f.StringVar(&schedule, "schedule", "", "cron schedule (omit for a one-off job)")
	f.StringVar(&timezone, "timezone", "", "IANA timezone for the schedule (e.g. UTC)")
	f.StringVar(&concurrencyPolicy, "concurrency-policy", "", "scheduled-job concurrency: Forbid, Allow, or Replace")
	f.IntVar(&activeDeadline, "active-deadline", 0, "max seconds a single run may take (0 = unset)")
	f.IntVar(&backoffLimit, "backoff-limit", 0, "number of retries before a run is marked failed")
	return cmd
}

func newJobsUpdateCmd() *cobra.Command {
	var (
		pricingSlug       string
		command           []string
		jobArgs           []string
		envs              []string
		secretEnvs        []string
		secretMounts      []string
		schedule          string
		timezone          string
		concurrencyPolicy string
		activeDeadline    int
		backoffLimit      int
	)
	cmd := &cobra.Command{
		Use:   "update <name>",
		Short: "Update a job",
		Long:  "Update a job. Only the flags you pass are changed.",
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

			req := &types.UpdateJobRequest{}
			f := cmd.Flags()
			if f.Changed("pricing-slug") {
				req.PricingSlug = pricingSlug
			}
			if f.Changed("command") {
				req.Command = command
			}
			if f.Changed("arg") {
				req.Args = jobArgs
			}
			if f.Changed("env") {
				ev, err := parseEnvFlags(envs)
				if err != nil {
					return err
				}
				req.Env = ev
			}
			if f.Changed("secret-env") || f.Changed("secret-mount") {
				refs, err := parseJobSecretFlags(secretEnvs, secretMounts)
				if err != nil {
					return err
				}
				req.SecretRefs = refs
			}
			if f.Changed("schedule") {
				req.Schedule = &schedule
			}
			if f.Changed("timezone") {
				req.Timezone = timezone
			}
			if f.Changed("concurrency-policy") {
				if _, err := parseConcurrencyPolicy(concurrencyPolicy); err != nil {
					return err
				}
				req.ConcurrencyPolicy = types.JobConcurrencyPolicy(concurrencyPolicy)
			}
			if f.Changed("active-deadline") {
				req.ActiveDeadlineSeconds = activeDeadline
			}
			if f.Changed("backoff-limit") {
				req.BackoffLimit = &backoffLimit
			}

			res, err := c.Jobs().Update(cmd.Context(), id, req)
			if err != nil {
				return mapJobError(err, args[0])
			}
			return output.Print(cmd.OutOrStdout(), s.Output, res, func(tw *tabwriter.Writer) {
				fmt.Fprintf(tw, "Updated job %q (id %d); deployment %s\n", res.Name, res.ID, res.DeploymentStatus)
			})
		},
	}
	f := cmd.Flags()
	f.StringVar(&pricingSlug, "pricing-slug", "", "resource plan slug")
	f.StringArrayVar(&command, "command", nil, "override the image entrypoint (repeatable)")
	f.StringArrayVar(&jobArgs, "arg", nil, "argument passed to the command (repeatable)")
	f.StringArrayVar(&envs, "env", nil, "environment variable KEY=VALUE (repeatable)")
	f.StringArrayVar(&secretEnvs, "secret-env", nil, "env var from a secret: SECRET_NAME:KEY:ENV_NAME (repeatable)")
	f.StringArrayVar(&secretMounts, "secret-mount", nil, "mount a file secret: SECRET_NAME:/mount/path (repeatable)")
	f.StringVar(&schedule, "schedule", "", "cron schedule (empty string clears it → one-off)")
	f.StringVar(&timezone, "timezone", "", "IANA timezone for the schedule")
	f.StringVar(&concurrencyPolicy, "concurrency-policy", "", "scheduled-job concurrency: Forbid, Allow, or Replace")
	f.IntVar(&activeDeadline, "active-deadline", 0, "max seconds a single run may take")
	f.IntVar(&backoffLimit, "backoff-limit", 0, "number of retries before a run is marked failed")
	return cmd
}

func newJobsDeleteCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "delete <name>",
		Short: "Delete a job",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, _, err := newClient()
			if err != nil {
				return err
			}
			id, j, err := resolveJobRef(cmd.Context(), c, args[0])
			if err != nil {
				return err
			}
			if !flagYes {
				ok, err := confirm(cmd, fmt.Sprintf("Delete job %q (id %d)? This cannot be undone.", j.Name, id))
				if err != nil {
					return err
				}
				if !ok {
					fmt.Fprintln(cmd.OutOrStdout(), "Aborted.")
					return nil
				}
			}
			if _, err := c.Jobs().Delete(cmd.Context(), id); err != nil {
				return mapJobError(err, args[0])
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Deletion queued for job %d\n", id)
			return nil
		},
	}
	return cmd
}

// parseJobKind maps the --kind flag to a types.JobKind, accepting the
// hyphenated CLI spelling "app-attached" as well as the wire "app_attached".
func parseJobKind(kind string) (types.JobKind, error) {
	switch kind {
	case "standalone":
		return types.JobKindStandalone, nil
	case "app-attached", "app_attached":
		return types.JobKindAppAttached, nil
	default:
		return "", fmt.Errorf("invalid --kind %q (use standalone or app-attached)", kind)
	}
}

// parseConcurrencyPolicy validates the --concurrency-policy flag.
func parseConcurrencyPolicy(p string) (types.JobConcurrencyPolicy, error) {
	switch types.JobConcurrencyPolicy(p) {
	case types.JobConcurrencyForbid, types.JobConcurrencyAllow, types.JobConcurrencyReplace:
		return types.JobConcurrencyPolicy(p), nil
	default:
		return "", fmt.Errorf("invalid --concurrency-policy %q (use Forbid, Allow, or Replace)", p)
	}
}

// parseJobSecretFlags builds JobSecretRefs from the env and mount flag forms.
// env form: SECRET_NAME:KEY:ENV_NAME (key inside the secret → env var name).
// mount form: SECRET_NAME:/mount/path (file secret mounted at an absolute path).
func parseJobSecretFlags(envs, mounts []string) ([]types.JobSecretRef, error) {
	var out []types.JobSecretRef
	for _, it := range envs {
		parts := strings.Split(it, ":")
		if len(parts) != 3 || parts[0] == "" || parts[1] == "" || parts[2] == "" {
			return nil, fmt.Errorf("invalid --secret-env %q: expected SECRET_NAME:KEY:ENV_NAME", it)
		}
		out = append(out, types.JobSecretRef{SecretName: parts[0], SourceFrom: parts[1], MountTo: parts[2]})
	}
	for _, it := range mounts {
		name, path, ok := strings.Cut(it, ":")
		if !ok || name == "" || !strings.HasPrefix(path, "/") {
			return nil, fmt.Errorf("invalid --secret-mount %q: expected SECRET_NAME:/mount/path", it)
		}
		out = append(out, types.JobSecretRef{SecretName: name, MountTo: path})
	}
	return out, nil
}
