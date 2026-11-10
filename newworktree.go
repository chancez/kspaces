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

// newWorktreeForm asks for a worktree's name, shows where it will go and which branch it will check out,
// creates it and switches to its session. It is meant for a kitty overlay, as the sidebar is too narrow to
// type a name and read back the path in.
type newWorktreeForm struct {
	cfg   Config
	space Space
	input textinput.Model
	busy  bool
	err   error
	width int
}

type createdMsg struct{ err error }

func (f *newWorktreeForm) Init() tea.Cmd {
	return textinput.Blink
}

func (f *newWorktreeForm) create() tea.Cmd {
	cfg, space, name := f.cfg, f.space, f.input.Value()
	return func() tea.Msg {
		wt, err := createWorktree(cfg, space, name)
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
		case "enter":
			if f.busy || strings.TrimSpace(f.input.Value()) == "" {
				return f, nil
			}
			f.busy, f.err = true, nil
			return f, f.create()
		}
	}
	var cmd tea.Cmd
	f.input, cmd = f.input.Update(msg)
	return f, cmd
}

func branchExists(root, branch string) bool {
	return exec.Command("git", "-C", root, "rev-parse", "--verify", "--quiet", "refs/heads/"+branch).Run() == nil
}

func (f *newWorktreeForm) View() string {
	width := max(f.width, 20)
	key := lipgloss.NewStyle().Foreground(muted).Width(11)
	lines := []string{spaceStyle.Render("New worktree in " + f.space.Name), "", f.input.View(), ""}
	if name := strings.TrimSpace(f.input.Value()); name != "" {
		dir, branch := newWorktreePlan(f.cfg, f.space.Root, name)
		if rel, err := filepath.Rel(f.space.Root, dir); err == nil {
			dir = rel
		}
		lines = append(lines, key.Render("directory")+truncate(dir, width-11))
		switch {
		case branchExists(f.space.Root, branch):
			lines = append(lines, key.Render("branch")+truncate(branch, width-22)+branchStyle.Render(" (existing)"))
		case len(f.space.Worktrees) > 0 && f.space.Worktrees[0].Branch != "":
			lines = append(lines, key.Render("branch")+truncate(branch, width-17)+branchStyle.Render(" (new)"))
			lines = append(lines, key.Render("from")+truncate(f.space.Worktrees[0].Branch, width-11))
		default:
			lines = append(lines, key.Render("branch")+truncate(branch, width-17)+branchStyle.Render(" (new)"))
		}
	} else {
		lines = append(lines, branchStyle.Render(truncate(fmt.Sprintf("Goes in %s/, on %s<name>", f.cfg.WorktreeDir, f.cfg.BranchPrefix), width)))
	}
	lines = append(lines, "")
	switch {
	case f.busy:
		lines = append(lines, workingStyle.Render("Creating..."))
	case f.err != nil:
		for _, l := range wrap(f.err.Error(), width) {
			lines = append(lines, errorStyle.Render(l))
		}
	}
	lines = append(lines, branchStyle.Render("enter create  esc cancel"))
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
	input := textinput.New()
	input.Prompt = "\uf126  " // code-fork
	input.Placeholder = "worktree name"
	input.Focus()
	f := &newWorktreeForm{cfg: cfg, space: space, input: input}
	if _, err := tea.NewProgram(f, tea.WithAltScreen()).Run(); err != nil {
		return fmt.Errorf("running the new worktree form: %w", err)
	}
	return nil
}
