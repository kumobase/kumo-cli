package cli

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/kumobase/kumo-go/client"
	"github.com/kumobase/kumo-go/codes"
	"github.com/kumobase/kumo-go/types"

	"github.com/kumobase/kumo-cli/internal/manifest"
	"github.com/kumobase/kumo-cli/internal/output"
)

func newAppsCreateCmd() *cobra.Command {
	var (
		file               string
		name               string
		image              string
		pricingSlug        string
		port               uint16
		replicas           int
		exposed            bool
		registryCredential string
		envs               []string
		secretVars         []string
		secretMounts       []string
		tlsSecret          string
		skipSecretChecks   bool
		wait               bool
		validate           bool
		timeout            time.Duration

		// git-build flags (deploy from a git source instead of an image)
		gitConn        uint
		repo           string
		branch         string
		tagPattern     string
		language       string
		dockerfilePath string
		outputDir      string
		buildCommand   string
	)
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create (deploy) an application",
		Long: "Create an application from a registry image (--image) or from a git source\n" +
			"(--git <connID> --repo <owner/repo>), and/or a manifest file (-f app.yaml).\n" +
			"When both flags and a manifest are given, flags override the manifest.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Git-build path: a separate SDK surface (source-based, no image
			// pull pre-flight). Detected by the presence of --git or --repo.
			f0 := cmd.Flags()
			if f0.Changed("git") || f0.Changed("repo") {
				return runGitBuildCreate(cmd, gitBuildParams{
					name: name, image: image, port: port, exposed: exposed,
					replicas: replicas, pricingSlug: pricingSlug,
					envs: envs, secretVars: secretVars, secretMounts: secretMounts,
					tlsSecret: tlsSecret, skipSecretChecks: skipSecretChecks,
					connID: gitConn, connSet: f0.Changed("git"), repo: repo,
					branch: branch, tagPattern: tagPattern, language: language,
					dockerfilePath: dockerfilePath, outputDir: outputDir, buildCommand: buildCommand,
				})
			}

			req := &types.CreateAppRequest{}
			if file != "" {
				m, err := manifest.Load(file)
				if err != nil {
					return err
				}
				req = m.ToCreateRequest()
			}

			f := cmd.Flags()
			if f.Changed("name") {
				req.Name = name
			}
			if f.Changed("image") {
				req.Image = image
			}
			if f.Changed("port") {
				req.Port = port
			}
			if f.Changed("replicas") {
				req.Replicas = replicas
			}
			if f.Changed("exposed") {
				req.IsExposed = exposed
			}
			if f.Changed("pricing-slug") {
				req.PricingSlug = pricingSlug
			}
			if f.Changed("registry-credential") {
				req.RegistryCredentialName = registryCredential
				req.RegistryCredentialId = 0
			}
			if f.Changed("env") {
				ev, err := parseEnvFlags(envs)
				if err != nil {
					return err
				}
				req.EnvironmentVariables = ev
			}
			if f.Changed("secret-var") {
				sv, err := parseSecretVarFlags(secretVars)
				if err != nil {
					return err
				}
				req.SecretVars = sv
			}
			if f.Changed("secret-file-mount") {
				sm, err := parseSecretFileMountFlags(secretMounts)
				if err != nil {
					return err
				}
				req.SecretFileMounts = sm
			}
			if certificateSecretsEnabled && f.Changed("tls-secret") {
				req.TLSSecretName = tlsSecret
				req.TLSSecretId = nil
			}

			if req.Name == "" || req.Image == "" {
				return fmt.Errorf("both --name and --image are required (via flags or manifest)")
			}

			c, s, err := newClient()
			if err != nil {
				return err
			}

			if !skipSecretChecks {
				if err := validateAppSecrets(cmd.Context(), c,
					collectAppSecretRefs(req.RegistryCredentialName, req.TLSSecretName, req.SecretVars, req.SecretFileMounts)); err != nil {
					return err
				}
			}

			// Pre-flight: every create validates that the image is pullable
			// before deploying. linux/amd64 is the required minimum platform;
			// linux/arm64 is optional.
			validateReq := &types.ValidateImagePullableRequest{
				Image:                req.Image,
				RegistryCredentialId: req.RegistryCredentialId,
			}
			if req.RegistryCredentialName != "" {
				rc := req.RegistryCredentialName
				validateReq.RegistryCredentialName = &rc
			}
			vres, err := c.Apps().ValidateImagePullable(cmd.Context(), validateReq)
			if err != nil {
				return err
			}

			// --validate is a check-only dry run: report pullability, then stop
			// without deploying.
			if validate {
				if err := output.Print(cmd.OutOrStdout(), s.Output, vres, func(tw *tabwriter.Writer) {
					if vres.LinuxAmd64 {
						fmt.Fprintf(tw, "Image %q is pullable:\n", req.Image)
					} else {
						fmt.Fprintf(tw, "Image %q is NOT pullable:\n", req.Image)
					}
					fmt.Fprintf(tw, "  linux/amd64\t%s\n", checkMark(vres.LinuxAmd64))
					fmt.Fprintf(tw, "  linux/arm64\t%s\n", checkMark(vres.LinuxArm64))
					if vres.LinuxAmd64 {
						fmt.Fprintln(tw, "(validation only; re-run without --validate to deploy)")
					}
				}); err != nil {
					return err
				}
				if !vres.LinuxAmd64 {
					return fmt.Errorf("image %q is not pullable for linux/amd64 (the required platform)", req.Image)
				}
				return nil
			}

			// Normal deploy: abort unless the required minimum platform is pullable.
			if !vres.LinuxAmd64 {
				return fmt.Errorf("image %q is not pullable for linux/amd64 (the required platform); aborting deploy", req.Image)
			}

			if !wait {
				res, err := c.Apps().Create(cmd.Context(), req)
				if err != nil {
					return err
				}
				return output.Print(cmd.OutOrStdout(), s.Output, res, func(tw *tabwriter.Writer) {
					fmt.Fprintf(tw, "Created app %q (id %d); operation %s queued\n", res.Name, res.ID, res.OperationID)
				})
			}

			fmt.Fprintf(cmd.ErrOrStderr(), "Deploying %q…\n", req.Name)
			app, err := c.Apps().CreateAndWait(cmd.Context(), req, pollOpts(timeout)...)
			if err != nil {
				return err
			}
			return output.Print(cmd.OutOrStdout(), s.Output, app, func(tw *tabwriter.Writer) {
				fmt.Fprintf(tw, "App %q (id %d) deployed — status %s\n", app.Name, app.Id, appStatus(app.AppStatus, app.IsSuspended))
				if app.IsExposed && app.GeneratedSubDomain != "" {
					fmt.Fprintf(tw, "URL:\thttps://%s\n", app.GeneratedSubDomain)
				}
			})
		},
	}
	f := cmd.Flags()
	f.StringVarP(&file, "file", "f", "", "manifest file (app.yaml)")
	f.StringVar(&name, "name", "", "application name (6-100 chars)")
	f.StringVar(&image, "image", "", "container image reference")
	f.Uint16Var(&port, "port", 0, "container port")
	f.IntVar(&replicas, "replicas", 1, "number of replicas")
	f.BoolVar(&exposed, "exposed", false, "expose the app publicly")
	f.StringVar(&pricingSlug, "pricing-slug", "", "pricing plan slug (see 'kumo apps plans')")
	f.StringVar(&registryCredential, "registry-credential", "", "registry credential secret name for private images")
	f.StringArrayVar(&envs, "env", nil, "environment variable KEY=VALUE (repeatable)")
	f.StringArrayVar(&secretVars, "secret-var", nil, "attach a secret as env vars: SECRET_NAME[:restart] (repeatable)")
	f.StringArrayVar(&secretMounts, "secret-file-mount", nil, "mount a secret as a file: SECRET_NAME:/mount/path[:restart] (repeatable)")
	if certificateSecretsEnabled {
		f.StringVar(&tlsSecret, "tls-secret", "", "certificate secret name to use for TLS termination")
	}
	f.BoolVar(&skipSecretChecks, "skip-secret-checks", false, "skip client-side validation that attached secrets exist and match the expected type")
	f.BoolVar(&wait, "wait", true, "wait for the deployment to complete")
	f.BoolVar(&validate, "validate", false, "check whether the image is pullable and exit without deploying")
	f.DurationVar(&timeout, "timeout", pollTimeout, "max time to wait when --wait is set")
	// git-build flags (alternative to --image; deploy from a git source)
	f.UintVar(&gitConn, "git", 0, "source connection id to build from (see 'kumo source list')")
	f.StringVar(&repo, "repo", "", "git repository owner/repo to build (with --git)")
	f.StringVar(&branch, "branch", "", "git branch to build (git-build apps)")
	f.StringVar(&tagPattern, "tag-pattern", "", "git tag glob to build, e.g. 'v*' (git-build apps)")
	f.StringVar(&language, "language", "", "build preset: auto|railpack|dockerfile|cnb|nodejs|…|static (see 'kumo apps builders')")
	f.StringVar(&dockerfilePath, "dockerfile-path", "", "Dockerfile path (with --language dockerfile)")
	f.StringVar(&outputDir, "output-dir", "", "static output dir to serve (with --language static)")
	f.StringVar(&buildCommand, "build-command", "", "build command before serving (with --language static)")
	return cmd
}

