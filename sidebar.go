package main

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const refreshEvery = 2 * time.Second

// headerRows is how many rows the title takes above the list, so a click can be mapped to an entry.
const headerRows = 2

// row is one line of the list. The zero row is the blank line between groups.
type row struct {
	space *Space
	// Nil when the space's worktrees could not be listed.
	worktree *Worktree
	// The space's own row, which opens its main worktree.
	main bool
	// A session that belongs to no space, see otherSessions.
	other bool
}

type snapshot struct {
	// Nil when nothing was loaded, for example because opening a worktree failed.
	cfg    *Config
	spaces []Space
	// Sessions that belong to no space, see otherSessions.
	others []Space
	kitty  KittyState
	err    error
}

type tickMsg struct{}

type sidebar struct {
	cfg    Config
	self   int
	spaces []Space
	kitty  KittyState
	rows   []row
	cursor int
	offset int
	width  int
	height int
	err    error
	// Whether kitty has given the sidebar keyboard focus. The cursor is only drawn then.
	focused bool
}

// Colors are terminal palette indexes rather than fixed colors, so the sidebar follows the kitty theme.
var (
	muted  = lipgloss.Color("8")
	accent = lipgloss.Color("4")

	titleStyle   = lipgloss.NewStyle().Bold(true).Foreground(muted)
	spaceStyle   = lipgloss.NewStyle().Bold(true).Foreground(accent)
	otherStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("6"))
	closedStyle  = lipgloss.NewStyle().Faint(true)
	branchStyle  = lipgloss.NewStyle().Foreground(muted)
	openStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
	errorStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
	currentStyle = lipgloss.NewStyle().Background(muted).Foreground(lipgloss.Color("15")).Bold(true)
	cursorStyle  = lipgloss.NewStyle().Reverse(true)
	borderStyle  = lipgloss.NewStyle().Border(lipgloss.NormalBorder(), false, true, false, false)
)

// load reads the config on every refresh, so edits show up without restarting every dock, and a broken
// config is shown in the sidebar rather than making it exit. kspaces_watcher.py would otherwise keep
// replacing a dock that exits the moment it starts.
func load(self int) snapshot {
	cfg, err := loadConfig()
	if err != nil {
		return snapshot{err: err}
	}
	spaces := make([]Space, 0, len(cfg.Spaces))
	for _, root := range cfg.Spaces {
		spaces = append(spaces, loadSpace(root))
	}
	st, err := loadKittyState(self)
	others := otherSessions(spaces, st.Sessions, savedSessions(cfg.SessionsDir))
	return snapshot{cfg: &cfg, spaces: spaces, others: others, kitty: st, err: err}
}

func (m *sidebar) refresh() tea.Cmd {
	self := m.self
	return func() tea.Msg { return load(self) }
}

func tick() tea.Cmd {
	return tea.Tick(refreshEvery, func(time.Time) tea.Msg { return tickMsg{} })
}

func (m *sidebar) Init() tea.Cmd {
	return tea.Batch(m.refresh(), tick())
}

func (m *sidebar) open(i int) tea.Cmd {
	if i < 0 || i >= len(m.rows) || m.rows[i].worktree == nil {
		return nil
	}
	cfg, st, wt := m.cfg, m.kitty, *m.rows[i].worktree
	self := m.self
	return func() tea.Msg {
		if err := openWorktree(cfg, st, wt); err != nil {
			return snapshot{err: err}
		}
		return load(self)
	}
}

func (m *sidebar) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
	case tea.FocusMsg:
		// Start from the session the sidebar is in, which is where keyboard navigation usually begins.
		m.focused = true
		for i, r := range m.rows {
			if r.worktree != nil && r.worktree.Session == m.kitty.Current {
				m.cursor = i
			}
		}
		m.scrollToCursor()
	case tea.BlurMsg:
		m.focused = false
	case tickMsg:
		return m, tea.Batch(m.refresh(), tick())
	case snapshot:
		m.err = msg.err
		if msg.cfg != nil {
			m.cfg, m.spaces, m.kitty = *msg.cfg, msg.spaces, msg.kitty
			m.rows = buildRows(msg.others, m.spaces)
			m.cursor = min(m.cursor, max(len(m.rows)-1, 0))
		}
	case tea.MouseMsg:
		switch {
		case msg.Button == tea.MouseButtonWheelUp:
			m.offset = max(m.offset-1, 0)
		case msg.Button == tea.MouseButtonWheelDown:
			m.offset = min(m.offset+1, max(len(m.rows)-m.listHeight(), 0))
		case msg.Button == tea.MouseButtonLeft && msg.Action == tea.MouseActionPress:
			i := msg.Y - headerRows + m.offset
			if i >= 0 && i < len(m.rows) {
				m.cursor = i
				return m, m.open(i)
			}
		}
	case tea.KeyMsg:
		switch msg.String() {
		case "j", "down":
			m.moveCursor(1)
		case "k", "up":
			m.moveCursor(-1)
		case "g", "home":
			m.cursor = 0
		case "G", "end":
			m.cursor = max(len(m.rows)-1, 0)
		case "enter", "l":
			return m, m.open(m.cursor)
		case "esc", "q":
			return m, m.leave()
		case "r":
			return m, m.refresh()
		case "ctrl+c":
			return m, tea.Quit
		}
		m.scrollToCursor()
	}
	return m, nil
}

func (m *sidebar) listHeight() int {
	return max(m.height-headerRows-1, 1)
}

// contentWidth leaves the last column for the border against the window the sidebar is docked to.
func (m *sidebar) contentWidth() int {
	return max(m.width-1, 1)
}

