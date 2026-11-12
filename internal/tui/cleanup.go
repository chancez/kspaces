package tui

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/chancez/kspaces/internal/config"
	"github.com/chancez/kspaces/internal/gitstate"
	"github.com/chancez/kspaces/internal/spaces"
	"github.com/chancez/kspaces/internal/tui/interact"
	"github.com/chancez/kspaces/internal/tui/look"
)

// cleanupForm lists the linked worktrees of a space that look done with, each with a box to tick, and
// removes the ticked ones when confirmed, as the remove worktree form does one. It is meant for a kitty
// overlay, like the new worktree form.
type cleanupForm struct {
	th    *look.Theme
	space spaces.Space
	items []spaces.Cleanup
	// checked are the items to remove: at first all but those with changes and those git would refuse.
	checked []bool
	// deleteBranches deletes the branches of the items removed, those that can go.
	deleteBranches bool
	cursor         int
	busy           bool
	// err is why starting the removal failed.
	err   error
	width int
	// zones and buttons are where the last frame drew its rows and buttons, and what each button runs.
	zones   map[string][]look.Zone
	buttons controls[*cleanupForm]
}

// cleanupZone is row i of the form's list, and cleanupBranchesZone the box that deletes their branches.
const (
	cleanupZone         = "cleanup:%d"
	cleanupBranchesZone = "cleanup:branches"
)

type cleanedMsg struct{ err error }

func newCleanupForm(space spaces.Space, items []spaces.Cleanup) *cleanupForm {
	f := &cleanupForm{th: look.New(true), space: space, items: items, checked: make([]bool, len(items))}
	for i, c := range items {
		f.checked[i] = c.Refused == nil && c.Changes == 0
	}
	return f
}

func (f *cleanupForm) Init() tea.Cmd { return nil }

// toggle ticks or clears item i, unless git would refuse to remove it.
func (f *cleanupForm) toggle(i int) {
	if i >= 0 && i < len(f.items) && f.items[i].Refused == nil {
		f.checked[i] = !f.checked[i]
	}
}

// branches reports whether any item has a branch that can go with it, for the box that deletes them.
func (f *cleanupForm) branches() bool {
	for _, c := range f.items {
		if c.Branch.Offered() {
			return true
		}
	}
	return false
}

// chosen are the ticked items.
func (f *cleanupForm) chosen() []spaces.Cleanup {
	var out []spaces.Cleanup
	for i, c := range f.items {
		if f.checked[i] {
			out = append(out, c)
		}
	}
	return out
}

// remove hands the ticked worktrees to one background process, which removes them one after another, and
// leaves, as the remove worktree form does. git is forced for those with changes, which the form said are
// lost, and for the branches with commits not merged, which it said are lost or merged by their PR.
func (f *cleanupForm) remove() tea.Cmd {
	f.busy, f.err = true, nil
	var paths []string
	opts := map[string]spaces.RemoveOptions{}
	for _, c := range f.chosen() {
		paths = append(paths, c.Worktree.Path)
		branch := f.deleteBranches && c.Branch.Offered()
		opts[c.Worktree.Path] = spaces.RemoveOptions{Force: c.Changes > 0, DeleteBranch: branch, ForceBranch: branch && c.Branch.Unmerged > 0}
	}
	return func() tea.Msg { return cleanedMsg{spaces.RemoveAllInBackground(paths, opts)} }
}

func (f *cleanupForm) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		f.width = msg.Width
	case cleanedMsg:
		f.busy, f.err = false, msg.err
		if msg.err == nil {
			return f, tea.Quit
		}
	case tea.KeyPressMsg:
		if cmd, ok := runOn(f, msg, theCleanup.Target(cleanupSubject{})); ok {
			return f, cmd
		}
	case tea.MouseClickMsg:
		if cmd, ok := f.buttons.click(f, f.zones, msg); ok {
			return f, cmd
		}
		for i := range f.items {
			for _, z := range f.zones[fmt.Sprintf(cleanupZone, i)] {
				if msg.Button == tea.MouseLeft && z.Contains(msg.X, msg.Y) && !f.busy {
					f.cursor = i
					f.toggle(i)
				}
			}
		}
		for _, z := range f.zones[cleanupBranchesZone] {
			if msg.Button == tea.MouseLeft && z.Contains(msg.X, msg.Y) && actCleanupBranches.Applies(f, cleanupSubject{}) {
				f.deleteBranches = !f.deleteBranches
			}
		}
	}
	return f, nil
}

func (f *cleanupForm) View() tea.View {
	frame, zones := look.Scan(f.render())
	f.zones = zones
	v := tea.NewView(frame)
	v.AltScreen = true
	v.MouseMode = tea.MouseModeCellMotion
	return v
}

