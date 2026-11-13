package tui

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/chancez/kspaces/internal/config"
	"github.com/chancez/kspaces/internal/gh"
	"github.com/chancez/kspaces/internal/git"
	"github.com/chancez/kspaces/internal/kitty"
	"github.com/chancez/kspaces/internal/spaces"
	"github.com/chancez/kspaces/internal/tui/interact"
	"github.com/chancez/kspaces/internal/tui/look"
)

// removeWorktreeForm shows what removing a linked worktree does, its changes and its session's windows,
// and starts removing it when confirmed. It is meant for a kitty popup, like the new worktree form.
type removeWorktreeForm struct {
	th    *look.Theme
	cfg   config.Config
	space spaces.Space
	wt    spaces.Worktree
	// changes is how many files git status reports in the worktree, which removing it loses.
	changes int
	// windows is how many windows the worktree's session has open, which removing it closes.
	windows int
	// branch is the worktree's branch as deleting it would find it, which deleteBranch does after removing.
	branch       spaces.Branch
	deleteBranch bool
	busy         bool
	// err is why starting the removal failed.
	err   error
	width int
	// zones and buttons are where the last frame drew its buttons, and what each runs, for a click.
	zones   map[string][]look.Zone
	buttons controls[*removeWorktreeForm]
}

// removeBranchZone is the delete branch box.
const removeBranchZone = "remove:branch"

type removedMsg struct{ err error }

func (f *removeWorktreeForm) Init() tea.Cmd { return nil }

// remove hands the removal to a background process and leaves: removing closes the worktree's windows
// first, and this form may be over one of them. git is forced when the worktree has changes, as the form
// has said they are lost, and for the branch when it has commits not merged, which the form said too;
// the background process shows anything else that goes wrong in kitty.
func (f *removeWorktreeForm) remove() tea.Cmd {
	f.busy, f.err = true, nil
	path := f.wt.Path
	opts := spaces.RemoveOptions{Force: f.changes > 0, DeleteBranch: f.deleteBranch, ForceBranch: f.deleteBranch && f.branch.Unmerged > 0}
	return func() tea.Msg { return removedMsg{spaces.RemoveInBackground(path, opts)} }
}

func (f *removeWorktreeForm) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		f.width = msg.Width
	case removedMsg:
		f.busy, f.err = false, msg.err
		if msg.err == nil {
			return f, tea.Quit
		}
	case tea.KeyPressMsg:
		if cmd, ok := runOn(f, msg, theRemoveForm.Target(removeSubject{})); ok {
			return f, cmd
		}
	case tea.MouseClickMsg:
		if cmd, ok := f.buttons.click(f, f.zones, msg); ok {
			return f, cmd
		}
		for _, z := range f.zones[removeBranchZone] {
			if msg.Button == tea.MouseLeft && z.Contains(msg.X, msg.Y) && actRemoveDeleteBranch.Applies(f, removeSubject{}) {
				f.deleteBranch = !f.deleteBranch
			}
		}
	}
	return f, nil
}

func (f *removeWorktreeForm) View() tea.View {
	frame, zones := look.Scan(f.render())
	f.zones = zones
	v := tea.NewView(frame)
	v.AltScreen = true
	v.MouseMode = tea.MouseModeCellMotion
	return v
}

func (f *removeWorktreeForm) render() string {
	th, width := f.th, max(f.width-4, 20)
	line := func(spans ...look.Span) string { return th.Render(look.Plain, spans...) }
	field := func(label string, spans ...look.Span) string {
		return th.Draw(look.Plain, labelWidth, look.Label(label)) + line(th.Cut(width-labelWidth, spans...)...)
	}
	// git reports the worktree's path with symlinks resolved, and the config may not have the root so.
	dir, root := f.wt.Path, f.space.Root
	if resolved, err := filepath.EvalSymlinks(root); err == nil {
		root = resolved
	}
	if rel, err := filepath.Rel(root, dir); err == nil && !strings.HasPrefix(rel, "..") {
		dir = rel
	}
	branch := f.wt.Branch
	if branch == "" {
		branch = "none, HEAD is detached"
	}
	windows := look.Muted("no windows open")
	if f.windows > 0 {
		windows = look.Text(fmt.Sprintf("%s closed", plural(f.windows, "window")))
	}
	changes := look.Muted("none")
	if f.changes > 0 {
		changes = look.Error(fmt.Sprintf("%s not committed, lost", plural(f.changes, "file")))
	}
	fate := look.Muted(" kept")
	if f.deleteBranch {
		fate = look.Muted(" deleted")
	}
	target := theRemoveForm.Target(removeSubject{})
	lines := []string{
		line(look.Heading(fmt.Sprintf("Remove worktree %s from %s", filepath.Base(f.wt.Path), f.space.Name))), "",
		field("directory", look.Text(dir)),
		field("branch", look.Text(branch), fate),
		field("session", windows),
		field("changes", changes),
		"",
	}
	if f.branch.Offered() {
		spans := []look.Span{look.Checkbox(f.deleteBranch, f.busy), look.Text(" Delete branch " + f.branch.Name)}
		if _, key, ok := offered(f, target, interact.Keys(f, target), actRemoveDeleteBranch); ok && key != "" {
			spans = append(spans, look.Default(" "), look.Muted(key))
		}
		spans = append(spans, look.Default("  "), branchFate(f.branch, true))
		lines = append(lines, th.DrawZone(removeBranchZone, look.Plain, width, th.Cut(width, spans...)...), "")
	}
	// What git would refuse is said before y, as the background process that removes has no screen.
	refused := spaces.CanRemove(f.space, f.wt)
	err := f.err
	if err == nil {
		err = refused
	}
	switch {
	case f.busy:
		lines = append(lines, line(look.Busy("Removing...")), "")
	case err != nil:
		for _, row := range look.Error(err.Error()).Wrapped(width) {
			lines = append(lines, line(row))
		}
		lines = append(lines, "")
	}
	f.buttons = controls[*removeWorktreeForm]{}
	lines = append(lines, line(f.buttons.draw(f, target,
		control{action: actRemove, label: "Remove", danger: true, off: !f.busy && refused != nil},
		control{action: actRemoveCancel, label: "Cancel"})...))
	return boxed(lines)
}

