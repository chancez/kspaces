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

// The new worktree form's tabs: a branch to create or check out, a pull request to check out, or an issue
// to create a branch for.
const (
	tabBranch = iota
	tabPR
	tabIssue
)

// tabNames are what the tab strip calls the tabs.
var tabNames = [...]string{"Branch", "Pull request", "Issue"}

// tabZone is the zone of the tab strip's tabs, by index.
const tabZone = "tab:%d"

// worktreeTab is what each tab of the new worktree form has of its own, so switching tabs loses nothing:
// where a new branch starts from, where the worktree goes, and what its session opens with, with its
// inputs.
type worktreeTab struct {
	// The branch a new branch starts from: none for what the main worktree has checked out, or another
	// local branch. Only on the branch and Issue tabs, with another branch to choose.
	base choiceFields[string]
	// Empty for the default, which spaces.WorktreeDir derives from the branch and the placeholder shows.
	dir lineField
	// What the session opens with, a shell tab for none, and its inputs. Only with layouts or commands
	// configured, the branch tab's starting on the space's worktree layout, the PR tab's on its PR layout
	// and the Issue tab's on its issue layout.
	open choiceFields[opener]
}

// opener is what a new worktree's session opens with: a layout's tabs, or a command as its one tab.
type opener struct {
	layout *config.Layout
	cmd    *config.Command
}

// openZone is the zone the choice of what the session opens with is drawn in, and its label.
const openZone = "open with"

// backgroundZone is the zone the open in background box is drawn in.
const backgroundZone = "background"

// newOpenFields are the fields that choose what a new worktree's session opens with, the one called name
// chosen, see newChoiceFields: a shell tab, as none, then the layouts, then the commands, each listed with
// how many tabs and panes it has, or what it runs. A layout and a command of the same name are both
// listed; name chooses the layout.
func newOpenFields(th *look.Theme, layouts []config.Layout, cmds []config.Command, name string) choiceFields[opener] {
	var openers []opener
	for i := range layouts {
		openers = append(openers, opener{layout: &layouts[i]})
	}
	for i := range cmds {
		openers = append(openers, opener{cmd: &cmds[i]})
	}
	return newChoiceFields(th, openZone, openers,
		func(o opener) string {
			if o.layout != nil {
				return o.layout.Name
			}
			return o.cmd.Name
		},
		func(o opener) []config.Input {
			if o.layout != nil {
				return o.layout.Inputs
			}
			return o.cmd.Inputs
		},
		func(o opener) string {
			if o.layout != nil {
				return layoutDetail(*o.layout)
			}
			return commandDetail(*o.cmd)
		},
		true, shellLayout, name, nil, nil)
}

// baseZone is the zone the choice of base is drawn in.
const baseZone = "from"

// newBaseFields are the fields that choose the base of a new branch in space: what the main worktree has
// checked out, by its branch, first and chosen, then the other local branches, the last committed to
// first.
func newBaseFields(th *look.Theme, space spaces.Space) choiceFields[string] {
	main := "HEAD"
	if len(space.Worktrees) > 0 && space.Worktrees[0].Branch != "" {
		main = space.Worktrees[0].Branch
	}
	others := slices.DeleteFunc(space.LocalBranches(), func(b string) bool { return b == main })
	return newChoiceFields(th, baseZone, others, func(b string) string { return b }, func(string) []config.Input { return nil },
		nil, false, &noChoice{name: main, detail: "the main worktree's"}, "", nil, nil)
}

