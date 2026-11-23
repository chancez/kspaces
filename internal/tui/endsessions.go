package tui

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/chancez/kspaces/internal/config"
	"github.com/chancez/kspaces/internal/spaces"
	"github.com/chancez/kspaces/internal/tui/interact"
	"github.com/chancez/kspaces/internal/tui/look"
)

// endSessionsForm ends the session provider's sessions no window shows when confirmed: kills the running
// ones and removes the ended ones. Given sessions it ends them all; cleaning up a space's it lists them each
// with a box, ticked at first only for those that exited or died, as a shelved one was ended on purpose to
// come back and a running one may be someone's work. It is meant for a kitty popup, like the forget session
// form.
type endSessionsForm struct {
	th *look.Theme
	// provider is the session provider, which ends them.
	provider []string
	sessions []spaces.ProviderSession
	// space is the space cleaned up, empty for sessions given, which have no boxes.
	space   string
	checked []bool
	cursor  int
	busy    bool
	err     error
	width   int
	// zones and buttons are where the last frame drew its rows and buttons, and what each button runs.
	zones   map[string][]look.Zone
	buttons controls[*endSessionsForm]
}

// endSessionsZone is row i of the form's list.
const endSessionsZone = "endsessions:%d"

type endedSessionsMsg struct{ err error }

// newEndSessionsForm is the form for sessions, of space when cleaning it up, else for sessions given.
func newEndSessionsForm(provider []string, space string, sessions []spaces.ProviderSession) *endSessionsForm {
	f := &endSessionsForm{th: look.New(true), provider: provider, sessions: sessions, space: space, checked: make([]bool, len(sessions))}
	for i, s := range sessions {
		f.checked[i] = space == "" || s.State == "exited" || s.State == "dead"
	}
	return f
}

func (f *endSessionsForm) Init() tea.Cmd { return nil }

// cleaning reports whether the form cleans up a space, with a box for each session.
func (f *endSessionsForm) cleaning() bool { return f.space != "" }

// toggle ticks or clears session i, when cleaning up, and says which it is now.
func (f *endSessionsForm) toggle(i int) {
	if f.cleaning() && i >= 0 && i < len(f.sessions) {
		f.checked[i] = !f.checked[i]
	}
}

// tickAll ticks every session.
func (f *endSessionsForm) tickAll() {
	for i := range f.checked {
		f.checked[i] = true
	}
}

// chosen are the ticked sessions.
func (f *endSessionsForm) chosen() []spaces.ProviderSession {
	var out []spaces.ProviderSession
	for i, s := range f.sessions {
		if f.checked[i] {
			out = append(out, s)
		}
	}
	return out
}

// end kills or removes the ticked sessions and leaves.
func (f *endSessionsForm) end() tea.Cmd {
	f.busy, f.err = true, nil
	provider, chosen := f.provider, f.chosen()
	return func() tea.Msg { return endedSessionsMsg{endSessions(provider, chosen)} }
}