// branchFate is what deleting b loses, for the forms that offer it: nothing when it is merged or its PR is,
// else its commits not merged, said to be lost if lost.
func branchFate(b spaces.Branch, lost bool) look.Span {
	switch {
	case b.Unmerged == 0:
		return look.Muted("merged into " + b.Into + ", nothing lost")
	case b.PR > 0:
		return look.Muted(fmt.Sprintf("merged by PR #%d, nothing lost", b.PR))
	case lost:
		return look.Error(plural(b.Unmerged, "commit") + " not merged, lost")
	}
	return look.Text(plural(b.Unmerged, "commit") + " not merged")
}

func plural(n int, what string) string {
	if n == 1 {
		return "1 " + what
	}
	return fmt.Sprintf("%d %ss", n, what)
}

// newRemoveForm is the form for removing wt from space, with what kitty and git say about it now. pr is
// the branch's PR, nil for none or not known. deleteBranch ticks deleting the branch, if it is offered.
func newRemoveForm(cfg config.Config, space spaces.Space, wt spaces.Worktree, st kitty.State, pr *gh.PR, deleteBranch bool) (*removeWorktreeForm, error) {
	changes, err := git.Changes(wt.Path)
	if err != nil {
		return nil, fmt.Errorf("checking %s for changes: %w", wt.Path, err)
	}
	branch := spaces.BranchOf(space, git.DefaultBranch(space.Root), wt, pr)
	return &removeWorktreeForm{th: look.New(true), cfg: cfg, space: space, wt: wt, changes: changes, windows: st.Windows[wt.Session],
		branch: branch, deleteBranch: deleteBranch && branch.Offered()}, nil
}

// RunRemoveWorktree runs the remove worktree form for the worktree at path, meant for a kitty popup,
// until the worktree is removed or the form is left. The branch's PR is what the sidebars' leader last saw;
// deleteBranch ticks deleting the branch at first.
func RunRemoveWorktree(ctx context.Context, cfg config.Config, path string, deleteBranch bool) error {
	space, wt, err := spaces.FindWorktree(cfg, path)
	if err != nil {
		return err
	}
	if len(space.Worktrees) > 0 && space.Worktrees[0].Path == wt.Path {
		return spaces.CanRemove(space, wt)
	}
	st, err := kitty.LoadState(kitty.SelfWindowID())
	if err != nil {
		return err
	}
	pr, _ := spaces.NewPoller().Git()[wt.Path].KnownPR(wt.Branch)
	f, err := newRemoveForm(cfg, space, wt, st, pr, deleteBranch)
	if err != nil {
		return err
	}
	if err := runPopup(ctx, f); err != nil {
		return fmt.Errorf("running the remove worktree form: %w", err)
	}
	return nil
}

// RemoveWorktrees removes the worktrees at paths without asking, one after another, each as opts has it,
// for the background process the forms start. It has no screen, so failures are shown in kitty as well
// as returned; one failing does not stop the rest, and a branch git refuses to delete leaves its worktree
// removed.
func RemoveWorktrees(cfg config.Config, paths []string, opts map[string]spaces.RemoveOptions) error {
	var failed []string
	branches := 0
	for _, path := range paths {
		space, wt, err := spaces.FindWorktree(cfg, path)
		if err == nil {
			err = spaces.RemoveWorktree(cfg, space, wt, opts[path])
		}
		if err != nil {
			failed = append(failed, fmt.Sprintf("%s: %v", path, err))
		}
		if _, ok := errors.AsType[*spaces.BranchError](err); ok {
			branches++
		}
	}
	if len(failed) == 0 {
		return nil
	}
	title := "Could not remove the worktree"
	switch {
	case branches == len(failed) && branches > 1:
		title = "Could not delete the branches"
	case branches == len(failed):
		title = "Could not delete the branch"
	case len(failed) > 1:
		title = "Could not remove the worktrees"
	}
	msg := strings.Join(failed, "\n")
	_ = kitty.ShowError(title, msg)
	return errors.New(msg)
}
