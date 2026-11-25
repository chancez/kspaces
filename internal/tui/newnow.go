package tui

import (
	"fmt"
	"strings"

	"github.com/chancez/kspaces/internal/config"
	"github.com/chancez/kspaces/internal/spaces"
)

// NewWorktree is what NewWorktreeNow creates and opens its session with, as the new worktree form's
// fields say it.
type NewWorktree struct {
	// The branch to create or check out, the configured prefix put before it when it does not start with
	// it. With Issue, empty for the branch named after the issue.
	Branch string
	// Where the worktree goes, see spaces.WorktreeDir; empty for the default.
	Dir string
	// The local branch a new branch starts from; empty for the main worktree's.
	Base string
	// The PR to check out, or the issue to create a branch for: N, #N or a URL. At most one.
	PR    string
	Issue string
	// What the session opens with: the layout or the command called Layout or Command, or the command
	// line Args, at most one; with none, the default layout the form starts with.
	Layout  string
	Command string
	Args    []string
	// The inputs of the layout or command, by name.
	Inputs map[string]string
	// Stay opens the session without switching to it.
	Stay bool
}

// NewWorktreeNow creates the worktree n says in space, without the form, and opens its session. global is
// the config without the space's file. What the session opens with is checked before anything is
// created, so a required input not given creates nothing.
func NewWorktreeNow(global config.Config, space spaces.Space, n NewWorktree) (spaces.Worktree, error) {
	cfg := space.ConfigOf(global)
	var links spaces.Links
	var pr spaces.PR
	branch, opener := strings.TrimSpace(n.Branch), cfg.LayoutFor(space.Name, true)
	switch {
	case n.PR != "" && n.Issue != "":
		return spaces.Worktree{}, fmt.Errorf("give a PR or an issue, not both")
	case n.PR != "":
		ref, ok := spaces.ParsePRRef(n.PR)
		if !ok {
			return spaces.Worktree{}, fmt.Errorf("%q is not a PR's number or URL", n.PR)
		}
		var err error
		if pr, err = spaces.ResolvePR(space, ref); err != nil {
			return spaces.Worktree{}, err
		}
		branch, links.PR, opener = pr.Branch, pr.Number, cfg.PRLayoutFor(space.Name)
	case n.Issue != "":
		ref, ok := spaces.ParseIssueRef(n.Issue)
		if !ok {
			return spaces.Worktree{}, fmt.Errorf("%q is not an issue's number or URL", n.Issue)
		}
		issue, err := spaces.ResolveIssue(space, ref)
		if err != nil {
			return spaces.Worktree{}, err
		}
		if branch == "" {
			branch = spaces.IssueBranch(cfg, issue.Number, issue.Title)
		}
		links.Issue, opener = issue.Number, cfg.IssueLayoutFor(space.Name)
	case branch == "":
		return spaces.Worktree{}, fmt.Errorf("give a branch, a PR or an issue")
	}
	if start := spaces.BranchStart(cfg); n.PR == "" && !strings.HasPrefix(branch, start) {
		branch = start + branch
	}
	links.Base = n.Base
	dir := spaces.WorktreeDir(cfg, space.Root, branch, n.Dir)
	vars := spaces.BuiltinVars(space, spaces.LinkedWorktree(space, branch, dir), links)
	layout, run, err := newOpener(cfg, n, opener, vars)
	if err != nil {
		return spaces.Worktree{}, err
	}

	// Marked until the session is open, so nothing else opens it in a directory half checked out.
	unmark, err := spaces.MarkPending(spaces.PendingCreate, space.Root, dir, branch)
	if err != nil {
		return spaces.Worktree{}, err
	}
	defer unmark()
	var wt spaces.Worktree
	switch {
	case n.PR != "":
		wt, err = spaces.CreatePRWorktree(space, pr, dir)
	case n.Issue != "":
		wt, err = spaces.CreateIssueWorktree(space, links.Issue, branch, n.Base, dir)
	default:
		wt, err = spaces.CreateWorktreeFrom(space, branch, n.Base, dir)
	}
	if err != nil {
		return spaces.Worktree{}, err
	}
	if layout != nil {
		layout.Links = links
	}
	if run != nil {
		run.Links = links
	}
	return wt, spaces.OpenNew(cfg, space, wt, layout, run, n.Stay)
}

// newOpener is what n opens the new session with, by vars, the built-ins of the worktree to be: the
// layout or command it names, its command line, or else the layout or command called def, as the form
// chooses it, or a shell, both nil, when def is empty.
func newOpener(cfg config.Config, n NewWorktree, def string, vars map[string]string) (*spaces.LayoutRun, *spaces.Run, error) {
	if len(n.Args) > 0 {
		r, err := spaces.NewRun(literalCommand(n.Args), n.Inputs, vars)
		return nil, &r, err
	}
	name, layouts, cmds := def, true, true
	switch {
	case n.Layout != "":
		name, cmds = n.Layout, false
	case n.Command != "":
		name, layouts = n.Command, false
	}
	if l, ok := cfg.Layout(name); ok && layouts {
		lr, err := spaces.NewLayoutRun(l, n.Inputs, vars)
		return &lr, nil, err
	}
	if c, ok := cfg.Command(name); ok && cmds {
		r, err := spaces.NewRun(c, n.Inputs, vars)
		return nil, &r, err
	}
	switch {
	case n.Layout != "":
		return nil, nil, fmt.Errorf("no layout %s in the config", n.Layout)
	case n.Command != "":
		return nil, nil, fmt.Errorf("no command %s in the config", n.Command)
	case len(n.Inputs) > 0:
		return nil, nil, fmt.Errorf("--input needs a layout or command that takes it")
	}
	return nil, nil, nil
}
