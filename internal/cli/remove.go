package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/chancez/kspaces/internal/config"
	"github.com/chancez/kspaces/internal/spaces"
	"github.com/chancez/kspaces/internal/tui"
)

func (a *app) removeCommand() *cobra.Command {
	var yes, force, deleteBranch, forceBranch bool
	var forcePaths, deletePaths, forceBranchPaths []string
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
				return tui.RemoveWorktrees(cfg, args, removeOptions(args, force, deleteBranch, forceBranch, forcePaths, deletePaths, forceBranchPaths))
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
	cmd.Flags().BoolVar(&deleteBranch, "delete-branch", false, "with --yes, delete each worktree's local branch after removing it, refusing one with commits not merged and keeping its worktree")
	cmd.Flags().StringArrayVar(&deletePaths, "delete-branch-path", nil, "with --yes, delete the branch of the worktree at this path, as --delete-branch does; repeatable")
	cmd.Flags().BoolVar(&forceBranch, "force-branch", false, "with --yes and --delete-branch, delete the branches even with commits not merged, losing them")
	cmd.Flags().StringArrayVar(&forceBranchPaths, "force-branch-path", nil, "with --yes, delete the branch of the worktree at this path even with commits not merged, as --force-branch does; repeatable")
	return cmd
}

// removeOptions are how to remove each of paths, from the remove command's flags: the ones for all of them,
// then the ones for one path each.
func removeOptions(paths []string, force, deleteBranch, forceBranch bool, forcePaths, deletePaths, forceBranchPaths []string) map[string]spaces.RemoveOptions {
	opts := map[string]spaces.RemoveOptions{}
	for _, p := range paths {
		opts[p] = spaces.RemoveOptions{Force: force, DeleteBranch: deleteBranch, ForceBranch: forceBranch}
	}
	set := func(ps []string, on func(*spaces.RemoveOptions)) {
		for _, p := range ps {
			o := opts[p]
			on(&o)
			opts[p] = o
		}
	}
	set(forcePaths, func(o *spaces.RemoveOptions) { o.Force = true })
	set(deletePaths, func(o *spaces.RemoveOptions) { o.DeleteBranch = true })
	set(forceBranchPaths, func(o *spaces.RemoveOptions) { o.ForceBranch = true })
	return opts
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

func (a *app) forgetCommand() *cobra.Command {
	var yes bool
	cmd := &cobra.Command{
		Use:   "forget SESSION",
		Short: "Delete the saved file of a session with no windows open, such as a missing worktree's, after confirming in a form",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			return tui.RunForgetSession(cmd.Context(), cfg, args[0], yes)
		},
	}
	cmd.Flags().BoolVar(&yes, "yes", false, "delete without asking")
	return cmd
}
