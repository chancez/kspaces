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
	var forcePaths, deletePaths, forceBranchPaths, forgetSessions []string
	cmd := &cobra.Command{
		Use:   "remove [PATH...]",
		Short: "Close the session of the linked worktree at PATH and remove the worktree, after confirming in a form",
		RunE: func(cmd *cobra.Command, args []string) error {
			switch {
			case len(forgetSessions) > 0 && !yes:
				return fmt.Errorf("--forget-session needs --yes")
			case len(args) == 0 && len(forgetSessions) == 0:
				return fmt.Errorf("give the PATH of a worktree to remove")
			}
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			if yes {
				return tui.RemoveWorktrees(cfg, args, removeOptions(args, force, deleteBranch, forceBranch, forcePaths, deletePaths, forceBranchPaths), forgetSessions)
			}
			if len(args) > 1 {
				return fmt.Errorf("the form removes one worktree; give --yes to remove several")
			}
			return tui.RunRemoveWorktree(cmd.Context(), cfg, args[0], deleteBranch)
		},
	}
	cmd.Flags().StringArrayVar(&forgetSessions, "forget-session", nil, "with --yes, after the worktrees, close this session's windows, wait for kitty to have, and delete its saved file, as for a missing worktree; repeatable")
	cmd.Flags().BoolVar(&yes, "yes", false, "remove without asking, as the form does once confirmed, one after another; errors are also shown in kitty")
	cmd.Flags().BoolVar(&force, "force", false, "with --yes, remove the worktrees even with changes that are not committed, losing them")
	cmd.Flags().StringArrayVar(&forcePaths, "force-path", nil, "with --yes, remove the worktree at this path even with changes that are not committed; repeatable")
	cmd.Flags().BoolVar(&deleteBranch, "delete-branch", false, "delete each worktree's local branch after removing it; with --yes refusing one with commits not merged and keeping its worktree, without it ticking the form's box")
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
		Use:   "close [SESSION...]",
		Short: "Close every window of the sessions, the current one by default, after confirming in a form",
		RunE: func(cmd *cobra.Command, args []string) error {
			return tui.RunCloseSession(cmd.Context(), args, yes)
		},
	}
	cmd.Flags().BoolVar(&yes, "yes", false, "close without asking")
	return cmd
}

func (a *app) forgetCommand() *cobra.Command {
	var yes bool
	cmd := &cobra.Command{
		Use:   "forget SESSION...",
		Short: "Delete the saved files of sessions with no windows open, such as missing worktrees', after confirming in a form",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			return tui.RunForgetSession(cmd.Context(), cfg, args, yes)
		},
	}
	cmd.Flags().BoolVar(&yes, "yes", false, "delete without asking")
	return cmd
}
