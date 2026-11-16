package tui

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/chancez/kspaces/internal/gh"
	"github.com/chancez/kspaces/internal/spaces"
	"github.com/chancez/kspaces/internal/tui/look"
)

// pickRowZone is the zone of the row of open ones below a picker's field, by index.
const pickRowZone = "pick:%d"

// pickRows is how many of the open ones show below a picker's field at once.
const pickRows = 5

// pickDebounce is how long a picker's field rests on one no open one is before it is looked up, so
// typing a number does not run gh for every digit.
const pickDebounce = 300 * time.Millisecond

// ghItem is what a picker shows of a PR or an issue, and matches what is typed against.
type ghItem struct {
	number int
	title  string
	author string
	draft  bool
	url    string
}

// detail is who opened it, and whether it is a draft.
func (it ghItem) detail() string {
	if it.draft {
		return it.author + ", draft"
	}
	return it.author
}

// itemName is how the form names number, title as known.
func itemName(number int, title string) string {
	if title == "" {
		return fmt.Sprintf("#%d", number)
	}
	return fmt.Sprintf("#%d %s", number, title)
}

// ghKind is what a picker chooses among, V as gh lists them, and L what choosing one comes to.
type ghKind[V, L any] struct {
	// What the form calls one: as a label, such as PR, at the start of a line, and after "type", as in
	// "a PR"; and several, as in "the open PRs".
	label, start, one, many string
	item                    func(V) ghItem
	list                    func(root string) ([]V, error)
	parse                   func(string) (spaces.Ref, bool)
	isURL                   func(string) bool
	// open is what choosing an open one comes to, from what gh listed, and resolve what looking one up
	// with gh does. Both run off the UI loop.
	open    func(spaces.Space, V) (L, error)
	resolve func(spaces.Space, spaces.Ref) (L, error)
	// title is the title of what a lookup found.
	title func(L) string
}

// prKind is what the PR tab chooses among: the open PRs, each checked out as CheckoutPR works out.
var prKind = &ghKind[gh.PRView, spaces.PR]{
	label: "PR", start: "PR", one: "a PR", many: "PRs",
	item: func(v gh.PRView) ghItem {
		return ghItem{number: v.Number, title: v.Title, author: v.Author, draft: v.Draft, url: v.URL}
	},
	list:    gh.OpenPRs,
	parse:   spaces.ParsePRRef,
	isURL:   spaces.IsPRURL,
	open:    spaces.CheckoutPR,
	resolve: spaces.ResolvePR,
	title:   func(pr spaces.PR) string { return pr.Title },
}

// issueKind is what the Issue tab chooses among: the open issues, choosing one needing nothing more.
var issueKind = &ghKind[gh.IssueView, gh.IssueView]{
	label: "issue", start: "Issue", one: "an issue", many: "issues",
	item: func(v gh.IssueView) ghItem {
		return ghItem{number: v.Number, title: v.Title, author: v.Author, url: v.URL}
	},
	list:    gh.OpenIssues,
	parse:   spaces.ParseIssueRef,
	isURL:   spaces.IsIssueURL,
	open:    func(_ spaces.Space, v gh.IssueView) (gh.IssueView, error) { return v, nil },
	resolve: spaces.ResolveIssue,
	title:   func(v gh.IssueView) string { return v.Title },
}

// ghPicker is a field that chooses a PR or an issue, by its number, URL or title: while it has focus the
// open ones it matches show below it to choose from, the one highlighted chosen. One that is not open is
// looked up with gh once the field rests on it.
type ghPicker[V, L any] struct {
	kind  *ghKind[V, L]
	space spaces.Space
	field lineField
	// The open ones, once listed; those the field matches, by index in them, and the one of those
	// highlighted.
	open    listed[V]
	matches []int
	sel     int
	// The one chosen, its Number 0 for none: the highlighted match, else the one the field names. Its
	// lookups, by gh's argument; seq counts the changes of the one chosen, so only a lookup it rested on
	// starts.
	chosen  spaces.Ref
	lookups map[string]lookup[L]
	seq     int
	// typed is the field as refollow last saw it, so it works out what it names only when it changes.
	typed string
}

// listed are the open ones as gh lists them, once asked, until done.
type listed[V any] struct {
	views       []V
	err         error
	asked, done bool
}

// lookup is what looking one up found, or, until done, that it is under way.
type lookup[L any] struct {
	found L
	err   error
	done  bool
}

// listedMsg is the list of open ones, dueMsg the field having rested on the one chosen, change seq of
// it, for pickDebounce, and foundMsg the lookup of the one gh knows as arg.
type listedMsg[V any] struct {
	views []V
	err   error
}

type dueMsg[V any] struct{ seq int }

type foundMsg[L any] struct {
	arg string
	lookup[L]
}

