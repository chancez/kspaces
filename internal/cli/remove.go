package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/chancez/kspaces/internal/config"
	"github.com/chancez/kspaces/internal/tui"
)

func (a *app) removeCommand() *cobra.Command {
	var yes, force bool
	var forcePaths []string
	cmd := &cobra.Command{
		Use:   "remove PATH...",
		Short: "Close the session of the linked worktree at PATH and remove the worktree, after confirming in a form",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			if yes {
				forced := map[string]bool{}
				for _, p := range args {
					forced[p] = force
				}
				for _, p := range forcePaths {
					forced[p] = true
				}
				return tui.RemoveWorktrees(cfg, args, forced)
			}
			if len(args) > 1 {
				return fmt.Errorf("the form removes one worktree; give --yes to remove several")
			}
			return tui.RunRemoveWorktree(cmd.Context(), cfg, args[0])
		},
	}
	cmd.Flags().BoolVar(&yes, "yes", false, "remove without asking, as the form does once confirmed, one after another; errors are also shown in kitty")
	cmd.Flags().BoolVar(&force, "force", false, "with --yes, remove the worktrees even with changes that are not committed, losing them")
	cmd.Flags().StringArrayVar(&forcePaths, "force-path", nil, "with --yes, remove the worktree at this path even with changes that are not committed; repeatable")
	return cmd
}

func (a *app) closeCommand() *cobra.Command {
	var yes bool
	cmd := &cobra.Command{
		Use:   "close [SESSION]",
		Short: "Close every window of a session, the current one by default, after confirming in a form",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			session := ""
			if len(args) > 0 {
				session = args[0]
			}
			return tui.RunCloseSession(cmd.Context(), session, yes)
		},
	}
	cmd.Flags().BoolVar(&yes, "yes", false, "close without asking")
	return cmd
}
