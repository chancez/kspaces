package tui

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/sahilm/fuzzy"

	"github.com/chancez/kspaces/internal/kitty"
	"github.com/chancez/kspaces/internal/spaces"
	"github.com/chancez/kspaces/internal/status"
	"github.com/chancez/kspaces/internal/tui/look"
)

// pickItem is one thing the picker can switch to: a worktree's session, or a window reporting a status.
type pickItem struct {
	// The text the query is matched against. It is also what is shown, so matches can be highlighted.
	text string
	// Where text stops being the name and becomes the faint detail after it, such as a branch.
	nameLen int
	icon    []look.Span

	// A worktree's row, with its space; or the status, for a window reporting one.
	row    spaces.Row
	status *status.Status
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
		items = append(items, pickItem{text: joinDetail(name, detail), nameLen: len(name), icon: look.StateMarker(s.State), status: s})
	}
	for _, r := range spaces.BuildRows(snap.Others, snap.Spaces, nil) {
		if r.Worktree == nil {
			continue
		}
		name := r.Space.Name
		if !r.Main {
			name += " / " + filepath.Base(r.Worktree.Path)
		}
		icon := []look.Span{look.NoMarker()}
		switch state := status.OfSession(snap.Kitty.Statuses, r.Worktree.Session); {
		case state != "":
			icon = look.StateMarker(state)
		case r.Worktree.Session == snap.Kitty.Current:
			icon = look.CurrentMarker()
		case snap.Kitty.Sessions[r.Worktree.Session] != 0:
			icon = look.SessionMarker()
		}
		items = append(items, pickItem{text: joinDetail(name, r.Worktree.Branch), nameLen: len(name), icon: icon, row: r})
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
	th      *look.Theme
	self    int
	snap    spaces.Snapshot
	items   pickItems
	matches fuzzy.Matches
	input   textinput.Model
	cursor  int
	width   int
	height  int
	err     error
	screen  pickScreen
	// getText fetches a window's screen, kitty.ScreenText but in tests.
	getText func(window int) (string, error)
}

func newPicker(self int) *picker {
	th := look.New(true)
	input := newInput(th, th.SearchPrompt())
	input.Placeholder = "space, worktree or status"
	input.Focus()
	return &picker{th: th, self: self, input: input, getText: kitty.ScreenText}
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
			err = spaces.OpenWindow(cfg, st, it.status.Window, it.status.Session)
		case it.row.Worktree.Session == st.Current:
			// Already here: closing the picker is all there is to do.
		default:
			err = spaces.OpenWorktree(cfg, st, *it.row.Worktree)
		}
		if err != nil {
			return spaces.Snapshot{Err: err}
		}
		return tea.QuitMsg{}
	}
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
		m.input.SetWidth(max(m.width-ansi.StringWidth(m.input.Prompt)-1, 1))
		return nil
	case spaces.Snapshot:
		m.err = msg.Err
		if msg.Config != nil {
			m.snap, m.items = msg, pickItemsFrom(msg)
			m.filter()
		}
		return nil
	case tea.KeyPressMsg:
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

// renderMatch draws an item with the characters the query matched highlighted, and its detail muted.
func (m *picker) renderMatch(i int, width int) string {
	th, match := m.th, m.matches[i]
	it := m.items[match.Index]
	if i == m.cursor {
		text := th.Cut(max(width-th.Width(it.icon...), 0), look.Text(it.text))
		return th.Draw(look.Cursor, width, append(append([]look.Span{}, it.icon...), text...)...)
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
	return th.Draw(look.Plain, width, spans...)
}

func (m *picker) View() tea.View {
	v := tea.NewView(m.render())
	v.AltScreen = true
	return v
}

func (m *picker) render() string {
	th, width := m.th, max(m.width, 1)
	lines := []string{m.input.View(), th.Render(look.Plain, look.Rule(width, false))}
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
	status := th.Render(look.Plain, th.Cut(width, look.Muted(fmt.Sprintf("%d/%d", len(m.matches), len(m.items))))...)
	if m.err != nil {
		status = th.Render(look.Plain, th.Cut(width, look.Error(m.err.Error()))...)
	}
	return strings.Join(append(lines, status), "\n")
}

// RunPicker runs the picker, meant for a kitty overlay, until something is chosen or it is left.
func RunPicker(ctx context.Context) error {
	if _, err := tea.NewProgram(newPicker(kitty.SelfWindowID()), tea.WithContext(ctx)).Run(); err != nil {
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
