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

// saveLayoutForm saves a session's tabs, as kitty has them, as a layout in the config: a name, this tab or
// every tab, and for each pane what the layout runs there, prefilled from what runs there now. It is meant
// for a kitty overlay, like the run form.
type saveLayoutForm struct {
	th   *look.Theme
	cfg  config.Config
	wt   spaces.Worktree
	tabs []spaces.CapturedTab
	// this is the index in tabs of this tab, the one saved alone.
	this  int
	name  textinput.Model
	scope choiceFields[string]
	// panes choose what each pane of each tab runs, by tab and pane.
	panes [][]choiceFields[paneChoice]
	// focus is the index in fields of the field that has focus.
	focus int
	// confirm asks to replace the layout of the name, which exists.
	confirm bool
	busy    bool
	err     error
	width   int
	height  int
	// zones and buttons are where the last frame drew its buttons, and what each runs, for a click.
	zones   map[string][]look.Zone
	buttons controls[*saveLayoutForm]
}

// paneChoice is what a saved pane can run: a configured command by name, its command line as run, or the
// shell.
type paneChoice struct {
	command string
	run     bool
}

const (
	runChoice   = "run"
	shellChoice = "shell"
	// The scopes.
	thisTab   = "this tab"
	everyTab  = "every tab"
	scopeZone = "save"
)

func (c paneChoice) name() string {
	switch {
	case c.command != "":
		return c.command
	case c.run:
		return runChoice
	}
	return shellChoice
}

type layoutSavedMsg struct{ err error }

// newSaveLayoutForm is the form for saving tabs, wt's session's, as the layout called name, this tab
// alone when only says so.
func newSaveLayoutForm(cfg config.Config, wt spaces.Worktree, tabs []spaces.CapturedTab, this int, name string, only bool) *saveLayoutForm {
	th := look.New(true)
	f := &saveLayoutForm{th: th, cfg: cfg, wt: wt, tabs: tabs, this: this, name: newInput(th, "")}
	f.name.SetValue(name)
	f.name.CursorEnd()
	f.name.Placeholder = "required"
	scope := everyTab
	if only {
		scope = thisTab
	}
	f.scope = newChoiceFields(th, scopeZone, []string{thisTab, everyTab}, func(s string) string { return s },
		func(string) []config.Input { return nil }, nil, false, scope, nil)
	choices := make([]paneChoice, 0, len(cfg.Commands)+2)
	for _, c := range cfg.Commands {
		choices = append(choices, paneChoice{command: c.Name})
	}
	choices = append(choices, paneChoice{run: true}, paneChoice{})
	runInput := []config.Input{{Name: runChoice, Label: runChoice}}
	for t, tab := range tabs {
		var row []choiceFields[paneChoice]
		for p, prog := range tab.Programs {
			c := newChoiceFields(th, "runs", choices, paneChoice.name, func(c paneChoice) []config.Input {
				if c.run {
					return runInput
				}
				return nil
			}, nil, false, prefill(cfg, prog), nil)
			c.zone = fmt.Sprintf("pane:%d:%d", t, p)
			// The run box has the command line whatever is chosen, for switching to it.
			box := &c.inputs[len(choices)-2][0]
			box.SetValue(strings.NewReplacer("{", "{{", "}", "}}").Replace(prog))
			box.CursorEnd()
			row = append(row, c)
		}
		f.panes = append(f.panes, row)
	}
	moveFocus(f.boxes(), &f.focus, 0)
	return f
}

// prefill is the choice for a pane running prog: the shell at a prompt; the first configured command
// whose program is prog's, by name, as claude --resume X is the claude command; else prog as run.
func prefill(cfg config.Config, prog string) string {
	if prog == "" {
		return shellChoice
	}
	words, err := spaces.SplitWords(prog)
	if err == nil && len(words) > 0 {
		for _, c := range cfg.Commands {
			if filepath.Base(c.Run[0]) == filepath.Base(words[0]) {
				return c.Name
			}
		}
	}
	return runChoice
}

func (f *saveLayoutForm) Init() tea.Cmd { return textinput.Blink }

// saved are the indexes of the tabs to save.
func (f *saveLayoutForm) saved() []int {
	if s, _ := f.scope.chosen(); s == thisTab {
		return []int{f.this}
	}
	out := make([]int, len(f.tabs))
	for i := range f.tabs {
		out[i] = i
	}
	return out
}

