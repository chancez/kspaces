// Package tui is kspaces' terminal UI: the sidebar, the picker and the new worktree form. How they draw
// comes from look, and what keys, menus and clicks do from the registry in registry.go.
package tui

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	"github.com/chancez/kspaces/internal/config"
	"github.com/chancez/kspaces/internal/gitstate"
	"github.com/chancez/kspaces/internal/kitty"
	"github.com/chancez/kspaces/internal/logs"
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

// rowHoverDelay is how long the mouse rests on a spaces row before its popup shows, so passing over rows
// shows nothing.
const rowHoverDelay = 2 * time.Second

// focusSettle is how long after the sidebar takes focus it scrolls to the cursor, unless a click came: kitty
// sends the focus just before the press that gave it, and scrolling first would move the rows under the
// mouse.
const focusSettle = 100 * time.Millisecond

type tickMsg struct{}

// failedMsg is an action run off the UI loop failing, as gh failing to open a PR.
type failedMsg struct{ err error }

// rowHoverMsg is rowHoverDelay passing since the mouse came to rest on a row, the gen-th time it did.
type rowHoverMsg struct {
	gen int
}

// focusSettledMsg is focusSettle passing since the sidebar took focus, the gen-th time it did.
type focusSettledMsg struct {
	gen int
}

type sidebar struct {
	th     *look.Theme
	cfg    config.Config
	self   int
	spaces []spaces.Space
	others []spaces.Space
	folded map[string]bool
	// The groups left out of the list, see spaces.Snapshot.
	hidden map[string]bool
	// How the rows are ordered, see spaces.Snapshot, and the order held while the mouse is on them, see
	// keepOrder.
	order spaces.Order
	held  map[string]int
	// Sessions with a saved file, see spaces.Snapshot.
	saved map[string]bool
	// Saved sessions that were suspended, see spaces.Snapshot.
	suspended map[string]bool
	// Sessions being suspended or closed, see spaces.Snapshot.
	keeping map[string]spaces.KeepMode
	// Worktrees being created or removed, by path, see spaces.Snapshot.
	busy  map[string]spaces.PendingOp
	kitty kitty.State
	// kitty's statuses, hidden groups' too, which kitty.Statuses leaves out, see spaces.Shown.
	statuses []status.Status
	rows     []spaces.Row
	// The context menu, nil when closed.
	menu *contextMenu[*sidebar]
	// help is the list of keys over the sidebar, nil when closed, see keyhelp.go.
	help   *keyHelp
	cursor int
	offset int
	// The first status entry shown, when there are more than fit.
	statusOffset int
	// The STATUS list's height in rows as dragging the rule left it, 0 for the default, see split.go, and
	// whether the rule is held or under the mouse.
	statusRows int
	splitting  bool
	splitHot   bool
	width      int
	height     int
	// err is the last load's error, until a load works.
	err error
	// failed is the last action's error. It is the user's own doing, so it shows over err, and stays
	// through the loads after it until a key or a click: a poll every second would otherwise wipe it.
	failed error
	// notice is what the last line says when there is no error: that the kitty files are out of date.
	notice *look.Span
	// Whether kitty has given the sidebar keyboard focus, and whether the scroll to the cursor that taking
	// it asks for waits on focusSettle, the focusGen-th focus's.
	focused     bool
	focusScroll bool
	focusGen    int
	// When keys arrive without focus, kitty is sending them from another window: kitty.conf's spaces mode
	// runs `send-key` to the sidebar. The cursor is drawn then too, until remoteAt is a while ago.
	remote   bool
	remoteAt time.Time
	// The window of the status entry the mouse rests on, drawn in full as a popup, or 0. It ends when the
	// mouse leaves the dock, which kitty says only in pixel mouse mode, and else hoverIdle after the last
	// move.
	hover   int
	hoverAt time.Time
	// The spaces row the mouse rests on, by rowKey, or "". Its details show as a popup, hoverShown, once
	// rowHoverDelay passes with the mouse still on it, and end as a status entry's do. hoverGen tells the
	// delay of the row the mouse is on from one it has left.
	hoverRow   string
	hoverShown bool
	hoverGen   int
	// Whether the mouse rests on the last line while it cuts an error, which then shows in full as a
	// popup, ending as a status entry's does.
	hoverError bool
	mouse      *pixelMouse
	// The left press on a spaces row until its release, nil for none: a click or a drag, see drag.go.
	press *press
	// The select mode, see select.go: whether it is on, the sessions ticked, and whether the action handed
	// to a form shows done, which ends it. bar is the bar's rows as the last frame drew them, barHints what
	// its hint line says of each, by action, and barHot the row under the mouse, see selectbar.go.
	selecting bool
	ticked    map[string]bool
	pending   func(m *sidebar) bool
	bar       controls[*sidebar]
	barHints  map[string]string
	barHot    string
	// Whether the mouse is on the add space button, the sort button, the attention filter's button or the
	// picker's button, which lights it.
	addHot, sortHot, attentionHot, pickHot bool
	// The zone of the clear button under the mouse, which lights it, or "", see statuslist.go.
	clearHot string
	// zones are where the last frame drew marked spans, by id, so a click maps back to what was drawn.
	zones map[string][]look.Zone
	// The spaces list's filters, see filter.go: the query narrows rows while filtering, the attention
	// filter while attending, the open filter while openOnly, and the suspended filter while awakeOnly.
	// All but the query are saved, see filtersChanged.
	filter    textinput.Model
	filtering bool
	attending bool
	openOnly  bool
	awakeOnly bool
	poller    *spaces.Poller
	// What git and gh say of each worktree, by path, from the poller.
	git map[string]gitstate.State
	// The worktrees' layouts diverged from their hosts', from the poller.
	diverged []spaces.Divergence
	// The provider's sessions no window shows, by space, see spaces.Snapshot, the spaces whose sessions
	// show under their head, see detached.go, and what the provider does.
	detached     map[string]spaces.Detached
	detachedOpen map[string]bool
	caps         spaces.Capabilities
	// providerSpaces are the provider's spaces, for removing a topic space.
	providerSpaces []string
	// The hosts tab, see hosts.go: whether it shows, every session the provider lists, and the host's and
	// space's rows folded there, by spaces.HostKey.
	hostsTab    bool
	sessions    []spaces.ProviderSession
	hostsFolded map[string]bool
	// remotes are the other hosts the config names, see spaces.RemoteView.
	remotes map[string]spaces.RemoteView
}

