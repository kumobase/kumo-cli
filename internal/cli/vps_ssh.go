package cli

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/kumobase/kumo-go/client"
	"github.com/kumobase/kumo-go/codes"

	"github.com/kumobase/kumo-cli/internal/output"
)

func newVPSPasswordCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "password <name>",
		Short: "Reveal the initial root password for a VPS instance",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, s, err := newClient()
			if err != nil {
				return err
			}
			id, _, err := resolveVPSRef(cmd.Context(), c, args[0])
			if err != nil {
				return err
			}
			pw, err := c.VPS().RevealInitialPassword(cmd.Context(), id)
			if err != nil {
				return err
			}
			payload := struct {
				Password string `json:"password"`
			}{Password: pw}
			return output.Print(cmd.OutOrStdout(), s.Output, payload, func(tw *tabwriter.Writer) {
				fmt.Fprintf(tw, "Password:\t%s\n", pw)
				fmt.Fprintln(tw, "Warning:\tThis is the initial password; rotate it after first login.")
			})
		},
	}
}

func newVPSResetPasswordCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "reset-password <name>",
		Short: "Reset the root password for a VPS instance",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, s, err := newClient()
			if err != nil {
				return err
			}
			id, v, err := resolveVPSRef(cmd.Context(), c, args[0])
			if err != nil {
				return err
			}
			if !flagYes {
				ok, err := confirm(cmd, fmt.Sprintf("Reset root password for vps %q (id %d)?", v.DisplayName, id))
				if err != nil {
					return err
				}
				if !ok {
					return printAborted(cmd)
				}
			}
			pw, err := c.VPS().ResetPassword(cmd.Context(), id)
			if err != nil {
				if client.IsCode(err, codes.SSHNotReady) {
					return fmt.Errorf("SSH setup not complete; provision the server first: %w", err)
				}
				return mapVPSActionError(err, args[0], v.Status)
			}
			payload := struct {
				Password string `json:"password"`
			}{Password: pw}
			return output.Print(cmd.OutOrStdout(), s.Output, payload, func(tw *tabwriter.Writer) {
				fmt.Fprintf(tw, "Password:\t%s\n", pw)
				fmt.Fprintln(tw, "Warning:\tThe new password is only valid after the reset action completes.")
			})
		},
	}
	return cmd
}

func newVPSSSHCmd() *cobra.Command {
	var (
		user     string
		identity string
	)
	cmd := &cobra.Command{
		Use:   "ssh <name> [-- ssh args...]",
		Short: "SSH into a VPS instance",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, _, err := newClient()
			if err != nil {
				return err
			}
			id, v, err := resolveVPSRef(cmd.Context(), c, args[0])
			if err != nil {
				return err
			}
			if v.Status != "running" {
				return fmt.Errorf("server is %s; start it first", v.Status)
			}
			if v.IPAddress == "" {
				return fmt.Errorf("server has no IP yet")
			}

			sshArgs := buildSSHArgs(v.SSHPort, identity, user, v.IPAddress, args[1:])
			sshPath, lookErr := exec.LookPath("ssh")
			if lookErr != nil {
				fmt.Fprintf(cmd.OutOrStdout(), "ssh not found on PATH. Connect manually:\n  ssh %s\n", strings.Join(sshArgs, " "))
				return nil
			}

			if !v.SSHSetupCompleted {
				pw, perr := c.VPS().RevealInitialPassword(cmd.Context(), id)
				if perr == nil && pw != "" {
					fmt.Fprintf(cmd.OutOrStdout(), "Initial password: %s\n", pw)
				}
			}

			ec := exec.Command(sshPath, sshArgs...)
			ec.Stdin = os.Stdin
			ec.Stdout = cmd.OutOrStdout()
			ec.Stderr = cmd.ErrOrStderr()
			return ec.Run()
		},
	}
	f := cmd.Flags()
	f.StringVar(&user, "user", "root", "SSH username")
	f.StringVar(&identity, "identity", "", "SSH private key path (passed to ssh -i)")
	return cmd
}

func buildSSHArgs(port int, identity, user, ip string, extra []string) []string {
	var args []string
	if port != 0 && port != 22 {
		args = append(args, "-p", strconv.Itoa(port))
	}
	if identity != "" {
		args = append(args, "-i", identity)
	}
	args = append(args, user+"@"+ip)
	args = append(args, extra...)
	return args
}
