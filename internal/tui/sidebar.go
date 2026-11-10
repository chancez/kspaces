// Package tui is kspaces' terminal UI: the sidebar, the picker and the new worktree form.
package tui

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/chancez/kspaces/internal/config"
	"github.com/chancez/kspaces/internal/kitty"
	"github.com/chancez/kspaces/internal/spaces"
	"github.com/chancez/kspaces/internal/status"
)

const refreshEvery = 2 * time.Second

// headerRows is how many rows the title takes above the list, so a click can be mapped to an entry.
const headerRows = 2

type tickMsg struct{}

type sidebar struct {
	cfg    config.Config
	self   int
	spaces []spaces.Space
	others []spaces.Space
	folded map[string]bool
	kitty  kitty.State
	rows   []spaces.Row
	// The context menu, nil when closed.
	menu   *contextMenu
	cursor int
	offset int
	// The first status entry shown, when there are more than fit.
	statusOffset int
	width        int
	height       int
	err          error
	// Whether kitty has given the sidebar keyboard focus.
	focused bool
	// When keys arrive without focus, kitty is sending them from another window: kitty.conf's spaces mode
	// runs `send-key` to the sidebar. The cursor is drawn then too, until remoteAt is a while ago.
	remote   bool
	remoteAt time.Time
	// The window of the status entry the mouse rests on, drawn in full as a popup, or 0. kitty sends no
	// event when the mouse leaves the dock, so it also ends hoverIdle after the last move.
	hover   int
	hoverAt time.Time
}

// remoteIdle is how long remote navigation shows the cursor after its last key. It ends sooner on enter or
// esc; this covers leaving kitty's spaces mode with any other key, which the sidebar never hears about.
const remoteIdle = 10 * time.Second

// hoverIdle is how long the hover popup stays after the mouse last moved.
const hoverIdle = 15 * time.Second

// navigating is whether keys are moving the cursor, so it and the entry under it should be drawn.
func (m *sidebar) navigating() bool {
	return m.focused || m.remote
}

// cursorToCurrent puts the cursor on the session the sidebar is in, where navigation usually begins, or on
// its space when the space is folded.
func (m *sidebar) cursorToCurrent() {
	for i, r := range m.rows {
		if r.Worktree != nil && r.Worktree.Session == m.kitty.Current || r.Folded && m.rowHolds(r, m.kitty.Current) {
			m.cursor = i
		}
	}
	m.scrollToCursor()
}

// rowSessions are the worktrees a row stands for: its own, and for a folded space all of the space's.
func rowSessions(r spaces.Row) []spaces.Worktree {
	switch {
	case r.Folded:
		return r.Space.Worktrees
	case r.Worktree != nil:
		return []spaces.Worktree{*r.Worktree}
	}
	return nil
}

func (m *sidebar) rowHolds(r spaces.Row, session string) bool {
	for _, wt := range rowSessions(r) {
		if wt.Session == session {
			return true
		}
	}
	return false
}

// Colors are terminal palette indexes rather than fixed colors, so the sidebar follows the kitty theme.
var (
	muted  = lipgloss.Color("8")
	accent = lipgloss.Color("4")

	titleStyle     = lipgloss.NewStyle().Bold(true).Foreground(muted)
	spaceStyle     = lipgloss.NewStyle().Bold(true).Foreground(accent)
	otherStyle     = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("6"))
	closedStyle    = lipgloss.NewStyle().Faint(true)
	branchStyle    = lipgloss.NewStyle().Foreground(muted)
	openStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
	errorStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
	blockedStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("3"))
	workingStyle   = lipgloss.NewStyle().Foreground(accent)
	separatorStyle = lipgloss.NewStyle().Foreground(muted)
	currentStyle   = lipgloss.NewStyle().Background(muted).Foreground(lipgloss.Color("15")).Bold(true)
	cursorStyle    = lipgloss.NewStyle().Reverse(true)
	borderStyle    = lipgloss.NewStyle().Border(lipgloss.NormalBorder(), false, true, false, false)
)

func (m *sidebar) refresh() tea.Cmd {
	self := m.self
	return func() tea.Msg { return spaces.Load(self) }
}

