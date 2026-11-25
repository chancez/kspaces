package tui

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/sahilm/fuzzy"

	"github.com/chancez/kspaces/internal/config"
	"github.com/chancez/kspaces/internal/kitty"
	"github.com/chancez/kspaces/internal/spaces"
	"github.com/chancez/kspaces/internal/status"
	"github.com/chancez/kspaces/internal/tui/look"
)

// pickItem is one thing the picker can switch to: a worktree's session, a window reporting a status, or
// any window.
type pickItem struct {
	// The text the query is matched against. It is also what is shown, so matches can be highlighted.
	text string
	// Where text stops being the name and becomes the faint detail after it, such as a branch.
	nameLen int
	icon    []look.Span

	// A worktree's row, with its space; the status, for a window reporting one; or the window.
	row    spaces.Row
	status *status.Status
	pane   *spaces.Pane
}

type pickItems []pickItem

func (p pickItems) String(i int) string { return p[i].text }
func (p pickItems) Len() int            { return len(p) }

// PickScope is what the picker lists: everything, the spaces with the status entries, or the windows.
type PickScope int

const (
	ScopeSpaces PickScope = iota
	ScopeWindows
	ScopeAll
)

// scopeNames are what kspaces pick --scope calls the scopes, scopeTitles the scope strip, by PickScope.
var (
	scopeNames  = [...]string{"spaces", "windows", "all"}
	scopeTitles = [...]string{"Spaces", "Windows", "All"}
)

// pickScopeZone is the zone of the scope strip's scopes, by PickScope.
const pickScopeZone = "scope:%d"

// ParsePickScope is the scope called name.
func ParsePickScope(name string) (PickScope, error) {
	for i, n := range scopeNames {
		if n == name {
			return PickScope(i), nil
		}
	}
	return 0, fmt.Errorf("unknown scope %q, want one of %s", name, strings.Join(scopeNames[:], ", "))
}

func (s PickScope) String() string { return scopeNames[s] }

// pickItemsFrom lists what scope has: the windows that report a status first, most urgent first, then
// every session and worktree in sidebar order, a grouped space's under its group's name, so a query finds
// them by it; then the windows, see paneItems.
func pickItemsFrom(snap spaces.Snapshot, panes []spaces.Pane, scope PickScope) pickItems {
	var items pickItems
	rows := spaces.BuildRows(snap.Others, snap.Spaces, nil, nil, snap.Order)
	if scope != ScopeWindows {
		items = append(items, spaceItems(snap, rows)...)
	}
	if scope != ScopeSpaces {
		items = append(items, paneItems(snap, rows, panes)...)
	}
	return items
}

// spaceItems are the status entries, then the sessions and worktrees of rows.
func spaceItems(snap spaces.Snapshot, rows []spaces.Row) pickItems {
	var items pickItems
	for i := range snap.Kitty.Statuses {
		s := &snap.Kitty.Statuses[i]
		name := s.Session
		if name == "" {
			name = s.WindowTitle
		}
		detail := s.Title
		if detail == "" {
			detail = s.Msg
		}
		if detail == "" {
			detail = s.App
		}
		items = append(items, pickItem{text: joinDetail(name, detail), nameLen: len(name), icon: look.StateMarker(s.State), status: s})
	}
	for _, r := range rows {
		if r.Worktree == nil {
			continue
		}
		name := rowName(r)
		icon, detail := []look.Span{look.NoMarker()}, r.Worktree.Branch
		switch state := status.OfSession(snap.Kitty.Statuses, r.Worktree.Session); {
		case r.Missing && state == "":
			icon, detail = look.GoneMarker(snap.Kitty.Sessions[r.Worktree.Session] != 0), "gone"
		case r.Missing:
			icon, detail = look.StateMarker(state), "gone"
		case state != "":
			icon = look.StateMarker(state)
		case r.Worktree.Session == snap.Kitty.Current:
			icon = look.CurrentMarker()
		case snap.Kitty.Sessions[r.Worktree.Session] != 0:
			icon = look.SessionMarker()
		}
		items = append(items, pickItem{text: joinDetail(name, detail), nameLen: len(name), icon: icon, row: r})
	}
	return items
}

// rowName is what the picker calls a worktree's row: its space, after its group, and its directory but for
// the main worktree.
func rowName(r spaces.Row) string {
	name := r.Space.Name
	if r.Space.Group != "" {
		name = r.Space.Group + " / " + name
	}
	if !r.Main {
		name += " / " + r.Dir()
	}
	return name
}