func newSidebar(self int) *sidebar {
	th := look.New(true)
	filters := spaces.LoadFilters()
	return &sidebar{
		th:         th,
		self:       self,
		folded:     map[string]bool{},
		hidden:     map[string]bool{},
		mouse:      &pixelMouse{},
		filter:     newFilter(th),
		poller:     spaces.NewPoller(),
		statusRows: spaces.LoadSplit(),
		attending:  filters.Attention,
		openOnly:   filters.Open,
		awakeOnly:  filters.Awake,
		hostsTab:   spaces.LoadHostsTab(),
	}
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

// focusCursor puts the cursor on the current session as cursorToCurrent does, for the sidebar taking focus.
// When that would scroll, the scroll waits for focusSettle, which a click before it cancels.
func (m *sidebar) focusCursor() tea.Cmd {
	offset, statusOffset := m.offset, m.statusOffset
	m.cursorToCurrent()
	if m.offset == offset && m.statusOffset == statusOffset {
		return nil
	}
	m.offset, m.statusOffset = offset, statusOffset
	m.focusScroll = true
	m.focusGen++
	gen := m.focusGen
	return tea.Tick(focusSettle, func(time.Time) tea.Msg { return focusSettledMsg{gen} })
}

// hostMsg is the host data, pushed by this host's server.
type hostMsg struct{ data spaces.HostData }

// remoteMsg is another host's host data loaded, or failing to.
type remoteMsg struct{}

// pushed polls with the host data pushed.
func (m *sidebar) pushed(hd spaces.HostData) tea.Cmd {
	p, self := m.poller, m.self
	return func() tea.Msg { return p.Pushed(hd).View(self) }
}

// poll loads, on the timer.
func (m *sidebar) poll() tea.Cmd {
	p, self := m.poller, m.self
	return func() tea.Msg { return p.Poll().View(self) }
}

// relist lists kitty's windows again into the latest poll, for kitty's focus moving.
func (m *sidebar) relist() tea.Cmd {
	p, self := m.poller, m.self
	return func() tea.Msg { return p.Windows().View(self) }
}

// refresh loads, for the refresh key, and has git and gh asked again.
func (m *sidebar) refresh() tea.Cmd {
	p, self := m.poller, m.self
	return func() tea.Msg { return p.Refresh().View(self) }
}

func tick() tea.Cmd {
	return tea.Tick(refreshEvery, func(time.Time) tea.Msg { return tickMsg{} })
}

// Init asks the terminal for its background too, which picks the controls' shade, see look.roles.
func (m *sidebar) Init() tea.Cmd {
	return tea.Batch(m.poll(), tick(), checkGlue, tea.RequestBackgroundColor)
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
			return failedMsg{err}
		}
		return p.Poll().View(self)
	}
}

