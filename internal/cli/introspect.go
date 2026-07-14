package cli

import (
	"encoding/json"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// cmdInfo is the machine-readable description of one command in the tree.
type cmdInfo struct {
	Path        string     `json:"path"`
	Short       string     `json:"short,omitempty"`
	Args        string     `json:"args,omitempty"`
	Flags       []flagInfo `json:"flags,omitempty"`
	Subcommands []cmdInfo  `json:"subcommands,omitempty"`
}

type flagInfo struct {
	Name      string `json:"name"`
	Shorthand string `json:"shorthand,omitempty"`
	Type      string `json:"type"`
	Default   string `json:"default,omitempty"`
	Usage     string `json:"usage,omitempty"`
}

// newIntrospectCmd emits the full command tree (commands, flags, types,
// defaults) as a JSON envelope so an agent can discover the surface
// deterministically instead of scraping --help. Hidden from normal help.
func newIntrospectCmd() *cobra.Command {
	return &cobra.Command{
		Use:    "introspect",
		Short:  "Emit the command tree as JSON (for tooling and AI agents)",
		Hidden: true,
		Args:   cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			tree := describeCommand(cmd.Root(), "kumo")
			enc := json.NewEncoder(cmd.OutOrStdout())
			enc.SetIndent("", "  ")
			return enc.Encode(tree)
		},
	}
}

func describeCommand(cmd *cobra.Command, path string) cmdInfo {
	info := cmdInfo{Path: path, Short: cmd.Short, Args: cmd.Use}
	cmd.LocalFlags().VisitAll(func(f *pflag.Flag) {
		info.Flags = append(info.Flags, describeFlag(f))
	})
	for _, sub := range cmd.Commands() {
		if sub.Hidden || sub.Name() == "help" || sub.Name() == "completion" {
			continue
		}
		info.Subcommands = append(info.Subcommands, describeCommand(sub, path+" "+sub.Name()))
	}
	return info
}

func describeFlag(f *pflag.Flag) flagInfo {
	return flagInfo{
		Name:      f.Name,
		Shorthand: f.Shorthand,
		Type:      f.Value.Type(),
		Default:   f.DefValue,
		Usage:     f.Usage,
	}
}
