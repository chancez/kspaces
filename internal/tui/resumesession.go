package tui

import (
	"context"
	"fmt"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/chancez/kspaces/internal/config"
	"github.com/chancez/kspaces/internal/kitty"
	"github.com/chancez/kspaces/internal/spaces"
	"github.com/chancez/kspaces/internal/tui/look"
)

// resumeSessionForm asks before resuming a suspended session, so a stray click on its row does not bring
// back all it ran. It is meant for a kitty popup, like the forget session form.
type resumeSessionForm struct {
	th  *look.Theme
	cfg config.Config
	st  kitty.State
	wt  spaces.Worktree
	// open is spaces.OpenWorktree, but in tests.
	open  func(cfg config.Config, st kitty.State, wt spaces.Worktree, opts spaces.OpenOptions) error
	busy  bool
	err   error
	width int
	// zones and buttons are where the last frame drew its buttons, and what each runs, for a click.
	zones   map[string][]look.Zone
	buttons controls[*resumeSessionForm]
}

type resumedMsg struct{ err error }

func (f *resumeSessionForm) Init() tea.Cmd { return nil }

// resume opens the session, which restores it from its saved file, and leaves.
func (f *resumeSessionForm) resume() tea.Cmd {
	f.busy, f.err = true, nil
	cfg, st, wt, open := f.cfg, f.st, f.wt, f.open
	return func() tea.Msg { return resumedMsg{open(cfg, st, wt, spaces.OpenOptions{})} }
}

func (f *resumeSessionForm) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		f.width = msg.Width
	case resumedMsg:
		f.busy, f.err = false, msg.err
		if msg.err == nil {
			return f, tea.Quit
		}
	case tea.KeyPressMsg:
		if cmd, ok := runOn(f, msg, theResumeForm.Target(resumeSubject{})); ok {
			return f, cmd
		}
	case tea.MouseClickMsg:
		if cmd, ok := f.buttons.click(f, f.zones, msg); ok {
			return f, cmd
		}
	}
	return f, nil
}

func (f *resumeSessionForm) View() tea.View {
	frame, zones := look.Scan(f.render())
	f.zones = zones
	v := tea.NewView(frame)
	v.AltScreen = true
	v.MouseMode = tea.MouseModeCellMotion
	return v
}

func (f *resumeSessionForm) render() string {
	th, width := f.th, max(f.width-4, 20)
	line := func(spans ...look.Span) string { return th.Render(look.Plain, spans...) }
	lines := []string{line(look.Heading("Resume session " + f.wt.Session)), ""}
	for _, row := range look.Text("It is suspended. Resuming restores its windows and what was kept running in them.").Wrapped(width) {
		lines = append(lines, line(row))
	}
	lines = append(lines, "")
	switch {
	case f.busy:
		lines = append(lines, line(look.Busy("Resuming...")), "")
	case f.err != nil:
		for _, l := range strings.Split(f.err.Error(), "\n") {
			for _, row := range look.Error(l).Wrapped(width) {
				lines = append(lines, line(row))
			}
		}
		lines = append(lines, "")
	}
	f.buttons = controls[*resumeSessionForm]{}
	lines = append(lines, line(f.buttons.draw(f, theResumeForm.Target(resumeSubject{}),
		control{action: actResume, label: "Resume"},
		control{action: actResumeCancel, label: "Cancel"})...))
	return boxed(lines)
}

// RunResumeSession runs the resume session form for session, meant for a kitty popup. With yes it resumes
// the session without asking.
func RunResumeSession(ctx context.Context, cfg config.Config, session string, yes bool) error {
	snap := spaces.LoadShared().View(0)
	if snap.Err != nil {
		return snap.Err
	}
	wt, ok := sessionWorktree(snap, session)
	if !ok {
		return fmt.Errorf("no session %s", session)
	}
	// The popup has the keyboard, so there is no window of the tab to hand it back to, as in the picker.
	st := snap.Kitty
	st.Home = 0
	if yes {
		return spaces.OpenWorktree(cfg, st, wt, spaces.OpenOptions{})
	}
	f := &resumeSessionForm{th: look.New(true), cfg: cfg, st: st, wt: wt, open: spaces.OpenWorktree}
	if err := runPopup(ctx, f); err != nil {
		return fmt.Errorf("running the resume session form: %w", err)
	}
	return nil
}

// sessionWorktree is the worktree of session in snap, of a space or of none.
func sessionWorktree(snap spaces.Snapshot, session string) (spaces.Worktree, bool) {
	for _, s := range slices.Concat(snap.Spaces, snap.Others) {
		for _, wt := range s.Worktrees {
			if wt.Session == session {
				return wt, true
			}
		}
	}
	return spaces.Worktree{}, false
}
