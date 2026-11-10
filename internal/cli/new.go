package cli

import (
	"github.com/spf13/cobra"

	"github.com/chancez/kspaces/internal/config"
	"github.com/chancez/kspaces/internal/spaces"
	"github.com/chancez/kspaces/internal/tui"
)

func (a *app) newCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "new [SPACE]",
		Short: "Create a worktree and switch to it, in the space given by root or name, or the current one",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			query := ""
			if len(args) > 0 {
				query = args[0]
			}
			space, err := spaces.Find(cfg, query)
			if err != nil {
				return err
			}
			return tui.RunNewWorktree(cmd.Context(), cfg, space)
		},
	}
}
