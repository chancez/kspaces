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
	"github.com/chancez/kspaces/internal/gh"
	"github.com/chancez/kspaces/internal/spaces"
	"github.com/chancez/kspaces/internal/tui/look"
)

// The new worktree form's tabs: a branch to create or check out, or a pull request to check out.
const (
	tabBranch = iota
	tabPR
)

// tabNames are what the tab strip calls the tabs.
var tabNames = [...]string{"Branch", "Pull request"}

// tabZone is the zone of the tab strip's tabs, by index.
const tabZone = "tab:%d"

// worktreeTab is what each tab of the new worktree form has of its own, so switching tabs loses nothing:
// where the worktree goes, and the layout and command, with their inputs.
type worktreeTab struct {
	// Empty for the default, which spaces.WorktreeDir derives from the branch and the placeholder shows.
	dir lineField
	// The layout the session opens with, none for a shell tab, and its inputs. Only with layouts
	// configured, the branch tab's starting on the space's worktree layout, the PR tab's on its PR layout.
	layouts choiceFields[config.Layout]
	// The command to run, none for none, and its inputs: as the first tab without a layout, else after
	// it, where the command opens. Only with commands configured.
	cmds choiceFields[config.Command]
}

// newWorktreeForm creates a worktree, on a branch to create or check out or on a pull request, a tab
// each, and, optionally, where its worktree goes, a layout and a command to run in it, shows what it will
// do, creates the worktree and switches to its session. The PR tab's field takes a PR's number, URL or
// title, the open PRs it matches listed below it to choose from. It is meant for a kitty popup, as the
// sidebar is too narrow to type a name and read back the path in.
type newWorktreeForm struct {
	th    *look.Theme
	cfg   config.Config
	space spaces.Space
	tab   int
	tabs  [2]worktreeTab
	// The branch tab's branch. Typing #N or a PR's URL in it goes to the PR tab.
	branch lineField
	// The PR tab's PR, and the choice of it follow last saw, so a new one clears the error.
	prs  *ghPicker[gh.PRView, spaces.PR]
	seen spaces.Ref
	// focus is the index in fields of the field that has focus.
	focus int
	// typed is the branch field as follow last saw it, so it works out what it names only when it
	// changes, not at every cursor blink.
	typed string
	// vars are the built-ins of the worktree the form would create now, which the inputs' defaults use.
	vars   map[string]string
	busy   bool
	err    error
	width  int
	height int
	// zones and buttons are where the last frame drew its buttons, and what each runs, for a click.
	zones   map[string][]look.Zone
	buttons controls[*newWorktreeForm]
}

type createdMsg struct{ err error }

// newForm is the form for a worktree in space, on the branch tab. The branch starts with the configured
// prefix, so typing just the name gives PREFIX/NAME; deleting it makes a branch without one.
func newForm(cfg config.Config, space spaces.Space) *newWorktreeForm {
	th := look.New(true)
	branch := newLineField(th, "")
	branch.fill(spaces.BranchStart(cfg))
	f := &newWorktreeForm{th: th, cfg: cfg, space: space, branch: branch, prs: newGHPicker(th, prKind, space), focus: 1}
	for i, layout := range [...]string{cfg.LayoutFor(space.Name, true), cfg.PRLayoutFor(space.Name)} {
		f.tabs[i] = worktreeTab{
			dir:     newLineField(th, ""),
			layouts: newLayoutFields(th, cfg.Layouts, true, layout, nil, nil),
			cmds:    newCommandFields(th, cfg.Commands, true, "", nil, nil),
		}
	}
	f.refocus()
	f.follow()
	return f
}

// Init lists the open ones when the form starts on a tab that picks among them, and looks up the one
// its field names.
func (f *newWorktreeForm) Init() tea.Cmd {
	p := f.shownPicker()
	if p == nil {
		return textinput.Blink
	}
	return tea.Batch(textinput.Blink, p.listOpen(), p.lookUp())
}

// current is the tab shown.
func (f *newWorktreeForm) current() *worktreeTab { return &f.tabs[f.tab] }

// shownPicker is the field of the tab shown that picks a PR, nil for the branch tab.
func (f *newWorktreeForm) shownPicker() tabPicker {
	if f.tab == tabPR {
		return f.prs
	}
	return nil
}