func (f *cleanupForm) render() string {
	th, width := f.th, max(f.width-4, 20)
	line := func(spans ...look.Span) string { return th.Render(look.Plain, spans...) }
	lines := []string{line(look.Heading("Clean up worktrees of " + f.space.Name)), ""}
	if len(f.items) == 0 {
		base := "the main worktree's branch"
		if len(f.space.Worktrees) > 0 && f.space.Worktrees[0].Branch != "" {
			base = f.space.Worktrees[0].Branch
		}
		text := fmt.Sprintf("No linked worktree has its PR merged, its upstream gone or its branch merged into %s.", base)
		for _, row := range look.Muted(text).Wrapped(width) {
			lines = append(lines, line(row))
		}
		lines = append(lines, "")
	}
	names := 0
	for _, c := range f.items {
		names = max(names, len(filepath.Base(c.Worktree.Path)))
	}
	for i, c := range f.items {
		spans := []look.Span{look.Checkbox(f.checked[i], c.Refused != nil), look.Text(" "),
			look.Worktree(c.Worktree), look.Default(strings.Repeat(" ", names-len(filepath.Base(c.Worktree.Path))+2)),
			look.Muted(strings.Join(c.Reasons, ", "))}
		switch {
		case c.Refused != nil:
			spans = append(spans, look.Muted("; "), look.Error(c.Refused.Error()))
		case c.Changes > 0 && f.checked[i]:
			spans = append(spans, look.Muted("; "), look.Error(plural(c.Changes, "file")+" not committed, lost"))
		case c.Changes > 0:
			spans = append(spans, look.Muted("; "), look.Text(plural(c.Changes, "file")+" not committed"))
		}
		surface := look.Plain
		if i == f.cursor {
			surface = look.Cursor
		}
		zone := fmt.Sprintf(cleanupZone, i)
		lines = append(lines, th.DrawZone(zone, surface, width, th.Cut(width, spans...)...))
		// The branch has a line of its own, under the name, so a long reason does not cut off what it loses.
		if f.deleteBranches && c.Refused == nil && c.Branch.Name != "" {
			fate := look.Muted("kept, as " + c.Branch.Kept)
			if c.Branch.Offered() {
				fate = branchFate(c.Branch, f.checked[i])
			}
			lines = append(lines, th.DrawZone(zone, surface, width, th.Cut(width, look.Default("    "), look.Muted("branch: "), fate)...))
		}
	}
	if len(f.items) > 0 {
		lines = append(lines, "")
		note := "Each closes its session and is removed; its branch is kept."
		if f.branches() {
			target := theCleanup.Target(cleanupSubject{})
			spans := []look.Span{look.Checkbox(f.deleteBranches, f.busy), look.Text(" Also delete their branches")}
			if _, key, ok := offered(f, target, interact.Keys(f, target), actCleanupBranches); ok && key != "" {
				spans = append(spans, look.Default(" "), look.Muted(key))
			}
			lines = append(lines, th.DrawZone(cleanupBranchesZone, look.Plain, width, th.Cut(width, spans...)...))
			if f.deleteBranches {
				note = "Each closes its session and is removed, and its branch deleted."
			}
		}
		lines = append(lines, line(th.Cut(width, look.Muted(note))...), "")
	}
	switch {
	case f.busy:
		lines = append(lines, line(look.Busy("Removing...")), "")
	case f.err != nil:
		for _, row := range look.Error(f.err.Error()).Wrapped(width) {
			lines = append(lines, line(row))
		}
		lines = append(lines, "")
	}
	n := len(f.chosen())
	f.buttons = controls[*cleanupForm]{}
	lines = append(lines, line(f.buttons.draw(f, theCleanup.Target(cleanupSubject{}),
		control{action: actCleanupRemove, label: "Remove " + plural(n, "worktree"), danger: true, off: !f.busy && n == 0},
		control{action: actCleanupCancel, label: "Cancel"})...))
	return boxed(lines)
}

// RunCleanup runs the clean up form for the space at root, or named root, meant for a kitty overlay. A
// worktree's PR is what the sidebars' leader last saw, or else asked of gh now.
func RunCleanup(ctx context.Context, cfg config.Config, root string) error {
	space, err := spaces.Find(cfg, root)
	if err != nil {
		return err
	}
	known := spaces.NewPoller().Git()
	pr := func(wt spaces.Worktree) *gitstate.PR {
		if st, ok := known[wt.Path]; ok && st.PRBranch == wt.Branch && !st.PRAt.IsZero() {
			return st.PR
		}
		p, _ := gitstate.LookupPR(space.Root, wt.Branch)
		return p
	}
	items, err := spaces.Cleanups(space, pr)
	if err != nil {
		return err
	}
	if _, err := tea.NewProgram(newCleanupForm(space, items), tea.WithContext(ctx)).Run(); err != nil {
		return fmt.Errorf("running the clean up form: %w", err)
	}
	return nil
}

// cleanup opens the clean up form for the space at root, in an overlay as newWorktree's.
func (m *sidebar) cleanup(root string) tea.Cmd {
	st := m.kitty
	return func() tea.Msg {
		if err := spaces.AskCleanup(st, root); err != nil {
			return spaces.Snapshot{Err: err}
		}
		return nil
	}
}
