package tui

import (
	tea "charm.land/bubbletea/v2"

	"github.com/chancez/kspaces/internal/spaces"
	"github.com/chancez/kspaces/internal/status"
	"github.com/chancez/kspaces/internal/tui/interact"
)

// registry.go is the TUI's side of gr's ADR 10 registry: its subjects, their kinds, actions and keys, and
// its places. Only this file makes kinds and places. Keys, the sidebar's right click menu and its clicks
// all run actions from here.

// Subjects.

// entrySubject is a row of the sidebar's spaces list: a space, a session in no space, or a linked
// worktree, by its index in the rows. A folded space's row stands for all its worktrees.
type entrySubject struct {
	index int
	row   spaces.Row
}

// statusSubject is an entry of the sidebar's status list, by its index in the statuses.
type statusSubject struct {
	index  int
	status status.Status
}

// paneSubject is the sidebar itself, for the moves through it and what acts on no entry.
type paneSubject struct{}

// spacesSubject is the spaces list as a whole, for what adds to it rather than acting on one row.
type spacesSubject struct{}

// menuSubject is the sidebar's open context menu.
type menuSubject struct{}

// pickerSubject is the picker, formSubject the new worktree form, removeSubject the remove worktree form,
// closeSubject the close session form and addSubject the add space form.
type pickerSubject struct{}
type formSubject struct{}
type removeSubject struct{}
type closeSubject struct{}
type addSubject struct{}

// filterSubject is the spaces list's filter, while it is open.
type filterSubject struct{}

// do adapts a function that changes the model without a command.
func do[M, S any](f func(m M)) func(M, S) tea.Cmd {
	return func(m M, _ S) tea.Cmd { f(m); return nil }
}

// cmd adapts a function that returns a command.
func cmd[M, S any](f func(m M) tea.Cmd) func(M, S) tea.Cmd {
	return func(m M, _ S) tea.Cmd { return f(m) }
}

// quit ends the program, from wherever ctrl+c is pressed.
func quit[M, S any]() *interact.Action[M, S] {
	return &interact.Action[M, S]{Title: "quit", Run: func(M, S) tea.Cmd { return tea.Quit }}
}

// foldable is whether an entry's space has linked worktrees to fold, and the list shows folds: the filter
// ignores them.
func foldable(m *sidebar, s entrySubject) bool {
	return !m.filtering && !s.row.Other && len(s.row.Space.Worktrees) > 1
}

// The spaces list's entries. The titles are the context menu's.
var (
	actOpenWorktree = &interact.Action[*sidebar, entrySubject]{
		Title:   "Open",
		Applies: func(_ *sidebar, s entrySubject) bool { return s.row.Worktree != nil },
		Run: func(m *sidebar, s entrySubject) tea.Cmd {
			m.remote = false
			return m.openWorktreeCmd(*s.row.Worktree)
		},
	}
	actFold = &interact.Action[*sidebar, entrySubject]{
		Title:   "Fold worktrees",
		Applies: func(m *sidebar, s entrySubject) bool { return foldable(m, s) && !m.folded[s.row.Space.Root] },
		Run:     func(m *sidebar, s entrySubject) tea.Cmd { m.toggleFold(s.row.Space.Root); return nil },
	}
	actUnfold = &interact.Action[*sidebar, entrySubject]{
		Title:   "Unfold worktrees",
		Applies: func(m *sidebar, s entrySubject) bool { return foldable(m, s) && m.folded[s.row.Space.Root] },
		Run:     func(m *sidebar, s entrySubject) tea.Cmd { m.toggleFold(s.row.Space.Root); return nil },
	}
	actNewWorktree = &interact.Action[*sidebar, entrySubject]{
		Title:   "New worktree...",
		Applies: func(_ *sidebar, s entrySubject) bool { return !s.row.Other && s.row.Space.Err == nil },
		Run: func(m *sidebar, s entrySubject) tea.Cmd {
			m.remote = false
			return m.newWorktree(s.row.Space.Root)
		},
	}
	actRemoveWorktree = &interact.Action[*sidebar, entrySubject]{
		Title: "Remove worktree...",
		// Only a linked worktree: the main one is the repository itself.
		Applies: func(_ *sidebar, s entrySubject) bool { return !s.row.Main && s.row.Worktree != nil },
		Run: func(m *sidebar, s entrySubject) tea.Cmd {
			m.remote = false
			return m.removeWorktree(s.row.Worktree.Path)
		},
	}
	actCloseSession = &interact.Action[*sidebar, entrySubject]{
		Title: "Close session...",
		Applies: func(m *sidebar, s entrySubject) bool {
			return s.row.Worktree != nil && m.kitty.Windows[s.row.Worktree.Session] > 0
		},
		Run: func(m *sidebar, s entrySubject) tea.Cmd {
			m.remote = false
			return m.closeSession(s.row.Worktree.Session)
		},
	}
)

