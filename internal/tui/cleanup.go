package tui

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/chancez/kspaces/internal/config"
	"github.com/chancez/kspaces/internal/gh"
	"github.com/chancez/kspaces/internal/kitty"
	"github.com/chancez/kspaces/internal/spaces"
	"github.com/chancez/kspaces/internal/tui/interact"
	"github.com/chancez/kspaces/internal/tui/look"
)

// cleanupForm lists worktrees to remove, each with a box to tick, and removes the ticked ones when
// confirmed, as the remove worktree form does one: the linked worktrees of a space that look done with and
// its missing ones, or the sessions given. It is meant for a kitty overlay, like the new worktree form.
type cleanupForm struct {
	th *look.Theme
	// space is the space cleaned up, nil for sessions given.
	space *spaces.Space
	items []spaces.Cleanup
	// bySession names the items by their sessions, as they span spaces.
	bySession bool
	// checked are the items to remove: at first all but those with changes, missing ones with windows
	// open and those git would refuse.
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
	// paint is a press on a row's box until its release, as in the sidebar's select mode, nil for none.
	paint *paint
}

// cleanupZone is row i of the form's list, and cleanupBranchesZone the box that deletes their branches.
const (
	cleanupZone         = "cleanup:%d"
	cleanupBranchesZone = "cleanup:branches"
)

type cleanedMsg struct{ err error }

func newCleanupForm(space *spaces.Space, items []spaces.Cleanup) *cleanupForm {
	f := &cleanupForm{th: look.New(true), space: space, items: items, checked: make([]bool, len(items))}
	for i, c := range items {
		f.checked[i] = c.Refused == nil && c.Changes == 0 && c.Windows == 0
		f.bySession = f.bySession || c.Space != items[0].Space
	}
	return f
}

// name is what the form calls c: its directory, or its session when the items span spaces or it has no
// directory, being in no space.
func (f *cleanupForm) name(c spaces.Cleanup) string {
	switch {
	case f.bySession || c.Space == "":
		return c.Worktree.Session
	case c.Missing:
		return strings.TrimPrefix(c.Worktree.Session, c.Space+"@")
	}
	return filepath.Base(c.Worktree.Path)
}

func (f *cleanupForm) Init() tea.Cmd { return nil }

// toggle ticks or clears item i, unless git would refuse to remove it, and says which it is now and
// whether it could be.
func (f *cleanupForm) toggle(i int) (bool, bool) {
	if i < 0 || i >= len(f.items) || f.items[i].Refused != nil {
		return false, false
	}
	f.checked[i] = !f.checked[i]
	return f.checked[i], true
}

// tick ticks or clears item i, unless git would refuse to remove it.
func (f *cleanupForm) tick(i int, on bool) {
	if f.items[i].Refused == nil {
		f.checked[i] = on
	}
}

// tickAll ticks every item git would remove.
func (f *cleanupForm) tickAll() {
	for i := range f.items {
		f.tick(i, true)
	}
}