// tabPicker is a picker as the new worktree form's tabs use it, whatever it picks.
type tabPicker interface {
	input() *lineField
	name() string
	choice() spaces.Ref
	matched() int
	listOpen() tea.Cmd
	lookUp() tea.Cmd
	refollow() tea.Cmd
	update(msg tea.Msg) (tea.Cmd, bool)
	move(delta int) tea.Cmd
	confirm(i int) tea.Cmd
	lines(th *look.Theme, focused bool, width int) []string
	status(muted func(string) string) ([]string, error)
}

func newGHPicker[V, L any](th *look.Theme, kind *ghKind[V, L], space spaces.Space) *ghPicker[V, L] {
	return &ghPicker[V, L]{kind: kind, space: space, field: newLineField(th, "number, URL or title"), lookups: map[string]lookup[L]{}}
}

func (p *ghPicker[V, L]) input() *lineField { return &p.field }

// name is what the form calls one, as a label.
func (p *ghPicker[V, L]) name() string { return p.kind.label }

// choice is the one chosen, its Number 0 for none.
func (p *ghPicker[V, L]) choice() spaces.Ref { return p.chosen }

// matched is how many open ones the field matches.
func (p *ghPicker[V, L]) matched() int { return len(p.matches) }

// lookup is what looking up the one chosen has found so far, ok false for none chosen.
func (p *ghPicker[V, L]) lookup() (l lookup[L], ok bool) {
	if p.chosen.Number == 0 {
		return lookup[L]{}, false
	}
	return p.lookups[p.chosen.Arg], true
}

// openIndex is the index in the open ones of the one ref names, by its number or a URL of its page, -1
// when it is none of them.
func (p *ghPicker[V, L]) openIndex(ref spaces.Ref) int {
	return slices.IndexFunc(p.open.views, func(v V) bool {
		it := p.kind.item(v)
		return it.number == ref.Number && (ref.Arg == strconv.Itoa(ref.Number) || strings.HasPrefix(ref.Arg, it.url))
	})
}

// listOpen asks gh for the open ones, the first time.
func (p *ghPicker[V, L]) listOpen() tea.Cmd {
	if p.open.asked {
		return nil
	}
	p.open.asked = true
	root, list := p.space.Root, p.kind.list
	return func() tea.Msg {
		views, err := list(root)
		return listedMsg[V]{views, err}
	}
}

// match matches the open ones against the field, the first highlighted: those with what is typed in
// their number, title or author, the one of its number first; for a URL, the open one it is.
func (p *ghPicker[V, L]) match() {
	text := strings.TrimSpace(p.field.Value())
	ref, isRef := p.kind.parse(text)
	p.matches, p.sel = nil, 0
	if p.kind.isURL(text) {
		if i := p.openIndex(ref); i >= 0 {
			p.matches = []int{i}
		}
		return
	}
	q := strings.ToLower(strings.TrimPrefix(text, "#"))
	for i, v := range p.open.views {
		it := p.kind.item(v)
		switch {
		case isRef && it.number == ref.Number:
			p.matches = slices.Insert(p.matches, 0, i)
		case strings.Contains(strings.ToLower(itemName(it.number, it.title)+" "+it.author), q):
			p.matches = append(p.matches, i)
		}
	}
}

// chosenRef is the one to choose now: the highlighted match, else the one the field names, else none.
func (p *ghPicker[V, L]) chosenRef() spaces.Ref {
	if len(p.matches) > 0 {
		n := p.kind.item(p.open.views[p.matches[p.sel]]).number
		return spaces.Ref{Arg: strconv.Itoa(n), Number: n}
	}
	ref, _ := p.kind.parse(p.field.Value())
	return ref
}

// choose chooses ref, unless it is chosen: an open one is looked up at once, from what gh listed, and
// any other once the field has rested on it.
func (p *ghPicker[V, L]) choose(ref spaces.Ref) tea.Cmd {
	if ref == p.chosen {
		return nil
	}
	p.chosen = ref
	p.seq++
	if ref.Number == 0 {
		return nil
	}
	if p.openIndex(ref) >= 0 {
		return p.lookUp()
	}
	seq := p.seq
	return tea.Tick(pickDebounce, func(time.Time) tea.Msg { return dueMsg[V]{seq} })
}

// lookUp looks up the one chosen, unless it has been: from the open ones when it is one, else with gh.
func (p *ghPicker[V, L]) lookUp() tea.Cmd {
	ref := p.chosen
	if _, asked := p.lookups[ref.Arg]; ref.Number == 0 || asked {
		return nil
	}
	p.lookups[ref.Arg] = lookup[L]{}
	space, kind, i := p.space, p.kind, p.openIndex(ref)
	var view V
	if i >= 0 {
		view = p.open.views[i]
	}
	return func() tea.Msg {
		var found L
		var err error
		if i >= 0 {
			found, err = kind.open(space, view)
		} else {
			found, err = kind.resolve(space, ref)
		}
		return foundMsg[L]{ref.Arg, lookup[L]{found: found, err: err, done: true}}
	}
}

