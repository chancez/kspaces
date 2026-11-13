package tui

import (
	"context"
	"fmt"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

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

// The zones of the tab strip's tabs, and of the rows of open PRs below the PR field, by index.
const (
	tabZone   = "tab:%d"
	prRowZone = "pr:%d"
)

// prRows is how many of the open PRs show below the PR field at once.
const prRows = 5

// prDebounce is how long the PR field rests on a PR no open one is before it is looked up, so typing a
// number does not run gh for every digit.
const prDebounce = 300 * time.Millisecond

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
	// The PR tab's field, what it is matched against: the open PRs, those it matches, by index in them,
	// and the one of those highlighted.
	prField lineField
	open    openPRs
	matches []int
	sel     int
	// The PR chosen, its Number 0 for none: the highlighted match, else the PR the field names. Its
	// lookups, by gh's argument; prSeq counts the changes of the PR chosen, so only a lookup it rested on
	// starts.
	pr      spaces.PRRef
	lookups map[string]prLookup
	prSeq   int
	// focus is the index in fields of the field that has focus.
	focus int
	// typed and prTyped are the branch and PR fields as follow last saw them, so it works out what they
	// name only when they change, not at every cursor blink.
	typed, prTyped string
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

// openPRs are the repository's open PRs as gh lists them, once asked, until done.
type openPRs struct {
	views       []gh.PRView
	err         error
	asked, done bool
}

// prLookup is what looking a PR up found, or, until done, that it is under way.
type prLookup struct {
	pr   spaces.PR
	err  error
	done bool
}

// openPRsMsg is the list of open PRs, prDueMsg the PR field having rested on the PR chosen, change seq of
// it, for prDebounce, and prFoundMsg the lookup of the PR gh knows as arg.
type openPRsMsg struct {
	views []gh.PRView
	err   error
}

type prDueMsg struct{ seq int }

type prFoundMsg struct {
	arg string
	prLookup
}

type createdMsg struct{ err error }

// newForm is the form for a worktree in space, on the branch tab. The branch starts with the configured
// prefix, so typing just the name gives PREFIX/NAME; deleting it makes a branch without one.
func newForm(cfg config.Config, space spaces.Space) *newWorktreeForm {
	th := look.New(true)
	branch := newLineField(th, "")
	branch.fill(spaces.BranchStart(cfg))
	f := &newWorktreeForm{th: th, cfg: cfg, space: space, branch: branch, prField: newLineField(th, "number, URL or title"),
		lookups: map[string]prLookup{}, focus: 1}
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

// Init lists the open PRs when the form starts on the PR tab, and looks up the PR its field names.
func (f *newWorktreeForm) Init() tea.Cmd {
	if f.tab != tabPR {
		return textinput.Blink
	}
	return tea.Batch(textinput.Blink, f.listOpen(), f.lookUp())
}

// current is the tab shown.
func (f *newWorktreeForm) current() *worktreeTab { return &f.tabs[f.tab] }

// lookup is what looking up the PR chosen has found so far, ok false when the PR tab is not shown or has
// none chosen.
func (f *newWorktreeForm) lookup() (l prLookup, ok bool) {
	if f.tab != tabPR || f.pr.Number == 0 {
		return prLookup{}, false
	}
	return f.lookups[f.pr.Arg], true
}

// plan is the branch and directory the form would create now. For a PR that is its local branch, empty
// until it has been looked up.
func (f *newWorktreeForm) plan() (branch, dir string) {
	branch = strings.TrimSpace(f.branch.Value())
	if f.tab == tabPR {
		l, _ := f.lookup()
		branch = l.pr.Branch
	}
	return branch, spaces.WorktreeDir(f.cfg, f.space.Root, branch, f.current().dir.Value())
}

func (f *newWorktreeForm) create() tea.Cmd {
	t := f.current()
	if f.tab == tabPR && f.pr.Number == 0 {
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
		ref, dir := f.pr, t.dir.Value()
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

// fields are the form's fields in order: the tab strip, nil; the branch, or the PR; the directory; the
// choice of layout, nil, when there is one, and the chosen layout's inputs; then the same for the command.
func (f *newWorktreeForm) fields() []field {
	t := f.current()
	out := []field{nil, &f.branch, &t.dir}
	if f.tab == tabPR {
		out[1] = &f.prField
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
	f.prField.Blur()
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
// onPR whether the PR field has.
func (f *newWorktreeForm) onTabs() bool { return f.focus == 0 }
func (f *newWorktreeForm) onPR() bool   { return f.tab == tabPR && f.focus == 1 }
func (f *newWorktreeForm) onLayout() bool {
	return f.current().layouts.picks() && f.focus == f.layoutAt()
}
func (f *newWorktreeForm) onCommand() bool {
	return f.current().cmds.picks() && f.focus == f.commandAt()
}

// onChoice says whether a choice has focus, which left and right change.
func (f *newWorktreeForm) onChoice() bool { return f.onTabs() || f.onLayout() || f.onCommand() }

// showTab shows tab, focus on field at, and lists the open PRs the first time it is the PR tab.
func (f *newWorktreeForm) showTab(tab, at int) tea.Cmd {
	f.tab, f.focus, f.err = tab, at, nil
	cmd := tea.Batch(f.refocus(), f.follow())
	if tab == tabPR {
		return tea.Batch(cmd, f.listOpen())
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

// prName is how the form names PR number, title as known.
func prName(number int, title string) string {
	if title == "" {
		return fmt.Sprintf("#%d", number)
	}
	return fmt.Sprintf("#%d %s", number, title)
}

// prDetail is who opened v, and whether it is a draft, for the open PRs below the PR field.
func prDetail(v gh.PRView) string {
	if v.Draft {
		return v.Author + ", draft"
	}
	return v.Author
}

// viewRef is v as a PRRef.
func viewRef(v gh.PRView) spaces.PRRef {
	return spaces.PRRef{Arg: strconv.Itoa(v.Number), Number: v.Number}
}

// openIndex is the index in the open PRs of the PR ref names, by its number or a URL of its page, -1 when
// it is none of them.
func (f *newWorktreeForm) openIndex(ref spaces.PRRef) int {
	return slices.IndexFunc(f.open.views, func(v gh.PRView) bool {
		return v.Number == ref.Number && (ref.Arg == strconv.Itoa(ref.Number) || strings.HasPrefix(ref.Arg, v.URL))
	})
}

// listOpen asks gh for the open PRs, the first time.
func (f *newWorktreeForm) listOpen() tea.Cmd {
	if f.open.asked {
		return nil
	}
	f.open.asked = true
	root := f.space.Root
	return func() tea.Msg {
		views, err := gh.OpenPRs(root)
		return openPRsMsg{views, err}
	}
}

// matchPRs matches the open PRs against the PR field, the first highlighted: those with what is typed in
// their number, title or author, the one of its number first; for a PR's URL, the open PR it is.
func (f *newWorktreeForm) matchPRs() {
	text := strings.TrimSpace(f.prField.Value())
	ref, isRef := spaces.ParsePRRef(text)
	f.matches, f.sel = nil, 0
	if spaces.IsPRURL(text) {
		if i := f.openIndex(ref); i >= 0 {
			f.matches = []int{i}
		}
		return
	}
	q := strings.ToLower(strings.TrimPrefix(text, "#"))
	for i, v := range f.open.views {
		switch {
		case isRef && v.Number == ref.Number:
			f.matches = slices.Insert(f.matches, 0, i)
		case strings.Contains(strings.ToLower(prName(v.Number, v.Title)+" "+v.Author), q):
			f.matches = append(f.matches, i)
		}
	}
}

// chosenRef is the PR the PR tab would check out now: the highlighted match, else the one the field
// names, else none.
func (f *newWorktreeForm) chosenRef() spaces.PRRef {
	if len(f.matches) > 0 {
		return viewRef(f.open.views[f.matches[f.sel]])
	}
	ref, _ := spaces.ParsePRRef(f.prField.Value())
	return ref
}

// choosePR chooses ref, unless it is chosen: an open PR is looked up at once, from what gh listed, and
// any other once the field has rested on it.
func (f *newWorktreeForm) choosePR(ref spaces.PRRef) tea.Cmd {
	if ref == f.pr {
		return nil
	}
	f.pr, f.err = ref, nil
	f.prSeq++
	if ref.Number == 0 {
		return nil
	}
	if f.openIndex(ref) >= 0 {
		return f.lookUp()
	}
	seq := f.prSeq
	return tea.Tick(prDebounce, func(time.Time) tea.Msg { return prDueMsg{seq} })
}

// lookUp looks up the PR chosen, unless it has been: from the open PRs when it is one, else with gh. gh
// asks GitHub, and checking out works out what the space's branches are, so off the UI loop.
func (f *newWorktreeForm) lookUp() tea.Cmd {
	ref := f.pr
	if _, asked := f.lookups[ref.Arg]; ref.Number == 0 || asked {
		return nil
	}
	f.lookups[ref.Arg] = prLookup{}
	space, i := f.space, f.openIndex(ref)
	var view gh.PRView
	if i >= 0 {
		view = f.open.views[i]
	}
	return func() tea.Msg {
		var pr spaces.PR
		var err error
		if i >= 0 {
			pr, err = spaces.CheckoutPR(space, view)
		} else {
			pr, err = spaces.ResolvePR(space, ref)
		}
		return prFoundMsg{ref.Arg, prLookup{pr: pr, err: err, done: true}}
	}
}

// movePR highlights the open PR delta rows from the one highlighted, and chooses it.
func (f *newWorktreeForm) movePR(delta int) tea.Cmd {
	f.sel = max(min(f.sel+delta, len(f.matches)-1), 0)
	cmd := f.choosePR(f.chosenRef())
	return tea.Batch(cmd, f.follow())
}

// confirmPR chooses the open PR of row i below the PR field, fills the field in with its number and moves
// on to the directory, which closes the rows.
func (f *newWorktreeForm) confirmPR(i int) tea.Cmd {
	ref := viewRef(f.open.views[f.matches[i]])
	f.prField.fill("#" + ref.Arg)
	cmd := f.follow()
	f.focus = 2
	return tea.Batch(cmd, f.choosePR(ref), f.refocus())
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
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		f.width, f.height = msg.Width, msg.Height
		// bubbles v2 draws only the first character of a placeholder without a width. The fields start
		// after the padding and their labels, and the cursor takes a cell past the text.
		w := max(f.width-4-labelWidth-1, 1)
		f.branch.SetWidth(w)
		f.prField.SetWidth(w)
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
	case openPRsMsg:
		f.open.views, f.open.err, f.open.done = msg.views, msg.err, true
		f.matchPRs()
		cmd := f.choosePR(f.chosenRef())
		return f, tea.Batch(cmd, f.follow())
	case prDueMsg:
		if msg.seq != f.prSeq {
			return f, nil
		}
		return f, f.lookUp()
	case prFoundMsg:
		f.lookups[msg.arg] = msg.prLookup
		return f, f.follow()
	case tea.KeyPressMsg:
		if keptBy(f.focused(), msg) {
			break
		}
		// up and down go through the open PRs the PR field matches, rather than the fields.
		if f.onPR() && len(f.matches) > 1 {
			switch msg.Code {
			case tea.KeyUp:
				return f, f.movePR(-1)
			case tea.KeyDown:
				return f, f.movePR(1)
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
		if f.onPR() {
			for i := range f.matches {
				if clickedChoice(f.zones, fmt.Sprintf(prRowZone, i), msg) {
					return f, f.confirmPR(i)
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

// follow works out what the branch field names, going to the PR tab for a PR, and what the PR field
// does, choosing the PR; makes the directory's placeholder the default for the branch the form would
// create, so the form always shows where the worktree would go; and fills in again the inputs left at
// their defaults, which follow it too.
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
			f.prField.fill(rest)
			cmds = append(cmds, f.showTab(tabPR, 1))
		}
	}
	if typed := f.prField.Value(); typed != f.prTyped {
		f.prTyped = typed
		f.matchPRs()
		cmds = append(cmds, f.choosePR(f.chosenRef()))
	}
	t := f.current()
	branch, planned := f.plan()
	dir := spaces.WorktreeDir(f.cfg, f.space.Root, branch, "")
	if rel, err := filepath.Rel(f.space.Root, dir); err == nil {
		dir = rel
	}
	t.dir.Placeholder = dir
	f.vars = spaces.BuiltinVars(f.space, spaces.LinkedWorktree(f.space, branch, planned), f.prNumber())
	t.layouts.setVars(f.vars)
	t.cmds.setVars(f.vars)
	return tea.Batch(cmds...)
}

// prNumber is the number of the PR chosen while the PR tab is shown, else 0.
func (f *newWorktreeForm) prNumber() int {
	if f.tab != tabPR {
		return 0
	}
	return f.pr.Number
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

// prLines draw the PR field and, while it has focus, the open PRs it matches below it, as many as prRows,
// the one highlighted on the cursor's surface, or that they are being listed. Without focus the field
// shows the PR chosen.
func (f *newWorktreeForm) prLines(width int) []string {
	th := f.th
	if !f.onPR() {
		if f.pr.Number == 0 {
			return fieldLines(th, "PR", &f.prField, width)
		}
		title := ""
		if i := f.openIndex(f.pr); i >= 0 {
			title = f.open.views[i].Title
		} else if l := f.lookups[f.pr.Arg]; l.done && l.err == nil {
			title = l.pr.Title
		}
		return []string{th.DrawZone(fieldZone(&f.prField), look.Plain, width,
			th.Cut(width, look.Label(fmt.Sprintf("%-*s", labelWidth, "PR")), look.Text(prName(f.pr.Number, title)))...)}
	}
	out := fieldLines(th, "PR", &f.prField, width)
	indent := look.Text(strings.Repeat(" ", labelWidth))
	if f.open.asked && !f.open.done {
		return append(out, th.Render(look.Plain, indent, look.Muted("Listing open PRs...")))
	}
	top := max(min(f.sel-prRows/2, len(f.matches)-prRows), 0)
	for i := top; i < min(top+prRows, len(f.matches)); i++ {
		v := f.open.views[f.matches[i]]
		surface := look.Plain
		if i == f.sel {
			surface = look.Cursor
		}
		row := th.Cut(width-labelWidth, look.Text(" "+prName(v.Number, v.Title)), look.Muted("  "+prDetail(v)))
		out = append(out, th.Render(look.Plain, indent)+th.DrawZone(fmt.Sprintf(prRowZone, i), surface, width-labelWidth, row...))
	}
	if more := len(f.matches) - min(top+prRows, len(f.matches)) + top; more > 0 {
		out = append(out, th.Render(look.Plain, indent, look.Muted(fmt.Sprintf(" %d more", more))))
	}
	return out
}

func (f *newWorktreeForm) render() string {
	th, width := f.th, max(f.width-4, 20)
	line := func(spans ...look.Span) string { return th.Render(look.Plain, spans...) }
	muted := func(s string) string { return line(th.Cut(width, look.Muted(s))...) }
	t := f.current()
	branch, _ := f.plan()
	lines := []string{line(look.Heading("New worktree in " + f.space.Name)), "", f.tabStrip(width), ""}
	if f.tab == tabPR {
		lines = append(lines, f.prLines(width)...)
	} else {
		lines = append(lines, fieldLines(th, "branch", &f.branch, width)...)
	}
	lines = append(lines, fieldLines(th, "directory", &t.dir, width)...)
	layoutAt := len(lines)
	lines = append(lines, t.layouts.lines(f.onLayout(), width)...)
	commandAt := len(lines)
	lines = append(append(lines, t.cmds.lines(f.onCommand(), width)...), "")
	failed := f.err
	switch l, ok := f.lookup(); {
	case ok:
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
	case f.tab == tabPR && f.open.err != nil:
		lines = append(lines, muted("Could not list the open PRs: type a PR's number or URL"))
		if failed == nil {
			failed = f.open.err
		}
	case f.tab == tabPR && f.open.done && len(f.open.views) == 0:
		lines = append(lines, muted("No open PRs: type a PR's number or URL"))
	case f.tab == tabPR && f.open.done:
		lines = append(lines, muted("No open PR matches: type a PR's number or URL"))
	case f.tab == tabPR:
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
	case f.onPR() && len(f.matches) > 1:
		hints = append(hints, look.Text("  "), look.Key("up/down"), look.KeyDesc(" PR"))
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
		f.prField.fill(pr)
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