func (m *sidebar) openWorktreeCmd(wt spaces.Worktree, opts spaces.OpenOptions) tea.Cmd {
	cfg, st, self, p := m.cfg, m.kitty, m.self, m.poller
	return func() tea.Msg {
		if err := spaces.OpenWorktree(cfg, st, wt, opts); err != nil {
			return failedMsg{err}
		}
		return p.Poll().View(self)
	}
}

// openOrResumeCmd opens wt's session as openWorktreeCmd does, unless it is suspended with no windows:
// then it asks first in the resume session form, as resuming brings back all it ran. A worktree being
// created or removed is not opened, see spaces.MarkPending: asked now, as the last poll may not know.
func (m *sidebar) openOrResumeCmd(wt spaces.Worktree, opts spaces.OpenOptions) tea.Cmd {
	if wt.Path != "" {
		if err := spaces.CheckPending(wt.Path); err != nil {
			m.failed = err
			return nil
		}
	}
	if !m.suspended[wt.Session] || m.isOpen(&wt) {
		return m.openWorktreeCmd(wt, opts)
	}
	cfg, st := m.cfg, m.kitty
	return func() tea.Msg {
		if err := spaces.AskResumeSession(cfg, st, wt.Session); err != nil {
			return failedMsg{err}
		}
		return nil
	}
}

func (m *sidebar) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var start tea.Cmd
	if _, ok := msg.(tea.MouseMsg); ok {
		start = m.mouse.start()
	}
	switch msg.(type) {
	case tea.KeyPressMsg, tea.MouseClickMsg:
		// The user has seen the error and moved on, unless to the error itself; what they do now may fail
		// anew.
		if !m.keepsError(msg) {
			m.failed = nil
		}
	}
	before := m.failed
	_, cmd := m.update(msg)
	if m.failed != nil && (before == nil || before.Error() != m.failed.Error()) {
		logs.Error("sidebar", m.failed)
	}
	// The filter belongs to navigating: it goes when the keys stop coming, so no sidebar is left narrowed.
	if m.filtering && !m.navigating() {
		m.closeFilter()
	}
	m.keepOrder()
	return m, tea.Batch(start, cmd)
}

