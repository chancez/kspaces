package tui

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/sahilm/fuzzy"

	"github.com/chancez/kspaces/internal/kitty"
	"github.com/chancez/kspaces/internal/spaces"
	"github.com/chancez/kspaces/internal/status"
)

// pickItem is one thing the picker can switch to: a worktree's session, or a window reporting a status.
type pickItem struct {
	// The text the query is matched against. It is also what is shown, so matches can be highlighted.
	text string
	// Where text stops being the name and becomes the faint detail after it, such as a branch.
	nameLen int
	icon    string
	style   lipgloss.Style

	worktree *spaces.Worktree
	status   *status.Status
}

type pickItems []pickItem

func (p pickItems) String(i int) string { return p[i].text }
func (p pickItems) Len() int            { return len(p) }

// pickItemsFrom lists the windows that report a status first, most urgent first, then every session and
// worktree in sidebar order.
func pickItemsFrom(snap spaces.Snapshot) pickItems {
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
		icon, style := stateMarker(s.State)
		items = append(items, pickItem{text: joinDetail(name, detail), nameLen: len(name), icon: icon, style: style, status: s})
	}
	for _, r := range spaces.BuildRows(snap.Others, snap.Spaces, nil) {
		if r.Worktree == nil {
			continue
		}
		name := r.Space.Name
		if !r.Main {
			name += " / " + filepath.Base(r.Worktree.Path)
		}
		icon, style := noMarker, lipgloss.NewStyle()
		switch {
		case status.OfSession(snap.Kitty.Statuses, r.Worktree.Session) != "":
			icon, style = stateMarker(status.OfSession(snap.Kitty.Statuses, r.Worktree.Session))
		case r.Worktree.Session == snap.Kitty.Current:
			icon = marker(iconCurrent)
		case snap.Kitty.Sessions[r.Worktree.Session] != 0:
			icon, style = marker(iconOpen), openStyle
		}
		items = append(items, pickItem{
			text: joinDetail(name, r.Worktree.Branch), nameLen: len(name), icon: icon, style: style, worktree: r.Worktree,
		})
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
	self    int
	snap    spaces.Snapshot
	items   pickItems
	matches fuzzy.Matches
	input   textinput.Model
	cursor  int
	width   int
	height  int
	err     error
}

func (m *picker) Init() tea.Cmd {
	self := m.self
	return tea.Batch(textinput.Blink, func() tea.Msg { return spaces.Load(self) })
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

// choose switches to the chosen item and exits, which closes the overlay the picker runs in.
func (m *picker) choose() tea.Cmd {
	if m.cursor >= len(m.matches) || m.snap.Config == nil {
		return nil
	}
	it, cfg, st := m.items[m.matches[m.cursor].Index], *m.snap.Config, m.snap.Kitty
	// The overlay is the window that has focus, so there is no window of the tab to hand back to: the
	// tab's own window gets focus again when the overlay closes.
	st.Home = 0
	return func() tea.Msg {
		var err error
		switch {
		case it.status != nil:
			err = spaces.OpenWindow(st, it.status.Window)
		case it.worktree.Session == st.Current:
			// Already here: closing the picker is all there is to do.
		default:
			err = spaces.OpenWorktree(cfg, st, *it.worktree)
		}
		if err != nil {
			return spaces.Snapshot{Err: err}
		}
		return tea.QuitMsg{}
	}
}

func (m *picker) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil
	case spaces.Snapshot:
		m.err = msg.Err
		if msg.Config != nil {
			m.snap, m.items = msg, pickItemsFrom(msg)
			m.filter()
		}
		return m, nil
	case tea.KeyMsg:
		switch msg.String() {
		case "esc", "ctrl+c":
			return m, tea.Quit
		case "enter":
			return m, m.choose()
		case "down", "ctrl+n", "ctrl+j":
			m.cursor = min(m.cursor+1, max(len(m.matches)-1, 0))
			return m, nil
		case "up", "ctrl+p", "ctrl+k":
			m.cursor = max(m.cursor-1, 0)
			return m, nil
		}
	}
	var cmd tea.Cmd
	before := m.input.Value()
	m.input, cmd = m.input.Update(msg)
	if m.input.Value() != before {
		m.cursor = 0
		m.filter()
	}
	return m, cmd
}

var matchStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("3"))

// renderMatch draws an item with the characters the query matched highlighted.
func (m *picker) renderMatch(i int, width int) string {
	match := m.matches[i]
	it := m.items[match.Index]
	matched := map[int]bool{}
	for _, b := range match.MatchedIndexes {
		matched[b] = true
	}
	var b strings.Builder
	n := cells(it.icon)
	for off, r := range it.text {
		if n >= width {
			break
		}
		style := lipgloss.NewStyle()
		if off >= it.nameLen {
			style = branchStyle
		}
		if matched[off] {
			style = matchStyle
		}
		b.WriteString(style.Render(string(r)))
		n++
	}
	pad := strings.Repeat(" ", max(width-n, 0))
	if i == m.cursor {
		return cursorStyle.Render(it.icon + truncate(it.text, max(width-cells(it.icon), 0)) + pad)
	}
	return it.style.Render(it.icon) + b.String() + pad
}

func (m *picker) View() string {
	width := max(m.width, 1)
	lines := []string{m.input.View(), separatorStyle.Render(strings.Repeat("\u2500", width))}
	list := max(m.height-3, 1)
	start := max(m.cursor-list+1, 0)
	for i := start; i < min(start+list, len(m.matches)); i++ {
		lines = append(lines, m.renderMatch(i, width))
	}
	if len(m.matches) == 0 && m.snap.Config != nil {
		lines = append(lines, closedStyle.Render("No matches"))
	}
	for len(lines) < m.height-1 {
		lines = append(lines, "")
	}
	status := branchStyle.Render(truncate(fmt.Sprintf("%d/%d", len(m.matches), len(m.items)), width))
	if m.err != nil {
		status = errorStyle.Render(truncate(m.err.Error(), width))
	}
	return strings.Join(append(lines, status), "\n")
}

// RunPicker runs the picker, meant for a kitty overlay, until something is chosen or it is left.
func RunPicker(ctx context.Context) error {
	input := textinput.New()
	input.Prompt = "\uf002  " // search
	input.Placeholder = "space, worktree or status"
	input.Focus()
	m := &picker{self: kitty.SelfWindowID(), input: input}
	if _, err := tea.NewProgram(m, tea.WithContext(ctx), tea.WithAltScreen()).Run(); err != nil {
		return fmt.Errorf("running the picker: %w", err)
	}
	return nil
}
