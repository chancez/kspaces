// Package tui is kspaces' terminal UI: the sidebar, the picker and the new worktree form. How they draw
// comes from look, and what keys, menus and clicks do from the registry in registry.go.
package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	"github.com/chancez/kspaces/internal/config"
	"github.com/chancez/kspaces/internal/gitstate"
	"github.com/chancez/kspaces/internal/kitty"
	"github.com/chancez/kspaces/internal/spaces"
	"github.com/chancez/kspaces/internal/status"
	"github.com/chancez/kspaces/internal/tui/look"
)

const refreshEvery = 2 * time.Second

// headerRows is how many rows the title takes above the list.
const headerRows = 2

// remoteIdle is how long remote navigation shows the cursor after its last key. It ends sooner on enter or
// esc; this covers leaving kitty's spaces mode with any other key, which the sidebar never hears about.
const remoteIdle = 10 * time.Second

// hoverIdle is how long the hover popup stays after the mouse last moved.
const hoverIdle = 15 * time.Second

type tickMsg struct{}

type sidebar struct {
	th     *look.Theme
	cfg    config.Config
	self   int
	spaces []spaces.Space
	others []spaces.Space
	folded map[string]bool
	// How the rows are ordered, see spaces.Snapshot, and the order held while the mouse is on them, see
	// keepOrder.
	order spaces.Order
	held  map[string]int
	// Sessions with a saved file, see spaces.Snapshot.
	saved map[string]bool
	kitty kitty.State
	rows  []spaces.Row
	// The context menu, nil when closed.
	menu   *contextMenu
	cursor int
	offset int
	// The first status entry shown, when there are more than fit.
	statusOffset int
	width        int
	height       int
	err          error
	// notice is what the last line says when there is no error: that the kitty files are out of date.
	notice *look.Span
	// Whether kitty has given the sidebar keyboard focus.
	focused bool
	// When keys arrive without focus, kitty is sending them from another window: kitty.conf's spaces mode
	// runs `send-key` to the sidebar. The cursor is drawn then too, until remoteAt is a while ago.
	remote   bool
	remoteAt time.Time
	// The window of the status entry the mouse rests on, drawn in full as a popup, or 0. It ends when the
	// mouse leaves the dock, which kitty says only in pixel mouse mode, and else hoverIdle after the last
	// move.
	hover   int
	hoverAt time.Time
	mouse   *pixelMouse
	// Whether the mouse is on the add space button or the sort button, which lights it.
	addHot, sortHot bool
	// zones are where the last frame drew marked spans, by id, so a click maps back to what was drawn.
	zones map[string][]look.Zone
	// The spaces list's filter, see filter.go; its query narrows rows while filtering.
	filter    textinput.Model
	filtering bool
	// poller shares one load between every sidebar of the kitty.
	poller *spaces.Poller
	// What git and gh say of each worktree, by path, from the poller's leader.
	git map[string]gitstate.State
}

func newSidebar(self int) *sidebar {
	th := look.New(true)
	return &sidebar{th: th, self: self, folded: map[string]bool{}, mouse: &pixelMouse{}, filter: newFilter(th), poller: spaces.NewPoller()}
}

// navigating is whether keys are moving the cursor, so it and the entry under it should be drawn.
func (m *sidebar) navigating() bool {
	return m.focused || m.remote
}

// cursorToCurrent puts the cursor on the session the sidebar is in, where navigation usually begins, or on
// its space when the space is folded.
func (m *sidebar) cursorToCurrent() {
	for i, r := range m.rows {
		if r.Worktree != nil && r.Worktree.Session == m.kitty.Current || r.Folded && rowHolds(r, m.kitty.Current) {
			m.cursor = i
		}
	}
	m.scrollToCursor()
}

// poll is the latest load, the one the leading sidebar shares, on the timer.
func (m *sidebar) poll() tea.Cmd {
	p, self := m.poller, m.self
	return func() tea.Msg { return p.Poll(false).View(self) }
}

// refresh loads now, rather than waiting for the leader, for the refresh key, and has the leader ask git
// and gh again.
func (m *sidebar) refresh() tea.Cmd {
	p, self := m.poller, m.self
	return func() tea.Msg { return p.Refresh().View(self) }
}

func tick() tea.Cmd {
	return tea.Tick(refreshEvery, func(time.Time) tea.Msg { return tickMsg{} })
}

func (m *sidebar) Init() tea.Cmd {
	return tea.Batch(m.poll(), tick(), checkGlue)
}

// The cursor runs over the spaces rows first, then the status entries: index i is m.rows[i] below
// len(m.rows), and m.kitty.Statuses[i-len(m.rows)] from there.
func (m *sidebar) numEntries() int {
	return len(m.rows) + len(m.kitty.Statuses)
}

func (m *sidebar) openWindowCmd(s status.Status) tea.Cmd {
	cfg, st, self, p := m.cfg, m.kitty, m.self, m.poller
	return func() tea.Msg {
		if err := spaces.OpenWindow(cfg, st, s.Window, s.Session); err != nil {
			return spaces.Snapshot{Err: err}
		}
		return p.Poll(true).View(self)
	}
}

func (m *sidebar) openWorktreeCmd(wt spaces.Worktree) tea.Cmd {
	cfg, st, self, p := m.cfg, m.kitty, m.self, m.poller
	return func() tea.Msg {
		if err := spaces.OpenWorktree(cfg, st, wt); err != nil {
			return spaces.Snapshot{Err: err}
		}
		return p.Poll(true).View(self)
	}
}

func (m *sidebar) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var start tea.Cmd
	if _, ok := msg.(tea.MouseMsg); ok {
		start = m.mouse.start()
	}
	_, cmd := m.update(msg)
	// The filter belongs to navigating: it goes when the keys stop coming, so no sidebar is left narrowed.
	if m.filtering && !m.navigating() {
		m.closeFilter()
	}
	m.keepOrder()
	return m, tea.Batch(start, cmd)
}

