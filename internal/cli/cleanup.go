package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/chancez/kspaces/internal/config"
	"github.com/chancez/kspaces/internal/tui"
)

func (a *app) cleanupCommand() *cobra.Command {
	var sessions []string
	cmd := &cobra.Command{
		Use:   "cleanup [SPACE] [--session SESSION...]",
		Short: "Offer removing the linked worktrees of SPACE, a root or a name, whose PR is merged, upstream gone or branch merged, and its missing ones, or the worktrees of the sessions given, in a form",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if (len(args) == 1) == (len(sessions) > 0) {
				return fmt.Errorf("give a SPACE or one or more --session, not both")
			}
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			root := ""
			if len(args) == 1 {
				root = args[0]
			}
			return tui.RunCleanup(cmd.Context(), cfg, root, sessions)
		},
	}
	cmd.Flags().StringArrayVar(&sessions, "session", nil, "a session whose worktree to offer removing, in place of SPACE; repeatable")
	return cmd
}