// itemAt is the item drawn on screen row y, its branch's line too.
func (f *cleanupForm) itemAt(y int) (int, bool) {
	for i := range f.items {
		for _, z := range f.zones[fmt.Sprintf(cleanupZone, i)] {
			if z.Y == y {
				return i, true
			}
		}
	}
	return 0, false
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
// ends the missing ones' sessions, and leaves, as the remove worktree form does. git is forced for those
// with changes, which the form said are lost, and for the branches with commits not merged, which it said
// are lost or merged by their PR.
func (f *cleanupForm) remove() tea.Cmd {
	f.busy, f.err = true, nil
	var paths, sessions []string
	opts := map[string]spaces.RemoveOptions{}
	for _, c := range f.chosen() {
		if c.Missing {
			sessions = append(sessions, c.Worktree.Session)
			continue
		}
		paths = append(paths, c.Worktree.Path)
		branch := f.deleteBranches && c.Branch.Offered()
		opts[c.Worktree.Path] = spaces.RemoveOptions{Force: c.Changes > 0, DeleteBranch: branch, ForceBranch: branch && c.Branch.Unmerged > 0}
	}
	return func() tea.Msg { return cleanedMsg{spaces.RemoveAllInBackground(paths, opts, sessions)} }
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
		f.paint = nil
		if cmd, ok := f.buttons.click(f, f.zones, msg); ok {
			return f, cmd
		}
		for i := range f.items {
			for _, z := range f.zones[fmt.Sprintf(cleanupZone, i)] {
				if msg.Button == tea.MouseLeft && z.Contains(msg.X, msg.Y) && !f.busy {
					f.cursor = i
					f.paint = startPaint(i, f.toggle)
				}
			}
		}
		for _, z := range f.zones[cleanupBranchesZone] {
			if msg.Button == tea.MouseLeft && z.Contains(msg.X, msg.Y) && actCleanupBranches.Applies(f, cleanupSubject{}) {
				f.deleteBranches = !f.deleteBranches
			}
		}
	case tea.MouseMotionMsg:
		// Dragging from a box ticks every row passed as the first went, as in the select mode.
		if i, ok := f.itemAt(msg.Y); ok && f.paint != nil && msg.Button == tea.MouseLeft && !f.busy {
			f.paint.to(i, f.tick)
		}
	case tea.MouseReleaseMsg:
		f.paint = nil
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
	title := "Remove worktrees"
	if f.space != nil {
		title = "Clean up worktrees of " + f.space.Name
	}
	lines := []string{line(look.Heading(title)), ""}
	if len(f.items) == 0 && f.space != nil {
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
		names = max(names, len(f.name(c)))
	}
	missing := false
	for i, c := range f.items {
		missing = missing || c.Missing && f.checked[i]
		spans := append(look.BoxOf(f.checked[i], c.Refused != nil),
			look.WorktreeName(f.name(c)), look.Default(strings.Repeat(" ", names-len(f.name(c))+2)))
		// Each detail after the first follows a semicolon; a linked worktree given may have no reason.
		detail := func(s look.Span) {
			if len(spans) > 4 {
				spans = append(spans, look.Muted("; "))
			}
			spans = append(spans, s)
		}
		if len(c.Reasons) > 0 {
			detail(look.Muted(strings.Join(c.Reasons, ", ")))
		}
		switch {
		case c.Refused != nil:
			detail(look.Error(c.Refused.Error()))
		case c.Changes > 0 && f.checked[i]:
			detail(look.Error(plural(c.Changes, "file") + " not committed, lost"))
		case c.Changes > 0:
			detail(look.Text(plural(c.Changes, "file") + " not committed"))
		case c.Windows > 0 && f.checked[i]:
			detail(look.Error(plural(c.Windows, "window") + " closed"))
		case c.Windows > 0:
			detail(look.Text(plural(c.Windows, "window") + " open"))
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
			// Under the name, past the box, which ASCII draws wider.
			indent := look.Default(strings.Repeat(" ", th.Width(look.Box(look.Unticked, false)...)))
			lines = append(lines, th.DrawZone(zone, surface, width, th.Cut(width, indent, look.Muted("branch: "), fate)...))
		}
	}
	if len(f.items) > 0 {
		lines = append(lines, "")
		note := "Each closes its session and is removed; its branch is kept."
		if f.branches() {
			target := theCleanup.Target(cleanupSubject{})
			spans := append(look.BoxOf(f.deleteBranches, f.busy), look.Text("Also delete their branches"))
			if _, key, ok := offered(f, target, interact.Keys(f, target), actCleanupBranches); ok && key != "" {
				spans = append(spans, look.Default(" "), look.Muted(key))
			}
			lines = append(lines, th.DrawZone(cleanupBranchesZone, look.Plain, width, th.Cut(width, spans...)...))
			if f.deleteBranches {
				note = "Each closes its session and is removed, and its branch deleted."
			}
		}
		lines = append(lines, line(th.Cut(width, look.Muted(note))...))
		if missing {
			lines = append(lines, line(th.Cut(width, look.Muted("A missing one has its windows closed and its saved file deleted."))...))
		}
		lines = append(lines, "")
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

// RunCleanup runs the clean up form, meant for a kitty popup: for the space at root, or named root, or,
// when sessions are given, for those. A worktree's PR is what the sidebars' leader last saw, or else
// asked of gh now. The missing worktrees and their windows are as the sidebars see them.
func RunCleanup(ctx context.Context, cfg config.Config, root string, sessions []string) error {
	snap := spaces.LoadShared().View(kitty.SelfWindowID())
	if snap.Err != nil {
		return snap.Err
	}
	known := spaces.NewPoller().Git()
	pr := func(wt spaces.Worktree) *gh.PR {
		if p, ok := known[wt.Path].KnownPR(wt.Branch); ok {
			return p
		}
		p, _ := gh.BranchPR(wt.Path, wt.Branch)
		return p
	}
	var f *cleanupForm
	if len(sessions) > 0 {
		f = newCleanupForm(nil, spaces.SessionCleanups(snap, sessions, pr))
	} else {
		space, err := spaces.Find(cfg, root)
		if err != nil {
			return err
		}
		for _, s := range snap.Spaces {
			if s.Root == space.Root {
				space.Missing = s.Missing
			}
		}
		items, err := spaces.Cleanups(space, snap.Kitty.Windows, pr)
		if err != nil {
			return err
		}
		f = newCleanupForm(&space, items)
	}
	if err := runPopup(ctx, f); err != nil {
		return fmt.Errorf("running the clean up form: %w", err)
	}
	return nil
}

// cleanup opens the clean up form for the space at root, in a popup as newWorktree's.
func (m *sidebar) cleanup(root string) tea.Cmd {
	cfg, st := m.cfg, m.kitty
	return func() tea.Msg {
		if err := spaces.AskCleanup(cfg, st, root); err != nil {
			return failedMsg{err}
		}
		return nil
	}
}