// The status list's entries.
var actOpenWindow = &interact.Action[*sidebar, statusSubject]{
	Title: "Open",
	Run: func(m *sidebar, s statusSubject) tea.Cmd {
		m.remote = false
		return m.openWindowCmd(s.status)
	},
}

// The spaces list's own, offered on every row and on the sidebar.
var actAddSpace = &interact.Action[*sidebar, spacesSubject]{
	Title: "Add space...",
	Run: func(m *sidebar, _ spacesSubject) tea.Cmd {
		m.remote = false
		return m.addSpace()
	},
}

// paneAction is an action on the sidebar.
func paneAction(title string, run func(m *sidebar) tea.Cmd) *interact.Action[*sidebar, paneSubject] {
	return &interact.Action[*sidebar, paneSubject]{Title: title, Run: cmd[*sidebar, paneSubject](run)}
}

// The sidebar's own: moves, leaving, refreshing, and the cursor's menu.
var (
	actDown   = paneAction("down", func(m *sidebar) tea.Cmd { m.moveCursor(1); return nil })
	actUp     = paneAction("up", func(m *sidebar) tea.Cmd { m.moveCursor(-1); return nil })
	actTop    = paneAction("top", func(m *sidebar) tea.Cmd { m.cursor = 0; m.scrollToCursor(); return nil })
	actBottom = paneAction("bottom", func(m *sidebar) tea.Cmd {
		m.cursor = max(m.numEntries()-1, 0)
		m.scrollToCursor()
		return nil
	})
	actLeave = paneAction("leave", func(m *sidebar) tea.Cmd {
		if m.remote {
			// Focus never left the window the keys came from, so there is nothing to give back.
			m.remote = false
			return nil
		}
		return m.leave()
	})
	actRefresh  = paneAction("refresh", func(m *sidebar) tea.Cmd { return m.refresh() })
	actMenuHere = paneAction("menu", func(m *sidebar) tea.Cmd { m.menuAtCursor(); return nil })
	// actEndRemote is enter where the cursor has nothing to open: remote navigation ends all the same,
	// as kitty.conf's mapping leaves its keyboard mode on enter.
	actEndRemote = paneAction("stop", func(m *sidebar) tea.Cmd { m.remote = false; return nil })
	actQuit      = quit[*sidebar, paneSubject]()
)

// actAttend focuses the window that needs me most, the first of the status list's blocked, failed and
// done entries, wherever the cursor is.
var actAttend = paneAction("attend", func(m *sidebar) tea.Cmd {
	need := spaces.Attention(m.kitty.Statuses)
	if len(need) == 0 {
		return nil
	}
	m.remote = false
	return m.openWindowCmd(need[0])
})

// actFilter opens the spaces list's filter.
var actFilter = paneAction("filter", (*sidebar).openFilter)

// The filter's: moving over the rows it shows, opening one, and closing it. Every other key types.
var (
	actFilterDown = &interact.Action[*sidebar, filterSubject]{
		Title: "down",
		Run:   do[*sidebar, filterSubject](func(m *sidebar) { m.moveFiltered(1) }),
	}
	actFilterUp = &interact.Action[*sidebar, filterSubject]{
		Title: "up",
		Run:   do[*sidebar, filterSubject](func(m *sidebar) { m.moveFiltered(-1) }),
	}
	actFilterOpen = &interact.Action[*sidebar, filterSubject]{
		Title: "open",
		Run:   cmd[*sidebar, filterSubject]((*sidebar).openFiltered),
	}
	actFilterClear = &interact.Action[*sidebar, filterSubject]{
		Title: "clear",
		Run:   do[*sidebar, filterSubject]((*sidebar).clearFilter),
	}
	actFilterQuit = quit[*sidebar, filterSubject]()
)

// The context menu's.
var (
	actMenuDown = &interact.Action[*sidebar, menuSubject]{
		Title: "down",
		Run:   do[*sidebar, menuSubject](func(m *sidebar) { m.menu.sel = min(m.menu.sel+1, len(m.menu.items)-1) }),
	}
	actMenuUp = &interact.Action[*sidebar, menuSubject]{
		Title: "up",
		Run:   do[*sidebar, menuSubject](func(m *sidebar) { m.menu.sel = max(m.menu.sel-1, 0) }),
	}
	actMenuRun = &interact.Action[*sidebar, menuSubject]{
		Title: "run",
		Run:   cmd[*sidebar, menuSubject](func(m *sidebar) tea.Cmd { return m.runMenuItem(m.menu.sel) }),
	}
	actMenuClose = &interact.Action[*sidebar, menuSubject]{
		Title: "close",
		Run:   do[*sidebar, menuSubject](func(m *sidebar) { m.menu = nil }),
	}
	actMenuQuit = quit[*sidebar, menuSubject]()
)

