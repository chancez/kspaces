package tui

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/chancez/kspaces/internal/config"
	"github.com/chancez/kspaces/internal/spaces"
)

// newWorktreeForm asks for a branch and, optionally, where its worktree goes, shows what it will do,
// creates the worktree and switches to its session. It is meant for a kitty overlay, as the sidebar is too
// narrow to type a name and read back the path in.
type newWorktreeForm struct {
	cfg    config.Config
	space  spaces.Space
	branch textinput.Model
	// Empty for the default, which spaces.WorktreeDir derives from the branch and the placeholder shows.
	dir   textinput.Model
	busy  bool
	err   error
	width int
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
		f.branch.SetWidth(max(f.width-4-11-1, 1))
		f.dir.SetWidth(max(f.width-4-11-1, 1))
		return f, nil
	case createdMsg:
		f.busy, f.err = false, msg.err
		if msg.err == nil {
			return f, tea.Quit
		}
		return f, nil
	case tea.KeyPressMsg:
		switch msg.String() {
		case "esc", "ctrl+c":
			return f, tea.Quit
		case "tab", "shift+tab", "up", "down":
			return f, f.switchField()
		case "enter":
			if f.busy {
				return f, nil
			}
			f.busy, f.err = true, nil
			return f, f.create()
		}
	}
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
	v := tea.NewView(f.render())
	v.AltScreen = true
	return v
}

func (f *newWorktreeForm) render() string {
	width := max(f.width-4, 20)
	key := lipgloss.NewStyle().Foreground(muted).Width(11)
	branch, _ := f.plan()
	lines := []string{
		spaceStyle.Render("New worktree in " + f.space.Name), "",
		key.Render("branch") + f.branch.View(),
		key.Render("directory") + f.dir.View(),
		"",
	}
	switch {
	case branch == "" || strings.HasSuffix(branch, "/"):
		lines = append(lines, branchStyle.Render("Name the branch to create or check out"))
	case f.space.HasBranch(branch):
		lines = append(lines, branchStyle.Render(truncate("Checks out the existing branch "+branch, width)))
	case len(f.space.Worktrees) > 0 && f.space.Worktrees[0].Branch != "":
		lines = append(lines, branchStyle.Render(truncate("Creates "+branch+" from "+f.space.Worktrees[0].Branch, width)))
	default:
		lines = append(lines, branchStyle.Render(truncate("Creates "+branch, width)))
	}
	lines = append(lines, "")
	switch {
	case f.busy:
		lines = append(lines, workingStyle.Render("Creating..."), "")
	case f.err != nil:
		for _, l := range wrap(f.err.Error(), width) {
			lines = append(lines, errorStyle.Render(l))
		}
		lines = append(lines, "")
	}
	lines = append(lines, branchStyle.Render("tab next field  enter create  esc cancel"))
	return lipgloss.NewStyle().Padding(1, 2).Render(strings.Join(lines, "\n"))
}

// RunNewWorktree runs the new worktree form for space, meant for a kitty overlay, until the worktree is
// created and opened or the form is left.
func RunNewWorktree(ctx context.Context, cfg config.Config, space spaces.Space) error {
	// The branch starts with the configured prefix, so typing just the name gives PREFIX/NAME; deleting
	// it makes a branch without one.
	branch := newInput("")
	branch.SetValue(spaces.BranchStart(cfg))
	branch.CursorEnd()
	branch.Focus()
	dir := newInput("")
	f := &newWorktreeForm{cfg: cfg, space: space, branch: branch, dir: dir}
	f.followBranch()
	if _, err := tea.NewProgram(f, tea.WithContext(ctx)).Run(); err != nil {
		return fmt.Errorf("running the new worktree form: %w", err)
	}
	return nil
}