// endSessions kills the running sessions of chosen and removes the ended ones. One failing does not stop
// the rest, and says which it was when there are several.
func endSessions(provider []string, chosen []spaces.ProviderSession) error {
	var errs []error
	for _, s := range chosen {
		end := spaces.KillSession
		if s.Ended() {
			end = spaces.RemoveSession
		}
		if err := end(provider, s); err != nil {
			if len(chosen) > 1 {
				err = fmt.Errorf("%s: %w", s.Name, err)
			}
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func (f *endSessionsForm) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		f.width = msg.Width
	case endedSessionsMsg:
		f.busy, f.err = false, msg.err
		if msg.err == nil {
			return f, tea.Quit
		}
	case tea.KeyPressMsg:
		if cmd, ok := runOn(f, msg, theEndSessionsForm.Target(endSessionsSubject{})); ok {
			return f, cmd
		}
	case tea.MouseClickMsg:
		if cmd, ok := f.buttons.click(f, f.zones, msg); ok {
			return f, cmd
		}
		for i := range f.sessions {
			for _, z := range f.zones[fmt.Sprintf(endSessionsZone, i)] {
				if msg.Button == tea.MouseLeft && z.Contains(msg.X, msg.Y) && !f.busy {
					f.cursor = i
					f.toggle(i)
				}
			}
		}
	}
	return f, nil
}

func (f *endSessionsForm) View() tea.View {
	frame, zones := look.Scan(f.render())
	f.zones = zones
	v := tea.NewView(frame)
	v.AltScreen = true
	v.MouseMode = tea.MouseModeCellMotion
	return v
}

// verb is what ending s does to it: kill a running one, remove an ended one.
func verb(s spaces.ProviderSession) string {
	if s.Ended() {
		return "Remove"
	}
	return "Kill"
}

// title is the form's heading: what it does to the one session given, how many it ends, or the space
// cleaned up.
func (f *endSessionsForm) title() string {
	switch {
	case f.cleaning():
		return "Clean up the sessions of " + f.space
	case len(f.sessions) == 1:
		return verb(f.sessions[0]) + " session " + f.sessions[0].Name
	}
	return fmt.Sprintf("End %d sessions", len(f.sessions))
}

// fate is what happens to s when ticked, said after it: a busy one says what it runs, which goes.
func fate(s spaces.ProviderSession, ticked bool) look.Span {
	switch {
	case !ticked:
		return look.Muted("kept")
	case s.Ended():
		return look.Text("removed")
	case s.Busy:
		return look.Error(cmp.Or(s.Command, "a command") + " running, killed")
	}
	return look.Error("killed")
}

func (f *endSessionsForm) render() string {
	th, width := f.th, max(f.width-4, 20)
	line := func(spans ...look.Span) string { return th.Render(look.Plain, spans...) }
	lines := []string{line(look.Heading(f.title())), ""}
	text := func(s string) {
		for _, row := range look.Muted(s).Wrapped(width) {
			lines = append(lines, line(row))
		}
	}
	switch {
	case len(f.sessions) == 0:
		text("No session of " + f.space + " is detached.")
		lines = append(lines, "")
	case f.cleaning():
		text("Killing a running session ends it, and removing an ended one discards it: neither comes back.")
		lines = append(lines, "")
	}
	names := 0
	for _, s := range f.sessions {
		names = max(names, len(s.Name))
	}
	for i, s := range f.sessions {
		var spans []look.Span
		if f.cleaning() {
			spans = look.BoxOf(f.checked[i], false)
		}
		spans = append(spans, look.Text(s.Name), look.Default(strings.Repeat(" ", names-len(s.Name)+2)),
			look.Muted(spaces.StateWords(s)+"; "), fate(s, f.checked[i]))
		if what := cmp.Or(strings.TrimSpace(s.Title), strings.TrimSpace(s.Command)); what != "" && !(f.checked[i] && !s.Ended() && s.Busy && what == s.Command) {
			spans = append(spans, look.Muted("; "+what))
		}
		surface := look.Plain
		if f.cleaning() && i == f.cursor {
			surface = look.Cursor
		}
		lines = append(lines, th.DrawZone(fmt.Sprintf(endSessionsZone, i), surface, width, th.Cut(width, spans...)...))
	}
	lines = append(lines, "")
	switch {
	case f.busy:
		lines = append(lines, line(look.Busy("Ending...")), "")
	case f.err != nil:
		for _, l := range strings.Split(f.err.Error(), "\n") {
			for _, row := range look.Error(l).Wrapped(width) {
				lines = append(lines, line(row))
			}
		}
		lines = append(lines, "")
	}
	n := len(f.chosen())
	label := fmt.Sprintf("End %s", plural(n, "session"))
	if !f.cleaning() && len(f.sessions) == 1 {
		label = verb(f.sessions[0])
	}
	f.buttons = controls[*endSessionsForm]{}
	lines = append(lines, line(f.buttons.draw(f, theEndSessionsForm.Target(endSessionsSubject{}),
		control{action: actEndSessions, label: label, danger: true, off: !f.busy && n == 0},
		control{action: actEndSessionsCancel, label: "Cancel"})...))
	return boxed(lines)
}

// RunEndSessions runs the form that ends the provider's sessions ids, meant for a kitty popup; with yes
// it ends them without asking.
func RunEndSessions(ctx context.Context, cfg config.Config, ids []string, yes bool) error {
	sessions, err := spaces.FindSessions(cfg.Provider, ids)
	if err != nil {
		return err
	}
	if yes {
		return endSessions(cfg.Provider, sessions)
	}
	if err := runPopup(ctx, newEndSessionsForm(cfg.Provider, "", sessions)); err != nil {
		return fmt.Errorf("running the end session form: %w", err)
	}
	return nil
}

// RunCleanupSessions runs the form that cleans up the provider's sessions of space no window shows, meant
// for a kitty popup.
func RunCleanupSessions(ctx context.Context, cfg config.Config, space string) error {
	sessions, err := spaces.DetachedOf(cfg, space)
	if err != nil {
		return err
	}
	if err := runPopup(ctx, newEndSessionsForm(cfg.Provider, space, sessions)); err != nil {
		return fmt.Errorf("running the clean up form: %w", err)
	}
	return nil
}

func (f *endSessionsForm) theme() *look.Theme { return f.th }
func (f *endSessionsForm) working() bool      { return f.busy }
func (f *endSessionsForm) failed() bool       { return f.err != nil }
func (f *endSessionsForm) keys() ([]interact.Section, bool) {
	return interact.Help(f, theEndSessionsForm.Target(endSessionsSubject{})), false
}
