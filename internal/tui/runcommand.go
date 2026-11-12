package tui

import (
	"context"
	"fmt"
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

// runForm chooses a configured command, fills in its inputs and runs it in a worktree's session, saying
// where it opens. It is meant for a kitty overlay, like the new worktree form.
type runForm struct {
	th    *look.Theme
	cfg   config.Config
	space spaces.Space
	wt    spaces.Worktree
	cmds  choiceFields[config.Command]
	// open overrides where the command opens, empty for where it is configured to.
	open string
	// windows says whether the session has windows, without which the command is its first tab.
	windows bool
	// focus is the index in fields of the field that has focus.
	focus int
	busy  bool
	err   error
	width int
	// zones and buttons are where the last frame drew its buttons, and what each runs, for a click.
	zones   map[string][]look.Zone
	buttons controls[*runForm]
}

type ranMsg struct{ err error }

// newRunForm is the form for running the command called name in wt, a worktree of space, its inputs
// filled in from inputs, opening where open says, or where configured when empty. st says whether the
// session has windows.
func newRunForm(cfg config.Config, space spaces.Space, wt spaces.Worktree, st kitty.State, name string, inputs map[string]string, open string) *runForm {
	th := look.New(true)
	_, windows := st.Sessions[wt.Session]
	f := &runForm{th: th, cfg: cfg, space: space, wt: wt, open: open, windows: windows,
		cmds: newCommandFields(th, cfg.Commands, false, name, inputs)}
	moveFocus(f.fields(), &f.focus, 0)
	return f
}

func (f *runForm) Init() tea.Cmd { return textinput.Blink }

// fields are the form's fields in order: the choice of command, nil, with more than one, and the chosen
// command's inputs.
func (f *runForm) fields() []*textinput.Model {
	var out []*textinput.Model
	if f.cmds.picks() {
		out = append(out, nil)
	}
	return append(out, f.cmds.fields()...)
}

// focused is the field typing goes to, nil for the choice of command or when there are no fields.
func (f *runForm) focused() *textinput.Model {
	fields := f.fields()
	if len(fields) == 0 {
		return nil
	}
	return fields[min(f.focus, len(fields)-1)]
}

func (f *runForm) nextField() tea.Cmd { return moveFocus(f.fields(), &f.focus, 1) }
func (f *runForm) prevField() tea.Cmd { return moveFocus(f.fields(), &f.focus, -1) }

// onCommand says whether the choice of command has focus.
func (f *runForm) onCommand() bool { return f.cmds.picks() && f.focused() == nil }

// cycleCommand chooses the next command, or the one before, and focuses the choice.
func (f *runForm) cycleCommand(delta int) tea.Cmd {
	f.cmds.cycle(delta)
	f.focus = 0
	return moveFocus(f.fields(), &f.focus, 0)
}

// opens is where the chosen command opens.
func (f *runForm) opens() string {
	if f.open != "" {
		return f.open
	}
	cmd, _ := f.cmds.chosen()
	return cmd.Open
}

// opensWhere says where the chosen command opens.
func (f *runForm) opensWhere() string {
	s := f.wt.Session
	if !f.windows {
		return "Opens as the first tab of " + s + ", which has no windows"
	}
	switch f.opens() {
	case "split":
		return "Opens in a split beside " + s + "'s window"
	case "hsplit":
		return "Opens in a horizontal split beside " + s + "'s window"
	case "vsplit":
		return "Opens in a vertical split beside " + s + "'s window"
	case "overlay":
		return "Opens over " + s + "'s window, closing back to it"
	case "window":
		return "Opens in a new OS window in " + s
	}
	return "Opens in a new tab in " + s
}

// run runs the chosen command and leaves, or says why it cannot.
func (f *runForm) run() tea.Cmd {
	cmd, _ := f.cmds.chosen()
	r, err := spaces.NewRun(cmd, f.cmds.values())
	if err != nil {
		f.err = err
		return nil
	}
	if f.open != "" {
		r.Open = f.open
	}
	f.busy, f.err = true, nil
	cfg, space, wt := f.cfg, f.space, f.wt
	return func() tea.Msg {
		// Read again: the session may have gained or lost windows since the form opened.
		st, err := kitty.LoadState(kitty.SelfWindowID())
		if err != nil {
			return ranMsg{err}
		}
		return ranMsg{spaces.RunCommand(cfg, st, space, wt, r)}
	}
}

func (f *runForm) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		f.width = msg.Width
		f.cmds.setWidth(max(f.width-4-labelWidth-1, 1))
		return f, nil
	case ranMsg:
		f.busy, f.err = false, msg.err
		if msg.err == nil {
			return f, tea.Quit
		}
		return f, nil
	case tea.KeyPressMsg:
		if cmd, ok := runOn(f, msg, theRunForm.Target(runSubject{})); ok {
			return f, cmd
		}
	case tea.MouseClickMsg:
		if cmd, ok := f.buttons.click(f, f.zones, msg); ok {
			return f, cmd
		}
		if clickedChoice(f.zones, commandZone, msg) {
			return f, f.cycleCommand(1)
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

func (f *runForm) View() tea.View {
	frame, zones := look.Scan(f.render())
	f.zones = zones
	v := tea.NewView(frame)
	v.AltScreen = true
	v.MouseMode = tea.MouseModeCellMotion
	return v
}

func (f *runForm) render() string {
	th, width := f.th, max(f.width-4, 20)
	line := func(spans ...look.Span) string { return th.Render(look.Plain, spans...) }
	muted := func(s string) string { return line(th.Cut(width, look.Muted(s))...) }
	lines := []string{line(look.Heading("Run in " + f.wt.Session)), ""}
	if fields := f.cmds.lines(f.onCommand()); len(fields) > 0 {
		lines = append(append(lines, fields...), "")
	}
	cmd, _ := f.cmds.chosen()
	lines = append(lines, muted("Runs "+cmd.Name), muted(f.opensWhere()), "")
	switch {
	case f.busy:
		lines = append(lines, line(look.Busy("Running...")), "")
	case f.err != nil:
		for _, row := range look.Error(f.err.Error()).Wrapped(width) {
			lines = append(lines, line(row))
		}
		lines = append(lines, "")
	}
	f.buttons = controls[*runForm]{}
	spans := f.buttons.draw(f, theRunForm.Target(runSubject{}),
		control{action: actRun, label: "Run"},
		control{action: actRunCancel, label: "Cancel"})
	if len(f.fields()) > 1 {
		spans = append(spans, look.Text("  "), look.Key("tab"), look.KeyDesc(" next field"))
	}
	if f.onCommand() {
		spans = append(spans, look.Text("  "), look.Key("left/right"), look.KeyDesc(" command"))
	}
	return boxed(append(lines, line(spans...)))
}

// findRun is the worktree the directory dir is in, its space, and the command called name, with inputs
// it has, opening where open says, one of config.Opens, or where configured when empty.
func findRun(cfg config.Config, dir, name string, inputs map[string]string, open string) (spaces.Space, spaces.Worktree, config.Command, error) {
	cmd, ok := cfg.Command(name)
	switch {
	case !ok:
		return spaces.Space{}, spaces.Worktree{}, config.Command{}, fmt.Errorf("no command %s in the config", name)
	case open != "" && !slices.Contains(config.Opens, open):
		return spaces.Space{}, spaces.Worktree{}, config.Command{}, fmt.Errorf("--open is %q, it must be one of %s", open, strings.Join(config.Opens, ", "))
	}
	for k := range inputs {
		if !slices.ContainsFunc(cmd.Inputs, func(in config.Input) bool { return in.Name == k }) {
			return spaces.Space{}, spaces.Worktree{}, config.Command{}, fmt.Errorf("%s has no input %s", name, k)
		}
	}
	top, err := git.TopLevel(dir)
	if err != nil {
		return spaces.Space{}, spaces.Worktree{}, config.Command{}, err
	}
	space, wt, err := spaces.FindWorktree(cfg, top)
	return space, wt, cmd, err
}

// RunCommandForm runs the run form for the command called name in the worktree dir is in, its inputs
// filled in from inputs, meant for a kitty overlay, until the command runs or the form is left. open
// overrides where it opens when not empty.
func RunCommandForm(ctx context.Context, cfg config.Config, dir, name string, inputs map[string]string, open string) error {
	space, wt, _, err := findRun(cfg, dir, name, inputs, open)
	if err != nil {
		return err
	}
	st, err := kitty.LoadState(kitty.SelfWindowID())
	if err != nil {
		return err
	}
	if _, err := tea.NewProgram(newRunForm(cfg, space, wt, st, name, inputs, open), tea.WithContext(ctx)).Run(); err != nil {
		return fmt.Errorf("running the run form: %w", err)
	}
	return nil
}

// RunCommandNow runs the command called name in the worktree dir is in, with inputs, without asking. A
// required input that is not given, and has no default, is an error.
func RunCommandNow(cfg config.Config, dir, name string, inputs map[string]string, open string) error {
	space, wt, cmd, err := findRun(cfg, dir, name, inputs, open)
	if err != nil {
		return err
	}
	r, err := spaces.NewRun(cmd, inputs)
	if err != nil {
		return err
	}
	if open != "" {
		r.Open = open
	}
	st, err := kitty.LoadState(kitty.SelfWindowID())
	if err != nil {
		return err
	}
	return spaces.RunCommand(cfg, st, space, wt, r)
}
