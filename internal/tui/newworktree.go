package tui

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	"github.com/chancez/kspaces/internal/config"
	"github.com/chancez/kspaces/internal/spaces"
	"github.com/chancez/kspaces/internal/tui/look"
)

// newWorktreeForm asks for a branch, or a PR to check out, and, optionally, where its worktree goes and a
// command to run in it, shows what it will do, creates the worktree and switches to its session. It is
// meant for a kitty overlay, as the sidebar is too narrow to type a name and read back the path in.
type newWorktreeForm struct {
	th     *look.Theme
	cfg    config.Config
	space  spaces.Space
	branch textinput.Model
	// Empty for the default, which spaces.WorktreeDir derives from the branch and the placeholder shows.
	dir textinput.Model
	// The layout the session opens with, none for a shell tab, and its inputs. Only with layouts
	// configured, the space's worktree layout chosen at first.
	layouts choiceFields[config.Layout]
	// The command to run, none for none, and its inputs: as the first tab without a layout, else after
	// it, where the command opens. Only with commands configured.
	cmds choiceFields[config.Command]
	// focus is the index in fields of the field that has focus.
	focus int
	// typed is the branch field as followBranch last saw it, so it works out what that names only when it
	// changes, not at every cursor blink.
	typed string
	// The PR the branch field names, when isPR, and the lookups of each PR it has named, by gh's argument.
	// prSeq counts the changes of the field, so only a lookup it rested on starts.
	pr      spaces.PRRef
	isPR    bool
	prSeq   int
	lookups map[string]prLookup
	busy    bool
	err     error
	width   int
	height  int
	// zones and buttons are where the last frame drew its buttons, and what each runs, for a click.
	zones   map[string][]look.Zone
	buttons controls[*newWorktreeForm]
}

// prLookup is what looking a PR up found, or, until done, that it is under way.
type prLookup struct {
	pr   spaces.PR
	err  error
	done bool
}

// prDebounce is how long the branch field rests on a PR before it is looked up, so typing a number does
// not run gh for every digit.
const prDebounce = 300 * time.Millisecond

// prDueMsg is the branch field having rested on a PR, change seq of it, for prDebounce.
type prDueMsg struct {
	seq int
	ref spaces.PRRef
}

// prFoundMsg is the lookup of the PR gh knows as arg.
type prFoundMsg struct {
	arg string
	prLookup
}

// newForm is the form for a worktree in space. The branch starts with the configured prefix, so typing
// just the name gives PREFIX/NAME; deleting it makes a branch without one.
func newForm(cfg config.Config, space spaces.Space) *newWorktreeForm {
	th := look.New(true)
	branch := newInput(th, "")
	branch.SetValue(spaces.BranchStart(cfg))
	branch.CursorEnd()
	branch.Focus()
	f := &newWorktreeForm{th: th, cfg: cfg, space: space, branch: branch, dir: newInput(th, ""), lookups: map[string]prLookup{}}
	f.layouts = newLayoutFields(th, cfg.Layouts, true, cfg.LayoutFor(space.Name, true), nil)
	f.cmds = newCommandFields(th, cfg.Commands, true, "", nil)
	f.followBranch()
	return f
}

type createdMsg struct{ err error }

func (f *newWorktreeForm) Init() tea.Cmd {
	return textinput.Blink
}

// lookup is what looking up the PR the branch field names has found so far, ok false when it names none.
func (f *newWorktreeForm) lookup() (l prLookup, ok bool) {
	if !f.isPR {
		return prLookup{}, false
	}
	return f.lookups[f.pr.Arg], true
}

// plan is the branch and directory the form would create now. For a PR that is its local branch, empty
// until it has been looked up.
func (f *newWorktreeForm) plan() (branch, dir string) {
	branch = strings.TrimSpace(f.branch.Value())
	if l, ok := f.lookup(); ok {
		branch = l.pr.Branch
	}
	return branch, spaces.WorktreeDir(f.cfg, f.space.Root, branch, f.dir.Value())
}

