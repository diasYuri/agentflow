package cli

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"
)

func newCompletionCommand(root *cobra.Command) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "completion bash|zsh|fish|powershell",
		Short: "Generate shell completion script",
		Long: `Generate shell completion scripts for agentflow.

To load completions:

Bash:
  source <(agentflow completion bash)
  # Or add to ~/.bashrc

Zsh:
  source <(agentflow completion zsh)
  # Or add to ~/.zshrc

Fish:
  agentflow completion fish | source
  # Or add to ~/.config/fish/completions/agentflow.fish

PowerShell:
  agentflow completion powershell | Out-String | Invoke-Expression
`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCompletion(cmd.OutOrStdout(), root, args[0])
		},
	}
	return cmd
}

func runCompletion(w io.Writer, root *cobra.Command, shell string) error {
	switch shell {
	case "bash":
		return root.GenBashCompletion(w)
	case "zsh":
		return root.GenZshCompletion(w)
	case "fish":
		return root.GenFishCompletion(w, true)
	case "powershell":
		return root.GenPowerShellCompletionWithDesc(w)
	default:
		return fmt.Errorf("unsupported shell %q; use bash, zsh, fish, or powershell", shell)
	}
}
