package tui

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	"github.com/chancez/kspaces/internal/config"
	"github.com/chancez/kspaces/internal/spaces"
	"github.com/chancez/kspaces/internal/tui/look"
)

// newWorktreeForm asks for a branch and, optionally, where its worktree goes, shows what it will do,
// creates the worktree and switches to its session. It is meant for a kitty overlay, as the sidebar is too
// narrow to type a name and read back the path in.
type newWorktreeForm struct {
	th     *look.Theme
	cfg    config.Config
	space  spaces.Space
	branch textinput.Model
	// Empty for the default, which spaces.WorktreeDir derives from the branch and the placeholder shows.
	dir   textinput.Model
	busy  bool
	err   error
	width int
	// zones and buttons are where the last frame drew its buttons, and what each runs, for a click.
	zones   map[string][]look.Zone
	buttons controls[*newWorktreeForm]
}

// newForm is the form for a worktree in space. The branch starts with the configured prefix, so typing
// just the name gives PREFIX/NAME; deleting it makes a branch without one.
func newForm(cfg config.Config, space spaces.Space) *newWorktreeForm {
	th := look.New(true)
	branch := newInput(th, "")
	branch.SetValue(spaces.BranchStart(cfg))
	branch.CursorEnd()
	branch.Focus()
	f := &newWorktreeForm{th: th, cfg: cfg, space: space, branch: branch, dir: newInput(th, "")}
	f.followBranch()
	return f
}

type createdMsg struct{ err error }

func (f *newWorktreeForm) Init() tea.Cmd {
	return textinput.Blink
}

// plan is the branch and directory the form would create now.
func (f *newWorktreeForm) plan() (branch, dir string) {
	branch = strings.TrimSpace(f.branch.Value())
	return branch, spaces.WorktreeDir(f.cfg, f.space.Root, branch, f.dir.Value())
}

func (f *newWorktreeForm) create() tea.Cmd {
	f.busy, f.err = true, nil
	cfg, space := f.cfg, f.space
	branch, dir := f.plan()
	return func() tea.Msg {
		wt, err := spaces.CreateWorktree(space, branch, dir)
		if err != nil {
			return createdMsg{err}
		}
		return createdMsg{spaces.OpenFromOverlay(cfg, wt)}
	}
}

// switchField moves typing between the branch and the directory.
func (f *newWorktreeForm) switchField() tea.Cmd {
	if f.branch.Focused() {
		f.branch.Blur()
		return f.dir.Focus()
	}
	f.dir.Blur()
	return f.branch.Focus()
}

func (f *newWorktreeForm) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		f.width = msg.Width
		// bubbles v2 draws only the first character of a placeholder without a width. The fields start
		// after the padding and their labels, and the cursor takes a cell past the text.
		f.branch.SetWidth(max(f.width-4-labelWidth-1, 1))
		f.dir.SetWidth(max(f.width-4-labelWidth-1, 1))
		return f, nil
	case createdMsg:
		f.busy, f.err = false, msg.err
		if msg.err == nil {
			return f, tea.Quit
		}
		return f, nil
	case tea.KeyPressMsg:
		if cmd, ok := runOn(f, msg, theForm.Target(formSubject{})); ok {
			return f, cmd
		}
	case tea.MouseClickMsg:
		if cmd, ok := f.buttons.click(f, f.zones, msg); ok {
			return f, cmd
		}
	}
	// Everything else is typing in the field that has focus.
	var cmd tea.Cmd
	if f.branch.Focused() {
		f.branch, cmd = f.branch.Update(msg)
	} else {
		f.dir, cmd = f.dir.Update(msg)
	}
	f.followBranch()
	return f, cmd
}

// followBranch makes the directory's placeholder the default for the current branch, so the form always
// shows where the worktree would go.
func (f *newWorktreeForm) followBranch() {
	dir := spaces.WorktreeDir(f.cfg, f.space.Root, strings.TrimSpace(f.branch.Value()), "")
	if rel, err := filepath.Rel(f.space.Root, dir); err == nil {
		dir = rel
	}
	f.dir.Placeholder = dir
}

func (f *newWorktreeForm) View() tea.View {
	frame, zones := look.Scan(f.render())
	f.zones = zones
	v := tea.NewView(frame)
	v.AltScreen = true
	v.MouseMode = tea.MouseModeCellMotion
	return v
}

// labelWidth is the column the fields start at, after their labels.
const labelWidth = 11

func (f *newWorktreeForm) render() string {
	th, width := f.th, max(f.width-4, 20)
	line := func(spans ...look.Span) string { return th.Render(look.Plain, spans...) }
	branch, _ := f.plan()
	lines := []string{
		line(look.Heading("New worktree in " + f.space.Name)), "",
		th.Draw(look.Plain, labelWidth, look.Label("branch")) + f.branch.View(),
		th.Draw(look.Plain, labelWidth, look.Label("directory")) + f.dir.View(),
		"",
	}
	switch {
	case branch == "" || strings.HasSuffix(branch, "/"):
		lines = append(lines, line(look.Muted("Name the branch to create or check out")))
	case f.space.HasBranch(branch):
		lines = append(lines, line(th.Cut(width, look.Muted("Checks out the existing branch "+branch))...))
	case len(f.space.Worktrees) > 0 && f.space.Worktrees[0].Branch != "":
		lines = append(lines, line(th.Cut(width, look.Muted("Creates "+branch+" from "+f.space.Worktrees[0].Branch))...))
	default:
		lines = append(lines, line(th.Cut(width, look.Muted("Creates "+branch))...))
	}
	lines = append(lines, "")
	switch {
	case f.busy:
		lines = append(lines, line(look.Busy("Creating...")), "")
	case f.err != nil:
		for _, row := range look.Error(f.err.Error()).Wrapped(width) {
			lines = append(lines, line(row))
		}
		lines = append(lines, "")
	}
	f.buttons = controls[*newWorktreeForm]{}
	buttons := f.buttons.draw(f, theForm.Target(formSubject{}),
		control{action: actCreate, label: "Create"},
		control{action: actCancel, label: "Cancel"})
	lines = append(lines, line(append(buttons, look.Text("  "), look.Key("tab"), look.KeyDesc(" next field"))...))
	return boxed(lines)
}

// RunNewWorktree runs the new worktree form for space, meant for a kitty overlay, until the worktree is
// created and opened or the form is left.
func RunNewWorktree(ctx context.Context, cfg config.Config, space spaces.Space) error {
	if _, err := tea.NewProgram(newForm(cfg, space), tea.WithContext(ctx)).Run(); err != nil {
		return fmt.Errorf("running the new worktree form: %w", err)
	}
	return nil
}