// The picker's.
var (
	actPickDown = &interact.Action[*picker, pickerSubject]{
		Title: "down",
		Run:   do[*picker, pickerSubject](func(m *picker) { m.cursor = min(m.cursor+1, max(len(m.matches)-1, 0)) }),
	}
	actPickUp = &interact.Action[*picker, pickerSubject]{
		Title: "up",
		Run:   do[*picker, pickerSubject](func(m *picker) { m.cursor = max(m.cursor-1, 0) }),
	}
	actPick = &interact.Action[*picker, pickerSubject]{
		Title: "switch",
		Run:   cmd[*picker, pickerSubject]((*picker).choose),
	}
	actPickQuit = quit[*picker, pickerSubject]()
)

// The new worktree form's.
var (
	actSwitchField = &interact.Action[*newWorktreeForm, formSubject]{
		Title: "next field",
		Run:   cmd[*newWorktreeForm, formSubject]((*newWorktreeForm).switchField),
	}
	actCreate = &interact.Action[*newWorktreeForm, formSubject]{
		Title:   "create",
		Applies: func(f *newWorktreeForm, _ formSubject) bool { return !f.busy },
		Run:     cmd[*newWorktreeForm, formSubject]((*newWorktreeForm).create),
	}
	// actWaitForCreate is enter while a worktree is being created, which does nothing.
	actWaitForCreate = &interact.Action[*newWorktreeForm, formSubject]{
		Title: "creating",
		Run:   func(*newWorktreeForm, formSubject) tea.Cmd { return nil },
	}
	actCancel = quit[*newWorktreeForm, formSubject]()
)

// The remove worktree form's.
var (
	actRemove = &interact.Action[*removeWorktreeForm, removeSubject]{
		Title: "remove",
		// Not what git would refuse: the background process that removes has no screen to say so.
		Applies: func(f *removeWorktreeForm, _ removeSubject) bool {
			return !f.busy && spaces.CanRemove(f.space, f.wt) == nil
		},
		Run: cmd[*removeWorktreeForm, removeSubject]((*removeWorktreeForm).remove),
	}
	actRemoveCancel = &interact.Action[*removeWorktreeForm, removeSubject]{
		Title:   "cancel",
		Applies: func(f *removeWorktreeForm, _ removeSubject) bool { return !f.busy },
		Run:     func(*removeWorktreeForm, removeSubject) tea.Cmd { return tea.Quit },
	}
	actRemoveQuit = quit[*removeWorktreeForm, removeSubject]()
)

// The close session form's.
var (
	actClose = &interact.Action[*closeSessionForm, closeSubject]{
		Title:   "close",
		Applies: func(f *closeSessionForm, _ closeSubject) bool { return !f.busy },
		Run:     cmd[*closeSessionForm, closeSubject]((*closeSessionForm).closeSession),
	}
	actCloseCancel = &interact.Action[*closeSessionForm, closeSubject]{
		Title:   "cancel",
		Applies: func(f *closeSessionForm, _ closeSubject) bool { return !f.busy },
		Run:     func(*closeSessionForm, closeSubject) tea.Cmd { return tea.Quit },
	}
	actCloseQuit = quit[*closeSessionForm, closeSubject]()
)

// The add space form's.
var (
	actAdd = &interact.Action[*addSpaceForm, addSubject]{
		Title:   "add",
		Applies: func(f *addSpaceForm, _ addSubject) bool { return !f.busy && f.problem == nil },
		Run:     cmd[*addSpaceForm, addSubject]((*addSpaceForm).add),
	}
	actAddCancel = quit[*addSpaceForm, addSubject]()
)

