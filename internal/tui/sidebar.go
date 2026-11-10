// Package tui is kspaces' terminal UI: the sidebar, the picker and the new worktree form. How they draw
// comes from look, and what keys, menus and clicks do from the registry in registry.go.
package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/chancez/kspaces/internal/config"
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
	// zones are where the last frame drew marked spans, by id, so a click maps back to what was drawn.
	zones map[string][]look.Zone
}

func newSidebar(self int) *sidebar {
	return &sidebar{th: look.New(true), self: self, folded: map[string]bool{}}
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

func (m *sidebar) openWindowCmd(s status.Status) tea.Cmd {
	cfg, st, self := m.cfg, m.kitty, m.self
	return func() tea.Msg {
		if err := spaces.OpenWindow(cfg, st, s.Window, s.Session); err != nil {
			return spaces.Snapshot{Err: err}
		}
		return spaces.Load(self)
	}
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
	case tea.MouseWheelMsg:
		m.wheel(msg)
	case tea.MouseClickMsg:
		return m, m.click(msg)
	case tea.MouseMotionMsg:
		m.motion(msg)
	case tea.KeyPressMsg:
		if !m.focused {
			if !m.remote {
				m.remote = true
				m.cursorToCurrent()
			}
			m.remoteAt = time.Now()
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

func (m *sidebar) frame() string {
	th, width, l := m.th, m.contentWidth(), m.layout()
	lines := []string{th.Draw(look.Plain, width, th.Cut(width, look.Title("SPACES"))...), ""}
	for i := m.offset; i < min(m.offset+l.spaces, len(m.rows)); i++ {
		lines = append(lines, m.renderRow(i))
	}
	for len(lines) < l.statusTop-2 {
		lines = append(lines, "")
	}
	lines = append(lines, th.Render(look.Plain, look.Rule(width, false)), th.Render(look.Plain, th.Cut(width, look.Title("STATUS"))...))
	if len(m.kitty.Statuses) == 0 {
		lines = append(lines, th.Render(look.Plain, th.Cut(width, look.Text("Nothing to report").With(look.Dim))...))
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
		status = th.Render(look.Plain, th.Cut(width, look.Error(m.err.Error()))...)
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
	if _, err := tea.NewProgram(newSidebar(kitty.SelfWindowID()), tea.WithContext(ctx)).Run(); err != nil {
		return fmt.Errorf("running the sidebar: %w", err)
	}
	return nil
}