func (m *sidebar) update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if m.help != nil {
		if cmd, handled := m.updateHelp(msg); handled {
			return m, cmd
		}
	}
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
		return m, m.focusCursor()
	case focusSettledMsg:
		if msg.gen == m.focusGen && m.focusScroll {
			m.focusScroll = false
			m.scrollToCursor()
		}
	case tea.BlurMsg:
		m.focused, m.focusScroll = false, false
	case tea.BackgroundColorMsg:
		m.th.SetDark(msg.IsDark())
	case hostMsg:
		return m, m.pushed(msg.data)
	case remoteMsg:
		return m, m.relist()
	case tickMsg:
		if m.remote && time.Since(m.remoteAt) > remoteIdle {
			m.remote = false
		}
		if (m.hover != 0 || m.hoverRow != "" || m.hoverError) && time.Since(m.hoverAt) > hoverIdle {
			m.unhover()
		}
		return m, tea.Batch(m.poll(), tick())
	case kittyFocusMsg:
		return m, m.relist()
	case rowHoverMsg:
		if msg.gen == m.hoverGen && m.hoverRow != "" {
			m.hoverShown = true
		}
	case glueMsg:
		m.notice = &msg.notice
	case failedMsg:
		m.failed = msg.err
	case spaces.Snapshot:
		m.err = msg.Err
		if msg.Config != nil {
			// Where the cursor is goes by the rows and entries it was drawn on, before they change.
			at := m.cursorAt()
			m.cfg, m.spaces, m.others, m.folded, m.saved, m.suspended, m.keeping, m.kitty = *msg.Config, msg.Spaces, msg.Others, msg.Folded, msg.Saved, msg.Suspended, msg.Keeping, msg.Kitty
			m.hidden, m.statuses, m.busy = msg.Hidden, msg.Kitty.Statuses, msg.Pending
			// Saying a worktree is being created or removed is over once it is not.
			if e, ok := errors.AsType[*spaces.PendingError](m.failed); ok && m.busy[e.Pending.Path] == "" {
				m.failed = nil
			}
			m.kitty.Statuses = spaces.Shown(m.cfg, m.hidden, m.statuses)
			m.order = msg.Order
			m.git, m.diverged = msg.Git, msg.Diverged
			m.detached, m.caps, m.providerSpaces, m.sessions, m.remotes = msg.Detached, msg.Caps, msg.ProviderSpaces, msg.Sessions, msg.Remotes
			m.rebuildRows(at)
			m.settle()
			m.statusOffset = min(m.statusOffset, max(len(m.kitty.Statuses)-1, 0))
		}
	case tea.MouseWheelMsg:
		m.focusScroll = false
		m.wheel(msg)
	case tea.MouseClickMsg:
		m.focusScroll = false
		return m, m.click(msg)
	case tea.MouseMotionMsg:
		switch {
		case msg.Button == tea.MouseLeft && m.splitting:
			m.splitTo(msg.Y)
		case m.splitting:
			// The release went elsewhere, as outside the window.
			m.endSplit()
			return m, m.motion(msg)
		case msg.Button == tea.MouseLeft:
			m.dragTo(msg.Y)
		case m.press != nil:
			// Moving with no button down: the release went elsewhere, as outside the window.
			m.cancelPress()
			return m, m.motion(msg)
		default:
			return m, m.motion(msg)
		}
	case tea.MouseReleaseMsg:
		if m.splitting {
			m.endSplit()
			return m, nil
		}
		return m, m.release(msg.X, msg.Y)
	case mouseLeftMsg:
		m.unhover()
		m.addHot, m.sortHot, m.attentionHot, m.pickHot, m.splitHot, m.barHot = false, false, false, false, false, ""
	case tea.KeyPressMsg:
		if m.focusScroll {
			m.focusScroll = false
			m.scrollToCursor()
		}
		if m.press != nil && m.press.drag != nil && msg.String() == "esc" {
			m.cancelPress()
			return m, nil
		}
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

// statusAt is where the status list starts by default, as a share of the rows below the SPACES title, so
// it stays put as entries come and go rather than growing up from the bottom.
const statusAt = 0.75

// layout splits the height between the two lists: spaces above, status below a separator and its title.
type layout struct {
	spaces int
	status int
	// The screen row of the first status entry, below the separator and the STATUS title.
	statusTop int
}