// saveField is a field of the form: a text box, or a choice when in is nil.
type saveField struct {
	in     *textinput.Model
	choice interface {
		cycle(int)
		zoneName() string
		openList()
		listOpen() bool
		updateList(tea.Msg, map[string][]look.Zone) (tea.Cmd, bool)
	}
}

// fields are the form's fields in order: the name, the scope, and each saved pane's choice, with its run
// box when it runs that.
func (f *saveLayoutForm) fields() []saveField {
	out := []saveField{{in: &f.name}, {choice: &f.scope}}
	for _, t := range f.saved() {
		for p := range f.panes[t] {
			c := &f.panes[t][p]
			out = append(out, saveField{choice: c})
			for _, in := range c.fields() {
				out = append(out, saveField{in: in})
			}
		}
	}
	return out
}

// boxes are the fields as moveFocus takes them, nil for a choice.
func (f *saveLayoutForm) boxes() []*textinput.Model {
	fields := f.fields()
	out := make([]*textinput.Model, len(fields))
	for i, fl := range fields {
		out[i] = fl.in
	}
	return out
}

func (f *saveLayoutForm) focused() saveField {
	fields := f.fields()
	return fields[min(f.focus, len(fields)-1)]
}

func (f *saveLayoutForm) nextField() tea.Cmd { return moveFocus(f.boxes(), &f.focus, 1) }
func (f *saveLayoutForm) prevField() tea.Cmd { return moveFocus(f.boxes(), &f.focus, -1) }

// onChoice says whether a choice has focus.
func (f *saveLayoutForm) onChoice() bool { return f.focused().choice != nil }

// cycleChoice chooses the next, or the one before, of the focused choice, which keeps focus.
func (f *saveLayoutForm) cycleChoice(delta int) tea.Cmd {
	f.focused().choice.cycle(delta)
	return moveFocus(f.boxes(), &f.focus, 0)
}

// layout is the layout the form saves, or what is wrong with it.
func (f *saveLayoutForm) layout() (config.Layout, error) {
	l := config.Layout{Name: strings.TrimSpace(f.name.Value())}
	if l.Name == "" {
		return config.Layout{}, fmt.Errorf("the layout needs a name")
	}
	for n, t := range f.saved() {
		tab := f.tabs[t].Tab
		tab.Panes = slices.Clone(tab.Panes)
		for p := range tab.Panes {
			c := &f.panes[t][p]
			ch, _ := c.chosen()
			switch {
			case ch.command != "":
				tab.Panes[p].Command = ch.command
			case ch.run:
				line := c.values()[runChoice]
				words, err := spaces.SplitWords(line)
				if err == nil && len(words) == 0 {
					err = fmt.Errorf("run is empty")
				}
				if err != nil {
					return config.Layout{}, fmt.Errorf("tab %d, pane %d: %w", n+1, p+1, err)
				}
				tab.Panes[p].Run = words
			}
		}
		l.Tabs = append(l.Tabs, tab)
	}
	return l, nil
}

// save saves the layout and leaves, or says why it cannot; a name the config has asks to replace it
// first, unless replace says to.
func (f *saveLayoutForm) save(replace bool) tea.Cmd {
	l, err := f.layout()
	f.err = err
	if err != nil {
		return nil
	}
	if _, exists := f.cfg.Layout(l.Name); exists && !replace {
		f.confirm = true
		return nil
	}
	f.busy, f.confirm = true, false
	return func() tea.Msg { return layoutSavedMsg{config.SaveLayout(l, replace)} }
}

func (f *saveLayoutForm) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// While a list is open, keys and the mouse are its. Only the focused choice opens one.
	if on := f.focused().choice; on != nil && on.listOpen() && isInput(msg) {
		cmd, picked := on.updateList(msg, f.zones)
		if picked {
			return f, tea.Batch(cmd, f.cycleChoice(0))
		}
		return f, cmd
	}
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		f.width, f.height = msg.Width, msg.Height
		w := max(f.width-4-labelWidth-1, 1)
		f.name.SetWidth(w)
		for t := range f.panes {
			for p := range f.panes[t] {
				f.panes[t][p].setWidth(w)
			}
		}
		return f, nil
	case layoutSavedMsg:
		f.busy, f.err = false, msg.err
		if msg.err == nil {
			return f, tea.Quit
		}
		return f, nil
	case tea.KeyPressMsg:
		cmd, ok := runOn(f, msg, theSaveForm.Target(saveSubject{}))
		if ok || f.confirm {
			return f, cmd
		}
	case tea.MouseClickMsg:
		if cmd, ok := f.buttons.click(f, f.zones, msg); ok || f.confirm {
			return f, cmd
		}
		for i, fl := range f.fields() {
			if fl.choice != nil && clickedChoice(f.zones, fl.choice.zoneName(), msg) {
				f.focus = i
				fl.choice.openList()
				return f, f.cycleChoice(0)
			}
		}
	}
	in := f.focused().in
	if in == nil {
		return f, nil
	}
	var cmd tea.Cmd
	*in, cmd = in.Update(msg)
	return f, cmd
}