func tick() tea.Cmd {
	return tea.Tick(refreshEvery, func(time.Time) tea.Msg { return tickMsg{} })
}

func (m *sidebar) Init() tea.Cmd {
	return tea.Batch(m.refresh(), tick())
}

// The cursor runs over the spaces rows first, then the status entries: index i is m.rows[i] below
// len(m.rows), and m.kitty.Statuses[i-len(m.rows)] from there.
func (m *sidebar) numEntries() int {
	return len(m.rows) + len(m.kitty.Statuses)
}

func (m *sidebar) open(i int) tea.Cmd {
	st, self := m.kitty, m.self
	switch {
	case i >= len(m.rows) && i < m.numEntries():
		id := st.Statuses[i-len(m.rows)].Window
		return func() tea.Msg {
			if err := spaces.OpenWindow(st, id); err != nil {
				return spaces.Snapshot{Err: err}
			}
			return spaces.Load(self)
		}
	case i < 0 || i >= len(m.rows) || m.rows[i].Worktree == nil:
		return nil
	}
	return m.openWorktreeCmd(*m.rows[i].Worktree)
}

func (m *sidebar) openWorktreeCmd(wt spaces.Worktree) tea.Cmd {
	cfg, st, self := m.cfg, m.kitty, m.self
	return func() tea.Msg {
		if err := spaces.OpenWorktree(cfg, st, wt); err != nil {
			return spaces.Snapshot{Err: err}
		}
		return spaces.Load(self)
	}
}

// foldCaret is drawn before a space's name when it has linked worktrees, and toggles them when clicked.
const (
	caretOpen   = "\u25bc " // down-pointing triangle
	caretFolded = "\u25b6 " // right-pointing triangle
	noCaret     = "  "
	// The columns the caret takes on screen, after the marker.
	caretFrom, caretTo = 3, 5
)