func (f *newWorktreeForm) create() tea.Cmd {
	var layout *spaces.LayoutRun
	if l, ok := f.layouts.chosen(); ok {
		lr, err := spaces.NewLayoutRun(l, f.layouts.values())
		if err != nil {
			f.err = err
			return nil
		}
		layout = &lr
	}
	var run *spaces.Run
	if cmd, ok := f.cmds.chosen(); ok {
		r, err := spaces.NewRun(cmd, f.cmds.values())
		if err != nil {
			f.err = err
			return nil
		}
		run = &r
	}
	f.busy, f.err = true, nil
	cfg, space := f.cfg, f.space
	open := func(wt spaces.Worktree, pr int) error {
		var lr *spaces.LayoutRun
		if layout != nil {
			v := *layout
			v.PR = pr
			lr = &v
		}
		var r *spaces.Run
		if run != nil {
			v := *run
			v.PR = pr
			r = &v
		}
		return spaces.OpenNew(cfg, space, wt, lr, r)
	}
	if l, ok := f.lookup(); ok {
		ref, dir := f.pr, f.dir.Value()
		return func() tea.Msg {
			// Enter may come before the lookup, or after one that failed for a reason since fixed.
			pr, err := l.pr, l.err
			if !l.done || err != nil {
				pr, err = spaces.ResolvePR(space, ref)
			}
			if err != nil {
				return createdMsg{err}
			}
			wt, err := spaces.CreatePRWorktree(space, pr, spaces.WorktreeDir(cfg, space.Root, pr.Branch, dir))
			if err != nil {
				return createdMsg{err}
			}
			return createdMsg{open(wt, pr.Number)}
		}
	}
	branch, dir := f.plan()
	return func() tea.Msg {
		wt, err := spaces.CreateWorktree(space, branch, dir)
		if err != nil {
			return createdMsg{err}
		}
		return createdMsg{open(wt, 0)}
	}
}

// fields are the form's fields in order: the branch, the directory, the choice of layout, nil, when there
// is one, the chosen layout's inputs, then the same for the command.
func (f *newWorktreeForm) fields() []*textinput.Model {
	out := []*textinput.Model{&f.branch, &f.dir}
	if f.layouts.picks() {
		out = append(out, nil)
	}
	out = append(out, f.layouts.fields()...)
	if f.cmds.picks() {
		out = append(out, nil)
	}
	return append(out, f.cmds.fields()...)
}

// layoutAt and commandAt are where in fields the choices of layout and command are, when there are.
func (f *newWorktreeForm) layoutAt() int { return 2 }
func (f *newWorktreeForm) commandAt() int {
	at := 2 + len(f.layouts.fields())
	if f.layouts.picks() {
		at++
	}
	return at
}

// focused is the field typing goes to, nil for a choice.
func (f *newWorktreeForm) focused() *textinput.Model {
	fields := f.fields()
	return fields[min(f.focus, len(fields)-1)]
}

// nextField and prevField move typing to the field below or above, round from the last to the first.
func (f *newWorktreeForm) nextField() tea.Cmd { return moveFocus(f.fields(), &f.focus, 1) }
func (f *newWorktreeForm) prevField() tea.Cmd { return moveFocus(f.fields(), &f.focus, -1) }

// onLayout and onCommand say whether the choice of layout or of command has focus.
func (f *newWorktreeForm) onLayout() bool  { return f.layouts.picks() && f.focus == f.layoutAt() }
func (f *newWorktreeForm) onCommand() bool { return f.cmds.picks() && f.focus == f.commandAt() }

// cycleLayout and cycleCommand choose the next layout or command, or the one before, and focus the
// choice, as the inputs below it change with it.
func (f *newWorktreeForm) cycleLayout(delta int) tea.Cmd {
	f.layouts.cycle(delta)
	f.focus = f.layoutAt()
	return moveFocus(f.fields(), &f.focus, 0)
}

func (f *newWorktreeForm) cycleCommand(delta int) tea.Cmd {
	f.cmds.cycle(delta)
	f.focus = f.commandAt()
	return moveFocus(f.fields(), &f.focus, 0)
}

// cycleChoice changes the choice that has focus.
func (f *newWorktreeForm) cycleChoice(delta int) tea.Cmd {
	if f.onLayout() {
		return f.cycleLayout(delta)
	}
	return f.cycleCommand(delta)
}

// openList opens the list of the choice that has focus.
func (f *newWorktreeForm) openList() {
	if f.onLayout() {
		f.layouts.openList()
	} else {
		f.cmds.openList()
	}
}

