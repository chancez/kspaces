package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/chancez/kspaces/internal/config"
	"github.com/chancez/kspaces/internal/kitty"
	"github.com/chancez/kspaces/internal/spaces"
	"github.com/chancez/kspaces/internal/tui"
)

func (a *app) newCommand() *cobra.Command {
	var pr, issue, base, host string
	var fromPR, fromIssue bool
	var window int
	var yes bool
	var now tui.NewWorktree
	var inputs []string
	cmd := &cobra.Command{
		Use:   "new [SPACE] [-- ARGS...]",
		Short: "Create a worktree and switch to it, in the space given by root or name, or the current one",
		Args: func(cmd *cobra.Command, args []string) error {
			if dash := cmd.ArgsLenAtDash(); dash >= 0 {
				args = args[:dash]
			}
			return cobra.MaximumNArgs(1)(cmd, args)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dash := cmd.ArgsLenAtDash(); dash >= 0 {
				args, now.Args = args[:dash], args[dash:]
				if !yes || len(now.Args) == 0 {
					return fmt.Errorf("a command after -- needs --yes, and a word")
				}
			}
			query := ""
			if len(args) > 0 {
				query = args[0]
			}
			if host != "" {
				cfg, err := config.Load()
				if err != nil {
					return err
				}
				if len(args) == 0 {
					return fmt.Errorf("--host needs the root of the space on that host")
				}
				return tui.RunNewCopyWorktree(cmd.Context(), cfg, host, query)
			}
			if yes {
				return a.newNow(query, now, pr, issue, base, inputs)
			}
			if now.Branch != "" || now.Dir != "" || now.Layout != "" || now.Command != "" || now.Stay || len(inputs) > 0 {
				return fmt.Errorf("--branch, --dir, --layout, --command, --input and --background need --yes")
			}
			if window != 0 {
				return a.popupFor(window, func(cfg config.Config, st kitty.State) error {
					var space spaces.Space
					var err error
					if query == "" {
						space, _, err = spaces.Current(cfg, st)
					} else {
						space, err = spaces.Find(cfg, query)
					}
					if err != nil {
						return err
					}
					from := spaces.FromBranch
					switch {
					case fromPR:
						from = spaces.FromPR
					case fromIssue:
						from = spaces.FromIssue
					}
					return spaces.NewWorktree(cfg, st, space.Root, from, base)
				})
			}
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			space, err := spaces.Find(cfg, query)
			if err != nil {
				return err
			}
			from, arg := spaces.FromBranch, ""
			switch {
			case fromPR || pr != "":
				from, arg = spaces.FromPR, pr
			case fromIssue || issue != "":
				from, arg = spaces.FromIssue, issue
			}
			return tui.RunNewWorktree(cmd.Context(), cfg, space, from, arg, base)
		},
	}
	cmd.Flags().StringVar(&host, "host", "", "create it in this host's copy of the space at the root given, one of [hosts], in a form")
	cmd.Flags().BoolVar(&fromPR, "from-pr", false, "open on the Pull request tab, to choose one of the open PRs")
	cmd.Flags().StringVar(&pr, "pr", "", "open on the Pull request tab on this PR, its number or URL; with --yes, check it out")
	cmd.Flags().BoolVar(&fromIssue, "from-issue", false, "open on the Issue tab, to choose one of the open issues")
	cmd.Flags().StringVar(&issue, "issue", "", "open on the Issue tab on this issue, its number or URL; with --yes, create a branch for it")
	cmd.Flags().StringVar(&base, "base", "", "start a new branch from this local branch, rather than the main worktree's")
	cmd.Flags().BoolVar(&yes, "yes", false, "create it without the form, from --branch, --pr or --issue, and print its path")
	cmd.Flags().StringVar(&now.Branch, "branch", "", "with --yes, the branch to create or check out, after the configured prefix unless it starts with it")
	cmd.Flags().StringVar(&now.Dir, "dir", "", "with --yes, where the worktree goes, rather than under [worktrees] dir")
	cmd.Flags().StringVar(&now.Layout, "layout", "", "with --yes, open the session with this layout rather than the default one")
	cmd.Flags().StringVar(&now.Command, "command", "", "with --yes, open the session with this configured command rather than the default layout")
	cmd.Flags().StringArrayVar(&inputs, "input", nil, "with --yes, an input's value of the layout or command, as NAME=VALUE; repeat for more")
	cmd.Flags().BoolVar(&now.Stay, "background", false, "with --yes, leave the focus where it is rather than switching to the new session")
	cmd.MarkFlagsMutuallyExclusive("layout", "command")
	cmd.MarkFlagsMutuallyExclusive("yes", "from-pr")
	cmd.MarkFlagsMutuallyExclusive("yes", "from-issue")
	a.fromFlag(cmd, &window, "the space of its session by default")
	for _, f := range []string{"yes", "from", "from-pr", "pr", "from-issue", "issue"} {
		cmd.MarkFlagsMutuallyExclusive("host", f)
	}
	cmd.MarkFlagsMutuallyExclusive("from", "yes")
	cmd.MarkFlagsMutuallyExclusive("from", "pr")
	cmd.MarkFlagsMutuallyExclusive("from", "issue")
	for _, pr := range []string{"from-pr", "pr"} {
		for _, issue := range []string{"from-issue", "issue"} {
			cmd.MarkFlagsMutuallyExclusive(pr, issue)
		}
	}
	return cmd
}

// newNow creates the worktree now says in the space query names, the current one when empty, without the
// form, and prints its path.
func (a *app) newNow(query string, now tui.NewWorktree, pr, issue, base string, inputs []string) error {
	if len(now.Args) > 0 && (now.Layout != "" || now.Command != "") {
		return fmt.Errorf("a command after -- takes no --layout or --command")
	}
	values, err := parseInputs(inputs)
	if err != nil {
		return err
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	space, err := spaces.Find(cfg, query)
	if err != nil {
		return err
	}
	now.PR, now.Issue, now.Base, now.Inputs = pr, issue, base, values
	wt, err := tui.NewWorktreeNow(cfg, space, now)
	if wt.Path != "" {
		fmt.Fprintln(a.stdout, wt.Path)
	}
	return err
}