func (m *sidebar) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if m.menu != nil {
		if cmd, handled := m.updateMenu(msg); handled {
			return m, cmd
		}
	}
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
	case tea.FocusMsg:
		m.focused, m.remote = true, false
		m.cursorToCurrent()
	case tea.BlurMsg:
		m.focused = false
	case tickMsg:
		if m.remote && time.Since(m.remoteAt) > remoteIdle {
			m.remote = false
		}
		if m.hover != 0 && time.Since(m.hoverAt) > hoverIdle {
			m.hover = 0
		}
		return m, tea.Batch(m.refresh(), tick())
	case spaces.Snapshot:
		m.err = msg.Err
		if msg.Config != nil {
			m.cfg, m.spaces, m.others, m.folded, m.kitty = *msg.Config, msg.Spaces, msg.Others, msg.Folded, msg.Kitty
			m.rows = spaces.BuildRows(m.others, m.spaces, m.folded)
			m.cursor = min(m.cursor, max(m.numEntries()-1, 0))
			m.statusOffset = min(m.statusOffset, max(len(m.kitty.Statuses)-1, 0))
		}
	case tea.MouseMsg:
		l := m.layout()
		inStatus := l.status > 0 && msg.Y >= l.statusTop
		switch {
		case msg.Button == tea.MouseButtonWheelUp && inStatus:
			m.statusOffset = max(m.statusOffset-1, 0)
		case msg.Button == tea.MouseButtonWheelDown && inStatus:
			m.statusOffset = min(m.statusOffset+1, max(len(m.kitty.Statuses)-1, 0))
		case msg.Button == tea.MouseButtonWheelUp:
			m.offset = max(m.offset-1, 0)
		case msg.Button == tea.MouseButtonWheelDown:
			m.offset = min(m.offset+1, max(len(m.rows)-l.spaces, 0))
		case msg.Button == tea.MouseButtonLeft && msg.Action == tea.MouseActionPress:
			// A click on the popup is a click on its entry, not on the one it covers.
			if j := m.hovered(); j >= 0 {
				if popup, top := m.hoverPopup(j); msg.Y >= top && msg.Y < top+len(popup) {
					m.hover, m.cursor = 0, len(m.rows)+j
					return m, m.open(m.cursor)
				}
			}
			m.hover = 0
			if i, ok := m.entryAt(msg.Y); ok {
				m.cursor = i
				if i < len(m.rows) && m.rows[i].Main && !m.rows[i].Other && len(m.rows[i].Space.Worktrees) > 1 &&
					msg.X >= caretFrom && msg.X < caretTo {
					m.toggleFold(m.rows[i].Space.Root)
					return m, nil
				}
				return m, m.open(i)
			}
		case msg.Button == tea.MouseButtonRight && msg.Action == tea.MouseActionPress:
			if i, ok := m.entryAt(msg.Y); ok && i < len(m.rows) {
				m.cursor = i
				m.openMenu(i, msg.Y)
			}
		case msg.Action == tea.MouseActionMotion:
			// Moving within the popup keeps it, rather than switching to the entry it covers.
			if j := m.hovered(); j >= 0 {
				if popup, top := m.hoverPopup(j); msg.Y >= top && msg.Y < top+len(popup) {
					m.hoverAt = time.Now()
					return m, nil
				}
			}
			m.hover = 0
			if i, ok := m.entryAt(msg.Y); ok && i >= len(m.rows) {
				m.hover, m.hoverAt = m.kitty.Statuses[i-len(m.rows)].Window, time.Now()
			}
		}
	case tea.KeyMsg:
		if !m.focused {
			if !m.remote {
				m.remote = true
				m.cursorToCurrent()
			}
			m.remoteAt = time.Now()
		}
		switch msg.String() {
		case "j", "down":
			m.moveCursor(1)
		case "k", "up":
			m.moveCursor(-1)
		case "g", "home":
			m.cursor = 0
		case "G", "end":
			m.cursor = max(m.numEntries()-1, 0)
		case "enter", "l":
			m.remote = false
			return m, m.open(m.cursor)
		case "esc", "q":
			if m.remote {
				// Focus never left the window the keys came from, so there is nothing to give back.
				m.remote = false
				return m, nil
			}
			return m, m.leave()
		case "r":
			return m, m.refresh()
		case "m":
			if m.cursor < len(m.rows) && m.cursor >= m.offset {
				m.openMenu(m.cursor, headerRows+m.cursor-m.offset)
			}
			return m, nil
		case " ", "space":
			if m.cursor < len(m.rows) && m.rows[m.cursor].Space != nil && !m.rows[m.cursor].Other {
				m.toggleFold(m.rows[m.cursor].Space.Root)
			}
			return m, nil
		case "n":
			if m.cursor < len(m.rows) && m.rows[m.cursor].Space != nil && !m.rows[m.cursor].Other {
				m.remote = false
				return m, m.newWorktree(m.rows[m.cursor].Space.Root)
			}
		case "ctrl+c":
			return m, tea.Quit
		}
		m.scrollToCursor()
	}
	return m, nil
}

// statusAt is where the status list starts, as a share of the rows below the SPACES title, so it stays put
// as entries come and go rather than growing up from the bottom.
const statusAt = 0.55

// layout splits the height between the two lists: spaces above, status below a separator and its title.
type layout struct {
	spaces int
	status int
	// The screen row of the first status entry, below the separator and the STATUS title.
	statusTop int
}

func (m *sidebar) layout() layout {
	avail := max(m.height-headerRows-1, 1)
	spaces := max(int(float64(avail)*statusAt), 1)
	status := max(avail-spaces-2, 1)
	return layout{spaces: spaces, status: status, statusTop: headerRows + spaces + 2}
}

// entryAt maps a screen row to a cursor index.
func (m *sidebar) entryAt(y int) (int, bool) {
	l := m.layout()
	if y >= headerRows && y < headerRows+l.spaces {
		i := y - headerRows + m.offset
		return i, i < len(m.rows) && m.rows[i].Space != nil
	}
	if l.status > 0 && y >= l.statusTop && y < l.statusTop+l.status {
		// Entries take several rows each, and some have a group title above, so walk them down to the one
		// drawn at y. A group title opens nothing.
		top := l.statusTop
		for j := m.statusOffset; j < len(m.kitty.Statuses); j++ {
			title := len(m.statusGroupTitle(j))
			top += title + len(m.statusEntry(j))
			if y < top {
				return len(m.rows) + j, y >= top-len(m.statusEntry(j))
			}
		}
	}
	return 0, false
}