// checkMark renders a tick or cross for a boolean result.
func checkMark(ok bool) string {
	if ok {
		return "✓"
	}
	return "✗"
}

func newAppsUpdateCmd() *cobra.Command {
	var (
		file               string
		name               string
		image              string
		pricingSlug        string
		port               uint16
		replicas           int
		exposed            bool
		registryCredential string
		envs               []string
		secretVars         []string
		secretMounts       []string
		tlsSecret          string
		skipSecretChecks   bool
		wait               bool
		timeout            time.Duration
	)
	cmd := &cobra.Command{
		Use:   "update <name>",
		Short: "Update an application",
		Long: "Update an application. The current spec is fetched first; flags and/or a\n" +
			"manifest file (-f app.yaml) are applied on top. An If-Match ETag is sent for\n" +
			"optimistic concurrency.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, s, err := newClient()
			if err != nil {
				return err
			}
			id, current, etag, err := resolveAppRef(cmd.Context(), c, args[0])
			if err != nil {
				return err
			}

			// Seed from the current spec so unspecified fields are preserved.
			req := updateFromCurrent(current)

			if file != "" {
				m, err := manifest.Load(file)
				if err != nil {
					return err
				}
				req = m.ToUpdateRequest()
			}

			f := cmd.Flags()
			if f.Changed("name") {
				v := name
				req.Name = &v
			}
			if f.Changed("image") {
				v := image
				req.Image = &v
			}
			if f.Changed("port") {
				v := port
				req.Port = &v
			}
			if f.Changed("replicas") {
				v := replicas
				req.Replicas = &v
			}
			if f.Changed("exposed") {
				v := exposed
				req.IsExposed = &v
			}
			if f.Changed("pricing-slug") {
				v := pricingSlug
				req.PricingSlug = &v
			}
			if f.Changed("registry-credential") {
				v := registryCredential
				req.RegistryCredentialName = &v
				req.RegistryCredentialId = nil
			}
			if f.Changed("env") {
				ev, err := parseEnvFlags(envs)
				if err != nil {
					return err
				}
				req.EnvironmentVariables = ev
			}
			if f.Changed("secret-var") {
				sv, err := parseSecretVarFlags(secretVars)
				if err != nil {
					return err
				}
				req.SecretVars = sv
			}
			if f.Changed("secret-file-mount") {
				sm, err := parseSecretFileMountFlags(secretMounts)
				if err != nil {
					return err
				}
				req.SecretFileMounts = sm
			}
			if certificateSecretsEnabled && f.Changed("tls-secret") {
				v := tlsSecret
				req.TLSSecretName = &v
				req.TLSSecretId = nil
			}

			if !skipSecretChecks {
				regCred := ""
				if req.RegistryCredentialName != nil {
					regCred = *req.RegistryCredentialName
				}
				tlsName := ""
				if req.TLSSecretName != nil {
					tlsName = *req.TLSSecretName
				}
				if err := validateAppSecrets(cmd.Context(), c,
					collectAppSecretRefs(regCred, tlsName, req.SecretVars, req.SecretFileMounts)); err != nil {
					return err
				}
			}

			since := time.Now()
			if err := c.Apps().Update(cmd.Context(), id, req, writeOpts(etag)...); err != nil {
				if errors.Is(err, client.ErrETagMismatch) {
					return fmt.Errorf("app changed since it was read; re-run the update: %w", err)
				}
				return err
			}

			if !wait {
				fmt.Fprintf(cmd.OutOrStdout(), "Update queued for app %d\n", id)
				return nil
			}
			fmt.Fprintf(cmd.ErrOrStderr(), "Updating app %d…\n", id)
			if _, err := waitForOperation(cmd.Context(), c, id, types.AppOperationActionUpdate, since, timeout); err != nil {
				return err
			}
			app, _, err := c.Apps().Get(cmd.Context(), id)
			if err != nil {
				return err
			}
			return output.Print(cmd.OutOrStdout(), s.Output, app, func(tw *tabwriter.Writer) {
				fmt.Fprintf(tw, "App %q (id %d) updated — status %s\n", app.Name, app.Id, appStatus(app.AppStatus, app.IsSuspended))
			})
		},
	}
	f := cmd.Flags()
	f.StringVarP(&file, "file", "f", "", "manifest file (app.yaml)")
	f.StringVar(&name, "name", "", "application name")
	f.StringVar(&image, "image", "", "container image reference")
	f.Uint16Var(&port, "port", 0, "container port")
	f.IntVar(&replicas, "replicas", 1, "number of replicas")
	f.BoolVar(&exposed, "exposed", false, "expose the app publicly")
	f.StringVar(&pricingSlug, "pricing-slug", "", "pricing plan slug (see 'kumo apps plans')")
	f.StringVar(&registryCredential, "registry-credential", "", "registry credential secret name for private images")
	f.StringArrayVar(&envs, "env", nil, "environment variable KEY=VALUE (repeatable)")
	f.StringArrayVar(&secretVars, "secret-var", nil, "attach a secret as env vars: SECRET_NAME[:restart] (repeatable)")
	f.StringArrayVar(&secretMounts, "secret-file-mount", nil, "mount a secret as a file: SECRET_NAME:/mount/path[:restart] (repeatable)")
	if certificateSecretsEnabled {
		f.StringVar(&tlsSecret, "tls-secret", "", "certificate secret name to use for TLS termination")
	}
	f.BoolVar(&skipSecretChecks, "skip-secret-checks", false, "skip client-side validation that attached secrets exist and match the expected type")
	f.BoolVar(&wait, "wait", true, "wait for the update to complete")
	f.DurationVar(&timeout, "timeout", pollTimeout, "max time to wait when --wait is set")
	return cmd
}