// layout is the height's split now: the select mode's bar takes the spaces list's last rows.
func (m *sidebar) layout() layout {
	avail := max(m.height-headerRows-1, 1)
	status := max(avail-max(int(float64(avail)*statusAt), 1)-2, 1)
	if m.statusRows > 0 {
		status = clampStatus(m.statusRows, avail)
	}
	spaces := max(avail-status-2, 1)
	l := layout{spaces: spaces, status: status, statusTop: headerRows + spaces + 2}
	if m.selecting {
		l.spaces = max(spaces-m.barHeight(), 1)
	}
	return l
}

func (m *sidebar) contentWidth() int {
	return max(m.width, 1)
}

// scrollToCursor scrolls the list the cursor is in so its entry shows.
func (m *sidebar) scrollToCursor() {
	if m.cursor >= len(m.rows) {
		m.scrollStatusToEntry(m.cursor - len(m.rows))
		return
	}
	m.scrollSpacesToCursor()
}

// moveCursor moves by delta entries, no further than the first or last, stepping over the blank row
// between spaces: on past it, or back when it is the last one that way.
func (m *sidebar) moveCursor(delta int) {
	if delta == 0 {
		return
	}
	step := 1
	if delta < 0 {
		step = -1
	}
	n := m.numEntries()
	blank := func(i int) bool { return i < len(m.rows) && m.rows[i].Blank() }
	target := min(max(m.cursor+delta, 0), n-1)
	i := target
	for i >= 0 && i < n && blank(i) {
		i += step
	}
	if i < 0 || i >= n {
		i = target - step
		for i*step > m.cursor*step && blank(i) {
			i -= step
		}
	}
	if i*step > m.cursor*step {
		m.cursor = i
	}
	m.scrollToCursor()
}

// pageRows is how many entries the list the cursor is in shows at once.
func (m *sidebar) pageRows() int {
	if m.cursor >= len(m.rows) {
		return m.layout().status
	}
	return m.layout().spaces
}

// leave gives keyboard focus back to the window the sidebar took it from.
func (m *sidebar) leave() tea.Cmd {
	st := m.kitty
	if st.Home == 0 {
		return nil
	}
	return func() tea.Msg {
		if err := spaces.Leave(st); err != nil {
			return failedMsg{err}
		}
		return nil
	}
}

// view is what the sidebar knows as a snapshot, for what reads one, as the row actions do.
func (m *sidebar) view() spaces.Snapshot {
	return spaces.Snapshot{Config: &m.cfg, Git: m.git, Diverged: m.diverged, Kitty: m.kitty, Saved: m.saved, Suspended: m.suspended, Keeping: m.keeping,
		Pending: m.busy, Caps: m.caps, ProviderSpaces: m.providerSpaces}
}

func (m *sidebar) isOpen(wt *spaces.Worktree) bool {
	_, ok := m.kitty.Sessions[wt.Session]
	return ok
}

