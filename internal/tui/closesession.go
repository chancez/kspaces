package tui

import (
	"context"
	"errors"
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/chancez/kspaces/internal/config"
	"github.com/chancez/kspaces/internal/kitty"
	"github.com/chancez/kspaces/internal/spaces"
	"github.com/chancez/kspaces/internal/tui/look"
)

// closeSessionForm shows what closing sessions does and closes them when confirmed, keeping them as mode
// says, see spaces.KeepSession, or only closing their windows when mode is empty. It is meant for a kitty
// overlay, like the new worktree form.
type closeSessionForm struct {
	th       *look.Theme
	cfg      config.Config
	mode     spaces.KeepMode
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

// closeSession closes the sessions' windows, keeping the sessions as the form's mode says, and leaves. This
// form may be over one of them, so it does not wait for kitty to have closed them. One failing does not stop
// the rest.
func (f *closeSessionForm) closeSession() tea.Cmd {
	f.busy, f.err = true, nil
	cfg, mode, sessions := f.cfg, f.mode, f.sessions
	return func() tea.Msg {
		var errs []error
		for _, s := range sessions {
			if err := closeOne(cfg, mode, s); err != nil {
				errs = append(errs, fmt.Errorf("%s: %w", s, err))
			}
		}
		return closedMsg{errors.Join(errs...)}
	}
}

// closeOne closes session's windows, keeping it as mode says, see closeSessionForm.
func closeOne(cfg config.Config, mode spaces.KeepMode, session string) error {
	if mode == "" {
		return kitty.CloseSession(session)
	}
	return spaces.KeepSession(cfg, session, mode)
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
	verb, busy := "Close", "Closing..."
	if f.mode == spaces.KeepSuspended {
		verb, busy = "Suspend", "Suspending..."
	}
	var lines []string
	text := func(s string) {
		for _, row := range look.Text(s).Wrapped(width) {
			lines = append(lines, line(row))
		}
	}
	kept := len(f.cfg.Keeper) > 0
	if len(f.sessions) == 1 {
		lines = []string{line(look.Heading(verb + " session " + f.sessions[0])), ""}
		text(closesWhat(f.mode, f.windows[0], kept))
		lines = append(lines, "")
	} else {
		lines = []string{line(look.Heading(fmt.Sprintf("%s %d sessions", verb, len(f.sessions)))), ""}
		names := 0
		for _, s := range f.sessions {
			names = max(names, len(s))
		}
		for i, s := range f.sessions {
			lines = append(lines, line(th.Cut(width, look.Text(s), look.Default(strings.Repeat(" ", names-len(s)+2)), look.Muted(plural(f.windows[i], "window")))...))
		}
		lines = append(lines, "")
		text(closeTheirs(f.mode, kept))
		lines = append(lines, "")
	}
	switch {
	case f.busy:
		lines = append(lines, line(look.Busy(busy)), "")
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
		control{action: actClose, label: verb, danger: true},
		control{action: actCloseCancel, label: "Cancel"})...))
	return boxed(lines)
}

// closesWhat says what closing a session with n windows does, keeping it as mode says, what runs in them
// too when kept says a keeper keeps it.
func closesWhat(mode spaces.KeepMode, n int, kept bool) string {
	windows, them, asWas := "its window", "it", "as it was"
	if n != 1 {
		windows, them, asWas = fmt.Sprintf("its %d windows", n), "them", "as they were"
	}
	switch {
	case mode == "":
		return fmt.Sprintf("Closes %s, and what runs in %s.", windows, them)
	case kept && mode == spaces.KeepSuspended:
		return fmt.Sprintf("Stops what runs in %s and closes %s. Opening it again brings %s back %s.", windows, them, them, asWas)
	case kept:
		return fmt.Sprintf("Closes %s, keeping what runs in %s. Opening it again brings %s back.", windows, them, them)
	}
	return fmt.Sprintf("Closes %s, and what runs in %s. Opening it again restores its layout, with fresh programs.", windows, them)
}

// closeTheirs says what closing several sessions does, keeping them as mode says, what runs in them too
// when kept says a keeper keeps it.
func closeTheirs(mode spaces.KeepMode, kept bool) string {
	switch {
	case mode == "":
		return "Closes their windows, and what runs in them."
	case kept && mode == spaces.KeepSuspended:
		return "Stops what runs in their windows and closes them. Opening one again brings it back as it was."
	case kept:
		return "Closes their windows, keeping what runs in them. Opening one again brings its windows back."
	}
	return "Closes their windows, and what runs in them. Opening one again restores its layout, with fresh programs."
}

// RunCloseSession runs the close session form for sessions, or for the session of the window with the
// keyboard when there are none, meant for a kitty popup. A session with no windows is refused. Closing keeps
// the sessions to open again when cfg's session saver can, see spaces.KeepClosed; with suspend they are
// suspended instead, see spaces.KeepSuspended, which needs the saver. With yes it closes them without
// asking, each waiting for kitty, one failing not stopping the rest.
func RunCloseSession(ctx context.Context, cfg config.Config, sessions []string, yes, suspend bool) error {
	var mode spaces.KeepMode
	switch {
	case suspend && !spaces.CanKeep(cfg):
		return errors.New("suspending a session needs [kitty] save_sessions and a sessions directory")
	case suspend:
		mode = spaces.KeepSuspended
	case spaces.CanKeep(cfg):
		mode = spaces.KeepClosed
	}
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
			err := closeOne(cfg, mode, s)
			if err == nil {
				err = spaces.WaitClosed(s)
			}
			if err != nil {
				errs = append(errs, fmt.Errorf("%s: %w", s, err))
			}
		}
		return errors.Join(errs...)
	}
	f := &closeSessionForm{th: look.New(true), cfg: cfg, mode: mode, sessions: sessions, windows: windows}
	if err := runPopup(ctx, f); err != nil {
		return fmt.Errorf("running the close session form: %w", err)
	}
	return nil
}