var (
	entryKind  *interact.Kind[*sidebar, entrySubject]
	statusKind *interact.Kind[*sidebar, statusSubject]
	paneKind   *interact.Kind[*sidebar, paneSubject]
	spacesKind *interact.Kind[*sidebar, spacesSubject]
	menuKind   *interact.Kind[*sidebar, menuSubject]
	pickerKind *interact.Kind[*picker, pickerSubject]
	formKind   *interact.Kind[*newWorktreeForm, formSubject]
	removeKind *interact.Kind[*removeWorktreeForm, removeSubject]
	closeKind  *interact.Kind[*closeSessionForm, closeSubject]
	addKind    *interact.Kind[*addSpaceForm, addSubject]
	filterKind *interact.Kind[*sidebar, filterSubject]

	spacesList    *interact.Place[*sidebar, entrySubject]
	statusList    *interact.Place[*sidebar, statusSubject]
	thePane       *interact.Place[*sidebar, paneSubject]
	theMenu       *interact.Place[*sidebar, menuSubject]
	thePicker     *interact.Place[*picker, pickerSubject]
	theForm       *interact.Place[*newWorktreeForm, formSubject]
	theRemoveForm *interact.Place[*removeWorktreeForm, removeSubject]
	theCloseForm  *interact.Place[*closeSessionForm, closeSubject]
	theAddForm    *interact.Place[*addSpaceForm, addSubject]
	theFilter     *interact.Place[*sidebar, filterSubject]
)

// Kinds and places are made in init, as their actions run code that reads them.
func init() {
	// Every row and the sidebar carry the list's own, so a row's menu offers them, even a row that has
	// nothing else, and their keys work wherever the cursor is.
	spacesKind = interact.NewKind(func(spacesSubject) string { return "Spaces" }, actAddSpace)
	spacesKind.Bind(actAddSpace, "a", "add space", "a")

	entryKind = interact.NewKind(func(s entrySubject) string { return s.row.Space.Name },
		actOpenWorktree, actFold, actUnfold, actNewWorktree, actCloseSession, actRemoveWorktree)
	entryKind.Bind(actFold, "space", "fold or unfold the space's worktrees", "space")
	entryKind.Bind(actUnfold, "", "", "space")
	entryKind.Bind(actNewWorktree, "n", "new worktree", "n")
	entryKind.Bind(actCloseSession, "c", "close session", "c")
	entryKind.Bind(actRemoveWorktree, "x", "remove worktree", "x")
	interact.Via(entryKind, func(entrySubject) (spacesSubject, bool) { return spacesSubject{}, true }, spacesKind)
	spacesList = interact.List(entryKind)
	spacesList.Bind(actOpenWorktree, "enter/l", "open", "enter", "l")

	statusKind = interact.NewKind(func(s statusSubject) string { return s.status.Session }, actOpenWindow)
	statusList = interact.List(statusKind)
	statusList.Bind(actOpenWindow, "enter/l", "open", "enter", "l")

	paneKind = interact.NewKind(func(paneSubject) string { return "Sidebar" },
		actDown, actUp, actTop, actBottom, actLeave, actRefresh, actMenuHere, actQuit,
		actAttend, actFilter)
	paneKind.Bind(actDown, "j/k", "down/up", "j", "down")
	paneKind.Bind(actUp, "", "", "k", "up")
	paneKind.Bind(actTop, "g/G", "top/bottom", "g", "home")
	paneKind.Bind(actBottom, "", "", "G", "end")
	paneKind.Bind(actLeave, "esc/q", "leave", "esc", "q")
	paneKind.Bind(actRefresh, "r", "refresh", "r")
	paneKind.Bind(actMenuHere, "m", "menu", "m")
	paneKind.BindQuiet(actEndRemote, "enter", "l")
	paneKind.Bind(actQuit, "ctrl+c", "quit", "ctrl+c")
	paneKind.Bind(actAttend, "!", "go to what needs you", "!")
	paneKind.Bind(actFilter, "/", "filter the spaces", "/")
	interact.Via(paneKind, func(paneSubject) (spacesSubject, bool) { return spacesSubject{}, true }, spacesKind)
	thePane = interact.List(paneKind)

	menuKind = interact.NewKind(func(menuSubject) string { return "Menu" },
		actMenuDown, actMenuUp, actMenuRun, actMenuClose, actMenuQuit)
	menuKind.Bind(actMenuDown, "j/k", "down/up", "j", "down")
	menuKind.Bind(actMenuUp, "", "", "k", "up")
	menuKind.Bind(actMenuRun, "enter/l", "run", "enter", "l")
	menuKind.Bind(actMenuClose, "esc/q/m", "close", "esc", "q", "m")
	menuKind.Bind(actMenuQuit, "ctrl+c", "quit", "ctrl+c")
	theMenu = interact.List(menuKind)

	pickerKind = interact.NewKind(func(pickerSubject) string { return "Picker" }, actPickDown, actPickUp, actPick, actPickQuit)
	pickerKind.Bind(actPickDown, "down/up", "down/up", "down", "ctrl+n", "ctrl+j")
	pickerKind.Bind(actPickUp, "", "", "up", "ctrl+p", "ctrl+k")
	pickerKind.Bind(actPick, "enter", "switch", "enter")
	pickerKind.Bind(actPickQuit, "esc", "cancel", "esc", "ctrl+c")
	thePicker = interact.List(pickerKind)

	formKind = interact.NewKind(func(formSubject) string { return "New worktree" }, actSwitchField, actCreate, actCancel)
	formKind.Bind(actSwitchField, "tab", "next field", "tab", "shift+tab", "up", "down")
	formKind.Bind(actCreate, "enter", "create", "enter")
	formKind.BindQuiet(actWaitForCreate, "enter")
	formKind.Bind(actCancel, "esc", "cancel", "esc", "ctrl+c")
	theForm = interact.List(formKind)

	removeKind = interact.NewKind(func(removeSubject) string { return "Remove worktree" }, actRemove, actRemoveCancel, actRemoveQuit)
	removeKind.Bind(actRemove, "y", "remove", "y")
	removeKind.Bind(actRemoveCancel, "n/esc", "cancel", "n", "esc")
	removeKind.Bind(actRemoveQuit, "ctrl+c", "quit", "ctrl+c")
	theRemoveForm = interact.List(removeKind)

	closeKind = interact.NewKind(func(closeSubject) string { return "Close session" }, actClose, actCloseCancel, actCloseQuit)
	closeKind.Bind(actClose, "y", "close", "y")
	closeKind.Bind(actCloseCancel, "n/esc", "cancel", "n", "esc")
	closeKind.Bind(actCloseQuit, "ctrl+c", "quit", "ctrl+c")
	theCloseForm = interact.List(closeKind)

	addKind = interact.NewKind(func(addSubject) string { return "Add space" }, actAdd, actAddCancel)
	addKind.Bind(actAdd, "enter", "add", "enter")
	addKind.Bind(actAddCancel, "esc", "cancel", "esc", "ctrl+c")
	theAddForm = interact.List(addKind)

	// j and k type, as worktrees are named with them, so only arrows and the picker's ctrl keys move.
	filterKind = interact.NewKind(func(filterSubject) string { return "Filter" },
		actFilterDown, actFilterUp, actFilterOpen, actFilterClear, actFilterQuit)
	filterKind.Bind(actFilterDown, "down/up", "down/up", "down", "ctrl+n", "ctrl+j")
	filterKind.Bind(actFilterUp, "", "", "up", "ctrl+p", "ctrl+k")
	filterKind.Bind(actFilterOpen, "enter", "open", "enter")
	filterKind.Bind(actFilterClear, "esc", "clear", "esc")
	filterKind.Bind(actFilterQuit, "ctrl+c", "quit", "ctrl+c")
	theFilter = interact.List(filterKind)
}