func (f *newWorktreeForm) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// While a list is open, keys and the mouse are its.
	if isInput(msg) {
		switch {
		case f.layouts.listOpen():
			cmd, picked := f.layouts.updateList(msg, f.zones)
			if picked {
				return f, tea.Batch(cmd, f.cycleLayout(0))
			}
			return f, cmd
		case f.cmds.listOpen():
			cmd, picked := f.cmds.updateList(msg, f.zones)
			if picked {
				return f, tea.Batch(cmd, f.cycleCommand(0))
			}
			return f, cmd
		}
	}
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		f.width, f.height = msg.Width, msg.Height
		// bubbles v2 draws only the first character of a placeholder without a width. The fields start
		// after the padding and their labels, and the cursor takes a cell past the text.
		f.branch.SetWidth(max(f.width-4-labelWidth-1, 1))
		f.dir.SetWidth(max(f.width-4-labelWidth-1, 1))
		f.layouts.setWidth(max(f.width-4-labelWidth-1, 1))
		f.cmds.setWidth(max(f.width-4-labelWidth-1, 1))
		return f, nil
	case createdMsg:
		f.busy, f.err = false, msg.err
		if msg.err == nil {
			return f, tea.Quit
		}
		return f, nil
	case prDueMsg:
		if msg.seq != f.prSeq || !f.isPR {
			return f, nil
		}
		if _, asked := f.lookups[msg.ref.Arg]; asked {
			return f, nil
		}
		f.lookups[msg.ref.Arg] = prLookup{}
		space, ref := f.space, msg.ref
		// gh asks GitHub, which takes a while, so off the UI loop.
		return f, func() tea.Msg {
			pr, err := spaces.ResolvePR(space, ref)
			return prFoundMsg{ref.Arg, prLookup{pr: pr, err: err, done: true}}
		}
	case prFoundMsg:
		f.lookups[msg.arg] = msg.prLookup
		f.followBranch()
		return f, nil
	case tea.KeyPressMsg:
		if cmd, ok := runOn(f, msg, theForm.Target(formSubject{})); ok {
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
		if clickedChoice(f.zones, commandZone, msg) {
			f.cmds.openList()
			return f, f.cycleCommand(0)
		}
	}
	// Everything else is typing in the field that has focus.
	in := f.focused()
	if in == nil {
		return f, nil
	}
	var cmd tea.Cmd
	*in, cmd = in.Update(msg)
	return f, tea.Batch(cmd, f.followBranch())
}

// followBranch works out whether the branch field names a PR, and makes the directory's placeholder the
// default for the branch it would create, so the form always shows where the worktree would go. A PR the
// field has come to name is looked up once it rests on it, which the command it returns waits for.
func (f *newWorktreeForm) followBranch() tea.Cmd {
	var cmd tea.Cmd
	if typed := f.branch.Value(); typed != f.typed {
		f.typed = typed
		ref, isPR := spaces.ParsePRRef(f.cfg, f.space, typed)
		if isPR != f.isPR || ref != f.pr {
			f.pr, f.isPR = ref, isPR
			f.prSeq++
			if _, asked := f.lookups[ref.Arg]; isPR && !asked {
				seq := f.prSeq
				cmd = tea.Tick(prDebounce, func(time.Time) tea.Msg { return prDueMsg{seq, ref} })
			}
		}
	}
	branch, _ := f.plan()
	dir := spaces.WorktreeDir(f.cfg, f.space.Root, branch, "")
	if rel, err := filepath.Rel(f.space.Root, dir); err == nil {
		dir = rel
	}
	f.dir.Placeholder = dir
	return cmd
}

func (f *newWorktreeForm) View() tea.View {
	frame, zones := look.Scan(f.render())
	f.zones = zones
	v := tea.NewView(frame)
	v.AltScreen = true
	v.MouseMode = tea.MouseModeCellMotion
	return v
}

// labelWidth is the column the fields start at, after their labels.
const labelWidth = 11