// contentWidth leaves the last column for the border against the window the sidebar is docked to.
func (m *sidebar) contentWidth() int {
	return max(m.width-1, 1)
}

func (m *sidebar) scrollToCursor() {
	l := m.layout()
	if m.cursor >= len(m.rows) {
		j := m.cursor - len(m.rows)
		if j < m.statusOffset {
			m.statusOffset = j
		}
		// Scroll until the whole entry fits, as the cursor expands it.
		for m.statusOffset < j && m.statusRows(m.statusOffset, j) > l.status {
			m.statusOffset++
		}
		return
	}
	if m.cursor < m.offset {
		m.offset = m.cursor
	} else if m.cursor >= m.offset+l.spaces {
		m.offset = m.cursor - l.spaces + 1
	}
}

// moveCursor moves by delta entries, stepping over the blank row between groups of spaces.
func (m *sidebar) moveCursor(delta int) {
	for i := m.cursor + delta; i >= 0 && i < m.numEntries(); i += delta {
		if i >= len(m.rows) || m.rows[i].Space != nil {
			m.cursor = i
			return
		}
	}
}

// leave gives keyboard focus back to the window the sidebar took it from.
func (m *sidebar) leave() tea.Cmd {
	st := m.kitty
	if st.Home == 0 {
		return nil
	}
	return func() tea.Msg {
		if err := spaces.Leave(st); err != nil {
			return spaces.Snapshot{Err: err}
		}
		return nil
	}
}

func truncate(s string, width int) string {
	r := []rune(s)
	if len(r) <= width {
		return s
	}
	if width <= 3 {
		return string(r[:max(width, 0)])
	}
	return string(r[:width-3]) + "..."
}

// Icons from the Nerd Fonts symbols kitty ships with, so they need no patched font. The state icons are the
// ones kitty uses for tab.program_status. kitty draws a symbol over two cells when a space follows it, so
// each is followed by two spaces: one it draws over and one to keep it off the text.
const (
	iconBlocked  = "\uf071" // warning
	iconError    = "\ue654" // error
	iconDone     = "\uf00c" // check
	iconWorking  = "\uf110" // spinner
	iconIdle     = "\uf10c" // circle outline
	iconCurrent  = "\uf054" // chevron-right
	iconOpen     = "\uf444" // dot-fill
	iconBroken   = "\uf00d" // times
	iconWorktree = "\ue725" // git-branch
)

// noMarker is as wide as a marker: an icon and its two spaces.
const noMarker = "   "

func marker(icon string) string {
	return icon + "  "
}

// cells is how many columns s takes, given one rune per cell: the icons are single runes kitty fits into
// the cells counted for them, and names are what git and kitty report, rarely anything wide.
func cells(s string) int {
	return len([]rune(s))
}

