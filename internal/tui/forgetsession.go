package tui

import (
	"context"
	"errors"
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/chancez/kspaces/internal/config"
	"github.com/chancez/kspaces/internal/spaces"
	"github.com/chancez/kspaces/internal/tui/look"
)

// forgetSessionForm shows what forgetting sessions does and deletes their saved files when confirmed: all
// that is left of a missing worktree with no windows, and what restores a session closed. It is meant for
// a kitty popup, like the close session form.
type forgetSessionForm struct {
	th       *look.Theme
	cfg      config.Config
	sessions []string
	// gone says which sessions are missing worktrees', whose directories are gone.
	gone  []bool
	busy  bool
	err   error
	width int
	// zones and buttons are where the last frame drew its buttons, and what each runs, for a click.
	zones   map[string][]look.Zone
	buttons controls[*forgetSessionForm]
}

type forgotMsg struct{ err error }

func (f *forgetSessionForm) Init() tea.Cmd { return nil }

// forget deletes the sessions' saved files and leaves. One failing does not stop the rest.
func (f *forgetSessionForm) forget() tea.Cmd {
	f.busy, f.err = true, nil
	cfg, sessions := f.cfg, f.sessions
	return func() tea.Msg { return forgotMsg{forgetAll(cfg, sessions)} }
}

// forgetAll deletes the saved files of sessions, saying which could not be.
func forgetAll(cfg config.Config, sessions []string) error {
	if len(sessions) == 1 {
		return spaces.ForgetSession(cfg, sessions[0])
	}
	var errs []error
	for _, s := range sessions {
		if err := spaces.ForgetSession(cfg, s); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", s, err))
		}
	}
	return errors.Join(errs...)
}

func (f *forgetSessionForm) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		f.width = msg.Width
	case forgotMsg:
		f.busy, f.err = false, msg.err
		if msg.err == nil {
			return f, tea.Quit
		}
	case tea.KeyPressMsg:
		if cmd, ok := runOn(f, msg, theForgetForm.Target(forgetSubject{})); ok {
			return f, cmd
		}
	case tea.MouseClickMsg:
		if cmd, ok := f.buttons.click(f, f.zones, msg); ok {
			return f, cmd
		}
	}
	return f, nil
}

func (f *forgetSessionForm) View() tea.View {
	frame, zones := look.Scan(f.render())
	f.zones = zones
	v := tea.NewView(frame)
	v.AltScreen = true
	v.MouseMode = tea.MouseModeCellMotion
	return v
}

func (f *forgetSessionForm) render() string {
	th, width := f.th, max(f.width-4, 20)
	line := func(spans ...look.Span) string { return th.Render(look.Plain, spans...) }
	var lines []string
	text := func(s string) {
		for _, row := range look.Text(s).Wrapped(width) {
			lines = append(lines, line(row))
		}
	}
	file := func(s string) look.Span { return look.Muted(config.Tilde(spaces.SessionFile(f.cfg, s))) }
	if len(f.sessions) == 1 {
		lines = []string{line(look.Heading("Forget session " + f.sessions[0])), ""}
		says := "Deletes its saved file, so kitty does not restore it."
		if f.gone[0] {
			says += " Its worktree directory is gone."
		}
		text(says)
		lines = append(lines, line(th.Cut(width, file(f.sessions[0]))...), "")
	} else {
		lines = []string{line(look.Heading(fmt.Sprintf("Forget %d sessions", len(f.sessions)))), ""}
		text("Deletes their saved files, so kitty does not restore them.")
		lines = append(lines, "")
		names := 0
		for _, s := range f.sessions {
			names = max(names, len(s))
		}
		for i, s := range f.sessions {
			spans := []look.Span{look.Text(s), look.Default(strings.Repeat(" ", names-len(s)+2)), file(s)}
			if f.gone[i] {
				spans = append(spans, look.Muted("; worktree directory gone"))
			}
			lines = append(lines, line(th.Cut(width, spans...)...))
		}
		lines = append(lines, "")
	}
	switch {
	case f.busy:
		lines = append(lines, line(look.Busy("Deleting...")), "")
	case f.err != nil:
		for _, l := range strings.Split(f.err.Error(), "\n") {
			for _, row := range look.Error(l).Wrapped(width) {
				lines = append(lines, line(row))
			}
		}
		lines = append(lines, "")
	}
	f.buttons = controls[*forgetSessionForm]{}
	lines = append(lines, line(f.buttons.draw(f, theForgetForm.Target(forgetSubject{}),
		control{action: actForget, label: "Delete", danger: true},
		control{action: actForgetCancel, label: "Cancel"})...))
	return boxed(lines)
}

// RunForgetSession runs the forget session form for sessions, meant for a kitty popup. With yes it
// deletes the saved files without asking. A session with windows open is refused: the file is not all
// there is of it, and closing it is what ends the rest.
func RunForgetSession(ctx context.Context, cfg config.Config, sessions []string, yes bool) error {
	snap := spaces.LoadShared().View(0)
	if snap.Err != nil {
		return snap.Err
	}
	missing := map[string]bool{}
	for _, s := range snap.Spaces {
		for _, wt := range s.Missing {
			missing[wt.Session] = true
		}
	}
	gone := make([]bool, len(sessions))
	for i, s := range sessions {
		if snap.Kitty.Windows[s] > 0 {
			return fmt.Errorf("session %s has windows open; close it first", s)
		}
		gone[i] = missing[s]
	}
	if yes {
		return forgetAll(cfg, sessions)
	}
	f := &forgetSessionForm{th: look.New(true), cfg: cfg, sessions: sessions, gone: gone}
	if err := runPopup(ctx, f); err != nil {
		return fmt.Errorf("running the forget session form: %w", err)
	}
	return nil
}