// parseSecretVarFlags parses repeated "SECRET_NAME[:restart]" flags into
// SecretVars. The optional ":restart" suffix sets RestartWhenUpdated.
func parseSecretVarFlags(items []string) ([]types.SecretVar, error) {
	out := make([]types.SecretVar, 0, len(items))
	for _, it := range items {
		nameStr, rest, hasRest := strings.Cut(it, ":")
		if nameStr == "" || strings.ContainsAny(nameStr, "/ ") {
			return nil, fmt.Errorf("invalid --secret-var %q: expected SECRET_NAME[:restart]", it)
		}
		restart, err := parseRestartSuffix(it, rest, hasRest)
		if err != nil {
			return nil, err
		}
		out = append(out, types.SecretVar{SecretName: nameStr, RestartWhenUpdated: restart})
	}
	return out, nil
}

// parseSecretFileMountFlags parses repeated "SECRET_NAME:/mount/path[:restart]"
// flags into SecretFileMounts of type secret_file.
func parseSecretFileMountFlags(items []string) ([]types.SecretFileMount, error) {
	out := make([]types.SecretFileMount, 0, len(items))
	for _, it := range items {
		nameStr, rest, ok := strings.Cut(it, ":")
		if !ok || nameStr == "" || strings.ContainsAny(nameStr, "/ ") {
			return nil, fmt.Errorf("invalid --secret-file-mount %q: expected SECRET_NAME:/mount/path[:restart]", it)
		}
		mountPath, restStr, hasRest := strings.Cut(rest, ":")
		if !strings.HasPrefix(mountPath, "/") {
			return nil, fmt.Errorf("invalid --secret-file-mount %q: mount path must be absolute", it)
		}
		restart, err := parseRestartSuffix(it, restStr, hasRest)
		if err != nil {
			return nil, err
		}
		out = append(out, types.SecretFileMount{
			Type:               types.SecretFileMountTypeSecretFile,
			MountTo:            mountPath,
			SecretName:         nameStr,
			RestartWhenUpdated: restart,
		})
	}
	return out, nil
}

