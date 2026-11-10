package cli

import (
	"github.com/spf13/cobra"

	"github.com/chancez/kspaces/internal/config"
	"github.com/chancez/kspaces/internal/tui"
)

func (a *app) removeCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "remove PATH",
		Short: "Remove the linked worktree at PATH and close its session, after confirming in a form",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			return tui.RunRemoveWorktree(cmd.Context(), cfg, args[0])
		},
	}
}
