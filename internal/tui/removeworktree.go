package tui

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/chancez/kspaces/internal/config"
	"github.com/chancez/kspaces/internal/git"
	"github.com/chancez/kspaces/internal/kitty"
	"github.com/chancez/kspaces/internal/spaces"
	"github.com/chancez/kspaces/internal/tui/look"
)

// removeWorktreeForm shows what removing a linked worktree does, its changes and its session's windows,
// and removes it when confirmed. It is meant for a kitty overlay, like the new worktree form.
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
	err     error
	width   int
}

type removedMsg struct{ err error }

func (f *removeWorktreeForm) Init() tea.Cmd { return nil }

// remove removes the worktree, forcing git when it has changes: the form has said they are lost.
func (f *removeWorktreeForm) remove() tea.Cmd {
	f.busy, f.err = true, nil
	cfg, space, wt, force := f.cfg, f.space, f.wt, f.changes > 0
	return func() tea.Msg { return removedMsg{spaces.RemoveWorktree(cfg, space, wt, force)} }
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
	}
	return f, nil
}

func (f *removeWorktreeForm) View() tea.View {
	v := tea.NewView(f.render())
	v.AltScreen = true
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
	switch {
	case f.busy:
		lines = append(lines, line(look.Busy("Removing...")), "")
	case f.err != nil:
		for _, row := range look.Error(f.err.Error()).Wrapped(width) {
			lines = append(lines, line(row))
		}
		lines = append(lines, "")
	}
	lines = append(lines, line(look.Key("y"), look.KeyDesc(" remove  "), look.Key("n/esc"), look.KeyDesc(" cancel")))
	inner := 0
	for _, l := range lines {
		inner = max(inner, ansi.StringWidth(l))
	}
	blank := strings.Repeat(" ", inner+4)
	out := []string{blank}
	for _, l := range lines {
		out = append(out, "  "+look.Fit(l, inner)+"  ")
	}
	return strings.Join(append(out, blank), "\n")
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
		return fmt.Errorf("%s is the main worktree of %s, which cannot be removed", wt.Path, space.Name)
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