// prLookup is what looking up the PR chosen has found so far, ok false when the PR tab is not shown or
// has none chosen.
func (f *newWorktreeForm) prLookup() (l lookup[spaces.PR], ok bool) {
	if f.tab != tabPR {
		return lookup[spaces.PR]{}, false
	}
	return f.prs.lookup()
}

// plan is the branch and directory the form would create now. For a PR that is its local branch, empty
// until it has been looked up.
func (f *newWorktreeForm) plan() (branch, dir string) {
	branch = strings.TrimSpace(f.branch.Value())
	if f.tab == tabPR {
		l, _ := f.prLookup()
		branch = l.found.Branch
	}
	return branch, spaces.WorktreeDir(f.cfg, f.space.Root, branch, f.current().dir.Value())
}

func (f *newWorktreeForm) create() tea.Cmd {
	t := f.current()
	if f.tab == tabPR && f.prs.chosen.Number == 0 {
		f.err = fmt.Errorf("choose a PR to check out")
		return nil
	}
	var layout *spaces.LayoutRun
	if l, ok := t.layouts.chosen(); ok {
		lr, err := spaces.NewLayoutRun(l, t.layouts.values(), f.vars)
		if err != nil {
			f.err = err
			return nil
		}
		layout = &lr
	}
	var run *spaces.Run
	if cmd, ok := t.cmds.chosen(); ok {
		r, err := spaces.NewRun(cmd, t.cmds.values(), f.vars)
		if err != nil {
			f.err = err
			return nil
		}
		run = &r
	}
	f.busy, f.err = true, nil
	cfg, space := f.cfg, f.space
	open := func(wt spaces.Worktree, links spaces.Links) error {
		var lr *spaces.LayoutRun
		if layout != nil {
			v := *layout
			v.Links = links
			lr = &v
		}
		var r *spaces.Run
		if run != nil {
			v := *run
			v.Links = links
			r = &v
		}
		return spaces.OpenNew(cfg, space, wt, lr, r)
	}
	if l, ok := f.prLookup(); ok {
		ref, dir := f.prs.chosen, t.dir.Value()
		return func() tea.Msg {
			// Enter may come before the lookup, or after one that failed for a reason since fixed.
			pr, err := l.found, l.err
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
			return createdMsg{open(wt, spaces.Links{PR: pr.Number})}
		}
	}
	branch, dir := f.plan()
	return func() tea.Msg {
		wt, err := spaces.CreateWorktree(space, branch, dir)
		if err != nil {
			return createdMsg{err}
		}
		return createdMsg{open(wt, spaces.Links{})}
	}
}

// fields are the form's fields in order: the tab strip, nil; the branch, or the PR; the directory; the
// choice of layout, nil, when there is one, and the chosen layout's inputs; then the same for the command.
func (f *newWorktreeForm) fields() []field {
	t := f.current()
	out := []field{nil, &f.branch, &t.dir}
	if p := f.shownPicker(); p != nil {
		out[1] = p.input()
	}
	if t.layouts.picks() {
		out = append(out, nil)
	}
	out = append(out, t.layouts.fields()...)
	if t.cmds.picks() {
		out = append(out, nil)
	}
	return append(out, t.cmds.fields()...)
}

// layoutAt and commandAt are where in fields the choices of layout and command are, when there are.
func (f *newWorktreeForm) layoutAt() int { return 3 }
func (f *newWorktreeForm) commandAt() int {
	t := f.current()
	at := 3 + len(t.layouts.fields())
	if t.layouts.picks() {
		at++
	}
	return at
}

// focused is the field typing goes to, nil for a choice.
func (f *newWorktreeForm) focused() field {
	fields := f.fields()
	return fields[min(f.focus, len(fields)-1)]
}

// refocus gives the field focus has the cursor, and takes it from every other, the other tab's too.
func (f *newWorktreeForm) refocus() tea.Cmd {
	f.branch.Blur()
	f.prs.field.Blur()
	for i := range f.tabs {
		t := &f.tabs[i]
		t.dir.Blur()
		for _, fl := range slices.Concat(t.layouts.fields(), t.cmds.fields()) {
			fl.Blur()
		}
	}
	return moveFocus(f.fields(), &f.focus, 0)
}

// nextField and prevField move typing to the field below or above, round from the last to the first.
func (f *newWorktreeForm) nextField() tea.Cmd { return moveFocus(f.fields(), &f.focus, 1) }
func (f *newWorktreeForm) prevField() tea.Cmd { return moveFocus(f.fields(), &f.focus, -1) }