// parseRestartSuffix interprets the optional trailing "restart" token shared by
// the secret-attach flag parsers.
func parseRestartSuffix(item, suffix string, present bool) (bool, error) {
	if !present || suffix == "" {
		return false, nil
	}
	if suffix == "restart" {
		return true, nil
	}
	return false, fmt.Errorf("invalid suffix %q in %q: only \"restart\" is allowed", suffix, item)
}

// appSecretRef is one secret an app references, paired with the secret type
// the referencing field expects. Secrets are addressed by name; the
// validation step resolves the name to confirm it exists and has the right
// type before the async create/update is queued.
type appSecretRef struct {
	name     string
	flag     string // human label, e.g. "--secret-var"
	expected types.SecretType
}

// collectAppSecretRefs gathers every secret the outgoing app request references,
// tagging each with the type its field requires. Empty names are skipped.
func collectAppSecretRefs(registryCredential, tlsSecret string,
	vars []types.SecretVar, mounts []types.SecretFileMount) []appSecretRef {
	var refs []appSecretRef
	if registryCredential != "" {
		refs = append(refs, appSecretRef{name: registryCredential, flag: "--registry-credential", expected: types.SecretTypeRegistry})
	}
	if tlsSecret != "" {
		refs = append(refs, appSecretRef{name: tlsSecret, flag: "--tls-secret", expected: types.SecretTypeCertificate})
	}
	for _, v := range vars {
		if v.SecretName != "" {
			refs = append(refs, appSecretRef{name: v.SecretName, flag: "--secret-var", expected: types.SecretTypeEnvVar})
		}
	}
	for _, m := range mounts {
		if m.SecretName != "" {
			refs = append(refs, appSecretRef{name: m.SecretName, flag: "--secret-file-mount", expected: types.SecretTypeFile})
		}
	}
	return refs
}