// refollow works out what the field names when it has changed, choosing it.
func (p *ghPicker[V, L]) refollow() tea.Cmd {
	if typed := p.field.Value(); typed != p.typed {
		p.typed = typed
		p.match()
		return p.choose(p.chosenRef())
	}
	return nil
}

// update takes the picker's own messages, saying whether msg was one.
func (p *ghPicker[V, L]) update(msg tea.Msg) (tea.Cmd, bool) {
	switch msg := msg.(type) {
	case listedMsg[V]:
		p.open.views, p.open.err, p.open.done = msg.views, msg.err, true
		p.match()
		return p.choose(p.chosenRef()), true
	case dueMsg[V]:
		if msg.seq != p.seq {
			return nil, true
		}
		return p.lookUp(), true
	case foundMsg[L]:
		p.lookups[msg.arg] = msg.lookup
		return nil, true
	}
	return nil, false
}

// move highlights the open one delta rows from the one highlighted, and chooses it.
func (p *ghPicker[V, L]) move(delta int) tea.Cmd {
	p.sel = max(min(p.sel+delta, len(p.matches)-1), 0)
	return p.choose(p.chosenRef())
}

// confirm chooses the open one of row i below the field and fills the field in with its number.
func (p *ghPicker[V, L]) confirm(i int) tea.Cmd {
	n := p.kind.item(p.open.views[p.matches[i]]).number
	ref := spaces.Ref{Arg: strconv.Itoa(n), Number: n}
	p.field.fill("#" + ref.Arg)
	p.typed = p.field.Value()
	p.match()
	return p.choose(ref)
}

// lines draw the field and, while it has focus, the open ones it matches below it, as many as pickRows,
// the one highlighted on the cursor's surface, or that they are being listed. Without focus the field
// shows the one chosen.
func (p *ghPicker[V, L]) lines(th *look.Theme, focused bool, width int) []string {
	label := p.kind.label
	if !focused {
		if p.chosen.Number == 0 {
			return fieldLines(th, label, &p.field, width)
		}
		title := p.title()
		return []string{th.DrawZone(fieldZone(&p.field), look.Plain, width,
			th.Cut(width, look.Label(fmt.Sprintf("%-*s", labelWidth, label)), look.Text(itemName(p.chosen.Number, title)))...)}
	}
	out := fieldLines(th, label, &p.field, width)
	indent := look.Text(strings.Repeat(" ", labelWidth))
	if p.open.asked && !p.open.done {
		return append(out, th.Render(look.Plain, indent, look.Muted("Listing open "+p.kind.many+"...")))
	}
	top := max(min(p.sel-pickRows/2, len(p.matches)-pickRows), 0)
	for i := top; i < min(top+pickRows, len(p.matches)); i++ {
		it := p.kind.item(p.open.views[p.matches[i]])
		surface := look.Plain
		if i == p.sel {
			surface = look.Cursor
		}
		row := th.Cut(width-labelWidth, look.Text(" "+itemName(it.number, it.title)), look.Muted("  "+it.detail()))
		out = append(out, th.Render(look.Plain, indent)+th.DrawZone(fmt.Sprintf(pickRowZone, i), surface, width-labelWidth, row...))
	}
	if more := len(p.matches) - min(top+pickRows, len(p.matches)) + top; more > 0 {
		out = append(out, th.Render(look.Plain, indent, look.Muted(fmt.Sprintf(" %d more", more))))
	}
	return out
}

// title is the title of the one chosen, once known: from the open ones, or from looking it up.
func (p *ghPicker[V, L]) title() string {
	if i := p.openIndex(p.chosen); i >= 0 {
		return p.kind.item(p.open.views[i]).title
	}
	if l := p.lookups[p.chosen.Arg]; l.done && l.err == nil {
		return p.kind.title(l.found)
	}
	return ""
}

// status says, with muted, how looking up the one chosen goes, or, with none chosen, what listing the
// open ones found; and the error to show, if any.
func (p *ghPicker[V, L]) status(muted func(string) string) ([]string, error) {
	hint := ": type " + p.kind.one + "'s number or URL"
	if l, ok := p.lookup(); ok {
		name := fmt.Sprintf("%s #%d", p.kind.start, p.chosen.Number)
		switch {
		case !l.done:
			return []string{muted("Looking up " + name + "...")}, nil
		case l.err != nil:
			return []string{muted("Could not look up " + name)}, l.err
		}
		return []string{muted(name + " " + p.kind.title(l.found))}, nil
	}
	switch {
	case p.open.err != nil:
		return []string{muted("Could not list the open " + p.kind.many + hint)}, p.open.err
	case p.open.done && len(p.open.views) == 0:
		return []string{muted("No open " + p.kind.many + hint)}, nil
	case p.open.done:
		return []string{muted("No open " + p.kind.label + " matches" + hint)}, nil
	}
	return nil, nil
}
