package cli

import (
	"github.com/spf13/cobra"

	"github.com/chancez/kspaces/internal/config"
	"github.com/chancez/kspaces/internal/spaces"
	"github.com/chancez/kspaces/internal/tui"
)

func (a *app) newCommand() *cobra.Command {
	var pr, issue, base string
	var fromPR, fromIssue bool
	cmd := &cobra.Command{
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
	cmd.Flags().BoolVar(&fromPR, "from-pr", false, "open on the Pull request tab, to choose one of the open PRs")
	cmd.Flags().StringVar(&pr, "pr", "", "open on the Pull request tab on this PR, its number or URL")
	cmd.Flags().BoolVar(&fromIssue, "from-issue", false, "open on the Issue tab, to choose one of the open issues")
	cmd.Flags().StringVar(&issue, "issue", "", "open on the Issue tab on this issue, its number or URL")
	cmd.Flags().StringVar(&base, "base", "", "start a new branch from this local branch, rather than the main worktree's")
	for _, pr := range []string{"from-pr", "pr"} {
		for _, issue := range []string{"from-issue", "issue"} {
			cmd.MarkFlagsMutuallyExclusive(pr, issue)
		}
	}
	return cmd
}
