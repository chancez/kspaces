package tui

import (
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/chancez/kspaces/internal/termtext"
)

// The screen of the window the picker's entry under the cursor stands for, at the bottom of its preview.
// It is fetched when the cursor lands on the entry and again every screenRefresh while it stays, one fetch
// at a time, off the update loop.

const (
	// screenRefresh is how often the screen is fetched again, about 25ms each.
	screenRefresh = time.Second
	// screenMinRows is the fewest rows of screen worth showing under the entry's details and the rule.
	screenMinRows = 3
)

// pickScreen is the screen the preview follows.
type pickScreen struct {
	// The window followed, zero for none, and how many windows have been followed: a fetch or tick for
	// one the cursor has left carries an older seq.
	window, seq int
	// The screen's lines, safe to draw; nil until fetched, and when the fetch failed.
	lines []string
	// A fetch is in flight.
	busy bool
}

// screenMsg is what a fetch of a window's screen gave.
type screenMsg struct {
	window, seq int
	text        string
	err         error
}

// screenTickMsg is time to fetch the screen again.
type screenTickMsg struct{ window, seq int }

// screenRoom is how many rows of screen fit in a preview of rows rows under details rows and the rule, or
// zero for too few.
func screenRoom(details, rows int) int {
	if n := rows - details - 1; n >= screenMinRows {
		return n
	}
	return 0
}

// screenWindow is the window whose screen the preview shows for the cursor's entry: a status entry's own
// window, a session's as kitty picks it, never the picker's or another of kspaces' own. Zero for none,
// and when the preview has no room for it.
func (m *picker) screenWindow() int {
	it, ok := m.current()
	l := m.layout()
	if !ok || l.previewRows == 0 || screenRoom(len(m.details(it, l.previewWidth)), l.previewRows) == 0 {
		return 0
	}
	st := m.snap.Kitty
	id := 0
	if it.status != nil {
		id = it.status.Window
	} else {
		id = st.Screens[it.row.Worktree.Session]
	}
	if id == m.self || st.Own[id] {
		return 0
	}
	return id
}

// follow starts following the screen of the cursor's entry when that is another window. A fetch for the
// window left is left to finish, and the new window's starts when it does.
func (m *picker) follow() tea.Cmd {
	w := m.screenWindow()
	if w == m.screen.window {
		return nil
	}
	m.screen = pickScreen{window: w, seq: m.screen.seq + 1, busy: m.screen.busy}
	if w == 0 || m.screen.busy {
		return nil
	}
	return m.fetchScreen()
}

func (m *picker) fetchScreen() tea.Cmd {
	m.screen.busy = true
	get, window, seq := m.getText, m.screen.window, m.screen.seq
	return func() tea.Msg {
		text, err := get(window)
		return screenMsg{window: window, seq: seq, text: text, err: err}
	}
}

// gotScreen shows a fetched screen and fetches it again after screenRefresh. A screen of a window the
// cursor has left is dropped, and the window it is on now fetched.
func (m *picker) gotScreen(msg screenMsg) tea.Cmd {
	m.screen.busy = false
	if msg.window != m.screen.window || msg.seq != m.screen.seq {
		if m.screen.window == 0 {
			return nil
		}
		return m.fetchScreen()
	}
	m.screen.lines = nil
	// A failed fetch, such as of a window just closed, shows no screen rather than an error.
	if msg.err == nil {
		m.screen.lines = screenLines(msg.text)
	}
	window, seq := msg.window, msg.seq
	return tea.Tick(screenRefresh, func(time.Time) tea.Msg { return screenTickMsg{window: window, seq: seq} })
}

// screenTick fetches the screen again while the cursor stays on its window.
func (m *picker) screenTick(msg screenTickMsg) tea.Cmd {
	if msg.window != m.screen.window || msg.seq != m.screen.seq || m.screen.busy {
		return nil
	}
	return m.fetchScreen()
}

// screenLines is a screen's text as lines safe to draw, without the blank rows below its last text.
func screenLines(text string) []string {
	lines := termtext.StyledLines(text)
	for len(lines) > 0 && strings.TrimSpace(ansi.Strip(lines[len(lines)-1])) == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

// screenBlock is the last rows of lines, where a shell has its prompt and latest output, each cut to width
// with its styles, which end with it.
func screenBlock(lines []string, width, rows int) []string {
	lines = lines[max(len(lines)-rows, 0):]
	out := make([]string, len(lines))
	for i, l := range lines {
		out[i] = ansi.Truncate(l, width, "")
		if strings.Contains(out[i], "\x1b") {
			out[i] += "\x1b[m"
		}
	}
	return out
}
