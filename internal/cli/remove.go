package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/chancez/kspaces/internal/config"
	"github.com/chancez/kspaces/internal/kitty"
	"github.com/chancez/kspaces/internal/spaces"
	"github.com/chancez/kspaces/internal/tui"
)

func (a *app) removeCommand() *cobra.Command {
	var yes, force, deleteBranch, forceBranch bool
	var forcePaths, deletePaths, forceBranchPaths, forgetSessions []string
	var sessions string
	var window int
	cmd := &cobra.Command{
		Use:   "remove [PATH...]",
		Short: "Close the session of the linked worktree at PATH and remove the worktree, after confirming in a form",
		RunE: func(cmd *cobra.Command, args []string) error {
			switch {
			case window != 0 && len(args) > 0:
				return fmt.Errorf("--from removes the window's worktree; give no PATH")
			case window != 0:
				return a.popupForWorktree(window, func(cfg config.Config, st kitty.State, s spaces.Space, wt spaces.Worktree) error {
					if wt.Path == s.Worktrees[0].Path {
						return fmt.Errorf("%s is the main worktree of %s, which cannot be removed", config.Tilde(wt.Path), s.Name)
					}
					return spaces.AskRemoveWorktree(cfg, st, wt.Path)
				})
			case len(forgetSessions) > 0 && !yes:
				return fmt.Errorf("--forget-session needs --yes")
			case len(args) == 0 && len(forgetSessions) == 0:
				return fmt.Errorf("give the PATH of a worktree to remove")
			}
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			fate, err := spaces.ParseFate(sessions)
			if err != nil {
				return fmt.Errorf("--sessions: %w", err)
			}
			if yes {
				opts := removeOptions(args, force, deleteBranch, forceBranch, forcePaths, deletePaths, forceBranchPaths)
				for p, o := range opts {
					o.Sessions = fate
					opts[p] = o
				}
				return tui.RemoveWorktrees(cfg, args, opts, forgetSessions, fate)
			}
			if len(args) > 1 {
				return fmt.Errorf("the form removes one worktree; give --yes to remove several")
			}
			return tui.RunRemoveWorktree(cmd.Context(), cfg, args[0], deleteBranch)
		},
	}
	cmd.Flags().StringArrayVar(&forgetSessions, "forget-session", nil, "with --yes, after the worktrees, close this session's windows, wait for kitty to have, and delete its saved file, as for a missing worktree; repeatable")
	cmd.Flags().BoolVar(&yes, "yes", false, "remove without asking, as the form does once confirmed, one after another; errors are also shown in kitty")
	cmd.Flags().StringVar(&sessions, "sessions", "", "with --yes, what becomes of the session provider's sessions the windows show, and those tagged with the worktree: end (the default), shelve or leave")
	cmd.Flags().BoolVar(&force, "force", false, "with --yes, remove the worktrees even with changes that are not committed, losing them")
	cmd.Flags().StringArrayVar(&forcePaths, "force-path", nil, "with --yes, remove the worktree at this path even with changes that are not committed; repeatable")
	cmd.Flags().BoolVar(&deleteBranch, "delete-branch", false, "delete each worktree's local branch after removing it; with --yes refusing one with commits not merged and keeping its worktree, without it ticking the form's box")
	cmd.Flags().StringArrayVar(&deletePaths, "delete-branch-path", nil, "with --yes, delete the branch of the worktree at this path, as --delete-branch does; repeatable")
	cmd.Flags().BoolVar(&forceBranch, "force-branch", false, "with --yes and --delete-branch, delete the branches even with commits not merged, losing them")
	cmd.Flags().StringArrayVar(&forceBranchPaths, "force-branch-path", nil, "with --yes, delete the branch of the worktree at this path even with commits not merged, as --force-branch does; repeatable")
	fromFlag(cmd, &window, "the worktree of its session")
	cmd.MarkFlagsMutuallyExclusive("from", "yes")
	cmd.MarkFlagsMutuallyExclusive("from", "delete-branch")
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
	var window int
	var sessions string
	cmd := &cobra.Command{
		Use:   "close [SESSION...]",
		Short: "Close every window of the sessions, the current one by default, after confirming in a form; with the session saver, opening one again brings its windows back",
		RunE: func(cmd *cobra.Command, args []string) error {
			if window != 0 {
				return a.popupForSession(window, args, spaces.AskCloseSession)
			}
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			fate, err := spaces.ParseFate(sessions)
			if err != nil {
				return fmt.Errorf("--sessions: %w", err)
			}
			if sessions == "" {
				fate = ""
			}
			return tui.RunCloseSession(cmd.Context(), cfg, args, yes, false, fate)
		},
	}
	cmd.Flags().BoolVar(&yes, "yes", false, "close without asking")
	cmd.Flags().StringVar(&sessions, "sessions", "", "what becomes of the session provider's sessions the windows show, chosen first in the form: end, shelve or leave; the first the provider allows by default")
	fromFlag(cmd, &window, "its session by default")
	cmd.MarkFlagsMutuallyExclusive("from", "yes")
	return cmd
}

func (a *app) suspendCommand() *cobra.Command {
	var yes bool
	var window int
	cmd := &cobra.Command{
		Use:   "suspend [SESSION...]",
		Short: "Stop what runs in the sessions, the current one by default, and close their windows, keeping them to resume, after confirming in a form",
		Long: "Stop what runs in the sessions, the current one by default, and close their windows, after confirming in a form.\n\n" +
			"The session saver keeps each session's file, and the configured keeper keeps what runs in its windows, " +
			"such as their multiplexer sessions, until the session is opened again or forgotten. " +
			"Opening it again brings them back. Needs [kitty] save_sessions.",
		RunE: func(cmd *cobra.Command, args []string) error {
			if window != 0 {
				return a.popupForSession(window, args, spaces.AskSuspendSession)
			}
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			return tui.RunCloseSession(cmd.Context(), cfg, args, yes, true, "")
		},
	}
	cmd.Flags().BoolVar(&yes, "yes", false, "suspend without asking")
	fromFlag(cmd, &window, "its session by default")
	cmd.MarkFlagsMutuallyExclusive("from", "yes")
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
