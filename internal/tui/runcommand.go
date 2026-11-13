package tui

import (
	"context"
	"errors"
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

// runTarget is a worktree to run a command in, its space, and the config as the space sees it, where the
// command is looked up.
type runTarget struct {
	cfg   config.Config
	space spaces.Space
	wt    spaces.Worktree
}

// runForm chooses a configured command, fills in its inputs and runs it in a worktree's session, saying
// where it opens, or in each of several worktrees' sessions. It is meant for a kitty popup, like the new
// worktree form.
type runForm struct {
	th      *look.Theme
	targets []runTarget
	cmds    choiceFields[config.Command]
	// open overrides where the command opens, empty for where it is configured to.
	open string
	// windows says whether the one worktree's session has windows, without which the command is its first
	// tab.
	windows bool
	// focus is the index in fields of the field that has focus.
	focus  int
	busy   bool
	err    error
	width  int
	height int
	// zones and buttons are where the last frame drew its buttons, and what each runs, for a click.
	zones   map[string][]look.Zone
	buttons controls[*runForm]
}

type ranMsg struct{ err error }

// newRunForm is the form for running the command called name in targets, its inputs filled in from
// inputs, opening where open says, or where configured when empty. It offers the commands every target's
// config has, in the first's order. st says whether the session has windows.
func newRunForm(targets []runTarget, st kitty.State, name string, inputs map[string]string, open string) *runForm {
	th := look.New(true)
	_, windows := st.Sessions[targets[0].wt.Session]
	f := &runForm{th: th, targets: targets, open: open, windows: windows,
		cmds: newCommandFields(th, commonCommands(targets), false, name, inputs)}
	moveFocus(f.fields(), &f.focus, 0)
	return f
}

// commonCommands are the commands of the first target's config that every target's config has one of by
// name.
func commonCommands(targets []runTarget) []config.Command {
	var out []config.Command
	for _, c := range targets[0].cfg.Commands {
		if !slices.ContainsFunc(targets[1:], func(t runTarget) bool { _, ok := t.cfg.Command(c.Name); return !ok }) {
			out = append(out, c)
		}
	}
	return out
}

func (f *runForm) Init() tea.Cmd { return textinput.Blink }

// fields are the form's fields in order: the choice of command, nil, with more than one, and the chosen
// command's inputs.
func (f *runForm) fields() []field {
	var out []field
	if f.cmds.picks() {
		out = append(out, nil)
	}
	return append(out, f.cmds.fields()...)
}

// focused is the field typing goes to, nil for the choice of command or when there are no fields.
func (f *runForm) focused() field {
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

// opensWhere says where the chosen command opens: in the one worktree's session, or in each.
func (f *runForm) opensWhere() string {
	if len(f.targets) > 1 {
		return opensBeside(f.opens(), "each session") + ", or as the first tab of one with no windows"
	}
	s := f.targets[0].wt.Session
	if !f.windows {
		return "Opens as the first tab of " + s + ", which has no windows"
	}
	return opensBeside(f.opens(), s)
}

// opensBeside says where a command opening at open opens in a session with windows, s naming it.
func opensBeside(open, s string) string {
	switch open {
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

// run runs the chosen command and leaves, or says why it cannot: its inputs are checked as the first
// target's command has them before running anything.
func (f *runForm) run() tea.Cmd {
	cmd, _ := f.cmds.chosen()
	values := f.cmds.values()
	if _, err := spaces.NewRun(cmd, values); err != nil {
		f.err = err
		return nil
	}
	f.busy, f.err = true, nil
	targets, open := f.targets, f.open
	return func() tea.Msg { return ranMsg{runIn(targets, cmd.Name, values, open)} }
}

// runIn runs the command called name in each of targets, as its own config has it, with the inputs of
// values it has, opening where open says, or where configured when empty. In several it stays in this
// window rather than switching to each session, see spaces.Run.Stay; one failing does not stop the rest,
// and the error says which failed and why, one line each.
func runIn(targets []runTarget, name string, values map[string]string, open string) error {
	// Read now: the sessions may have gained or lost windows since a form opened.
	st, err := kitty.LoadState(kitty.SelfWindowID())
	if err != nil {
		return err
	}
	if len(targets) == 1 {
		return targets[0].run(st, name, values, open, false)
	}
	var errs []error
	for _, t := range targets {
		if err := t.run(st, name, values, open, true); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", t.wt.Session, err))
		}
	}
	return errors.Join(errs...)
}

// run runs t's command called name with the inputs of values it has, see runIn.
func (t runTarget) run(st kitty.State, name string, values map[string]string, open string, stay bool) error {
	cmd, ok := t.cfg.Command(name)
	if !ok {
		return fmt.Errorf("no command %s in the config", name)
	}
	inputs := map[string]string{}
	for _, in := range cmd.Inputs {
		if v, ok := values[in.Name]; ok {
			inputs[in.Name] = v
		}
	}
	r, err := spaces.NewRun(cmd, inputs)
	if err != nil {
		return err
	}
	if open != "" {
		r.Open = open
	}
	r.Stay = stay
	return spaces.RunCommand(t.cfg, st, t.space, t.wt, r)
}

func (f *runForm) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// While the list is open, keys and the mouse are its.
	if f.cmds.listOpen() && isInput(msg) {
		cmd, picked := f.cmds.updateList(msg, f.zones)
		if picked {
			return f, tea.Batch(cmd, f.cycleCommand(0))
		}
		return f, cmd
	}
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		f.width, f.height = msg.Width, msg.Height
		f.cmds.setWidth(max(f.width-4-labelWidth-1, 1))
		return f, nil
	case ranMsg:
		f.busy, f.err = false, msg.err
		if msg.err == nil {
			return f, tea.Quit
		}
		return f, nil
	case tea.KeyPressMsg:
		if keptBy(f.focused(), msg) {
			break
		}
		if cmd, ok := runOn(f, msg, theRunForm.Target(runSubject{})); ok {
			return f, cmd
		}
	case tea.MouseClickMsg:
		if cmd, ok := f.buttons.click(f, f.zones, msg); ok {
			return f, cmd
		}
		if clickedChoice(f.zones, commandZone, msg) {
			f.cmds.openList()
			return f, f.cycleCommand(0)
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
	var lines []string
	if len(f.targets) == 1 {
		lines = []string{line(look.Heading("Run in " + f.targets[0].wt.Session)), ""}
	} else {
		lines = []string{line(look.Heading(fmt.Sprintf("Run in %d worktrees", len(f.targets)))), ""}
		for _, t := range f.targets {
			lines = append(lines, line(th.Cut(width, look.Text(t.wt.Session))...))
		}
		lines = append(lines, "")
	}
	at := len(lines)
	if fields := f.cmds.lines(f.onCommand(), width); len(fields) > 0 {
		lines = append(append(lines, fields...), "")
	}
	cmd, _ := f.cmds.chosen()
	lines = append(lines, muted("Runs "+cmd.Name), muted(f.opensWhere()), "")
	switch {
	case f.busy:
		lines = append(lines, line(look.Busy("Running...")), "")
	case f.err != nil:
		for _, l := range strings.Split(f.err.Error(), "\n") {
			for _, row := range look.Error(l).Wrapped(width) {
				lines = append(lines, line(row))
			}
		}
		lines = append(lines, "")
	}
	f.buttons = controls[*runForm]{}
	if f.cmds.listOpen() {
		return boxed(append(f.cmds.drawList(lines, at, width, f.height), line(listHints()...)))
	}
	spans := f.buttons.draw(f, theRunForm.Target(runSubject{}),
		control{action: actRun, label: "Run"},
		control{action: actRunCancel, label: "Cancel"})
	if len(f.fields()) > 1 {
		spans = append(spans, look.Text("  "), look.Key("tab"), look.KeyDesc(" next field"))
	}
	if f.onCommand() {
		spans = append(spans, choiceHints("command")...)
	}
	spans = append(spans, newlineHints(f.focused())...)
	return boxed(append(lines, line(spans...)))
}

// findRun is the worktree each directory of dirs is in, its space and the config as the space sees it,
// and the command called name to run in them, with inputs it has, opening where open says, one of
// config.Opens, or where configured when empty. In several worktrees the command is one they all have;
// with pick, the first they all have stands in for name when not every one has it, as the sidebar asks
// for the first worktree's first command.
func findRun(global config.Config, dirs []string, name string, inputs map[string]string, open string, pick bool) ([]runTarget, config.Command, error) {
	if open != "" && !slices.Contains(config.Opens, open) {
		return nil, config.Command{}, fmt.Errorf("--open is %q, it must be one of %s", open, strings.Join(config.Opens, ", "))
	}
	var targets []runTarget
	for _, dir := range dirs {
		top, err := git.TopLevel(dir)
		if err != nil {
			return nil, config.Command{}, err
		}
		space, wt, err := spaces.FindWorktree(global, top)
		if err != nil {
			return nil, config.Command{}, err
		}
		targets = append(targets, runTarget{cfg: space.ConfigOf(global), space: space, wt: wt})
	}
	cmds := commonCommands(targets)
	i := slices.IndexFunc(cmds, func(c config.Command) bool { return c.Name == name })
	switch {
	case i < 0 && len(targets) == 1:
		return nil, config.Command{}, fmt.Errorf("no command %s in the config", name)
	case len(cmds) == 0:
		return nil, config.Command{}, fmt.Errorf("the worktrees have no command in common")
	case i < 0 && !pick:
		return nil, config.Command{}, fmt.Errorf("not every worktree has command %s", name)
	case i < 0:
		i = 0
	}
	cmd := cmds[i]
	for k := range inputs {
		if !slices.ContainsFunc(cmd.Inputs, func(in config.Input) bool { return in.Name == k }) {
			return nil, config.Command{}, fmt.Errorf("%s has no input %s", cmd.Name, k)
		}
	}
	return targets, cmd, nil
}

// RunCommandForm runs the run form for the command called name in the worktrees dirs are in, its inputs
// filled in from inputs, meant for a kitty popup, until the command runs or the form is left. open
// overrides where it opens when not empty.
func RunCommandForm(ctx context.Context, global config.Config, dirs []string, name string, inputs map[string]string, open string) error {
	targets, cmd, err := findRun(global, dirs, name, inputs, open, true)
	if err != nil {
		return err
	}
	st, err := kitty.LoadState(kitty.SelfWindowID())
	if err != nil {
		return err
	}
	if err := runPopup(ctx, newRunForm(targets, st, cmd.Name, inputs, open)); err != nil {
		return fmt.Errorf("running the run form: %w", err)
	}
	return nil
}

// RunCommandNow runs the command called name in the worktrees dirs are in, with inputs, without asking. A
// required input that is not given, and has no default, is an error.
func RunCommandNow(global config.Config, dirs []string, name string, inputs map[string]string, open string) error {
	targets, cmd, err := findRun(global, dirs, name, inputs, open, false)
	if err != nil {
		return err
	}
	if _, err := spaces.NewRun(cmd, inputs); err != nil {
		return err
	}
	return runIn(targets, name, inputs, open)
}