func (m *sidebar) update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if m.menu != nil {
		if cmd, handled := m.updateMenu(msg); handled {
			return m, cmd
		}
	}
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.filter.SetWidth(m.filterWidth())
		m.mouse.resize()
		// A shorter list can leave the cursor under an indicator, or out of view.
		m.scrollToCursor()
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
		return m, tea.Batch(m.poll(), tick())
	case glueMsg:
		m.notice = &msg.notice
	case spaces.Snapshot:
		m.err = msg.Err
		if msg.Config != nil {
			// Where the cursor is goes by the rows and entries it was drawn on, before they change.
			at := m.cursorAt()
			m.cfg, m.spaces, m.others, m.folded, m.saved, m.kitty = *msg.Config, msg.Spaces, msg.Others, msg.Folded, msg.Saved, msg.Kitty
			m.order = msg.Order
			m.git = msg.Git
			m.rebuildRows(at)
			m.statusOffset = min(m.statusOffset, max(len(m.kitty.Statuses)-1, 0))
		}
	case tea.MouseWheelMsg:
		m.wheel(msg)
	case tea.MouseClickMsg:
		return m, m.click(msg)
	case tea.MouseMotionMsg:
		m.motion(msg)
	case mouseLeftMsg:
		m.hover, m.addHot, m.sortHot = 0, false, false
	case tea.KeyPressMsg:
		if !m.focused {
			if !m.remote {
				m.remote = true
				m.cursorToCurrent()
			}
			m.remoteAt = time.Now()
		}
		if m.filtering {
			return m, m.typeFilter(msg)
		}
		cmd, _ := runOn(m, msg, m.chain()...)
		return m, cmd
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

// contentWidth leaves the last column for the border against the window the sidebar is docked to.
func (m *sidebar) contentWidth() int {
	return max(m.width-1, 1)
}

// scrollToCursor scrolls the list the cursor is in so its entry shows.
func (m *sidebar) scrollToCursor() {
	if m.cursor >= len(m.rows) {
		m.scrollStatusToEntry(m.cursor - len(m.rows))
		return
	}
	m.scrollSpacesToCursor()
}

// moveCursor moves by delta entries, stepping over the blank row between groups of spaces.
func (m *sidebar) moveCursor(delta int) {
	for i := m.cursor + delta; i >= 0 && i < m.numEntries(); i += delta {
		if i >= len(m.rows) || m.rows[i].Space != nil {
			m.cursor = i
			break
		}
	}
	m.scrollToCursor()
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

func (m *sidebar) isOpen(wt *spaces.Worktree) bool {
	_, ok := m.kitty.Sessions[wt.Session]
	return ok
}

func (m *sidebar) View() tea.View {
	v := tea.NewView(m.render())
	v.AltScreen = true
	// All motion, not just drags, so resting the mouse on a status entry can show it in full.
	v.MouseMode = tea.MouseModeAllMotion
	v.ReportFocus = true
	return v
}

// render draws the frame, and records where its marked spans landed for the mouse.
func (m *sidebar) render() string {
	out, zones := look.Scan(m.frame())
	m.zones = zones
	return out
}

// titleLine is SPACES with the sort button and the add space button at its right end. The title is cut
// first, keeping a cell between it and the buttons.
func (m *sidebar) titleLine(width int) string {
	th := m.th
	buttons := append(look.Mark(sortZone, look.SortButton(m.order.Mode, m.sortHot)), look.Default(" "))
	buttons = append(buttons, look.Mark(addZone, look.AddButton(m.addHot))...)
	title := th.Cut(max(width-th.Width(buttons...)-1, 0), look.Title("SPACES"))
	gap := look.Default(strings.Repeat(" ", max(width-th.Width(title...)-th.Width(buttons...), 0)))
	return th.Draw(look.Plain, width, append(append(title, gap), buttons...)...)
}

func (m *sidebar) frame() string {
	th, width, l := m.th, m.contentWidth(), m.layout()
	lines := []string{m.titleLine(width), m.filterLine()}
	lines = append(lines, m.spacesLines()...)
	for len(lines) < l.statusTop-2 {
		lines = append(lines, "")
	}
	lines = append(lines, th.Render(look.Plain, look.Rule(width, false)), th.Render(look.Plain, th.Cut(width, look.Title("STATUS"))...))
	if len(m.kitty.Statuses) == 0 {
		lines = append(lines, th.Render(look.Plain, th.Cut(width, look.Text("Nothing to report").With(look.Dim))...))
	}
	statusList, _ := m.statusLines()
	lines = append(lines, statusList...)
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
	switch {
	case m.err != nil:
		status = th.Render(look.Plain, th.Cut(width, look.Error(m.err.Error()))...)
	case m.notice != nil:
		status = th.Render(look.Plain, th.Cut(width, *m.notice)...)
	}
	lines = append(lines, status)
	// The border against the window the dock is next to, in the accent while keys move the cursor.
	border := th.Render(look.Plain, look.Border("\u2502", m.navigating()))
	var b strings.Builder
	for i, line := range lines {
		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(look.Fit(line, width))
		b.WriteString(border)
	}
	return b.String()
}

// RunSidebar runs the sidebar, meant for a kitty dock, until it is quit.
func RunSidebar(ctx context.Context) error {
	m := newSidebar(kitty.SelfWindowID())
	if _, err := tea.NewProgram(m, tea.WithContext(ctx), tea.WithFilter(m.mouse.filter)).Run(); err != nil {
		return fmt.Errorf("running the sidebar: %w", err)
	}
	return nil
}