func (m *sidebar) renderRow(i int) string {
	r, width := m.rows[i], m.contentWidth()
	if r.Space == nil {
		return strings.Repeat(" ", width)
	}
	// The caret column folds a space's linked worktrees, and indents those worktrees under its name.
	caret, label := noCaret, r.Space.Name
	switch {
	case !r.Main:
		label = marker(iconWorktree) + filepath.Base(r.Worktree.Path)
	case r.Folded:
		caret = caretFolded
	case !r.Other && len(r.Space.Worktrees) > 1:
		caret = caretOpen
	}
	// A folded space stands for all its worktrees: it is current, open or in a state if any of them is.
	current, open, state := false, false, ""
	for _, wt := range rowSessions(r) {
		current = current || wt.Session == m.kitty.Current
		open = open || m.isOpen(&wt)
		if s := status.OfSession(m.kitty.Statuses, wt.Session); status.Urgency(s) > status.Urgency(state) {
			state = s
		}
	}
	mark, markStyle := noMarker, openStyle
	switch {
	case r.Worktree == nil:
		mark, markStyle = marker(iconBroken), errorStyle
	case state != "":
		// The most urgent status of the session's windows, which the status list below details.
		mark, markStyle = stateMarker(state)
	case current:
		mark = marker(iconCurrent)
	case open:
		mark = marker(iconOpen)
	}
	label = truncate(label, max(width-cells(mark)-cells(caret), 0))
	branch := ""
	if rest := width - cells(mark) - cells(caret) - cells(label); r.Worktree != nil && r.Worktree.Branch != "" && rest > 2 {
		branch = " " + truncate(r.Worktree.Branch, rest-1)
	}
	pad := strings.Repeat(" ", max(width-cells(mark)-cells(caret)-cells(label)-cells(branch), 0))

	// A highlighted row is drawn in one style across its whole width, so it reads as one bar.
	switch {
	case i == m.cursor && m.navigating():
		return cursorStyle.Render(mark + caret + label + branch + pad)
	case current:
		return currentStyle.Render(mark + caret + label + branch + pad)
	}
	labelStyle := lipgloss.NewStyle()
	switch {
	case r.Other:
		labelStyle = otherStyle
	case r.Main:
		labelStyle = spaceStyle
	}
	if !open {
		labelStyle = labelStyle.Inherit(closedStyle)
	}
	return markStyle.Render(mark) + branchStyle.Render(caret) + labelStyle.Render(label) + branchStyle.Render(branch) + pad
}

// stateMarker is the marker and its color for a program status state.
func stateMarker(state string) (string, lipgloss.Style) {
	switch state {
	case "blocked":
		return marker(iconBlocked), blockedStyle
	case "error":
		return marker(iconError), errorStyle
	case "done":
		return marker(iconDone), openStyle
	case "idle":
		return marker(iconIdle), branchStyle
	default:
		return marker(iconWorking), workingStyle
	}
}

// statusRows is how many rows status entries from through to take, with their group titles.
func (m *sidebar) statusRows(from, to int) int {
	n := 0
	for j := from; j <= to; j++ {
		n += len(m.statusGroupTitle(j)) + len(m.statusEntry(j))
	}
	return n
}

// stateGroup names the group of entries in a state, in the order the list sorts them.
var stateGroup = map[string]string{"blocked": "Blocked", "error": "Failed", "done": "Done", "working": "Working", "idle": "Idle"}

// statusGroupTitle is drawn above entry j when it starts a group of entries in one state, or is the first
// one shown after scrolling, so a scrolled list still says what it shows. Later groups get a blank row
// above their title.
func (m *sidebar) statusGroupTitle(j int) []string {
	all := m.kitty.Statuses
	if j != m.statusOffset && all[j-1].State == all[j].State {
		return nil
	}
	n := 0
	for _, s := range all {
		if s.State == all[j].State {
			n++
		}
	}
	width := m.contentWidth()
	_, style := stateMarker(all[j].State)
	name, count := stateGroup[all[j].State], fmt.Sprintf(" %d", n)
	title := style.Render(name) + branchStyle.Render(count) + strings.Repeat(" ", max(width-cells(name)-cells(count), 0))
	if j == m.statusOffset {
		return []string{title}
	}
	return []string{strings.Repeat(" ", width), title}
}

// kindLabel says what a blocked program is waiting for, from the protocol's kind field.
var kindLabel = map[string]string{"permission": "wants permission", "question": "has a question", "auth": "needs you to sign in"}

// summary is the one line that best says what a program is up to: its progress, its title, its message,
// or what it is blocked on, after the program's name.
func summary(r status.Record, width int) string {
	text := r.Title
	switch {
	case r.Progress != nil:
		text = progressBar(*r.Progress, max(width-cells(r.App)-3, 6))
	case text == "" && r.Msg != "":
		text = r.Msg
	case text == "":
		text = kindLabel[r.Kind]
	}
	switch {
	case r.App == "":
		return text
	case text == "":
		return r.App
	}
	return r.App + " \u00b7 " + text
}

