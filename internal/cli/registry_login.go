package cli

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"

	"github.com/kumobase/kumo-cli/internal/output"
)

const defaultRegistryHost = "registry.kumo.run"

func newRegistryLoginCmd() *cobra.Command {
	var host string
	cmd := &cobra.Command{
		Use:   "login",
		Short: "Authenticate docker against the Kumo registry",
		Long: "Reuses your personal API key as the docker password (the backend's\n" +
			"/v2/token endpoint accepts Basic Auth with your email + API key).\n" +
			"Run `kumo auth login` first if you don't have an active session.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			c, s, err := newClient()
			if err != nil {
				return err
			}
			if s.APIKey == "" {
				return fmt.Errorf("no API key on the active profile; run `kumo auth login` first")
			}
			profile, err := c.Profile().Get(cmd.Context())
			if err != nil {
				return fmt.Errorf("fetch profile: %w", err)
			}
			if profile.Email == "" {
				return fmt.Errorf("profile is missing an email address; cannot configure docker login")
			}
			h := resolveRegistryHost(host)
			if _, err := exec.LookPath("docker"); err != nil {
				fmt.Fprintf(cmd.OutOrStdout(),
					"docker is not installed. Run this command manually to log in:\n  echo $KUMO_API_KEY | docker login %s --username %s --password-stdin\n",
					h, profile.Email)
				return nil
			}
			dockerCmd := exec.CommandContext(cmd.Context(), "docker", "login", h,
				"--username", profile.Email, "--password-stdin")
			dockerCmd.Stdin = strings.NewReader(s.APIKey + "\n")
			dockerCmd.Stdout = cmd.OutOrStdout()
			dockerCmd.Stderr = cmd.ErrOrStderr()
			if err := dockerCmd.Run(); err != nil {
				return fmt.Errorf("docker login failed: %w", err)
			}
			return printResult(cmd, output.ActionResult{
				Resource: "registry-login", Action: "login", Status: "done",
				Message: fmt.Sprintf("Logged in to %s as %s.", h, profile.Email),
			})
		},
	}
	cmd.Flags().StringVar(&host, "registry-host", "", "registry host (default registry.kumo.run, env KUMO_REGISTRY_HOST)")
	return cmd
}

func newRegistryLogoutCmd() *cobra.Command {
	var host string
	cmd := &cobra.Command{
		Use:   "logout",
		Short: "Remove docker credentials for the Kumo registry",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			h := resolveRegistryHost(host)
			if _, err := exec.LookPath("docker"); err != nil {
				fmt.Fprintf(cmd.OutOrStdout(),
					"docker is not installed. Run this command manually to log out:\n  docker logout %s\n", h)
				return nil
			}
			dockerCmd := exec.CommandContext(cmd.Context(), "docker", "logout", h)
			dockerCmd.Stdout = cmd.OutOrStdout()
			dockerCmd.Stderr = cmd.ErrOrStderr()
			if err := dockerCmd.Run(); err != nil {
				return fmt.Errorf("docker logout failed: %w", err)
			}
			return printResult(cmd, output.ActionResult{
				Resource: "registry-login", Action: "logout", Status: "done",
				Message: fmt.Sprintf("Logged out of %s.", h),
			})
		},
	}
	cmd.Flags().StringVar(&host, "registry-host", "", "registry host (default registry.kumo.run, env KUMO_REGISTRY_HOST)")
	return cmd
}

func resolveRegistryHost(explicit string) string {
	if explicit != "" {
		return explicit
	}
	if env := os.Getenv("KUMO_REGISTRY_HOST"); env != "" {
		return env
	}
	return defaultRegistryHost
}
