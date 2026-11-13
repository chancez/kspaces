package tui

import (
	"context"
	"errors"
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/chancez/kspaces/internal/kitty"
	"github.com/chancez/kspaces/internal/spaces"
	"github.com/chancez/kspaces/internal/tui/look"
)

// closeSessionForm shows what closing sessions does and closes them when confirmed. It is meant for a
// kitty overlay, like the new worktree form.
type closeSessionForm struct {
	th       *look.Theme
	sessions []string
	// windows is how many windows each session has open, which closing it closes.
	windows []int
	busy    bool
	err     error
	width   int
	// zones and buttons are where the last frame drew its buttons, and what each runs, for a click.
	zones   map[string][]look.Zone
	buttons controls[*closeSessionForm]
}

type closedMsg struct{ err error }

func (f *closeSessionForm) Init() tea.Cmd { return nil }

// closeSession asks kitty to close the sessions' windows and leaves. This form may be over one of them,
// so it does not wait for kitty to have closed them. One failing does not stop the rest.
func (f *closeSessionForm) closeSession() tea.Cmd {
	f.busy, f.err = true, nil
	sessions := f.sessions
	return func() tea.Msg {
		var errs []error
		for _, s := range sessions {
			if err := kitty.CloseSession(s); err != nil {
				errs = append(errs, fmt.Errorf("%s: %w", s, err))
			}
		}
		return closedMsg{errors.Join(errs...)}
	}
}

func (f *closeSessionForm) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		f.width = msg.Width
	case closedMsg:
		f.busy, f.err = false, msg.err
		if msg.err == nil {
			return f, tea.Quit
		}
	case tea.KeyPressMsg:
		if cmd, ok := runOn(f, msg, theCloseForm.Target(closeSubject{})); ok {
			return f, cmd
		}
	case tea.MouseClickMsg:
		if cmd, ok := f.buttons.click(f, f.zones, msg); ok {
			return f, cmd
		}
	}
	return f, nil
}

func (f *closeSessionForm) View() tea.View {
	frame, zones := look.Scan(f.render())
	f.zones = zones
	v := tea.NewView(frame)
	v.AltScreen = true
	v.MouseMode = tea.MouseModeCellMotion
	return v
}

func (f *closeSessionForm) render() string {
	th, width := f.th, max(f.width-4, 20)
	line := func(spans ...look.Span) string { return th.Render(look.Plain, spans...) }
	var lines []string
	if len(f.sessions) == 1 {
		lines = []string{
			line(look.Heading("Close session " + f.sessions[0])), "",
			line(th.Cut(width, look.Text(closesWhat(f.windows[0])))...),
			"",
		}
	} else {
		lines = []string{line(look.Heading(fmt.Sprintf("Close %d sessions", len(f.sessions)))), ""}
		names := 0
		for _, s := range f.sessions {
			names = max(names, len(s))
		}
		for i, s := range f.sessions {
			lines = append(lines, line(th.Cut(width, look.Text(s), look.Default(strings.Repeat(" ", names-len(s)+2)), look.Muted(plural(f.windows[i], "window")))...))
		}
		lines = append(lines, "", line(th.Cut(width, look.Text("Closes their windows, and what runs in them."))...), "")
	}
	switch {
	case f.busy:
		lines = append(lines, line(look.Busy("Closing...")), "")
	case f.err != nil:
		for _, l := range strings.Split(f.err.Error(), "\n") {
			for _, row := range look.Error(l).Wrapped(width) {
				lines = append(lines, line(row))
			}
		}
		lines = append(lines, "")
	}
	f.buttons = controls[*closeSessionForm]{}
	lines = append(lines, line(f.buttons.draw(f, theCloseForm.Target(closeSubject{}),
		control{action: actClose, label: "Close", danger: true},
		control{action: actCloseCancel, label: "Cancel"})...))
	return boxed(lines)
}

// closesWhat says what closing a session with n windows closes.
func closesWhat(n int) string {
	if n == 1 {
		return "Closes its window, and what runs in it."
	}
	return fmt.Sprintf("Closes its %d windows, and what runs in them.", n)
}

// RunCloseSession runs the close session form for sessions, or for the session of the window with the
// keyboard when there are none, meant for a kitty popup. A session with no windows is refused. With yes
// it closes them without asking, each waiting for kitty, one failing not stopping the rest.
func RunCloseSession(ctx context.Context, sessions []string, yes bool) error {
	self := kitty.SelfWindowID()
	if len(sessions) == 0 {
		self = kitty.FocusedWindow
	}
	st, err := kitty.LoadState(self)
	if err != nil {
		return err
	}
	if len(sessions) == 0 {
		if st.Current == "" {
			return fmt.Errorf("the current window is not in a session")
		}
		sessions = []string{st.Current}
	}
	windows := make([]int, len(sessions))
	for i, s := range sessions {
		if windows[i] = st.Windows[s]; windows[i] == 0 {
			return fmt.Errorf("session %s has no windows open", s)
		}
	}
	if yes {
		var errs []error
		for _, s := range sessions {
			if err := spaces.CloseSession(s); err != nil {
				errs = append(errs, fmt.Errorf("%s: %w", s, err))
			}
		}
		return errors.Join(errs...)
	}
	f := &closeSessionForm{th: look.New(true), sessions: sessions, windows: windows}
	if err := runPopup(ctx, f); err != nil {
		return fmt.Errorf("running the close session form: %w", err)
	}
	return nil
}