// statusEntry draws status entry j. The first row has its state icon, the session it belongs to, or the
// window title for a window in no session, and how long ago the record changed to its state. Below it is
// the program's name and what it said: its progress, its title or else its message. With the cursor on it,
// the entry opens up to show all of it: what a blocked program waits for, title, progress, the whole
// message, the record's id and the window's other records.
func (m *sidebar) statusEntry(j int) []string {
	s, inner := m.kitty.Statuses[j], max(m.contentWidth()-cells(noMarker), 1)
	expanded := len(m.rows)+j == m.cursor && m.navigating()
	lines := []string{m.statusHeader(j, expanded)}
	if expanded {
		return append(lines, statusDetails(s, inner, 3, 6)...)
	}
	if sum := truncate(summary(s.Record, inner), inner); sum != "" {
		lines = append(lines, noMarker+branchStyle.Render(sum)+strings.Repeat(" ", max(inner-cells(sum), 0)))
	}
	return lines
}

// statusHeader is the first row of status entry j, drawn as the cursor's bar when highlighted.
func (m *sidebar) statusHeader(j int, highlighted bool) string {
	s, width := m.kitty.Statuses[j], m.contentWidth()
	mark, markStyle := stateMarker(s.State)
	// A linked worktree's session is SPACE@DIR; the worktree's name alone fits the narrow dock and is what
	// its row in the spaces list shows.
	label := s.Session
	if _, dir, ok := strings.Cut(label, "@"); ok {
		label = dir
	}
	if label == "" {
		label = s.WindowTitle
	}
	right := status.Age(s.Since, time.Now())
	if len(s.Others) > 0 {
		right = strings.TrimSpace(fmt.Sprintf("+%d %s", len(s.Others), right))
	}
	if right != "" {
		right = " " + right
	}
	label = truncate(label, max(width-cells(mark)-cells(right), 0))
	pad := strings.Repeat(" ", max(width-cells(mark)-cells(label)-cells(right), 0))
	if highlighted {
		return cursorStyle.Render(mark + label + pad + right)
	}
	return markStyle.Render(mark) + label + pad + branchStyle.Render(right)
}

// statusDetails is everything a status entry has to say, indented under its label and wrapped to inner
// columns: the program, what a blocked one waits for, the title, progress, the message, the record's id
// and the window's other records. Title and message are cut at titleRows and msgRows, 0 for no limit.
func statusDetails(s status.Status, inner, titleRows, msgRows int) []string {
	var lines []string
	add := func(style lipgloss.Style, text string, maxRows int) {
		for i, l := range wrap(text, inner) {
			if maxRows > 0 && i == maxRows {
				break
			}
			lines = append(lines, noMarker+style.Render(l)+strings.Repeat(" ", max(inner-cells(l), 0)))
		}
	}
	plain := lipgloss.NewStyle()
	add(branchStyle, s.App, 1)
	if l := kindLabel[s.Kind]; l != "" {
		add(blockedStyle, l, 1)
	}
	add(plain, s.Title, titleRows)
	if s.Progress != nil {
		add(plain, progressBar(*s.Progress, inner), 1)
	}
	add(branchStyle, s.Msg, msgRows)
	if s.ID != "" {
		add(branchStyle, "id "+s.ID, 1)
	}
	now := time.Now()
	for _, o := range s.Others {
		name := o.ID
		if name == "" {
			name = o.App
		}
		add(branchStyle, strings.Join(strings.Fields(strings.Join([]string{o.State, name, status.Age(o.Since, now), o.Title}, " ")), " "), 1)
	}
	return lines
}

// hovered is the index of the status entry under the mouse, or -1. Kept by window rather than by index, as
// a refresh can reorder the list under a resting mouse.
func (m *sidebar) hovered() int {
	if m.hover == 0 {
		return -1
	}
	for j, s := range m.kitty.Statuses {
		if s.Window == m.hover {
			return j
		}
	}
	return -1
}