func (m *sidebar) View() tea.View {
	v := tea.NewView(m.render())
	v.AltScreen = true
	// All motion, not just drags, so resting the mouse on a status entry or a row can show it in full.
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

// titleLine is SPACES with the filters on after it, see filterSummary, and the picker's button, the
// attention filter's button, the sort button and the add space button at its right end. The funnel is lit
// while either filter is on. The summary goes first when the line is short, whole, then the picker's
// button, which P stands in for, then the cell between the title and the buttons, so the narrowest sidebar
// still reads SPACES; only narrower is the title cut.
func (m *sidebar) titleLine(width int) string {
	th := m.th
	buttons := append(look.Mark(attentionZone, look.AttentionButton(m.attending || m.openOnly || m.awakeOnly, m.attentionHot)), look.Default(" "))
	buttons = append(append(buttons, look.Mark(sortZone, look.SortButton(m.order.Mode, m.sortHot))...), look.Default(" "))
	buttons = append(buttons, look.Mark(addZone, look.AddButton(m.addHot))...)
	if withPick := append(append(look.Mark(pickZone, look.PickButton(m.pickHot)), look.Default(" ")), buttons...); th.Width(m.tabTitle()...)+1+th.Width(withPick...) <= width {
		buttons = withPick
	}
	title := th.Cut(max(width-th.Width(buttons...), 0), m.tabTitle()...)
	if summary := m.filterSummary(); summary != "" {
		withSummary := append(append([]look.Span{}, title...), look.Default(" "), look.Muted(summary))
		if th.Width(withSummary...)+1+th.Width(buttons...) <= width {
			title = withSummary
		}
	}
	gap := look.Default(strings.Repeat(" ", max(width-th.Width(title...)-th.Width(buttons...), 0)))
	return th.Draw(look.Plain, width, append(append(title, gap), buttons...)...)
}

func (m *sidebar) frame() string {
	th, width, l := m.th, m.contentWidth(), m.layout()
	lines := []string{m.titleLine(width), m.filterLine()}
	lines = append(lines, m.spacesLines()...)
	for len(lines) < headerRows+l.spaces {
		lines = append(lines, "")
	}
	lines = append(lines, m.barLines(width)...)
	for len(lines) < l.statusTop-2 {
		lines = append(lines, "")
	}
	lines = append(lines, th.DrawZone(splitZone, look.Plain, width, look.Rule(width, m.splitting || m.splitHot)), m.statusTitleLine(width))
	if len(m.kitty.Statuses) == 0 {
		lines = append(lines, th.Render(look.Plain, th.Cut(width, look.Text("Nothing to report").With(look.Dim))...))
	}
	statusList, _ := m.statusLines()
	lines = append(lines, statusList...)
	// Fill the height, with errors on the last line.
	for len(lines) < m.height-1 {
		lines = append(lines, "")
	}
	if j := m.hovered(); j >= 0 {
		if popup, top := m.hoverPopup(j); top < len(lines) {
			copy(lines[top:], popup)
		}
	}
	if popup, top := m.rowPopup(); popup != nil && top < len(lines) {
		copy(lines[top:], popup)
	}
	if popup, top := m.errorPopup(); popup != nil && top < len(lines) {
		copy(lines[top:], popup)
	}
	m.drawMenu(lines)
	m.drawHelp(lines)
	status := ""
	if err := m.lastError(); err != nil {
		status = m.errorLine(err)
	} else if m.notice != nil {
		status = th.Render(look.Plain, th.Cut(width, *m.notice)...)
	}
	lines = append(lines, status)
	var b strings.Builder
	for i, line := range lines {
		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(look.Fit(line, width))
	}
	return b.String()
}

// RunSidebar runs the sidebar, meant for a kitty dock, until it is quit.
func RunSidebar(ctx context.Context) error {
	// kitty hangs up on the sidebar when its window closes, which would end it before it removes its socket.
	ctx, cancel := signal.NotifyContext(ctx, syscall.SIGHUP)
	defer cancel()
	// bubbletea reads TEA_TRACE when the program is made.
	if cfg, err := config.Load(); err == nil && cfg.Sidebar.Trace != "" {
		os.Setenv("TEA_TRACE", cfg.Sidebar.Trace)
	}
	m := newSidebar(kitty.SelfWindowID())
	prog := tea.NewProgram(m, tea.WithContext(ctx), tea.WithFilter(m.mouse.filter))
	if path := os.Getenv(focusSocketEnv); path != "" {
		defer listenFocus(path, func() { prog.Send(kittyFocusMsg{}) })()
	}
	// What changes the host data elsewhere, a form or a command, shows at once, and so does what another
	// host's server loads.
	defer spaces.ThisHost().Subscribe(func(hd spaces.HostData) { prog.Send(hostMsg{hd}) })()
	m.poller.OnRemote(func() { prog.Send(remoteMsg{}) })
	defer m.poller.Close()
	if _, err := prog.Run(); err != nil {
		return fmt.Errorf("running the sidebar: %w", err)
	}
	return nil
}