// paneItems are the windows of panes, each named for its worktree's row, else its session, else its tab,
// with what runs in it and its title. panes are the most recently focused first, so the first is the
// window the picker is for, which comes last as switching to it does nothing.
func paneItems(snap spaces.Snapshot, rows []spaces.Row, panes []spaces.Pane) pickItems {
	names := map[string]string{}
	for _, r := range rows {
		if r.Worktree != nil {
			names[r.Worktree.Session] = rowName(r)
		}
	}
	states := map[int]string{}
	for _, s := range snap.Kitty.Statuses {
		states[s.Window] = s.State
	}
	if len(panes) > 1 {
		panes = append(slices.Clone(panes[1:]), panes[0])
	}
	items := make(pickItems, 0, len(panes))
	for i := range panes {
		p := &panes[i]
		name := cmp.Or(names[p.Session], p.Session, p.Tab)
		detail := p.Program
		if p.Title != "" && p.Title != p.Program {
			detail = joinDetail(detail, p.Title)
		}
		icon := []look.Span{look.NoMarker()}
		if state := states[p.Window]; state != "" {
			icon = look.StateMarker(state)
		}
		items = append(items, pickItem{text: joinDetail(name, strings.TrimSpace(detail)), nameLen: len(name), icon: icon, pane: p})
	}
	return items
}

func joinDetail(name, detail string) string {
	if detail == "" {
		return name
	}
	return name + "  " + detail
}

type picker struct {
	th    *look.Theme
	self  int
	scope PickScope
	snap  spaces.Snapshot
	// kitty's windows, nil until loaded.
	panes   []spaces.Pane
	items   pickItems
	matches fuzzy.Matches
	input   textinput.Model
	cursor  int
	width   int
	height  int
	err     error
	// choosing is whether a choice is being switched to, or a form opened.
	choosing bool
	// The menu of the entry under the cursor, nil when closed, see pickmenu.go.
	menu *contextMenu[*picker]
	// zones are where the last frame drew marked spans, by id.
	zones  map[string][]look.Zone
	screen pickScreen
	// getText fetches a window's screen, kitty.ScreenText but in tests.
	getText func(window int) (string, error)
}

// scopePlaceholders are the query's placeholder in each scope, by PickScope.
var scopePlaceholders = [...]string{"space, worktree or status", "program, title or space", "space, status or program"}

func newPicker(self int, scope PickScope) *picker {
	th := look.New(true)
	input := newInput(th, th.SearchPrompt())
	input.Focus()
	m := &picker{th: th, self: self, input: input, getText: kitty.ScreenText}
	m.setScope(scope)
	return m
}

// panesMsg is kitty's windows, as the picker lists them.
type panesMsg struct {
	panes []spaces.Pane
	err   error
}

// Init loads afresh, as the picker's own window is new, with the git states the sidebar has, and kitty's
// windows apart, as asking the session provider what they run takes longer.
func (m *picker) Init() tea.Cmd {
	self := m.self
	return tea.Batch(textinput.Blink, func() tea.Msg {
		snap := spaces.Load(self)
		snap.Git = spaces.GitStates()
		return snap
	}, func() tea.Msg {
		cfg, err := config.Load()
		if err != nil {
			return panesMsg{err: err}
		}
		panes, err := spaces.Panes(cfg)
		return panesMsg{panes: panes, err: err}
	})
}

// setScope lists what scope has, the cursor on its first entry.
func (m *picker) setScope(scope PickScope) {
	m.scope, m.cursor = scope, 0
	m.input.Placeholder = scopePlaceholders[scope]
	m.items = pickItemsFrom(m.snap, m.panes, scope)
	m.filter()
}

// nextScope lists the scope after this one's, the first after the last.
func (m *picker) nextScope() {
	m.setScope((m.scope + 1) % PickScope(len(scopeNames)))
}

// filter ranks the items against the query. An empty query keeps the items in their own order.
func (m *picker) filter() {
	q := m.input.Value()
	if q == "" {
		m.matches = make(fuzzy.Matches, len(m.items))
		for i, it := range m.items {
			m.matches[i] = fuzzy.Match{Str: it.text, Index: i}
		}
	} else {
		m.matches = fuzzy.FindFrom(q, m.items)
	}
	m.cursor = min(m.cursor, max(len(m.matches)-1, 0))
}

// choose switches to the chosen item, opening it as opts says, and exits, which closes the popup the
// picker runs in.
func (m *picker) choose(opts spaces.OpenOptions) tea.Cmd {
	it, ok := m.chosen()
	if !ok {
		return nil
	}
	return m.chooseItem(it, opts)
}

