package tui

import (
	"context"
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	"github.com/chancez/kspaces/internal/config"
	"github.com/chancez/kspaces/internal/git"
	"github.com/chancez/kspaces/internal/kitty"
	"github.com/chancez/kspaces/internal/spaces"
	"github.com/chancez/kspaces/internal/tui/look"
)

// layoutForm chooses a configured layout, fills in its inputs and adds its tabs to a worktree's session,
// open or not, saying what they hold. It is meant for a kitty overlay, like the run form.
type layoutForm struct {
	th      *look.Theme
	cfg     config.Config
	space   spaces.Space
	wt      spaces.Worktree
	layouts choiceFields[config.Layout]
	// windows says whether the session has windows, without which the layout opens it.
	windows bool
	// focus is the index in fields of the field that has focus.
	focus int
	busy  bool
	err   error
	width int
	// zones and buttons are where the last frame drew its buttons, and what each runs, for a click.
	zones   map[string][]look.Zone
	buttons controls[*layoutForm]
}

type layoutOpenedMsg struct{ err error }

// newLayoutForm is the form for opening the layout called name in wt, a worktree of space, its inputs
// filled in from inputs. st says whether the session has windows.
func newLayoutForm(cfg config.Config, space spaces.Space, wt spaces.Worktree, st kitty.State, name string, inputs map[string]string) *layoutForm {
	th := look.New(true)
	_, windows := st.Sessions[wt.Session]
	f := &layoutForm{th: th, cfg: cfg, space: space, wt: wt, windows: windows,
		layouts: newLayoutFields(th, cfg.Layouts, false, name, inputs)}
	moveFocus(f.fields(), &f.focus, 0)
	return f
}

func (f *layoutForm) Init() tea.Cmd { return textinput.Blink }

// fields are the form's fields in order: the choice of layout, nil, with more than one, and the chosen
// layout's inputs.
func (f *layoutForm) fields() []*textinput.Model {
	var out []*textinput.Model
	if f.layouts.picks() {
		out = append(out, nil)
	}
	return append(out, f.layouts.fields()...)
}

// focused is the field typing goes to, nil for the choice of layout or when there are no fields.
func (f *layoutForm) focused() *textinput.Model {
	fields := f.fields()
	if len(fields) == 0 {
		return nil
	}
	return fields[min(f.focus, len(fields)-1)]
}

func (f *layoutForm) nextField() tea.Cmd { return moveFocus(f.fields(), &f.focus, 1) }
func (f *layoutForm) prevField() tea.Cmd { return moveFocus(f.fields(), &f.focus, -1) }

// onLayout says whether the choice of layout has focus.
func (f *layoutForm) onLayout() bool { return f.layouts.picks() && f.focused() == nil }

// cycleLayout chooses the next layout, or the one before, and focuses the choice.
func (f *layoutForm) cycleLayout(delta int) tea.Cmd {
	f.layouts.cycle(delta)
	f.focus = 0
	return moveFocus(f.fields(), &f.focus, 0)
}

// open opens the chosen layout and leaves, or says why it cannot.
func (f *layoutForm) open() tea.Cmd {
	l, _ := f.layouts.chosen()
	lr, err := spaces.NewLayoutRun(l, f.layouts.values())
	if err != nil {
		f.err = err
		return nil
	}
	f.busy, f.err = true, nil
	cfg, space, wt := f.cfg, f.space, f.wt
	return func() tea.Msg {
		// Read again: the session may have gained or lost windows since the form opened.
		st, err := kitty.LoadState(kitty.SelfWindowID())
		if err != nil {
			return layoutOpenedMsg{err}
		}
		return layoutOpenedMsg{spaces.OpenLayout(cfg, st, space, wt, lr)}
	}
}

func (f *layoutForm) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		f.width = msg.Width
		f.layouts.setWidth(max(f.width-4-labelWidth-1, 1))
		return f, nil
	case layoutOpenedMsg:
		f.busy, f.err = false, msg.err
		if msg.err == nil {
			return f, tea.Quit
		}
		return f, nil
	case tea.KeyPressMsg:
		if cmd, ok := runOn(f, msg, theLayoutForm.Target(layoutSubject{})); ok {
			return f, cmd
		}
	case tea.MouseClickMsg:
		if cmd, ok := f.buttons.click(f, f.zones, msg); ok {
			return f, cmd
		}
		if clickedChoice(f.zones, layoutZone, msg) {
			return f, f.cycleLayout(1)
		}
	}
	in := f.focused()
	if in == nil {
		return f, nil
	}
	var cmd tea.Cmd
	*in, cmd = in.Update(msg)
	return f, cmd
}

func (f *layoutForm) View() tea.View {
	frame, zones := look.Scan(f.render())
	f.zones = zones
	v := tea.NewView(frame)
	v.AltScreen = true
	v.MouseMode = tea.MouseModeCellMotion
	return v
}

