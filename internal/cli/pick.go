package cli

import (
	"github.com/spf13/cobra"

	"github.com/chancez/kspaces/internal/tui"
)

func (a *app) pickCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "pick",
		Short: "Pick a space, worktree or status entry to switch to, meant for a kitty overlay",
		Args:  cobra.NoArgs,
		// The picker loads the config itself, so an error shows in its UI.
		RunE: func(cmd *cobra.Command, _ []string) error { return tui.RunPicker(cmd.Context()) },
	}
}