// chooseItem switches to it, as choose does.
func (m *picker) chooseItem(it pickItem, opts spaces.OpenOptions) tea.Cmd {
	cfg, st := *m.snap.Config, m.snap.Kitty
	// The picker's window has focus, so there is no window of the tab to hand back to: the tab's own
	// window gets focus again when an overlay closes, and the popup gives it back from a panel.
	st.Home = 0
	m.choosing = true
	return func() tea.Msg {
		var err error
		switch {
		case it.status != nil:
			err = spaces.OpenWindow(cfg, st, it.status.Window, it.status.Session)
		case it.pane != nil:
			err = spaces.OpenWindow(cfg, st, it.pane.Window, it.pane.Session)
		case it.row.Worktree.Session == st.Current:
			// Already here: closing the picker is all there is to do.
		case it.row.Missing && st.Windows[it.row.Worktree.Session] == 0:
			// Opening it would make it afresh in a directory that is gone.
			err = fmt.Errorf("the worktree directory of %s is gone", it.row.Worktree.Session)
		default:
			err = spaces.OpenWorktree(cfg, st, *it.row.Worktree, opts)
		}
		if err != nil {
			return spaces.Snapshot{Err: err}
		}
		return tea.QuitMsg{}
	}
}

// chosen is the item under the cursor, once loaded.
func (m *picker) chosen() (pickItem, bool) {
	if m.cursor >= len(m.matches) || m.snap.Config == nil {
		return pickItem{}, false
	}
	return m.items[m.matches[m.cursor].Index], true
}

// opensLayout says whether the item under the cursor would open with its default layout, which
// choosing it without one skips.
func (m *picker) opensLayout() bool {
	it, ok := m.chosen()
	return ok && it.status == nil && it.pane == nil && spaces.OpensDefaultLayout(*m.snap.Config, m.snap.Kitty, *it.row.Space, *it.row.Worktree)
}

// Update follows the cursor's entry with the preview's screen after whatever moved it: a key, the query,
// the snapshot or a resize.
func (m *picker) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	cmd := m.update(msg)
	return m, tea.Batch(cmd, m.follow())
}

func (m *picker) update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case screenMsg:
		return m.gotScreen(msg)
	case screenTickMsg:
		return m.screenTick(msg)
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		// bubbles v2 draws only the first character of a placeholder without a width, and the cursor
		// takes a cell past the text.
		m.input.SetWidth(max(m.width-ansi.StringWidth(m.input.Prompt)-1-m.th.Width(m.scopeStrip()...), 1))
		return nil
	case spaces.Snapshot:
		m.err, m.choosing = msg.Err, false
		if msg.Config != nil {
			m.snap, m.items = msg, pickItemsFrom(msg, m.panes, m.scope)
			m.filter()
		}
		return nil
	case panesMsg:
		if msg.err != nil {
			m.err = fmt.Errorf("listing kitty's windows: %w", msg.err)
			return nil
		}
		m.panes, m.items = msg.panes, pickItemsFrom(m.snap, msg.panes, m.scope)
		m.filter()
		return nil
	case tea.MouseClickMsg, tea.MouseWheelMsg, tea.MouseMotionMsg, tea.MouseReleaseMsg:
		return m.mouse(msg.(tea.MouseMsg))
	case tea.KeyPressMsg:
		// Keys neither the menu nor its items take do nothing while it is open.
		if m.menu != nil {
			a, item, ok := m.menu.lookup(m, msg)
			if item {
				m.menu = nil
			}
			if !ok {
				return nil
			}
			return a.Run(m)
		}
		if cmd, ok := runOn(m, msg, thePicker.Target(pickerSubject{})); ok {
			return cmd
		}
	}
	// Everything else is typing the query.
	var cmd tea.Cmd
	before := m.input.Value()
	m.input, cmd = m.input.Update(msg)
	if m.input.Value() != before {
		m.cursor = 0
		m.filter()
	}
	return cmd
}

// renderMatch draws an item with the characters the query matched highlighted, and its detail muted,
// marked so the mouse finds it.
func (m *picker) renderMatch(i int, width int) string {
	th, match := m.th, m.matches[i]
	it := m.items[match.Index]
	zone := fmt.Sprintf(entryZone, i)
	if i == m.cursor {
		text := th.Cut(max(width-th.Width(it.icon...), 0), look.Text(it.text))
		return th.DrawZone(zone, look.Cursor, width, append(append([]look.Span{}, it.icon...), text...)...)
	}
	matched := map[int]bool{}
	for _, b := range match.MatchedIndexes {
		matched[b] = true
	}
	spans := append([]look.Span{}, it.icon...)
	for off, r := range it.text {
		switch {
		case matched[off]:
			spans = append(spans, look.Match(string(r)))
		case off >= it.nameLen:
			spans = append(spans, look.Muted(string(r)))
		default:
			spans = append(spans, look.Text(string(r)))
		}
	}
	return th.DrawZone(zone, look.Plain, width, spans...)
}