// newWorktreeForm creates a worktree, on a branch to create or check out, on a pull request, or on a
// branch for an issue, a tab each, and, optionally, where its worktree goes, a layout and a command to run
// in it, shows what it will do, creates the worktree and switches to its session. The PR and Issue tabs'
// fields take a number, URL or title, the open ones they match listed below to choose from. It is meant
// for a kitty popup, as the sidebar is too narrow to type a name and read back the path in.
type newWorktreeForm struct {
	th    *look.Theme
	cfg   config.Config
	space spaces.Space
	tab   int
	tabs  [3]worktreeTab
	// The branch tab's branch. Typing #N or a PR's URL in it goes to the PR tab, an issue's URL to the
	// Issue tab.
	branch lineField
	// The PR tab's PR, and the Issue tab's issue and the branch it creates for it, filled in from the
	// issue until typed in: issueFilled is what it was last filled in with.
	prs         *ghPicker[gh.PRView, spaces.PR]
	issues      *ghPicker[gh.IssueView, gh.IssueView]
	issueBranch lineField
	issueFilled string
	// The choice of the tab shown as follow last saw it, so a new one clears the error.
	seen spaces.Ref
	// focus is the index in fields of the field that has focus.
	focus int
	// typed is the branch field as follow last saw it, so it works out what it names only when it
	// changes, not at every cursor blink.
	typed string
	// vars are the built-ins of the worktree the form would create now, which the inputs' defaults use.
	vars map[string]string
	// background opens the new session without switching to it, see spaces.OpenNew.
	background bool
	busy       bool
	err        error
	width      int
	height     int
	// zones and buttons are where the last frame drew its buttons, and what each runs, for a click.
	zones   map[string][]look.Zone
	buttons controls[*newWorktreeForm]
}

type createdMsg struct{ err error }

