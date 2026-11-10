package main

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// newWorktreeForm asks for a branch and, optionally, where its worktree goes, shows what it will do,
// creates the worktree and switches to its session. It is meant for a kitty overlay, as the sidebar is too
// narrow to type a name and read back the path in.
type newWorktreeForm struct {
	cfg    Config
	space  Space
	branch textinput.Model
	// Empty for the default, which worktreeDir derives from the branch and the placeholder shows.
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
	return branch, worktreeDir(f.cfg, f.space.Root, branch, f.dir.Value())
}

func (f *newWorktreeForm) create() tea.Cmd {
	cfg, space := f.cfg, f.space
	branch, dir := f.plan()
	return func() tea.Msg {
		wt, err := createWorktree(space, branch, dir)
		if err != nil {
			return createdMsg{err}
		}
		st, err := loadKittyState(selfWindowID())
		if err != nil {
			return createdMsg{err}
		}
		// The form is an overlay with focus, so there is no window of the tab to hand back to: the tab's
		// own window gets focus again when the overlay closes.
		st.Home = 0
		return createdMsg{openWorktree(cfg, st, wt)}
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
		return f, nil
	case createdMsg:
		f.busy, f.err = false, msg.err
		if msg.err == nil {
			return f, tea.Quit
		}
		return f, nil
	case tea.KeyMsg:
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
	dir := worktreeDir(f.cfg, f.space.Root, strings.TrimSpace(f.branch.Value()), "")
	if rel, err := filepath.Rel(f.space.Root, dir); err == nil {
		dir = rel
	}
	f.dir.Placeholder = dir
}

func branchExists(root, branch string) bool {
	return exec.Command("git", "-C", root, "rev-parse", "--verify", "--quiet", "refs/heads/"+branch).Run() == nil
}

func (f *newWorktreeForm) View() string {
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
	case branchExists(f.space.Root, branch):
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

// findSpace is the space given by root or name, or, with an empty query, the space of the session of the
// window that has the keyboard.
func findSpace(cfg Config, query string) (Space, error) {
	var current string
	if query == "" {
		st, err := loadKittyState(focusedWindow)
		if err != nil {
			return Space{}, err
		}
		current = st.Current
	}
	abs, _ := filepath.Abs(query)
	for _, root := range cfg.Spaces {
		s := loadSpace(root)
		switch {
		case query != "" && (root == abs || s.Name == query):
			return s, s.Err
		case query == "":
			for _, wt := range s.Worktrees {
				if wt.Session == current {
					return s, nil
				}
			}
		}
	}
	if query == "" {
		return Space{}, fmt.Errorf("the current window is not in a space")
	}
	return Space{}, fmt.Errorf("%s is not a configured space", query)
}

func runNewWorktree(query string) error {
	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	space, err := findSpace(cfg, query)
	if err != nil {
		return err
	}
	// The branch starts with the configured prefix, so typing just the name gives PREFIX/NAME; deleting
	// it makes a branch without one.
	branch := textinput.New()
	branch.Prompt = ""
	branch.SetValue(branchStart(cfg))
	branch.CursorEnd()
	branch.Focus()
	dir := textinput.New()
	dir.Prompt = ""
	f := &newWorktreeForm{cfg: cfg, space: space, branch: branch, dir: dir}
	f.followBranch()
	if _, err := tea.NewProgram(f, tea.WithAltScreen()).Run(); err != nil {
		return fmt.Errorf("running the new worktree form: %w", err)
	}
	return nil
}
