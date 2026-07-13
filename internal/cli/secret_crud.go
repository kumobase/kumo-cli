package cli

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/kumobase/kumo-go/client"
	"github.com/kumobase/kumo-go/codes"
	"github.com/kumobase/kumo-go/types"

	"github.com/kumobase/kumo-cli/internal/output"
)

// secretPayloadFlags holds the type-specific create/update flag values.
type secretPayloadFlags struct {
	registryHost string
	registryUser string
	registryPass string
	envs         []string
	fromFile     string
	content      string
	certFile     string
	keyFile      string
}

// secretFlagsByType maps each secret type to the names of the payload flags
// that belong to it. Used to reject flags from a non-selected type.
var secretFlagsByType = map[types.SecretType][]string{
	types.SecretTypeRegistry:    {"registry-host", "registry-username", "registry-password"},
	types.SecretTypeEnvVar:      {"env"},
	types.SecretTypeFile:        {"from-file", "content"},
	types.SecretTypeCertificate: {"cert-file", "key-file"},
}

// addSecretPayloadFlags registers the payload flags shared by create and
// update. Certificate flags are registered only when the type is gated on.
func addSecretPayloadFlags(cmd *cobra.Command, p *secretPayloadFlags) {
	f := cmd.Flags()
	f.StringVar(&p.registryHost, "registry-host", "", "registry host (registry type; defaults to Docker Hub)")
	f.StringVar(&p.registryUser, "registry-username", "", "registry username (registry type)")
	f.StringVar(&p.registryPass, "registry-password", "", "registry password (registry type)")
	f.StringArrayVar(&p.envs, "env", nil, "environment variable KEY=VALUE (env_var type, repeatable)")
	f.StringVar(&p.fromFile, "from-file", "", "read file content from a path (file type)")
	f.StringVar(&p.content, "content", "", "inline file content (file type)")
	if certificateSecretsEnabled {
		f.StringVar(&p.certFile, "cert-file", "", "PEM certificate file path (certificate type)")
		f.StringVar(&p.keyFile, "key-file", "", "PEM private key file path (certificate type)")
	}
}

// assertNoForeignSecretFlags returns an error if any payload flag belonging to
// a type other than t was changed.
func assertNoForeignSecretFlags(cmd *cobra.Command, t types.SecretType) error {
	allowed := map[string]bool{}
	for _, name := range secretFlagsByType[t] {
		allowed[name] = true
	}
	f := cmd.Flags()
	var foreign []string
	for st, names := range secretFlagsByType {
		if st == t {
			continue
		}
		for _, name := range names {
			if !allowed[name] && f.Changed(name) {
				foreign = append(foreign, "--"+name)
			}
		}
	}
	if len(foreign) > 0 {
		return fmt.Errorf("flag(s) %s do not apply to a %s secret", strings.Join(foreign, ", "), t)
	}
	return nil
}

func newSecretCreateCmd() *cobra.Command {
	var (
		name    string
		typ     string
		payload secretPayloadFlags
	)
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a secret",
		Long: "Create a secret of a given --type. Provide exactly the payload flags for\n" +
			"that type: registry (--registry-username/--registry-password), env_var\n" +
			"(--env), or file (--from-file/--content).",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if name == "" {
				return fmt.Errorf("--name is required")
			}
			t := types.SecretType(typ)
			if typ == "" {
				return fmt.Errorf("--type is required (one of %s)", enabledSecretTypesString())
			}
			if !isEnabledSecretType(t) {
				return fmt.Errorf("invalid --type %q (use one of %s)", typ, enabledSecretTypesString())
			}
			if err := assertNoForeignSecretFlags(cmd, t); err != nil {
				return err
			}

			req := &types.CreateSecretRequest{
				RequestSecretBase: types.RequestSecretBase{Name: name, Type: t},
			}
			if err := buildSecretPayload(req, t, &payload); err != nil {
				return err
			}

			c, s, err := newClient()
			if err != nil {
				return err
			}
			res, err := c.Secrets().Create(cmd.Context(), req)
			if err != nil {
				return err
			}
			return output.Print(cmd.OutOrStdout(), s.Output, res, func(tw *tabwriter.Writer) {
				fmt.Fprintf(tw, "Created secret %q (id %d, type %s)\n", res.Name, res.ID, res.Type)
			})
		},
	}
	f := cmd.Flags()
	f.StringVar(&name, "name", "", "secret name (3-100 chars)")
	f.StringVar(&typ, "type", "", "secret type ("+enabledSecretTypesString()+")")
	addSecretPayloadFlags(cmd, &payload)
	return cmd
}