// newForm is the form for a worktree in space, on the branch tab. The branch starts with the configured
// prefix, so typing just the name gives PREFIX/NAME; deleting it makes a branch without one.
func newForm(cfg config.Config, space spaces.Space) *newWorktreeForm {
	th := look.New(true)
	start := spaces.BranchStart(cfg)
	branch := newLineField(th, "")
	branch.fill(start)
	issueBranch := newLineField(th, "")
	issueBranch.fill(start)
	f := &newWorktreeForm{th: th, cfg: cfg, space: space, branch: branch, prs: newGHPicker(th, prKind, space),
		issues: newGHPicker(th, issueKind, space), issueBranch: issueBranch, issueFilled: start, focus: 1}
	bases := newBaseFields(th, space)
	for i, layout := range [...]string{cfg.LayoutFor(space.Name, true), cfg.PRLayoutFor(space.Name), cfg.IssueLayoutFor(space.Name)} {
		f.tabs[i] = worktreeTab{
			base: bases,
			dir:  newLineField(th, ""),
			open: newOpenFields(th, cfg.Layouts, cfg.Commands, layout),
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

// shownPicker is the field of the tab shown that picks a PR or an issue, nil for the branch tab.
func (f *newWorktreeForm) shownPicker() tabPicker {
	switch f.tab {
	case tabPR:
		return f.prs
	case tabIssue:
		return f.issues
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
	switch f.tab {
	case tabBranch:
		branch = strings.TrimSpace(f.branch.Value())
	case tabPR:
		l, _ := f.prLookup()
		branch = l.found.Branch
	case tabIssue:
		branch = strings.TrimSpace(f.issueBranch.Value())
	}
	return branch, spaces.WorktreeDir(f.cfg, f.space.Root, branch, f.current().dir.Value())
}

func (f *newWorktreeForm) create() tea.Cmd {
	t := f.current()
	switch {
	case f.tab == tabPR && f.prs.chosen.Number == 0:
		f.err = fmt.Errorf("choose a PR to check out")
		return nil
	case f.tab == tabIssue && f.issues.chosen.Number == 0:
		f.err = fmt.Errorf("choose an issue to create a branch for")
		return nil
	}
	var layout *spaces.LayoutRun
	var run *spaces.Run
	if o, ok := t.open.chosen(); ok {
		var err error
		if o.layout != nil {
			var lr spaces.LayoutRun
			lr, err = spaces.NewLayoutRun(*o.layout, t.open.values(), f.vars)
			layout = &lr
		} else {
			var r spaces.Run
			r, err = spaces.NewRun(*o.cmd, t.open.values(), f.vars)
			run = &r
		}
		if err != nil {
			f.err = err
			return nil
		}
	}
	f.busy, f.err = true, nil
	cfg, space, background := f.cfg, f.space, f.background
	open := func(wt spaces.Worktree, links spaces.Links) error {
		if layout != nil {
			layout.Links = links
		}
		if run != nil {
			run.Links = links
		}
		return spaces.OpenNew(cfg, space, wt, layout, run, background)
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
	base := f.base()
	if f.tab == tabIssue {
		l, _ := f.issues.lookup()
		ref := f.issues.chosen
		return func() tea.Msg {
			// Recording an issue gh does not know would leave {issue} naming nothing.
			if !l.done || l.err != nil {
				if _, err := spaces.ResolveIssue(space, ref); err != nil {
					return createdMsg{err}
				}
			}
			wt, err := spaces.CreateIssueWorktree(space, ref.Number, branch, base, dir)
			if err != nil {
				return createdMsg{err}
			}
			return createdMsg{open(wt, spaces.Links{Issue: ref.Number})}
		}
	}
	return func() tea.Msg {
		wt, err := spaces.CreateWorktreeFrom(space, branch, base, dir)
		if err != nil {
			return createdMsg{err}
		}
		return createdMsg{open(wt, spaces.Links{})}
	}
}

// head are the fields before the layout's, in order: the tab strip, nil; the branch, the PR, or the issue
// and the branch for it; the base, nil, when there is a choice of one; the directory.
func (f *newWorktreeForm) head() []field {
	t := f.current()
	var out []field
	switch f.tab {
	case tabPR:
		return []field{nil, f.prs.input(), &t.dir}
	case tabIssue:
		out = []field{nil, f.issues.input(), &f.issueBranch}
	default:
		out = []field{nil, &f.branch}
	}
	if t.base.picks() {
		out = append(out, nil)
	}
	return append(out, &t.dir)
}

// fields are the form's fields in order: head; the choice of what the session opens with, nil, when
// there is one, and the chosen one's inputs; the open in background box, nil.
func (f *newWorktreeForm) fields() []field {
	t := f.current()
	out := f.head()
	if t.open.picks() {
		out = append(out, nil)
	}
	return append(append(out, t.open.fields()...), nil)
}

// baseAt and openAt are where in fields the choices of base and of what the session opens with are,
// when there are.
func (f *newWorktreeForm) baseAt() int { return len(f.head()) - 2 }
func (f *newWorktreeForm) openAt() int { return len(f.head()) }

// base is the branch a new branch starts from, empty for what the main worktree has checked out.
func (f *newWorktreeForm) base() string {
	if f.tab == tabPR {
		return ""
	}
	b, _ := f.current().base.chosen()
	return b
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
	f.issues.field.Blur()
	f.issueBranch.Blur()
	for i := range f.tabs {
		t := &f.tabs[i]
		t.dir.Blur()
		for _, fl := range t.open.fields() {
			fl.Blur()
		}
	}
	return moveFocus(f.fields(), &f.focus, 0)
}

// nextField and prevField move typing to the field below or above, round from the last to the first.
func (f *newWorktreeForm) nextField() tea.Cmd { return moveFocus(f.fields(), &f.focus, 1) }
func (f *newWorktreeForm) prevField() tea.Cmd { return moveFocus(f.fields(), &f.focus, -1) }

// onTabs, onBase and onOpen say whether the tab strip, the choice of base or of what the session opens
// with has focus, onPicker whether the field of a tab that picks a PR or an issue has.
func (f *newWorktreeForm) onTabs() bool   { return f.focus == 0 }
func (f *newWorktreeForm) onPicker() bool { return f.shownPicker() != nil && f.focus == 1 }
func (f *newWorktreeForm) onBase() bool {
	return f.tab != tabPR && f.current().base.picks() && f.focus == f.baseAt()
}
func (f *newWorktreeForm) onOpen() bool {
	return f.current().open.picks() && f.focus == f.openAt()
}

// onBackground says whether the open in background box has focus, the last of fields.
func (f *newWorktreeForm) onBackground() bool { return f.focus == len(f.fields())-1 }

// toggleBackground ticks or clears the open in background box.
func (f *newWorktreeForm) toggleBackground() { f.background = !f.background }

// onChoice says whether a choice has focus, which left and right change.
func (f *newWorktreeForm) onChoice() bool { return f.onTabs() || f.onBase() || f.onOpen() }

// showTab shows tab, focus on field at, and lists the open ones the first time it is a tab that picks.
func (f *newWorktreeForm) showTab(tab, at int) tea.Cmd {
	f.tab, f.focus, f.err = tab, at, nil
	cmd := tea.Batch(f.refocus(), f.follow())
	if p := f.shownPicker(); p != nil {
		return tea.Batch(cmd, p.listOpen())
	}
	return cmd
}

// cycleBase and cycleOpen choose the next base or thing to open with, or the one before, and focus the
// choice, as what follows it changes with it.
func (f *newWorktreeForm) cycleBase(delta int) tea.Cmd {
	f.current().base.cycle(delta)
	f.focus = f.baseAt()
	return tea.Batch(f.follow(), moveFocus(f.fields(), &f.focus, 0))
}

func (f *newWorktreeForm) cycleOpen(delta int) tea.Cmd {
	f.current().open.cycle(delta)
	f.focus = f.openAt()
	return moveFocus(f.fields(), &f.focus, 0)
}

// cycleChoice changes the choice that has focus.
func (f *newWorktreeForm) cycleChoice(delta int) tea.Cmd {
	switch {
	case f.onTabs():
		return f.showTab((f.tab+delta+len(f.tabs))%len(f.tabs), 0)
	case f.onBase():
		return f.cycleBase(delta)
	}
	return f.cycleOpen(delta)
}

// openList opens the list of the choice that has focus.
func (f *newWorktreeForm) openList() {
	if t := f.current(); f.onBase() {
		t.base.openList()
	} else {
		t.open.openList()
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
		case t.base.listOpen():
			cmd, picked := t.base.updateList(msg, f.zones)
			if picked {
				return f, tea.Batch(cmd, f.cycleBase(0))
			}
			return f, cmd
		case t.open.listOpen():
			cmd, picked := t.open.updateList(msg, f.zones)
			if picked {
				return f, tea.Batch(cmd, f.cycleOpen(0))
			}
			return f, cmd
		}
	}
	for _, p := range []tabPicker{f.prs, f.issues} {
		if cmd, ok := p.update(msg); ok {
			return f, tea.Batch(cmd, f.follow())
		}
	}
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		f.width, f.height = msg.Width, msg.Height
		// bubbles v2 draws only the first character of a placeholder without a width. The fields start
		// after the padding and their labels, and the cursor takes a cell past the text.
		w := max(f.width-4-labelWidth-1, 1)
		f.branch.SetWidth(w)
		f.prs.field.SetWidth(w)
		f.issues.field.SetWidth(w)
		f.issueBranch.SetWidth(w)
		for i := range f.tabs {
			f.tabs[i].dir.SetWidth(w)
			f.tabs[i].open.setWidth(w)
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
		if f.tab != tabPR && clickedChoice(f.zones, baseZone, msg) {
			t.base.openList()
			return f, f.cycleBase(0)
		}
		if clickedChoice(f.zones, openZone, msg) {
			t.open.openList()
			return f, f.cycleOpen(0)
		}
		if clickedChoice(f.zones, backgroundZone, msg) {
			f.toggleBackground()
			f.focus = len(f.fields()) - 1
			return f, moveFocus(f.fields(), &f.focus, 0)
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

// follow works out what the branch field names, going to the PR tab for a PR and the Issue tab for an
// issue, and what the pickers' fields do, choosing what they name; fills the issue's branch in again while
// left as it was; makes the directory's placeholder the default for the branch the form would create, so
// the form always shows where the worktree would go; and fills in again the inputs left at their defaults,
// which follow it too.
func (f *newWorktreeForm) follow() tea.Cmd {
	var cmds []tea.Cmd
	if typed := f.branch.Value(); typed != f.typed {
		f.typed = typed
		start := spaces.BranchStart(f.cfg)
		rest := strings.TrimPrefix(strings.TrimSpace(typed), start)
		switch number, hash := strings.CutPrefix(rest, "#"); {
		case hash || spaces.IsPRURL(rest):
			// Typing goes on in the PR field, from what follows the #, or with the URL in it.
			if hash {
				rest = number
			}
			f.branch.fill(start)
			f.typed = f.branch.Value()
			f.prs.field.fill(rest)
			cmds = append(cmds, f.showTab(tabPR, 1))
		case spaces.IsIssueURL(rest):
			f.branch.fill(start)
			f.typed = f.branch.Value()
			f.issues.field.fill(rest)
			cmds = append(cmds, f.showTab(tabIssue, 1))
		}
	}
	cmds = append(cmds, f.prs.refollow(), f.issues.refollow())
	if p := f.shownPicker(); p != nil && p.choice() != f.seen {
		f.seen, f.err = p.choice(), nil
	}
	if f.issueBranch.Value() == f.issueFilled {
		want := spaces.BranchStart(f.cfg)
		if n := f.issues.chosen.Number; n != 0 {
			want = spaces.IssueBranch(f.cfg, n, f.issues.title())
		}
		f.issueBranch.fill(want)
		f.issueFilled = want
	}
	t := f.current()
	branch, planned := f.plan()
	dir := spaces.WorktreeDir(f.cfg, f.space.Root, branch, "")
	if rel, err := filepath.Rel(f.space.Root, dir); err == nil {
		dir = rel
	}
	t.dir.Placeholder = dir
	f.vars = spaces.BuiltinVars(f.space, spaces.LinkedWorktree(f.space, branch, planned),
		spaces.Links{PR: f.chosen(tabPR), Issue: f.chosen(tabIssue), Base: f.base()})
	t.open.setVars(f.vars)
	return tea.Batch(cmds...)
}

// chosen is the number of the PR or issue chosen on tab while it is shown, else 0.
func (f *newWorktreeForm) chosen(tab int) int {
	if p := f.shownPicker(); p != nil && f.tab == tab {
		return p.choice().Number
	}
	return 0
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
	if f.tab == tabIssue {
		lines = append(lines, fieldLines(th, "branch", &f.issueBranch, width)...)
	}
	baseAt := len(lines)
	if f.tab != tabPR {
		lines = append(lines, t.base.lines(f.onBase(), width)...)
	}
	lines = append(lines, fieldLines(th, "directory", &t.dir, width)...)
	openAt := len(lines)
	lines = append(lines, t.open.lines(f.onOpen(), width)...)
	lines = append(lines, f.backgroundLine(width), "")
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
		if f.tab == tabIssue && f.issues.chosen.Number != 0 {
			lines = append(lines, muted(f.branchDoes(branch, "")))
		}
	default:
		lines = append(lines, muted(f.branchDoes(branch, ", or # for a PR")))
	}
	switch o, ok := t.open.chosen(); {
	case ok && o.layout != nil:
		lines = append(lines, muted("Opens with the "+o.layout.Name+" layout"))
	case ok:
		lines = append(lines, muted("Runs "+o.cmd.Name+" in the first tab"))
	}
	if f.background {
		lines = append(lines, muted("Stays here, without switching to the new session"))
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
	if t.base.listOpen() || t.open.listOpen() {
		lines = t.base.drawList(lines, baseAt, width, f.height)
		lines = t.open.drawList(lines, openAt, width, f.height)
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
	case f.onBase():
		hints = append(hints, choiceHints("base")...)
	case f.onOpen():
		hints = append(hints, choiceHints("open with")...)
	case f.onBackground():
		hints = append(hints, look.Text("  "), look.Key("space"), look.KeyDesc(" tick"))
	}
	hints = append(hints, newlineHints(f.focused())...)
	lines = append(lines, line(append(buttons, hints...)...))
	return boxed(lines)
}

// backgroundLine draws the open in background box after its label, its keys while it has focus.
func (f *newWorktreeForm) backgroundLine(width int) string {
	th := f.th
	spans := look.Mark(backgroundZone, append(look.BoxOf(f.background, f.busy), look.Text("Open in the background"))...)
	if room := width - labelWidth - th.Width(spans...); f.onBackground() && room > 0 {
		spans = append(spans, th.Cut(room, look.Muted("  space tick"))...)
	}
	return th.DrawCut(look.Plain, labelWidth-1, look.Label("background")) + " " + th.Render(look.Plain, spans...)
}

// branchDoes is what creating the worktree does with branch, or, while it is not a branch's name, to
// name one, or what else to type.
func (f *newWorktreeForm) branchDoes(branch, or string) string {
	switch base := f.base(); {
	case branch == "" || strings.HasSuffix(branch, "/"):
		return "Name the branch to create or check out" + or
	case f.space.HasBranch(branch):
		return "Checks out the existing branch " + branch
	case base != "":
		return "Creates " + branch + " from " + base
	case len(f.space.Worktrees) > 0 && f.space.Worktrees[0].Branch != "":
		return "Creates " + branch + " from " + f.space.Worktrees[0].Branch
	}
	return "Creates " + branch
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

// startOn starts the form on tab, whose field is p's, filled in with arg, a number or URL of what p picks,
// when not empty.
func startOn[V, L any](f *newWorktreeForm, tab int, p *ghPicker[V, L], arg string) error {
	if arg != "" {
		if _, ok := p.kind.parse(arg); !ok {
			return fmt.Errorf("%s is not %s's number or URL", arg, p.kind.one)
		}
		p.field.fill(arg)
	}
	f.tab, f.focus = tab, 1
	f.refocus()
	f.follow()
	return nil
}

// startFrom starts the choice of base of the branch and Issue tabs on base, a local branch, when not
// empty: the branch of the worktree whose row the form was opened from.
func (f *newWorktreeForm) startFrom(base string) error {
	if base == "" {
		return nil
	}
	for i := range f.tabs {
		t := &f.tabs[i].base
		t.choose(base)
		if _, ok := t.chosen(); !ok && base != t.none.name {
			return fmt.Errorf("%s is not a local branch", base)
		}
	}
	f.follow()
	return nil
}

// RunNewWorktree runs the new worktree form for space, with the config as the space sees it, meant for a
// kitty popup, until the worktree is created and opened or the form is left. It starts on the tab from
// says, on arg, a PR's or an issue's number or URL, when not empty, and a new branch's base on base, a
// local branch, when not empty.
func RunNewWorktree(ctx context.Context, cfg config.Config, space spaces.Space, from spaces.From, arg, base string) error {
	f := newForm(space.ConfigOf(cfg), space)
	err := f.startFrom(base)
	switch {
	case err != nil:
	case from == spaces.FromPR:
		err = startOn(f, tabPR, f.prs, arg)
	case from == spaces.FromIssue:
		err = startOn(f, tabIssue, f.issues, arg)
	}
	if err != nil {
		return err
	}
	if err := runPopup(ctx, f); err != nil {
		return fmt.Errorf("running the new worktree form: %w", err)
	}
	return nil
}
