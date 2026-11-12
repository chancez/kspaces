package tui

import (
	"context"
	"fmt"

	tea "charm.land/bubbletea/v2"

	"github.com/chancez/kspaces/internal/config"
	"github.com/chancez/kspaces/internal/kitty"
	"github.com/chancez/kspaces/internal/spaces"
	"github.com/chancez/kspaces/internal/tui/look"
)

// forgetSessionForm shows what forgetting a session does and deletes its saved file when confirmed: all
// that is left of a missing worktree with no windows. It is meant for a kitty popup, like the close
// session form.
type forgetSessionForm struct {
	th      *look.Theme
	cfg     config.Config
	session string
	busy    bool
	err     error
	width   int
	// zones and buttons are where the last frame drew its buttons, and what each runs, for a click.
	zones   map[string][]look.Zone
	buttons controls[*forgetSessionForm]
}

type forgotMsg struct{ err error }

func (f *forgetSessionForm) Init() tea.Cmd { return nil }

// forget deletes the session's saved file and leaves.
func (f *forgetSessionForm) forget() tea.Cmd {
	f.busy, f.err = true, nil
	cfg, session := f.cfg, f.session
	return func() tea.Msg { return forgotMsg{spaces.ForgetSession(cfg, session)} }
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
	lines := []string{line(look.Heading("Forget session " + f.session)), ""}
	for _, row := range look.Text("Deletes its saved file, so kitty does not restore it. Its worktree directory is gone.").Wrapped(width) {
		lines = append(lines, line(row))
	}
	lines = append(lines, line(th.Cut(width, look.Muted(config.Tilde(spaces.SessionFile(f.cfg, f.session))))...), "")
	switch {
	case f.busy:
		lines = append(lines, line(look.Busy("Deleting...")), "")
	case f.err != nil:
		for _, row := range look.Error(f.err.Error()).Wrapped(width) {
			lines = append(lines, line(row))
		}
		lines = append(lines, "")
	}
	f.buttons = controls[*forgetSessionForm]{}
	lines = append(lines, line(f.buttons.draw(f, theForgetForm.Target(forgetSubject{}),
		control{action: actForget, label: "Delete", danger: true},
		control{action: actForgetCancel, label: "Cancel"})...))
	return boxed(lines)
}

// RunForgetSession runs the forget session form for session, meant for a kitty popup. With yes it
// deletes the saved file without asking. A session with windows open is refused: the file is not all
// there is of it, and closing it is what ends the rest.
func RunForgetSession(ctx context.Context, cfg config.Config, session string, yes bool) error {
	st, err := kitty.LoadState(0)
	if err != nil {
		return err
	}
	switch {
	case st.Windows[session] > 0:
		return fmt.Errorf("session %s has windows open; close it first", session)
	case yes:
		return spaces.ForgetSession(cfg, session)
	}
	f := &forgetSessionForm{th: look.New(true), cfg: cfg, session: session}
	if err := runPopup(ctx, f); err != nil {
		return fmt.Errorf("running the forget session form: %w", err)
	}
	return nil
}