// layoutTabs say what each of l's tabs holds, a line each: its panes' commands, or a shell.
func layoutTabs(l config.Layout) []string {
	var out []string
	for _, tab := range l.Tabs {
		var panes []string
		for _, p := range tab.Panes {
			switch {
			case p.Command != "":
				panes = append(panes, p.Command)
			case p.Run != nil:
				panes = append(panes, filepath.Base(p.Run[0]))
			default:
				panes = append(panes, "a shell")
			}
		}
		out = append(out, "A tab of "+strings.Join(panes, ", "))
	}
	return out
}

func (f *layoutForm) render() string {
	th, width := f.th, max(f.width-4, 20)
	line := func(spans ...look.Span) string { return th.Render(look.Plain, spans...) }
	muted := func(s string) string { return line(th.Cut(width, look.Muted(s))...) }
	lines := []string{line(look.Heading("Open a layout in " + f.wt.Session)), ""}
	if fields := f.layouts.lines(f.onLayout()); len(fields) > 0 {
		lines = append(append(lines, fields...), "")
	}
	l, _ := f.layouts.chosen()
	for _, s := range layoutTabs(l) {
		lines = append(lines, muted(s))
	}
	if f.windows {
		lines = append(lines, muted("Adds its tabs to "+f.wt.Session), "")
	} else {
		lines = append(lines, muted("Opens "+f.wt.Session+" with its tabs, as it has no windows"), "")
	}
	switch {
	case f.busy:
		lines = append(lines, line(look.Busy("Opening...")), "")
	case f.err != nil:
		for _, row := range look.Error(f.err.Error()).Wrapped(width) {
			lines = append(lines, line(row))
		}
		lines = append(lines, "")
	}
	f.buttons = controls[*layoutForm]{}
	spans := f.buttons.draw(f, theLayoutForm.Target(layoutSubject{}),
		control{action: actOpenLayoutNow, label: "Open"},
		control{action: actLayoutCancel, label: "Cancel"})
	if len(f.fields()) > 1 {
		spans = append(spans, look.Text("  "), look.Key("tab"), look.KeyDesc(" next field"))
	}
	if f.onLayout() {
		spans = append(spans, look.Text("  "), look.Key("left/right"), look.KeyDesc(" layout"))
	}
	return boxed(append(lines, line(spans...)))
}

// findLayout is the worktree the directory dir is in, its space, and the layout called name, with inputs
// it has.
func findLayout(cfg config.Config, dir, name string, inputs map[string]string) (spaces.Space, spaces.Worktree, config.Layout, error) {
	l, ok := cfg.Layout(name)
	if !ok {
		return spaces.Space{}, spaces.Worktree{}, config.Layout{}, fmt.Errorf("no layout %s in the config", name)
	}
	for k := range inputs {
		if !slices.ContainsFunc(l.Inputs, func(in config.Input) bool { return in.Name == k }) {
			return spaces.Space{}, spaces.Worktree{}, config.Layout{}, fmt.Errorf("%s has no input %s", name, k)
		}
	}
	top, err := git.TopLevel(dir)
	if err != nil {
		return spaces.Space{}, spaces.Worktree{}, config.Layout{}, err
	}
	space, wt, err := spaces.FindWorktree(cfg, top)
	return space, wt, l, err
}

// OpenLayoutForm runs the layout form for the layout called name in the worktree dir is in, its inputs
// filled in from inputs, meant for a kitty overlay, until the layout opens or the form is left.
func OpenLayoutForm(ctx context.Context, cfg config.Config, dir, name string, inputs map[string]string) error {
	space, wt, _, err := findLayout(cfg, dir, name, inputs)
	if err != nil {
		return err
	}
	st, err := kitty.LoadState(kitty.SelfWindowID())
	if err != nil {
		return err
	}
	if _, err := tea.NewProgram(newLayoutForm(cfg, space, wt, st, name, inputs), tea.WithContext(ctx)).Run(); err != nil {
		return fmt.Errorf("running the layout form: %w", err)
	}
	return nil
}

// OpenLayoutNow opens the layout called name in the worktree dir is in, with inputs, without asking. A
// required input that is not given, and has no default, is an error.
func OpenLayoutNow(cfg config.Config, dir, name string, inputs map[string]string) error {
	space, wt, l, err := findLayout(cfg, dir, name, inputs)
	if err != nil {
		return err
	}
	lr, err := spaces.NewLayoutRun(l, inputs)
	if err != nil {
		return err
	}
	st, err := kitty.LoadState(kitty.SelfWindowID())
	if err != nil {
		return err
	}
	return spaces.OpenLayout(cfg, st, space, wt, lr)
}
