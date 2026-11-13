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

// layoutForm chooses a configured layout, or a shell tab, fills in its inputs and adds its tabs to a
// worktree's session, open or not, saying what they hold. It is meant for a kitty popup, like the run form.
type layoutForm struct {
	th      *look.Theme
	cfg     config.Config
	space   spaces.Space
	wt      spaces.Worktree
	layouts choiceFields[config.Layout]
	// windows says whether the session has windows, without which the layout opens it.
	windows bool
	// focus is the index in fields of the field that has focus.
	focus  int
	busy   bool
	err    error
	width  int
	height int
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
		layouts: newLayoutFields(th, cfg.Layouts, true, name, inputs)}
	moveFocus(f.fields(), &f.focus, 0)
	return f
}

func (f *layoutForm) Init() tea.Cmd { return textinput.Blink }

// fields are the form's fields in order: the choice of layout, nil, with any layout, and the chosen
// layout's inputs.
func (f *layoutForm) fields() []field {
	var out []field
	if f.layouts.picks() {
		out = append(out, nil)
	}
	return append(out, f.layouts.fields()...)
}

// focused is the field typing goes to, nil for the choice of layout or when there are no fields.
func (f *layoutForm) focused() field {
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

// open opens the chosen layout, or the shell tab, and leaves, or says why it cannot.
func (f *layoutForm) open() tea.Cmd {
	l, layout := f.layouts.chosen()
	lr, err := spaces.NewLayoutRun(l, f.layouts.values())
	if layout && err != nil {
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
		if !layout {
			return layoutOpenedMsg{spaces.OpenShell(cfg, st, wt)}
		}
		return layoutOpenedMsg{spaces.OpenLayout(cfg, st, space, wt, lr)}
	}
}

func (f *layoutForm) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// While the list is open, keys and the mouse are its.
	if f.layouts.listOpen() && isInput(msg) {
		cmd, picked := f.layouts.updateList(msg, f.zones)
		if picked {
			return f, tea.Batch(cmd, f.cycleLayout(0))
		}
		return f, cmd
	}
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		f.width, f.height = msg.Width, msg.Height
		f.layouts.setWidth(max(f.width-4-labelWidth-1, 1))
		return f, nil
	case layoutOpenedMsg:
		f.busy, f.err = false, msg.err
		if msg.err == nil {
			return f, tea.Quit
		}
		return f, nil
	case tea.KeyPressMsg:
		if keptBy(f.focused(), msg) {
			break
		}
		if cmd, ok := runOn(f, msg, theLayoutForm.Target(layoutSubject{})); ok {
			return f, cmd
		}
	case tea.MouseClickMsg:
		if cmd, ok := f.buttons.click(f, f.zones, msg); ok {
			return f, cmd
		}
		if clickedChoice(f.zones, layoutZone, msg) {
			f.layouts.openList()
			return f, f.cycleLayout(0)
		}
		if i, ok := clickedField(f.fields(), f.zones, msg); ok {
			f.focus = i
			return f, moveFocus(f.fields(), &f.focus, 0)
		}
	}
	in := f.focused()
	if in == nil {
		return f, nil
	}
	return f, in.update(msg)
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
	at := len(lines)
	if fields := f.layouts.lines(f.onLayout(), width); len(fields) > 0 {
		lines = append(append(lines, fields...), "")
	}
	var tabs []string
	if l, ok := f.layouts.chosen(); ok {
		tabs = layoutTabs(l)
	}
	for _, s := range tabs {
		lines = append(lines, muted(s))
	}
	_, chosen := f.layouts.chosen()
	switch {
	case !chosen && f.windows:
		lines = append(lines, muted("Adds one tab, a shell, to "+f.wt.Session), "")
	case !chosen:
		lines = append(lines, muted("Opens "+f.wt.Session+" with one tab, a shell, as it has no windows"), "")
	case f.windows:
		lines = append(lines, muted("Adds its tabs to "+f.wt.Session), "")
	default:
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
	if f.layouts.listOpen() {
		return boxed(append(f.layouts.drawList(lines, at, width, f.height), line(listHints()...)))
	}
	spans := f.buttons.draw(f, theLayoutForm.Target(layoutSubject{}),
		control{action: actOpenLayoutNow, label: "Open"},
		control{action: actLayoutCancel, label: "Cancel"})
	if len(f.fields()) > 1 {
		spans = append(spans, look.Text("  "), look.Key("tab"), look.KeyDesc(" next field"))
	}
	if f.onLayout() {
		spans = append(spans, choiceHints("layout")...)
	}
	spans = append(spans, newlineHints(f.focused())...)
	return boxed(append(lines, line(spans...)))
}

// findLayout is the worktree the directory dir is in, its space, the config as the space sees it, and its
// layout called name, with inputs it has.
func findLayout(global config.Config, dir, name string, inputs map[string]string) (config.Config, spaces.Space, spaces.Worktree, config.Layout, error) {
	top, err := git.TopLevel(dir)
	if err != nil {
		return config.Config{}, spaces.Space{}, spaces.Worktree{}, config.Layout{}, err
	}
	space, wt, err := spaces.FindWorktree(global, top)
	if err != nil {
		return config.Config{}, spaces.Space{}, spaces.Worktree{}, config.Layout{}, err
	}
	cfg := space.ConfigOf(global)
	l, ok := cfg.Layout(name)
	if !ok {
		return config.Config{}, spaces.Space{}, spaces.Worktree{}, config.Layout{}, fmt.Errorf("no layout %s in the config", name)
	}
	for k := range inputs {
		if !slices.ContainsFunc(l.Inputs, func(in config.Input) bool { return in.Name == k }) {
			return config.Config{}, spaces.Space{}, spaces.Worktree{}, config.Layout{}, fmt.Errorf("%s has no input %s", name, k)
		}
	}
	return cfg, space, wt, l, nil
}

// OpenLayoutForm runs the layout form for the layout called name in the worktree dir is in, its inputs
// filled in from inputs, meant for a kitty popup, until the layout opens or the form is left.
func OpenLayoutForm(ctx context.Context, global config.Config, dir, name string, inputs map[string]string) error {
	cfg, space, wt, _, err := findLayout(global, dir, name, inputs)
	if err != nil {
		return err
	}
	st, err := kitty.LoadState(kitty.SelfWindowID())
	if err != nil {
		return err
	}
	if err := runPopup(ctx, newLayoutForm(cfg, space, wt, st, name, inputs)); err != nil {
		return fmt.Errorf("running the layout form: %w", err)
	}
	return nil
}

// OpenLayoutNow opens the layout called name in the worktree dir is in, with inputs, without asking. A
// required input that is not given, and has no default, is an error.
func OpenLayoutNow(global config.Config, dir, name string, inputs map[string]string) error {
	cfg, space, wt, l, err := findLayout(global, dir, name, inputs)
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