// hoverPopup is status entry j in full, for the mouse resting on it, and the screen row it starts at. It
// covers the entries below it so the list keeps its layout, with a rule closing it off from what it covers,
// and moves up over the entries above when it would run off the bottom.
func (m *sidebar) hoverPopup(j int) ([]string, int) {
	l, width := m.layout(), m.contentWidth()
	if j < m.statusOffset || j >= len(m.kitty.Statuses) {
		return nil, 0
	}
	top := l.statusTop
	for k := m.statusOffset; k < j; k++ {
		top += len(m.statusGroupTitle(k)) + len(m.statusEntry(k))
	}
	top += len(m.statusGroupTitle(j))
	popup := append([]string{m.statusHeader(j, true)}, statusDetails(m.kitty.Statuses[j], max(width-cells(noMarker), 1), 0, 0)...)
	popup = append(popup, workingStyle.Render(strings.Repeat("\u2500", width)))
	bottom := l.statusTop + l.status
	top = max(min(top, bottom-len(popup)), headerRows)
	return popup[:min(len(popup), bottom-top)], top
}

// progressBar draws a percentage as a bar that fits width, followed by the number.
func progressBar(percent, width int) string {
	n := max(min(width-5, 16), 1)
	filled := n * min(max(percent, 0), 100) / 100
	return strings.Repeat("\u2588", filled) + strings.Repeat("\u2591", n-filled) + fmt.Sprintf(" %d%%", percent)
}

// wrap breaks text into rows of at most width cells at spaces, and inside words longer than a row.
func wrap(text string, width int) []string {
	var rows []string
	for _, para := range strings.Split(text, "\n") {
		row := ""
		for _, word := range strings.Fields(para) {
			for cells(word) > width {
				if row != "" {
					rows, row = append(rows, row), ""
				}
				r := []rune(word)
				rows, word = append(rows, string(r[:width])), string(r[width:])
			}
			switch {
			case row == "":
				row = word
			case cells(row)+1+cells(word) <= width:
				row += " " + word
			default:
				rows, row = append(rows, row), word
			}
		}
		if row != "" {
			rows = append(rows, row)
		}
	}
	return rows
}

func (m *sidebar) isOpen(wt *spaces.Worktree) bool {
	_, ok := m.kitty.Sessions[wt.Session]
	return ok
}

func (m *sidebar) View() string {
	width, l := m.contentWidth(), m.layout()
	lines := []string{titleStyle.Render(truncate("SPACES", width)), ""}
	for i := m.offset; i < min(m.offset+l.spaces, len(m.rows)); i++ {
		lines = append(lines, m.renderRow(i))
	}
	for len(lines) < l.statusTop-2 {
		lines = append(lines, "")
	}
	lines = append(lines, separatorStyle.Render(strings.Repeat("\u2500", width)), titleStyle.Render(truncate("STATUS", width)))
	if len(m.kitty.Statuses) == 0 {
		lines = append(lines, closedStyle.Render(truncate("Nothing to report", width)))
	}
	for j := m.statusOffset; j < len(m.kitty.Statuses); j++ {
		for _, line := range append(m.statusGroupTitle(j), m.statusEntry(j)...) {
			if len(lines) >= l.statusTop+l.status {
				break
			}
			lines = append(lines, line)
		}
	}
	// Fill the height so the border runs the full length of the dock, with errors on the last line.
	for len(lines) < m.height-1 {
		lines = append(lines, "")
	}
	if j := m.hovered(); j >= 0 {
		if popup, top := m.hoverPopup(j); top < len(lines) {
			copy(lines[top:], popup)
		}
	}
	m.drawMenu(lines)
	status := ""
	if m.err != nil {
		status = errorStyle.Render(truncate(m.err.Error(), width))
	}
	lines = append(lines, status)
	border := muted
	if m.navigating() {
		border = accent
	}
	return borderStyle.BorderForeground(border).Width(width).Render(strings.Join(lines, "\n"))
}

// RunSidebar runs the sidebar, meant for a kitty dock, until it is quit.
func RunSidebar(ctx context.Context) error {
	m := &sidebar{self: kitty.SelfWindowID()}
	// All motion, not just drags, so resting the mouse on a status entry can show it in full.
	p := tea.NewProgram(m, tea.WithContext(ctx), tea.WithAltScreen(), tea.WithMouseAllMotion(), tea.WithReportFocus())
	if _, err := p.Run(); err != nil {
		return fmt.Errorf("running the sidebar: %w", err)
	}
	return nil
}