func (f *saveLayoutForm) View() tea.View {
	frame, zones := look.Scan(f.render())
	f.zones = zones
	v := tea.NewView(frame)
	v.AltScreen = true
	v.MouseMode = tea.MouseModeCellMotion
	return v
}

func (f *saveLayoutForm) render() string {
	th, width := f.th, max(f.width-4, 20)
	line := func(spans ...look.Span) string { return th.Render(look.Plain, spans...) }
	muted := func(s string) string { return line(th.Cut(width, look.Muted(s))...) }
	on := f.focused().choice
	lines := []string{
		line(look.Heading("Save " + f.wt.Session + " as a layout")), "",
		th.Draw(look.Plain, labelWidth, look.Label("name")) + f.name.View(),
	}
	// list draws the open list, if any, over the lines once they are all there.
	list := func(lines []string) []string { return lines }
	if f.scope.listOpen() {
		at := len(lines)
		list = func(lines []string) []string { return f.scope.drawList(lines, at, width, f.height) }
	}
	lines = append(lines, f.scope.lines(on == &f.scope, width)...)
	saved := f.saved()
	for n, t := range saved {
		for p, prog := range f.tabs[t].Programs {
			if prog == "" {
				prog = "a shell"
			}
			c := &f.panes[t][p]
			lines = append(lines, line(th.Cut(width, look.Text(fmt.Sprintf("tab %d, pane %d: %s", n+1, p+1, prog)))...))
			if c.listOpen() {
				at := len(lines)
				list = func(lines []string) []string { return c.drawList(lines, at, width, f.height) }
			}
			lines = append(lines, c.lines(on == c, width)...)
		}
	}
	lines = append(lines, "", muted(fmt.Sprintf("Adds %s to the config's layouts", plural(len(saved), "tab"))), "")
	switch {
	case f.busy:
		lines = append(lines, line(look.Busy("Saving...")), "")
	case f.err != nil:
		for _, row := range look.Error(f.err.Error()).Wrapped(width) {
			lines = append(lines, line(row))
		}
		lines = append(lines, "")
	}
	f.buttons = controls[*saveLayoutForm]{}
	if on != nil && on.listOpen() {
		return boxed(append(list(lines), line(listHints()...)))
	}
	t := theSaveForm.Target(saveSubject{})
	if f.confirm {
		lines = append(lines, line(th.Cut(width, look.Text("A layout called "+strings.TrimSpace(f.name.Value())+" exists. Replace it?"))...), "")
		return boxed(append(lines, line(f.buttons.draw(f, t,
			control{action: actSaveReplace, label: "Replace", danger: true},
			control{action: actSaveKeep, label: "Keep it"})...)))
	}
	spans := f.buttons.draw(f, t,
		control{action: actSaveLayout, label: "Save"},
		control{action: actSaveCancel, label: "Cancel"})
	spans = append(spans, look.Text("  "), look.Key("tab"), look.KeyDesc(" next field"))
	if on != nil {
		spans = append(spans, choiceHints("choose")...)
	}
	return boxed(append(lines, line(spans...)))
}

// SaveLayoutForm runs the save form for the session of the worktree dir is in, its name filled in with
// name, saving this tab alone at first when only says so, meant for a kitty overlay, until the layout is
// saved or the form is left.
func SaveLayoutForm(ctx context.Context, cfg config.Config, dir, name string, only bool) error {
	top, err := git.TopLevel(dir)
	if err != nil {
		return err
	}
	_, wt, err := spaces.FindWorktree(cfg, top)
	if err != nil {
		return err
	}
	tabs, this, err := spaces.CaptureSession(wt, kitty.SelfWindowID())
	if err != nil {
		return err
	}
	if _, err := tea.NewProgram(newSaveLayoutForm(cfg, wt, tabs, this, name, only), tea.WithContext(ctx)).Run(); err != nil {
		return fmt.Errorf("running the save layout form: %w", err)
	}
	return nil
}