func (m *sidebar) scrollToCursor() {
	if m.cursor < m.offset {
		m.offset = m.cursor
	} else if h := m.listHeight(); m.cursor >= m.offset+h {
		m.offset = m.cursor - h + 1
	}
}

// moveCursor moves by delta rows, stepping over the blank row between groups.
func (m *sidebar) moveCursor(delta int) {
	for i := m.cursor + delta; i >= 0 && i < len(m.rows); i += delta {
		if m.rows[i].space != nil {
			m.cursor = i
			return
		}
	}
}

// leave gives keyboard focus back to the window the sidebar took it from.
func (m *sidebar) leave() tea.Cmd {
	home := m.kitty.Home
	if home == 0 {
		return nil
	}
	return func() tea.Msg {
		if err := focusWindow(home); err != nil {
			return snapshot{err: err}
		}
		return nil
	}
}

// buildRows lists the sessions that belong to no space first, then each space followed by its linked
// worktrees, with a blank row between the two groups. A space's own row opens its main worktree.
func buildRows(others, spaces []Space) []row {
	var rows []row
	for i := range others {
		rows = append(rows, row{space: &others[i], worktree: &others[i].Worktrees[0], main: true, other: true})
	}
	if len(rows) > 0 && len(spaces) > 0 {
		rows = append(rows, row{})
	}
	for i := range spaces {
		s := &spaces[i]
		r := row{space: s, main: true}
		if len(s.Worktrees) > 0 {
			r.worktree = &s.Worktrees[0]
		}
		rows = append(rows, r)
		for j := 1; j < len(s.Worktrees); j++ {
			rows = append(rows, row{space: s, worktree: &s.Worktrees[j]})
		}
	}
	return rows
}

// otherSessions are the sessions that are open or have a saved file but belong to no worktree, such as the
// one kitty starts with. Each is a space of its own with one worktree that has no path, so it can only be
// switched to or restored, never created.
func otherSessions(spaces []Space, open map[string]int, saved []string) []Space {
	inSpace := map[string]bool{}
	for _, s := range spaces {
		for _, wt := range s.Worktrees {
			inSpace[wt.Session] = true
		}
	}
	names := map[string]bool{}
	for name := range open {
		names[name] = true
	}
	for _, name := range saved {
		names[name] = true
	}
	var ans []Space
	for name := range names {
		if !inSpace[name] {
			ans = append(ans, Space{Name: name, Worktrees: []Worktree{{Session: name}}})
		}
	}
	slices.SortFunc(ans, func(a, b Space) int { return strings.Compare(a.Name, b.Name) })
	return ans
}

// savedSessions names the session files in dir. An unreadable or missing directory has none.
func savedSessions(dir string) []string {
	if dir == "" {
		return nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var ans []string
	for _, e := range entries {
		if name, ok := strings.CutSuffix(e.Name(), ".kitty-session"); ok && !e.IsDir() {
			ans = append(ans, name)
		}
	}
	return ans
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

func (m *sidebar) renderRow(i int) string {
	r, width := m.rows[i], m.contentWidth()
	if r.space == nil {
		return strings.Repeat(" ", width)
	}
	label := r.space.Name
	if !r.main {
		label = "  " + filepath.Base(r.worktree.Path)
	}
	current := r.worktree != nil && r.worktree.Session == m.kitty.Current
	open := r.worktree != nil && m.isOpen(r.worktree)
	marker, markerStyle := "  ", openStyle
	switch {
	case r.worktree == nil:
		marker, markerStyle = "! ", errorStyle
	case current:
		marker = "> "
	case open:
		marker = "* "
	}
	label = truncate(label, max(width-len(marker), 0))
	branch := ""
	if rest := width - len(marker) - len([]rune(label)); r.worktree != nil && r.worktree.Branch != "" && rest > 2 {
		branch = " " + truncate(r.worktree.Branch, rest-1)
	}
	pad := strings.Repeat(" ", max(width-len(marker)-len([]rune(label))-len([]rune(branch)), 0))

	// A highlighted row is drawn in one style across its whole width, so it reads as one bar.
	switch {
	case i == m.cursor && m.focused:
		return cursorStyle.Render(marker + label + branch + pad)
	case current:
		return currentStyle.Render(marker + label + branch + pad)
	}
	labelStyle := lipgloss.NewStyle()
	switch {
	case r.other:
		labelStyle = otherStyle
	case r.main:
		labelStyle = spaceStyle
	}
	if !open {
		labelStyle = labelStyle.Inherit(closedStyle)
	}
	return markerStyle.Render(marker) + labelStyle.Render(label) + branchStyle.Render(branch) + pad
}

func (m *sidebar) isOpen(wt *Worktree) bool {
	_, ok := m.kitty.Sessions[wt.Session]
	return ok
}

func (m *sidebar) View() string {
	width := m.contentWidth()
	lines := []string{titleStyle.Render(truncate("SPACES", width)), ""}
	end := min(m.offset+m.listHeight(), len(m.rows))
	for i := m.offset; i < end; i++ {
		lines = append(lines, m.renderRow(i))
	}
	// Fill the height so the border runs the full length of the dock, with errors on the last line.
	for len(lines) < m.height-1 {
		lines = append(lines, "")
	}
	status := ""
	if m.err != nil {
		status = errorStyle.Render(truncate(m.err.Error(), width))
	}
	lines = append(lines, status)
	border := muted
	if m.focused {
		border = accent
	}
	return borderStyle.BorderForeground(border).Width(width).Render(strings.Join(lines, "\n"))
}

func runSidebar() error {
	m := &sidebar{self: selfWindowID()}
	p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseCellMotion(), tea.WithReportFocus())
	if _, err := p.Run(); err != nil {
		return fmt.Errorf("running the sidebar: %w", err)
	}
	return nil
}