func (f *newWorktreeForm) render() string {
	th, width := f.th, max(f.width-4, 20)
	line := func(spans ...look.Span) string { return th.Render(look.Plain, spans...) }
	muted := func(s string) string { return line(th.Cut(width, look.Muted(s))...) }
	branch, _ := f.plan()
	lines := []string{
		line(look.Heading("New worktree in " + f.space.Name)), "",
		th.Draw(look.Plain, labelWidth, look.Label("branch")) + f.branch.View(),
		th.Draw(look.Plain, labelWidth, look.Label("directory")) + f.dir.View(),
	}
	layoutAt := len(lines)
	lines = append(lines, f.layouts.lines(f.onLayout(), width)...)
	commandAt := len(lines)
	lines = append(append(lines, f.cmds.lines(f.onCommand(), width)...), "")
	failed := f.err
	if l, ok := f.lookup(); ok {
		pr := fmt.Sprintf("PR #%d", f.pr.Number)
		switch {
		case !l.done:
			lines = append(lines, muted("Looking up "+pr+"..."))
		case l.err != nil:
			lines = append(lines, muted("Could not look up "+pr))
			if failed == nil {
				failed = l.err
			}
		default:
			lines = append(lines, muted(pr+" "+l.pr.Title), muted(prDoes(l.pr)))
		}
	} else {
		switch {
		case branch == "" || strings.HasSuffix(branch, "/"):
			lines = append(lines, muted("Name the branch to create or check out, or a PR"))
		case f.space.HasBranch(branch):
			lines = append(lines, muted("Checks out the existing branch "+branch))
		case len(f.space.Worktrees) > 0 && f.space.Worktrees[0].Branch != "":
			lines = append(lines, muted("Creates "+branch+" from "+f.space.Worktrees[0].Branch))
		default:
			lines = append(lines, muted("Creates "+branch))
		}
	}
	l, layout := f.layouts.chosen()
	if layout {
		lines = append(lines, muted("Opens with the "+l.Name+" layout"))
	}
	if cmd, ok := f.cmds.chosen(); ok {
		if layout {
			lines = append(lines, muted("Then runs "+cmd.Name+" in "+opensIn[cmd.Open]))
		} else {
			lines = append(lines, muted("Runs "+cmd.Name+" in the first tab"))
		}
	}
	lines = append(lines, "")
	switch {
	case f.busy:
		lines = append(lines, line(look.Busy("Creating...")), "")
	case failed != nil:
		for _, row := range look.Error(failed.Error()).Wrapped(width) {
			lines = append(lines, line(row))
		}
		lines = append(lines, "")
	}
	f.buttons = controls[*newWorktreeForm]{}
	if f.layouts.listOpen() || f.cmds.listOpen() {
		lines = f.layouts.drawList(lines, layoutAt, width, f.height)
		lines = f.cmds.drawList(lines, commandAt, width, f.height)
		return boxed(append(lines, line(listHints()...)))
	}
	buttons := f.buttons.draw(f, theForm.Target(formSubject{}),
		control{action: actCreate, label: "Create"},
		control{action: actCancel, label: "Cancel"})
	hints := []look.Span{look.Text("  "), look.Key("tab"), look.KeyDesc(" next field")}
	switch {
	case f.onLayout():
		hints = append(hints, choiceHints("layout")...)
	case f.onCommand():
		hints = append(hints, choiceHints("command")...)
	}
	lines = append(lines, line(append(buttons, hints...)...))
	return boxed(lines)
}

// opensIn says where a command opens in a session with windows, by its open.
var opensIn = map[string]string{
	"tab": "a new tab", "split": "a split", "hsplit": "a horizontal split", "vsplit": "a vertical split",
	"overlay": "an overlay", "window": "a new OS window",
}

// prDoes is what creating the worktree does with pr.
func prDoes(pr spaces.PR) string {
	switch {
	case pr.Exists:
		return "Checks out the existing branch " + pr.Branch
	case pr.Cross && pr.Branch != pr.HeadRef:
		return "Checks out " + pr.Owner + ":" + pr.HeadRef + " from " + pr.Remote + " as " + pr.Branch
	case pr.Cross:
		return "Checks out " + pr.Owner + ":" + pr.HeadRef + " from " + pr.Remote
	default:
		return "Checks out " + pr.Branch + " from " + pr.Remote + ", tracking it"
	}
}

// RunNewWorktree runs the new worktree form for space, with the config as the space sees it, meant for a
// kitty overlay, until the worktree is created and opened or the form is left.
func RunNewWorktree(ctx context.Context, cfg config.Config, space spaces.Space) error {
	if _, err := tea.NewProgram(newForm(space.ConfigOf(cfg), space), tea.WithContext(ctx)).Run(); err != nil {
		return fmt.Errorf("running the new worktree form: %w", err)
	}
	return nil
}