func (m *picker) View() tea.View {
	v := tea.NewView(m.render())
	v.AltScreen = true
	// All motion, not only while a button is down, so the menu lights the item under the mouse.
	v.MouseMode = tea.MouseModeAllMotion
	return v
}

// render is the frame, its zones kept for the mouse.
func (m *picker) render() string {
	out, zones := look.Scan(m.frame())
	m.zones = zones
	return out
}

// pickHeaderRows are the query's line and the rule under it, above the list.
const pickHeaderRows = 2

func (m *picker) frame() string {
	th, width := m.th, max(m.width, 1)
	strip := m.scopeStrip()
	input := look.Fit(m.input.View(), max(width-th.Width(strip...), 0))
	lines := []string{input + th.Render(look.Plain, strip...), th.Render(look.Plain, look.Rule(width, false))}
	l := m.layout()
	var list []string
	start := max(m.cursor-l.listRows+1, 0)
	for i := start; i < min(start+l.listRows, len(m.matches)); i++ {
		list = append(list, m.renderMatch(i, l.listWidth))
	}
	if len(m.matches) == 0 && m.snap.Config != nil {
		list = append(list, th.Render(look.Plain, look.Text("No matches").With(look.Dim)))
	}
	var preview [][]look.Span
	if l.previewRows > 0 {
		preview = m.preview(l.previewWidth, l.previewRows)
	}
	switch {
	case l.beside:
		sep := th.Render(look.Plain, look.Default(" "), look.Border("\u2502", false), look.Default(" "))
		for i := range l.listRows {
			row := ""
			if i < len(list) {
				row = list[i]
			}
			var spans []look.Span
			if i < len(preview) {
				spans = th.Cut(l.previewWidth, preview[i]...)
			}
			lines = append(lines, look.Fit(row, l.listWidth)+sep+th.Draw(look.Plain, l.previewWidth, spans...))
		}
	case l.previewRows > 0:
		for len(list) < l.listRows {
			list = append(list, "")
		}
		lines = append(append(lines, list...), th.Render(look.Plain, look.Rule(width, false)))
		for _, row := range preview[:min(len(preview), l.previewRows)] {
			lines = append(lines, th.Render(look.Plain, th.Cut(width, row...)...))
		}
	default:
		lines = append(lines, list...)
	}
	for len(lines) < m.height-1 {
		lines = append(lines, "")
	}
	m.drawPickMenu(lines, pickHeaderRows+m.cursor-start, m.height-1, l.listWidth)
	spans := []look.Span{look.Muted(fmt.Sprintf("%d/%d", len(m.matches), len(m.items)))}
	if m.opensLayout() {
		spans = append(spans, look.Text("  "), look.Key("shift+enter"), look.KeyDesc(" switch without layout"))
	}
	if _, ok := m.pickTarget(); ok {
		spans = append(spans, look.Text("  "), look.Key("tab"), look.KeyDesc(" actions"))
	}
	status := th.Render(look.Plain, th.Cut(width, spans...)...)
	if m.err != nil {
		status = th.Render(look.Plain, th.Cut(width, look.Error(m.err.Error()))...)
	}
	return strings.Join(append(lines, status), "\n")
}

// scopeStripWidth is the narrowest picker with the scope strip, which leaves the query room.
const scopeStripWidth = 60

// scopeStrip is the scopes, the one listed on a chip, each marked so a click lists it. None in a picker
// too narrow, where the query's placeholder says what is listed.
func (m *picker) scopeStrip() []look.Span {
	if m.width < scopeStripWidth {
		return nil
	}
	var spans []look.Span
	for i, name := range scopeTitles {
		tab := []look.Span{look.Muted(" " + name + " ")}
		if PickScope(i) == m.scope {
			tab = look.Button(name, "")
		}
		spans = append(append(spans, look.Text(" ")), look.Mark(fmt.Sprintf(pickScopeZone, i), tab...)...)
	}
	return spans
}

// RunPicker runs the picker listing scope, meant for a kitty popup, until something is chosen or it is
// left.
func RunPicker(ctx context.Context, scope PickScope) error {
	if err := runPopup(ctx, newPicker(kitty.SelfWindowID(), scope)); err != nil {
		return fmt.Errorf("running the picker: %w", err)
	}
	return nil
}

// newInput is a text input after prompt, in th's styles.
func newInput(th *look.Theme, prompt string) textinput.Model {
	in := textinput.New()
	in.Prompt = prompt
	in.SetStyles(th.InputStyles())
	return in
}