// buildSecretPayload populates the payload field on req matching t, requiring a
// complete payload (used by create). Returns an error if required values are
// missing.
func buildSecretPayload(req *types.CreateSecretRequest, t types.SecretType, p *secretPayloadFlags) error {
	switch t {
	case types.SecretTypeRegistry:
		if p.registryUser == "" || p.registryPass == "" {
			return fmt.Errorf("registry secret requires --registry-username and --registry-password")
		}
		req.SecretRegistry = types.SecretRegistry{
			RegistryHost: p.registryHost,
			Username:     p.registryUser,
			Password:     p.registryPass,
		}
	case types.SecretTypeEnvVar:
		if len(p.envs) == 0 {
			return fmt.Errorf("env_var secret requires at least one --env KEY=VALUE")
		}
		ev, err := parseEnvFlags(p.envs)
		if err != nil {
			return err
		}
		req.EnvironmentVariables = ev
	case types.SecretTypeFile:
		body, err := fileSecretContent(p)
		if err != nil {
			return err
		}
		req.FileContent = body
	case types.SecretTypeCertificate:
		cc, err := certificateContent(p)
		if err != nil {
			return err
		}
		req.CertificateContent = cc
	default:
		return fmt.Errorf("unsupported secret type %q", t)
	}
	return nil
}

// fileSecretContent resolves the file payload from --from-file or --content.
func fileSecretContent(p *secretPayloadFlags) (string, error) {
	if p.fromFile != "" {
		b, err := os.ReadFile(p.fromFile)
		if err != nil {
			return "", fmt.Errorf("read --from-file %s: %w", p.fromFile, err)
		}
		return string(b), nil
	}
	if p.content != "" {
		return p.content, nil
	}
	return "", fmt.Errorf("file secret requires --from-file or --content")
}

// certificateContent resolves the certificate payload from --cert-file and
// --key-file.
func certificateContent(p *secretPayloadFlags) (types.CertificateContent, error) {
	if p.certFile == "" || p.keyFile == "" {
		return types.CertificateContent{}, fmt.Errorf("certificate secret requires --cert-file and --key-file")
	}
	cert, err := os.ReadFile(p.certFile)
	if err != nil {
		return types.CertificateContent{}, fmt.Errorf("read --cert-file %s: %w", p.certFile, err)
	}
	key, err := os.ReadFile(p.keyFile)
	if err != nil {
		return types.CertificateContent{}, fmt.Errorf("read --key-file %s: %w", p.keyFile, err)
	}
	return types.CertificateContent{Certificate: string(cert), PrivateKey: string(key)}, nil
}

func newSecretUpdateCmd() *cobra.Command {
	var (
		name    string
		payload secretPayloadFlags
	)
	cmd := &cobra.Command{
		Use:   "update <name>",
		Short: "Update a secret",
		Long: "Update a secret. The current secret is fetched first; --name and the\n" +
			"payload flags for its type are applied on top. The secret type is\n" +
			"immutable. An If-Match ETag is sent for optimistic concurrency.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, s, err := newClient()
			if err != nil {
				return err
			}
			id, current, etag, err := resolveSecretRef(cmd.Context(), c, args[0])
			if err != nil {
				return err
			}
			if err := assertNoForeignSecretFlags(cmd, current.Type); err != nil {
				return err
			}

			req := updateRequestFromSecret(current)
			f := cmd.Flags()
			if f.Changed("name") {
				req.Name = name
			}
			if err := applySecretPayloadOverrides(cmd, req, current.Type, &payload); err != nil {
				return err
			}

			res, err := c.Secrets().Update(cmd.Context(), id, req, writeOpts(etag)...)
			if err != nil {
				if errors.Is(err, client.ErrETagMismatch) {
					return fmt.Errorf("secret changed since it was read; re-run the update: %w", err)
				}
				return err
			}
			return output.Print(cmd.OutOrStdout(), s.Output, res, func(tw *tabwriter.Writer) {
				fmt.Fprintf(tw, "Updated secret %q (id %d, type %s)\n", res.Name, res.ID, res.Type)
			})
		},
	}
	f := cmd.Flags()
	f.StringVar(&name, "name", "", "new secret name")
	addSecretPayloadFlags(cmd, &payload)
	return cmd
}