// validateAppSecrets resolves each referenced secret once (deduped by name)
// and fails fast if it is missing or the wrong type, turning a cryptic async
// server error into an immediate, actionable message.
func validateAppSecrets(ctx context.Context, c *client.Client, refs []appSecretRef) error {
	seen := map[string]bool{}
	for _, r := range refs {
		if seen[r.name] {
			continue
		}
		seen[r.name] = true
		sec, _, err := c.Secrets().GetByName(ctx, r.name)
		if err != nil {
			if client.IsCode(err, codes.AmbiguousName) {
				return fmt.Errorf("%s references secret %q, but multiple secrets share that name; pass a unique name", r.flag, r.name)
			}
			if client.IsNotFound(err) {
				return fmt.Errorf("%s references secret %q, which does not exist", r.flag, r.name)
			}
			return err
		}
		if sec.Type != r.expected {
			return fmt.Errorf("%s requires a %q secret, but secret %q is type %q",
				r.flag, r.expected, r.name, sec.Type)
		}
	}
	return nil
}

func newAppsDeleteCmd() *cobra.Command {
	var (
		wait    bool
		timeout time.Duration
	)
	cmd := &cobra.Command{
		Use:   "delete <name>",
		Short: "Delete an application",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, _, err := newClient()
			if err != nil {
				return err
			}
			id, app, _, err := resolveAppRef(cmd.Context(), c, args[0])
			if err != nil {
				return err
			}
			if !flagYes {
				ok, err := confirm(cmd, fmt.Sprintf("Delete app %q (id %d)? This cannot be undone.", app.Name, id))
				if err != nil {
					return err
				}
				if !ok {
					fmt.Fprintln(cmd.OutOrStdout(), "Aborted.")
					return nil
				}
			}
			if err := c.Apps().Delete(cmd.Context(), id); err != nil {
				return err
			}
			if !wait {
				fmt.Fprintf(cmd.OutOrStdout(), "Deletion queued for app %d\n", id)
				return nil
			}
			fmt.Fprintf(cmd.ErrOrStderr(), "Deleting app %d…\n", id)
			if err := waitForDeletion(cmd, c, id, timeout); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "App %d deleted\n", id)
			return nil
		},
	}
	f := cmd.Flags()
	f.BoolVar(&wait, "wait", false, "wait until the app is fully deleted")
	f.DurationVar(&timeout, "timeout", pollTimeout, "max time to wait when --wait is set")
	return cmd
}