// onTabs, onLayout and onCommand say whether the tab strip, the choice of layout or of command has focus,
// onPicker whether the field of a tab that picks a PR has.
func (f *newWorktreeForm) onTabs() bool   { return f.focus == 0 }
func (f *newWorktreeForm) onPicker() bool { return f.shownPicker() != nil && f.focus == 1 }
func (f *newWorktreeForm) onLayout() bool {
	return f.current().layouts.picks() && f.focus == f.layoutAt()
}
func (f *newWorktreeForm) onCommand() bool {
	return f.current().cmds.picks() && f.focus == f.commandAt()
}

// onChoice says whether a choice has focus, which left and right change.
func (f *newWorktreeForm) onChoice() bool { return f.onTabs() || f.onLayout() || f.onCommand() }

// showTab shows tab, focus on field at, and lists the open ones the first time it is a tab that picks.
func (f *newWorktreeForm) showTab(tab, at int) tea.Cmd {
	f.tab, f.focus, f.err = tab, at, nil
	cmd := tea.Batch(f.refocus(), f.follow())
	if p := f.shownPicker(); p != nil {
		return tea.Batch(cmd, p.listOpen())
	}
	return cmd
}

// cycleLayout and cycleCommand choose the next layout or command, or the one before, and focus the
// choice, as the inputs below it change with it.
func (f *newWorktreeForm) cycleLayout(delta int) tea.Cmd {
	f.current().layouts.cycle(delta)
	f.focus = f.layoutAt()
	return moveFocus(f.fields(), &f.focus, 0)
}

func (f *newWorktreeForm) cycleCommand(delta int) tea.Cmd {
	f.current().cmds.cycle(delta)
	f.focus = f.commandAt()
	return moveFocus(f.fields(), &f.focus, 0)
}

// cycleChoice changes the choice that has focus.
func (f *newWorktreeForm) cycleChoice(delta int) tea.Cmd {
	switch {
	case f.onTabs():
		return f.showTab((f.tab+delta+len(f.tabs))%len(f.tabs), 0)
	case f.onLayout():
		return f.cycleLayout(delta)
	}
	return f.cycleCommand(delta)
}

// openList opens the list of the choice that has focus.
func (f *newWorktreeForm) openList() {
	if f.onLayout() {
		f.current().layouts.openList()
	} else {
		f.current().cmds.openList()
	}
}

// movePick highlights the open one delta rows from the one highlighted, below the picker's field, and
// chooses it.
func (f *newWorktreeForm) movePick(delta int) tea.Cmd {
	return tea.Batch(f.shownPicker().move(delta), f.follow())
}

// confirmPick chooses the open one of row i below the picker's field, fills the field in with its number
// and moves on to the field below, which closes the rows.
func (f *newWorktreeForm) confirmPick(i int) tea.Cmd {
	cmd := f.shownPicker().confirm(i)
	f.focus = 2
	return tea.Batch(cmd, f.follow(), f.refocus())
}