// updateRequestFromSecret seeds an UpdateSecretRequest from the current secret
// so unspecified fields are preserved. The type is carried through unchanged
// (it is immutable server-side).
func updateRequestFromSecret(s *types.ResponseGetSecret) *types.UpdateSecretRequest {
	req := &types.UpdateSecretRequest{
		CreateSecretRequest: types.CreateSecretRequest{
			RequestSecretBase:    types.RequestSecretBase{Name: s.Name, Type: s.Type},
			EnvironmentVariables: s.EnvironmentVariables,
			FileContent:          s.FileContent,
		},
	}
	if s.SecretRegistry != nil {
		req.SecretRegistry = *s.SecretRegistry
	}
	if s.CertificateContent != nil {
		req.CertificateContent = *s.CertificateContent
	}
	return req
}

// applySecretPayloadOverrides mutates only the payload fields whose flags were
// changed, for the secret's (immutable) type.
func applySecretPayloadOverrides(cmd *cobra.Command, req *types.UpdateSecretRequest, t types.SecretType, p *secretPayloadFlags) error {
	f := cmd.Flags()
	switch t {
	case types.SecretTypeRegistry:
		if f.Changed("registry-host") {
			req.SecretRegistry.RegistryHost = p.registryHost
		}
		if f.Changed("registry-username") {
			req.SecretRegistry.Username = p.registryUser
		}
		if f.Changed("registry-password") {
			req.SecretRegistry.Password = p.registryPass
		}
	case types.SecretTypeEnvVar:
		if f.Changed("env") {
			ev, err := parseEnvFlags(p.envs)
			if err != nil {
				return err
			}
			req.EnvironmentVariables = ev
		}
	case types.SecretTypeFile:
		if f.Changed("from-file") || f.Changed("content") {
			body, err := fileSecretContent(p)
			if err != nil {
				return err
			}
			req.FileContent = body
		}
	case types.SecretTypeCertificate:
		if f.Changed("cert-file") || f.Changed("key-file") {
			cc, err := certificateContent(p)
			if err != nil {
				return err
			}
			req.CertificateContent = cc
		}
	}
	return nil
}

func newSecretDeleteCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "delete <name>",
		Short: "Delete a secret",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, _, err := newClient()
			if err != nil {
				return err
			}
			id, sec, _, err := resolveSecretRef(cmd.Context(), c, args[0])
			if err != nil {
				return err
			}
			if !flagYes {
				ok, err := confirm(cmd, fmt.Sprintf("Delete secret %q (id %d)? This cannot be undone.", sec.Name, id))
				if err != nil {
					return err
				}
				if !ok {
					return printAborted(cmd)
				}
			}
			if err := c.Secrets().Delete(cmd.Context(), id); err != nil {
				if client.IsCode(err, codes.SecretInUse) {
					return fmt.Errorf("secret %d is in use by one or more apps; run `kumo secret get %d` to see them, detach it, then retry: %w", id, id, err)
				}
				return err
			}
			return printResult(cmd, output.ActionResult{
				Resource: "secret", ID: id, Action: "delete", Status: "done",
				Message: fmt.Sprintf("Secret %d deleted", id),
			})
		},
	}
	return cmd
}

// maskValue renders a fixed mask for a sensitive value, never revealing its
// length or content.
func maskValue(string) string {
	return "••••••"
}
