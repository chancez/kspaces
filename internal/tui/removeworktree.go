package tui

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/chancez/kspaces/internal/config"
	"github.com/chancez/kspaces/internal/git"
	"github.com/chancez/kspaces/internal/kitty"
	"github.com/chancez/kspaces/internal/spaces"
	"github.com/chancez/kspaces/internal/tui/look"
)

// removeWorktreeForm shows what removing a linked worktree does, its changes and its session's windows,
// and starts removing it when confirmed. It is meant for a kitty overlay, like the new worktree form.
type removeWorktreeForm struct {
	th    *look.Theme
	cfg   config.Config
	space spaces.Space
	wt    spaces.Worktree
	// changes is how many files git status reports in the worktree, which removing it loses.
	changes int
	// windows is how many windows the worktree's session has open, which removing it closes.
	windows int
	busy    bool
	// err is why starting the removal failed.
	err   error
	width int
	// zones and buttons are where the last frame drew its buttons, and what each runs, for a click.
	zones   map[string][]look.Zone
	buttons controls[*removeWorktreeForm]
}

type removedMsg struct{ err error }

func (f *removeWorktreeForm) Init() tea.Cmd { return nil }

// remove hands the removal to a background process and leaves: removing closes the worktree's windows
// first, and this form may be over one of them. git is forced when the worktree has changes, as the form
// has said they are lost; the background process shows anything else that goes wrong in kitty.
func (f *removeWorktreeForm) remove() tea.Cmd {
	f.busy, f.err = true, nil
	path, force := f.wt.Path, f.changes > 0
	return func() tea.Msg { return removedMsg{spaces.RemoveInBackground(path, force)} }
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
	lines := []string{
		line(look.Heading(fmt.Sprintf("Remove worktree %s from %s", filepath.Base(f.wt.Path), f.space.Name))), "",
		field("directory", look.Text(dir)),
		field("branch", look.Text(branch), look.Muted(" kept")),
		field("session", windows),
		field("changes", changes),
		"",
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
	lines = append(lines, line(f.buttons.draw(f, theRemoveForm.Target(removeSubject{}),
		control{action: actRemove, label: "Remove", danger: true, off: !f.busy && refused != nil},
		control{action: actRemoveCancel, label: "Cancel"})...))
	return boxed(lines)
}

func plural(n int, what string) string {
	if n == 1 {
		return "1 " + what
	}
	return fmt.Sprintf("%d %ss", n, what)
}

// newRemoveForm is the form for removing wt from space, with what kitty and git say about it now.
func newRemoveForm(cfg config.Config, space spaces.Space, wt spaces.Worktree, st kitty.State) (*removeWorktreeForm, error) {
	changes, err := git.Changes(wt.Path)
	if err != nil {
		return nil, fmt.Errorf("checking %s for changes: %w", wt.Path, err)
	}
	return &removeWorktreeForm{th: look.New(true), cfg: cfg, space: space, wt: wt, changes: changes, windows: st.Windows[wt.Session]}, nil
}

// RunRemoveWorktree runs the remove worktree form for the worktree at path, meant for a kitty overlay,
// until the worktree is removed or the form is left.
func RunRemoveWorktree(ctx context.Context, cfg config.Config, path string) error {
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
	f, err := newRemoveForm(cfg, space, wt, st)
	if err != nil {
		return err
	}
	if _, err := tea.NewProgram(f, tea.WithContext(ctx)).Run(); err != nil {
		return fmt.Errorf("running the remove worktree form: %w", err)
	}
	return nil
}

// RemoveWorktrees removes the worktrees at paths without asking, one after another, forcing git for those
// in force, for the background process the forms start. It has no screen, so failures are shown in kitty
// as well as returned; one failing does not stop the rest.
func RemoveWorktrees(cfg config.Config, paths []string, force map[string]bool) error {
	var failed []string
	for _, path := range paths {
		space, wt, err := spaces.FindWorktree(cfg, path)
		if err == nil {
			err = spaces.RemoveWorktree(cfg, space, wt, force[path])
		}
		if err != nil {
			failed = append(failed, fmt.Sprintf("%s: %v", path, err))
		}
	}
	if len(failed) == 0 {
		return nil
	}
	title := "Could not remove the worktree"
	if len(failed) > 1 {
		title = "Could not remove the worktrees"
	}
	msg := strings.Join(failed, "\n")
	_ = kitty.ShowError(title, msg)
	return errors.New(msg)
}