// Targets: the subjects on screen, each in its place.

// entryTarget is entry i of the sidebar, a row of the spaces list or a status entry, if there is one
// there: a blank row between groups is none.
func (m *sidebar) entryTarget(i int) (interact.Target[*sidebar], bool) {
	switch {
	case i >= 0 && i < len(m.rows) && m.rows[i].Space != nil:
		return spacesList.Target(entrySubject{index: i, row: m.rows[i]}), true
	case i >= len(m.rows) && i < m.numEntries():
		j := i - len(m.rows)
		return statusList.Target(statusSubject{index: j, status: m.kitty.Statuses[j]}), true
	}
	return interact.Target[*sidebar]{}, false
}

// chain is what a key reaches: the cursor's entry, then the sidebar.
func (m *sidebar) chain() []interact.Target[*sidebar] {
	var out []interact.Target[*sidebar]
	if t, ok := m.entryTarget(m.cursor); ok {
		out = append(out, t)
	}
	return append(out, thePane.Target(paneSubject{}))
}

// runOn runs what msg reaches on chain, and reports whether anything ran.
func runOn[M any](m M, msg tea.KeyPressMsg, chain ...interact.Target[M]) (tea.Cmd, bool) {
	a, ok := interact.Lookup(m, msg, chain...)
	if !ok {
		return nil, false
	}
	return a.Run(m), true
}

// runFirst runs the first of actions that applies on t, as a click does what a key would there.
func runFirst[M any](m M, t interact.Target[M], actions ...any) tea.Cmd {
	for _, a := range actions {
		for _, g := range t.Groups(m) {
			for _, b := range g.Actions {
				if b.Is(a) {
					return b.Run(m)
				}
			}
		}
	}
	return nil
}