// waitForDeletion polls Get until the app returns a not-found error.
func waitForDeletion(cmd *cobra.Command, c *client.Client, id uint, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		_, _, err := c.Apps().Get(cmd.Context(), id)
		if err != nil {
			if client.IsNotFound(err) {
				return nil
			}
			return err
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("timed out after %s waiting for deletion", timeout)
		}
		select {
		case <-cmd.Context().Done():
			return cmd.Context().Err()
		case <-time.After(2 * time.Second):
		}
	}
}

// updateFromCurrent seeds a PATCH-style UpdateAppRequest from the current
// app spec so unspecified fields round-trip unchanged. Every field is sent
// as a present pointer; flag-driven overrides replace them in place.
func updateFromCurrent(app *types.AppByIdResponse) *types.UpdateAppRequest {
	c := app.CreateAppRequest
	name := c.Name
	image := c.Image
	port := c.Port
	exposed := c.IsExposed
	replicas := c.Replicas
	pricing := c.PricingSlug
	req := &types.UpdateAppRequest{
		Name:                 &name,
		Image:                &image,
		Port:                 &port,
		IsExposed:            &exposed,
		Replicas:             &replicas,
		Autoscaling:          c.Autoscaling,
		PricingSlug:          &pricing,
		EnvironmentVariables: c.EnvironmentVariables,
		SecretVars:           c.SecretVars,
		SecretFileMounts:     c.SecretFileMounts,
		HealthCheck:          c.HealthCheck,
	}
	if c.RegistryCredentialId != 0 {
		v := c.RegistryCredentialId
		req.RegistryCredentialId = &v
	}
	if c.RegistryCredentialName != "" {
		v := c.RegistryCredentialName
		req.RegistryCredentialName = &v
	}
	if c.TLSSecretId != nil {
		v := *c.TLSSecretId
		req.TLSSecretId = &v
	}
	if c.TLSSecretName != "" {
		v := c.TLSSecretName
		req.TLSSecretName = &v
	}
	return req
}

// parseEnvFlags parses repeated KEY=VALUE flags into environment variables.
func parseEnvFlags(pairs []string) ([]types.EnvironmentVariable, error) {
	out := make([]types.EnvironmentVariable, 0, len(pairs))
	for _, p := range pairs {
		k, v, ok := strings.Cut(p, "=")
		if !ok || k == "" {
			return nil, fmt.Errorf("invalid --env %q: expected KEY=VALUE", p)
		}
		out = append(out, types.EnvironmentVariable{Key: k, Value: v})
	}
	return out, nil
}
