package tui

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	"github.com/chancez/kspaces/internal/config"
	"github.com/chancez/kspaces/internal/spaces"
	"github.com/chancez/kspaces/internal/tui/interact"
	"github.com/chancez/kspaces/internal/tui/look"
)

// moveSessionsForm moves the provider's sessions to another space: one of its spaces, chosen from those the
// name typed matches, or a new one, which the provider creates. It is meant for a kitty popup, like the add
// space form. This is how sessions that all landed in default are filed where they belong.
type moveSessionsForm struct {
	th       *look.Theme
	provider []string
	sessions []spaces.ProviderSession
	// spaces are the provider's, the sessions' own one left out when they share it.
	spaces []string
	space  lineField
	// sel is the match chosen with up and down, -1 for the name as typed.
	sel   int
	busy  bool
	err   error
	width int
	// zones and buttons are where the last frame drew its buttons, and what each runs, for a click.
	zones   map[string][]look.Zone
	buttons controls[*moveSessionsForm]
}

// moveMatchesShown is how many of the spaces the name matches show under it.
const moveMatchesShown = 5

type movedMsg struct{ err error }

// newMoveSessionsForm is the form moving sessions to one of all, the provider's spaces, or a new one.
func newMoveSessionsForm(provider []string, sessions []spaces.ProviderSession, all []string) *moveSessionsForm {
	th := look.New(true)
	f := &moveSessionsForm{th: th, provider: provider, sessions: sessions, space: newLineField(th, "a space, or a new one's name"), sel: -1}
	from := ""
	for i, s := range sessions {
		if i == 0 {
			from = s.SpaceName()
		} else if s.SpaceName() != from {
			from = ""
		}
	}
	for _, name := range all {
		if name != from {
			f.spaces = append(f.spaces, name)
		}
	}
	f.space.Focus()
	return f
}

func (f *moveSessionsForm) Init() tea.Cmd { return textinput.Blink }

// matches are the spaces with the name typed in them, ignoring case, at most moveMatchesShown.
func (f *moveSessionsForm) matches() []string {
	q := strings.ToLower(strings.TrimSpace(f.space.Value()))
	var out []string
	for _, name := range f.spaces {
		if strings.Contains(strings.ToLower(name), q) && len(out) < moveMatchesShown {
			out = append(out, name)
		}
	}
	return out
}

// target is the space the sessions go to: the match chosen, else the name typed.
func (f *moveSessionsForm) target() string {
	if m := f.matches(); f.sel >= 0 && f.sel < len(m) {
		return m[f.sel]
	}
	return strings.TrimSpace(f.space.Value())
}

// choose moves the choice among the matches by delta, from the name as typed, -1, to the last match.
func (f *moveSessionsForm) choose(delta int) {
	f.sel = min(max(f.sel+delta, -1), len(f.matches())-1)
}

func (f *moveSessionsForm) move() tea.Cmd {
	f.busy, f.err = true, nil
	provider, space, sessions := f.provider, f.target(), f.sessions
	return func() tea.Msg { return movedMsg{spaces.MoveSessions(provider, space, sessions...)} }
}

func (f *moveSessionsForm) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		f.width = msg.Width
		f.space.SetWidth(max(f.width-4-labelWidth-1, 1))
		return f, nil
	case movedMsg:
		f.busy, f.err = false, msg.err
		if msg.err == nil {
			return f, tea.Quit
		}
		return f, nil
	case tea.KeyPressMsg:
		if !f.busy && keptBy(&f.space, msg) {
			break
		}
		if cmd, ok := runOn(f, msg, theMoveForm.Target(moveSubject{})); ok {
			return f, cmd
		}
	case tea.MouseClickMsg:
		if cmd, ok := f.buttons.click(f, f.zones, msg); ok {
			return f, cmd
		}
	}
	if f.busy {
		return f, nil
	}
	before := f.space.Value()
	cmd := f.space.update(msg)
	if f.space.Value() != before {
		f.sel = -1
	}
	return f, cmd
}

func (f *moveSessionsForm) View() tea.View {
	frame, zones := look.Scan(f.render())
	f.zones = zones
	v := tea.NewView(frame)
	v.AltScreen = true
	v.MouseMode = tea.MouseModeCellMotion
	return v
}

func (f *moveSessionsForm) render() string {
	th, width := f.th, max(f.width-4, 20)
	line := func(spans ...look.Span) string { return th.Render(look.Plain, spans...) }
	title := fmt.Sprintf("Move %d sessions", len(f.sessions))
	if len(f.sessions) == 1 {
		title = "Move session " + f.sessions[0].Name
	}
	lines := []string{line(look.Heading(title)), ""}
	if len(f.sessions) > 1 {
		for _, s := range f.sessions {
			lines = append(lines, line(th.Cut(width, look.Text(s.Name), look.Muted(" in "+s.SpaceName()))...))
		}
		lines = append(lines, "")
	}
	lines = append(lines, fieldLines(th, "to", &f.space, width)...)
	indent := look.Default(strings.Repeat(" ", labelWidth))
	for i, m := range f.matches() {
		surface := look.Plain
		if i == f.sel {
			surface = look.Cursor
		}
		lines = append(lines, th.Draw(surface, width, th.Cut(width, indent, look.Text(m))...))
	}
	lines = append(lines, "")
	target := f.target()
	switch {
	case target == "":
		lines = append(lines, line(look.Muted("Name the space to move to")))
	case slices.Contains(f.spaces, target):
		lines = append(lines, line(th.Cut(width, look.Muted("Moves to "+target))...))
	default:
		lines = append(lines, line(th.Cut(width, look.Muted("Moves to "+target+", a new space"))...))
	}
	lines = append(lines, "")
	switch {
	case f.busy:
		lines = append(lines, line(look.Busy("Moving...")), "")
	case f.err != nil:
		for _, l := range strings.Split(f.err.Error(), "\n") {
			for _, row := range look.Error(l).Wrapped(width) {
				lines = append(lines, line(row))
			}
		}
		lines = append(lines, "")
	}
	f.buttons = controls[*moveSessionsForm]{}
	lines = append(lines, line(f.buttons.draw(f, theMoveForm.Target(moveSubject{}),
		control{action: actMove, label: "Move", off: !f.busy && target == ""},
		control{action: actMoveCancel, label: "Cancel"})...))
	return boxed(lines)
}

func (f *moveSessionsForm) theme() *look.Theme { return f.th }
func (f *moveSessionsForm) working() bool      { return f.busy }
func (f *moveSessionsForm) failed() bool       { return f.err != nil }
func (f *moveSessionsForm) keys() ([]interact.Section, bool) {
	return interact.Help(f, theMoveForm.Target(moveSubject{})), true
}

// RunMoveSessions runs the form that moves the provider's sessions ids to another space, meant for a kitty
// popup; with to, it moves them there without asking.
func RunMoveSessions(ctx context.Context, cfg config.Config, ids []string, to string) error {
	st, err := spaces.ListNow(cfg.Provider)
	if err != nil {
		return err
	}
	sessions, err := st.Find(ids)
	if err != nil {
		return err
	}
	if to != "" {
		return spaces.MoveSessions(cfg.Provider, to, sessions...)
	}
	if err := runPopup(ctx, newMoveSessionsForm(cfg.Provider, sessions, st.Spaces)); err != nil {
		return fmt.Errorf("running the move form: %w", err)
	}
	return nil
}