func (f *newWorktreeForm) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	t := f.current()
	// While a list is open, keys and the mouse are its.
	if isInput(msg) {
		switch {
		case t.layouts.listOpen():
			cmd, picked := t.layouts.updateList(msg, f.zones)
			if picked {
				return f, tea.Batch(cmd, f.cycleLayout(0))
			}
			return f, cmd
		case t.cmds.listOpen():
			cmd, picked := t.cmds.updateList(msg, f.zones)
			if picked {
				return f, tea.Batch(cmd, f.cycleCommand(0))
			}
			return f, cmd
		}
	}
	if cmd, ok := f.prs.update(msg); ok {
		return f, tea.Batch(cmd, f.follow())
	}
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		f.width, f.height = msg.Width, msg.Height
		// bubbles v2 draws only the first character of a placeholder without a width. The fields start
		// after the padding and their labels, and the cursor takes a cell past the text.
		w := max(f.width-4-labelWidth-1, 1)
		f.branch.SetWidth(w)
		f.prs.field.SetWidth(w)
		for i := range f.tabs {
			f.tabs[i].dir.SetWidth(w)
			f.tabs[i].layouts.setWidth(w)
			f.tabs[i].cmds.setWidth(w)
		}
		return f, nil
	case createdMsg:
		f.busy, f.err = false, msg.err
		if msg.err == nil {
			return f, tea.Quit
		}
		return f, nil
	case tea.KeyPressMsg:
		if keptBy(f.focused(), msg) {
			break
		}
		// up and down go through the open ones a picker's field matches, rather than the fields.
		if f.onPicker() && f.shownPicker().matched() > 1 {
			switch msg.Code {
			case tea.KeyUp:
				return f, f.movePick(-1)
			case tea.KeyDown:
				return f, f.movePick(1)
			}
		}
		if cmd, ok := runOn(f, msg, theForm.Target(formSubject{})); ok {
			return f, cmd
		}
	case tea.MouseClickMsg:
		if cmd, ok := f.buttons.click(f, f.zones, msg); ok {
			return f, cmd
		}
		for i := range f.tabs {
			if clickedChoice(f.zones, fmt.Sprintf(tabZone, i), msg) {
				return f, f.showTab(i, 0)
			}
		}
		if f.onPicker() {
			for i := range f.shownPicker().matched() {
				if clickedChoice(f.zones, fmt.Sprintf(pickRowZone, i), msg) {
					return f, f.confirmPick(i)
				}
			}
		}
		if clickedChoice(f.zones, layoutZone, msg) {
			t.layouts.openList()
			return f, f.cycleLayout(0)
		}
		if clickedChoice(f.zones, commandZone, msg) {
			t.cmds.openList()
			return f, f.cycleCommand(0)
		}
		if i, ok := clickedField(f.fields(), f.zones, msg); ok {
			f.focus = i
			return f, moveFocus(f.fields(), &f.focus, 0)
		}
	}
	// Everything else is typing in the field that has focus.
	in := f.focused()
	if in == nil {
		return f, nil
	}
	return f, tea.Batch(in.update(msg), f.follow())
}

// follow works out what the branch field names, going to the PR tab for a PR, and what the pickers'
// fields do, choosing what they name; makes the directory's placeholder the default for the branch the
// form would create, so the form always shows where the worktree would go; and fills in again the inputs
// left at their defaults, which follow it too.
func (f *newWorktreeForm) follow() tea.Cmd {
	var cmds []tea.Cmd
	if typed := f.branch.Value(); typed != f.typed {
		f.typed = typed
		rest := strings.TrimPrefix(strings.TrimSpace(typed), spaces.BranchStart(f.cfg))
		if number, hash := strings.CutPrefix(rest, "#"); hash || spaces.IsPRURL(rest) {
			// Typing goes on in the PR field, from what follows the #, or with the URL in it.
			if hash {
				rest = number
			}
			f.branch.fill(spaces.BranchStart(f.cfg))
			f.typed = f.branch.Value()
			f.prs.field.fill(rest)
			cmds = append(cmds, f.showTab(tabPR, 1))
		}
	}
	cmds = append(cmds, f.prs.refollow())
	if p := f.shownPicker(); p != nil && p.choice() != f.seen {
		f.seen, f.err = p.choice(), nil
	}
	t := f.current()
	branch, planned := f.plan()
	dir := spaces.WorktreeDir(f.cfg, f.space.Root, branch, "")
	if rel, err := filepath.Rel(f.space.Root, dir); err == nil {
		dir = rel
	}
	t.dir.Placeholder = dir
	f.vars = spaces.BuiltinVars(f.space, spaces.LinkedWorktree(f.space, branch, planned), spaces.Links{PR: f.prNumber()})
	t.layouts.setVars(f.vars)
	t.cmds.setVars(f.vars)
	return tea.Batch(cmds...)
}

