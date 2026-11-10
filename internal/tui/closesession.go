package tui

import (
	"context"
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/chancez/kspaces/internal/kitty"
	"github.com/chancez/kspaces/internal/spaces"
	"github.com/chancez/kspaces/internal/tui/look"
)

// closeSessionForm shows what closing a session does and closes it when confirmed. It is meant for a kitty
// overlay, like the new worktree form.
type closeSessionForm struct {
	th      *look.Theme
	session string
	// windows is how many windows the session has open, which closing it closes.
	windows int
	busy    bool
	err     error
	width   int
}

type closedMsg struct{ err error }

func (f *closeSessionForm) Init() tea.Cmd { return nil }

// closeSession asks kitty to close the session's windows and leaves. This form may be over one of them,
// so it does not wait for kitty to have closed them.
func (f *closeSessionForm) closeSession() tea.Cmd {
	f.busy, f.err = true, nil
	session := f.session
	return func() tea.Msg { return closedMsg{kitty.CloseSession(session)} }
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
	}
	return f, nil
}

func (f *closeSessionForm) View() tea.View {
	v := tea.NewView(f.render())
	v.AltScreen = true
	return v
}

func (f *closeSessionForm) render() string {
	th, width := f.th, max(f.width-4, 20)
	line := func(spans ...look.Span) string { return th.Render(look.Plain, spans...) }
	lines := []string{
		line(look.Heading("Close session " + f.session)), "",
		line(th.Cut(width, look.Text(closesWhat(f.windows)))...),
		line(th.Cut(width, look.Muted("Its saved file is kept, so opening it again restores its tabs."))...),
		"",
	}
	switch {
	case f.busy:
		lines = append(lines, line(look.Busy("Closing...")), "")
	case f.err != nil:
		for _, row := range look.Error(f.err.Error()).Wrapped(width) {
			lines = append(lines, line(row))
		}
		lines = append(lines, "")
	}
	lines = append(lines, line(look.Key("y"), look.KeyDesc(" close  "), look.Key("n/esc"), look.KeyDesc(" cancel")))
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

// closesWhat says what closing a session with n windows closes.
func closesWhat(n int) string {
	if n == 1 {
		return "Closes its window, and what runs in it."
	}
	return fmt.Sprintf("Closes its %d windows, and what runs in them.", n)
}

// RunCloseSession runs the close session form for session, or for the session of the window with the
// keyboard when it is empty, meant for a kitty overlay. With yes it closes the session without asking.
func RunCloseSession(ctx context.Context, session string, yes bool) error {
	self := kitty.SelfWindowID()
	if session == "" {
		self = kitty.FocusedWindow
	}
	st, err := kitty.LoadState(self)
	if err != nil {
		return err
	}
	if session == "" {
		session = st.Current
	}
	switch {
	case session == "":
		return fmt.Errorf("the current window is not in a session")
	case st.Windows[session] == 0:
		return fmt.Errorf("session %s has no windows open", session)
	case yes:
		return spaces.CloseSession(session)
	}
	f := &closeSessionForm{th: look.New(true), session: session, windows: st.Windows[session]}
	if _, err := tea.NewProgram(f, tea.WithContext(ctx)).Run(); err != nil {
		return fmt.Errorf("running the close session form: %w", err)
	}
	return nil
}
