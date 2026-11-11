package cli

import (
	"github.com/spf13/cobra"

	"github.com/chancez/kspaces/internal/config"
	"github.com/chancez/kspaces/internal/tui"
)

func (a *app) cleanupCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "cleanup SPACE",
		Short: "Offer removing the linked worktrees of SPACE, a root or a name, whose PR is merged, upstream gone or branch merged, in a form",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			return tui.RunCleanup(cmd.Context(), cfg, args[0])
		},
	}
}