// prNumber is the number of the PR chosen while the PR tab is shown, else 0.
func (f *newWorktreeForm) prNumber() int {
	if f.tab != tabPR {
		return 0
	}
	return f.prs.chosen.Number
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

// tabStrip draws the tabs, the one shown on a chip, and how to switch them while the strip has focus.
func (f *newWorktreeForm) tabStrip(width int) string {
	th := f.th
	var spans []look.Span
	for i, name := range tabNames {
		if i > 0 {
			spans = append(spans, look.Text(" "))
		}
		tab := []look.Span{look.Muted(" " + name + " ")}
		if i == f.tab {
			tab = look.Button(name, "")
		}
		spans = append(spans, look.Mark(fmt.Sprintf(tabZone, i), tab...)...)
	}
	if room := width - th.Width(spans...); f.onTabs() && room > 0 {
		spans = append(spans, th.Cut(room, look.Muted("  left/right switch"))...)
	}
	return th.Render(look.Plain, spans...)
}

func (f *newWorktreeForm) render() string {
	th, width := f.th, max(f.width-4, 20)
	line := func(spans ...look.Span) string { return th.Render(look.Plain, spans...) }
	muted := func(s string) string { return line(th.Cut(width, look.Muted(s))...) }
	t := f.current()
	branch, _ := f.plan()
	lines := []string{line(look.Heading("New worktree in " + f.space.Name)), "", f.tabStrip(width), ""}
	if p := f.shownPicker(); p != nil {
		lines = append(lines, p.lines(th, f.onPicker(), width)...)
	} else {
		lines = append(lines, fieldLines(th, "branch", &f.branch, width)...)
	}
	lines = append(lines, fieldLines(th, "directory", &t.dir, width)...)
	layoutAt := len(lines)
	lines = append(lines, t.layouts.lines(f.onLayout(), width)...)
	commandAt := len(lines)
	lines = append(append(lines, t.cmds.lines(f.onCommand(), width)...), "")
	failed := f.err
	switch p := f.shownPicker(); {
	case p != nil:
		says, err := p.status(muted)
		lines = append(lines, says...)
		if failed == nil {
			failed = err
		}
		if l, ok := f.prLookup(); ok && l.done && l.err == nil {
			lines = append(lines, muted(prDoes(l.found)))
		}
	case branch == "" || strings.HasSuffix(branch, "/"):
		lines = append(lines, muted("Name the branch to create or check out, or # for a PR"))
	case f.space.HasBranch(branch):
		lines = append(lines, muted("Checks out the existing branch "+branch))
	case len(f.space.Worktrees) > 0 && f.space.Worktrees[0].Branch != "":
		lines = append(lines, muted("Creates "+branch+" from "+f.space.Worktrees[0].Branch))
	default:
		lines = append(lines, muted("Creates "+branch))
	}
	l, layout := t.layouts.chosen()
	if layout {
		lines = append(lines, muted("Opens with the "+l.Name+" layout"))
	}
	if cmd, ok := t.cmds.chosen(); ok {
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
	if t.layouts.listOpen() || t.cmds.listOpen() {
		lines = t.layouts.drawList(lines, layoutAt, width, f.height)
		lines = t.cmds.drawList(lines, commandAt, width, f.height)
		return boxed(append(lines, line(listHints()...)))
	}
	buttons := f.buttons.draw(f, theForm.Target(formSubject{}),
		control{action: actCreate, label: "Create"},
		control{action: actCancel, label: "Cancel"})
	hints := []look.Span{look.Text("  "), look.Key("tab"), look.KeyDesc(" next field")}
	switch {
	case f.onTabs():
		hints = append(hints, look.Text("  "), look.Key("left/right"), look.KeyDesc(" tab"))
	case f.onPicker() && f.shownPicker().matched() > 1:
		hints = append(hints, look.Text("  "), look.Key("up/down"), look.KeyDesc(" "+f.shownPicker().name()))
	case f.onLayout():
		hints = append(hints, choiceHints("layout")...)
	case f.onCommand():
		hints = append(hints, choiceHints("command")...)
	}
	hints = append(hints, newlineHints(f.focused())...)
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

// startOnPR starts the form on the PR tab, its field filled in with pr, a PR's number or URL, when not
// empty.
func (f *newWorktreeForm) startOnPR(pr string) error {
	if pr != "" {
		if _, ok := spaces.ParsePRRef(pr); !ok {
			return fmt.Errorf("%s is not a PR's number or URL", pr)
		}
		f.prs.field.fill(pr)
	}
	f.tab, f.focus = tabPR, 1
	f.refocus()
	f.follow()
	return nil
}

// RunNewWorktree runs the new worktree form for space, with the config as the space sees it, meant for a
// kitty popup, until the worktree is created and opened or the form is left. onPR starts it on the PR
// tab, on pr, a PR's number or URL, when not empty.
func RunNewWorktree(ctx context.Context, cfg config.Config, space spaces.Space, onPR bool, pr string) error {
	f := newForm(space.ConfigOf(cfg), space)
	if onPR || pr != "" {
		if err := f.startOnPR(pr); err != nil {
			return err
		}
	}
	if err := runPopup(ctx, f); err != nil {
		return fmt.Errorf("running the new worktree form: %w", err)
	}
	return nil
}
