package tui

import (
	"fmt"

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

// forgetSubject is the forget session form.
type forgetSubject struct{}

// cleanupSubject is the clean up worktrees form.
type cleanupSubject struct{}

// runSubject is the run form.
type runSubject struct{}

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
	return !m.filtering && !s.row.Other && s.row.Space.HasLinked()
}

// The context menu's sections. A menu draws them in the order their first actions come in the kinds'
// lists: the session first, as opening it is what a row is mostly for; then what acts on the row's own
// worktree; then what is the same from any row of the space, and the list's own last.
const (
	sectionSession  = "Session"
	sectionWorktree = "Worktree"
	sectionSpace    = "Space"
)

// The spaces list's entries. The titles are the context menu's.
var (
	actOpenWorktree = &interact.Action[*sidebar, entrySubject]{
		Title:   "Open",
		Section: sectionSession,
		// A missing worktree only while its session has windows: opening one with none would make it
		// afresh in a directory that is gone.
		Applies: func(m *sidebar, s entrySubject) bool {
			return s.row.Worktree != nil && (!s.row.Missing || m.isOpen(s.row.Worktree))
		},
		Run: func(m *sidebar, s entrySubject) tea.Cmd {
			m.remote = false
			return m.openWorktreeCmd(*s.row.Worktree)
		},
	}
	actFold = &interact.Action[*sidebar, entrySubject]{
		Title:   "Fold worktrees",
		Section: sectionSpace,
		Applies: func(m *sidebar, s entrySubject) bool { return foldable(m, s) && !m.folded[s.row.Space.Root] },
		Run:     func(m *sidebar, s entrySubject) tea.Cmd { m.toggleFold(s.row.Space.Root); return nil },
	}
	actUnfold = &interact.Action[*sidebar, entrySubject]{
		Title:   "Unfold worktrees",
		Section: sectionSpace,
		Applies: func(m *sidebar, s entrySubject) bool { return foldable(m, s) && m.folded[s.row.Space.Root] },
		Run:     func(m *sidebar, s entrySubject) tea.Cmd { m.toggleFold(s.row.Space.Root); return nil },
	}
	actNewWorktree = &interact.Action[*sidebar, entrySubject]{
		Title:   "New worktree",
		Section: sectionSpace,
		Applies: func(_ *sidebar, s entrySubject) bool { return !s.row.Other && !s.row.Missing && s.row.Space.Err == nil },
		Run: func(m *sidebar, s entrySubject) tea.Cmd {
			m.remote = false
			return m.newWorktree(s.row.Space.Root)
		},
	}
	actRemoveWorktree = &interact.Action[*sidebar, entrySubject]{
		Title:   "Remove worktree",
		Section: sectionWorktree,
		// Only a linked worktree: the main one is the repository itself.
		Applies: func(_ *sidebar, s entrySubject) bool { return !s.row.Main && !s.row.Missing && s.row.Worktree != nil },
		Run: func(m *sidebar, s entrySubject) tea.Cmd {
			m.remote = false
			return m.removeWorktree(s.row.Worktree.Path)
		},
	}
	actOpenPR = &interact.Action[*sidebar, entrySubject]{
		Title:   "Open PR",
		Name:    func(m *sidebar, s entrySubject) string { return fmt.Sprintf("Open PR #%d", m.rowPR(s.row).Number) },
		Section: sectionWorktree,
		// Not on a missing worktree: its directory, where gh would run, is gone.
		Applies: func(m *sidebar, s entrySubject) bool { return !s.row.Missing && m.rowPR(s.row) != nil },
		Run: func(m *sidebar, s entrySubject) tea.Cmd {
			m.remote = false
			return openPR(s.row.Worktree.Path, m.rowPR(s.row).Number)
		},
	}
	actRunCommand = &interact.Action[*sidebar, entrySubject]{
		Title:   "Run",
		Section: sectionSession,
		Applies: func(m *sidebar, s entrySubject) bool {
			return !s.row.Other && !s.row.Missing && s.row.Worktree != nil && len(m.cfg.Commands) > 0
		},
		Run: func(m *sidebar, s entrySubject) tea.Cmd {
			m.remote = false
			return m.runCommand(s.row.Worktree.Path)
		},
	}
	actCloseSession = &interact.Action[*sidebar, entrySubject]{
		Title:   "Close session",
		Section: sectionSession,
		Applies: func(m *sidebar, s entrySubject) bool {
			return s.row.Worktree != nil && m.kitty.Windows[s.row.Worktree.Session] > 0
		},
		Run: func(m *sidebar, s entrySubject) tea.Cmd {
			m.remote = false
			return m.closeSession(s.row.Worktree.Session)
		},
	}
	actForgetSession = &interact.Action[*sidebar, entrySubject]{
		Title:   "Forget",
		Section: sectionWorktree,
		// A missing worktree with only its saved file left, which kitty would restore it from.
		Applies: func(m *sidebar, s entrySubject) bool {
			return s.row.Missing && m.kitty.Windows[s.row.Worktree.Session] == 0 && m.saved[s.row.Worktree.Session]
		},
		Run: func(m *sidebar, s entrySubject) tea.Cmd {
			m.remote = false
			return m.forgetSession(s.row.Worktree.Session)
		},
	}
	actCleanup = &interact.Action[*sidebar, entrySubject]{
		Title:   "Clean up worktrees",
		Section: sectionSpace,
		// On the space's own row: it offers removing any of the space's linked worktrees.
		Applies: func(_ *sidebar, s entrySubject) bool {
			return s.row.Main && !s.row.Other && s.row.Space.Err == nil && len(s.row.Space.Worktrees) > 1
		},
		Run: func(m *sidebar, s entrySubject) tea.Cmd {
			m.remote = false
			return m.cleanup(s.row.Space.Root)
		},
	}
)

// The status list's entries.
var actOpenWindow = &interact.Action[*sidebar, statusSubject]{
	Title:   "Open",
	Section: sectionSession,
	Run: func(m *sidebar, s statusSubject) tea.Cmd {
		m.remote = false
		return m.openWindowCmd(s.status)
	},
}

// The spaces list's own, offered on every row and on the sidebar.
var actAddSpace = &interact.Action[*sidebar, spacesSubject]{
	Title:   "Add space",
	Section: sectionSpace,
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
		Run:   cmd[*newWorktreeForm, formSubject]((*newWorktreeForm).nextField),
	}
	actPrevField = &interact.Action[*newWorktreeForm, formSubject]{
		Title: "previous field",
		Run:   cmd[*newWorktreeForm, formSubject]((*newWorktreeForm).prevField),
	}
	// On the choice of command only, so left and right move the cursor in a text field.
	actNextCommand = &interact.Action[*newWorktreeForm, formSubject]{
		Title:   "next command",
		Applies: func(f *newWorktreeForm, _ formSubject) bool { return f.onCommand() },
		Run:     func(f *newWorktreeForm, _ formSubject) tea.Cmd { return f.cycleCommand(1) },
	}
	actPrevCommand = &interact.Action[*newWorktreeForm, formSubject]{
		Title:   "previous command",
		Applies: func(f *newWorktreeForm, _ formSubject) bool { return f.onCommand() },
		Run:     func(f *newWorktreeForm, _ formSubject) tea.Cmd { return f.cycleCommand(-1) },
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
	actRemoveDeleteBranch = &interact.Action[*removeWorktreeForm, removeSubject]{
		Title:   "delete branch",
		Applies: func(f *removeWorktreeForm, _ removeSubject) bool { return !f.busy && f.branch.Offered() },
		Run:     do[*removeWorktreeForm, removeSubject](func(f *removeWorktreeForm) { f.deleteBranch = !f.deleteBranch }),
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

// The forget session form's.
var (
	actForget = &interact.Action[*forgetSessionForm, forgetSubject]{
		Title:   "delete",
		Applies: func(f *forgetSessionForm, _ forgetSubject) bool { return !f.busy },
		Run:     cmd[*forgetSessionForm, forgetSubject]((*forgetSessionForm).forget),
	}
	actForgetCancel = &interact.Action[*forgetSessionForm, forgetSubject]{
		Title:   "cancel",
		Applies: func(f *forgetSessionForm, _ forgetSubject) bool { return !f.busy },
		Run:     func(*forgetSessionForm, forgetSubject) tea.Cmd { return tea.Quit },
	}
	actForgetQuit = quit[*forgetSessionForm, forgetSubject]()
)

// The clean up worktrees form's.
var (
	actCleanupDown = &interact.Action[*cleanupForm, cleanupSubject]{
		Title: "down",
		Run:   do[*cleanupForm, cleanupSubject](func(f *cleanupForm) { f.cursor = min(f.cursor+1, max(len(f.items)-1, 0)) }),
	}
	actCleanupUp = &interact.Action[*cleanupForm, cleanupSubject]{
		Title: "up",
		Run:   do[*cleanupForm, cleanupSubject](func(f *cleanupForm) { f.cursor = max(f.cursor-1, 0) }),
	}
	actCleanupToggle = &interact.Action[*cleanupForm, cleanupSubject]{
		Title:   "tick",
		Applies: func(f *cleanupForm, _ cleanupSubject) bool { return !f.busy && len(f.items) > 0 },
		Run:     do[*cleanupForm, cleanupSubject](func(f *cleanupForm) { f.toggle(f.cursor) }),
	}
	actCleanupRemove = &interact.Action[*cleanupForm, cleanupSubject]{
		Title:   "remove",
		Applies: func(f *cleanupForm, _ cleanupSubject) bool { return !f.busy && len(f.chosen()) > 0 },
		Run:     cmd[*cleanupForm, cleanupSubject]((*cleanupForm).remove),
	}
	actCleanupCancel = &interact.Action[*cleanupForm, cleanupSubject]{
		Title:   "cancel",
		Applies: func(f *cleanupForm, _ cleanupSubject) bool { return !f.busy },
		Run:     func(*cleanupForm, cleanupSubject) tea.Cmd { return tea.Quit },
	}
	actCleanupQuit = quit[*cleanupForm, cleanupSubject]()
)

// The run form's.
var (
	actRunNextField = &interact.Action[*runForm, runSubject]{
		Title: "next field",
		Run:   cmd[*runForm, runSubject]((*runForm).nextField),
	}
	actRunPrevField = &interact.Action[*runForm, runSubject]{
		Title: "previous field",
		Run:   cmd[*runForm, runSubject]((*runForm).prevField),
	}
	actRunNextCommand = &interact.Action[*runForm, runSubject]{
		Title:   "next command",
		Applies: func(f *runForm, _ runSubject) bool { return f.onCommand() },
		Run:     func(f *runForm, _ runSubject) tea.Cmd { return f.cycleCommand(1) },
	}
	actRunPrevCommand = &interact.Action[*runForm, runSubject]{
		Title:   "previous command",
		Applies: func(f *runForm, _ runSubject) bool { return f.onCommand() },
		Run:     func(f *runForm, _ runSubject) tea.Cmd { return f.cycleCommand(-1) },
	}
	actRun = &interact.Action[*runForm, runSubject]{
		Title:   "run",
		Applies: func(f *runForm, _ runSubject) bool { return !f.busy },
		Run:     cmd[*runForm, runSubject]((*runForm).run),
	}
	// actWaitForRun is enter while the command is being started, which does nothing.
	actWaitForRun = &interact.Action[*runForm, runSubject]{
		Title: "running",
		Run:   func(*runForm, runSubject) tea.Cmd { return nil },
	}
	actRunCancel = quit[*runForm, runSubject]()
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
	forgetKind *interact.Kind[*forgetSessionForm, forgetSubject]
	addKind    *interact.Kind[*addSpaceForm, addSubject]
	cleanKind  *interact.Kind[*cleanupForm, cleanupSubject]
	runKind    *interact.Kind[*runForm, runSubject]
	filterKind *interact.Kind[*sidebar, filterSubject]

	spacesList    *interact.Place[*sidebar, entrySubject]
	statusList    *interact.Place[*sidebar, statusSubject]
	thePane       *interact.Place[*sidebar, paneSubject]
	theMenu       *interact.Place[*sidebar, menuSubject]
	thePicker     *interact.Place[*picker, pickerSubject]
	theForm       *interact.Place[*newWorktreeForm, formSubject]
	theRemoveForm *interact.Place[*removeWorktreeForm, removeSubject]
	theCloseForm  *interact.Place[*closeSessionForm, closeSubject]
	theForgetForm *interact.Place[*forgetSessionForm, forgetSubject]
	theAddForm    *interact.Place[*addSpaceForm, addSubject]
	theCleanup    *interact.Place[*cleanupForm, cleanupSubject]
	theRunForm    *interact.Place[*runForm, runSubject]
	theFilter     *interact.Place[*sidebar, filterSubject]
)

// Kinds and places are made in init, as their actions run code that reads them.
func init() {
	// Every row and the sidebar carry the list's own, so a row's menu offers them, even a row that has
	// nothing else, and their keys work wherever the cursor is.
	spacesKind = interact.NewKind(func(spacesSubject) string { return "Spaces" }, actAddSpace)
	spacesKind.Bind(actAddSpace, "a", "add space", "a")

	entryKind = interact.NewKind(func(s entrySubject) string { return s.row.Space.Name },
		actOpenWorktree, actRunCommand, actCloseSession, actOpenPR, actRemoveWorktree, actForgetSession, actNewWorktree, actFold, actUnfold, actCleanup)
	entryKind.Bind(actFold, "space", "fold or unfold the space's worktrees", "space")
	entryKind.Bind(actUnfold, "", "", "space")
	entryKind.Bind(actNewWorktree, "n", "new worktree", "n")
	entryKind.Bind(actRunCommand, "R", "run a command", "R")
	entryKind.Bind(actCloseSession, "c", "close session", "c")
	entryKind.Bind(actOpenPR, "b", "open the PR in the browser", "b")
	entryKind.Bind(actRemoveWorktree, "x", "remove worktree", "x")
	// x on a missing worktree, which has nothing to remove but its saved file.
	entryKind.Bind(actForgetSession, "", "", "x")
	entryKind.Bind(actCleanup, "p", "clean up worktrees", "p")
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

	formKind = interact.NewKind(func(formSubject) string { return "New worktree" },
		actSwitchField, actPrevField, actNextCommand, actPrevCommand, actCreate, actCancel)
	formKind.Bind(actSwitchField, "tab", "next field", "tab", "down")
	formKind.Bind(actPrevField, "shift+tab", "previous field", "shift+tab", "up")
	formKind.Bind(actNextCommand, "left/right", "command", "right")
	formKind.Bind(actPrevCommand, "", "", "left")
	formKind.Bind(actCreate, "enter", "create", "enter")
	formKind.BindQuiet(actWaitForCreate, "enter")
	formKind.Bind(actCancel, "esc", "cancel", "esc", "ctrl+c")
	theForm = interact.List(formKind)

	removeKind = interact.NewKind(func(removeSubject) string { return "Remove worktree" },
		actRemove, actRemoveCancel, actRemoveDeleteBranch, actRemoveQuit)
	removeKind.Bind(actRemove, "y", "remove", "y")
	removeKind.Bind(actRemoveCancel, "n/esc", "cancel", "n", "esc")
	removeKind.Bind(actRemoveDeleteBranch, "d", "delete branch", "d")
	removeKind.Bind(actRemoveQuit, "ctrl+c", "quit", "ctrl+c")
	theRemoveForm = interact.List(removeKind)

	cleanKind = interact.NewKind(func(cleanupSubject) string { return "Clean up worktrees" },
		actCleanupDown, actCleanupUp, actCleanupToggle, actCleanupRemove, actCleanupCancel, actCleanupQuit)
	cleanKind.Bind(actCleanupDown, "j/k", "down/up", "j", "down")
	cleanKind.Bind(actCleanupUp, "", "", "k", "up")
	cleanKind.Bind(actCleanupToggle, "space", "tick", "space")
	cleanKind.Bind(actCleanupRemove, "y", "remove", "y")
	cleanKind.Bind(actCleanupCancel, "n/esc", "cancel", "n", "esc")
	cleanKind.Bind(actCleanupQuit, "ctrl+c", "quit", "ctrl+c")
	theCleanup = interact.List(cleanKind)

	closeKind = interact.NewKind(func(closeSubject) string { return "Close session" }, actClose, actCloseCancel, actCloseQuit)
	closeKind.Bind(actClose, "y", "close", "y")
	closeKind.Bind(actCloseCancel, "n/esc", "cancel", "n", "esc")
	closeKind.Bind(actCloseQuit, "ctrl+c", "quit", "ctrl+c")
	theCloseForm = interact.List(closeKind)

	forgetKind = interact.NewKind(func(forgetSubject) string { return "Forget session" }, actForget, actForgetCancel, actForgetQuit)
	forgetKind.Bind(actForget, "y", "delete", "y")
	forgetKind.Bind(actForgetCancel, "n/esc", "cancel", "n", "esc")
	forgetKind.Bind(actForgetQuit, "ctrl+c", "quit", "ctrl+c")
	theForgetForm = interact.List(forgetKind)

	addKind = interact.NewKind(func(addSubject) string { return "Add space" }, actAdd, actAddCancel)
	addKind.Bind(actAdd, "enter", "add", "enter")
	addKind.Bind(actAddCancel, "esc", "cancel", "esc", "ctrl+c")
	theAddForm = interact.List(addKind)

	runKind = interact.NewKind(func(runSubject) string { return "Run" },
		actRunNextField, actRunPrevField, actRunNextCommand, actRunPrevCommand, actRun, actRunCancel)
	runKind.Bind(actRunNextField, "tab", "next field", "tab", "down")
	runKind.Bind(actRunPrevField, "shift+tab", "previous field", "shift+tab", "up")
	runKind.Bind(actRunNextCommand, "left/right", "command", "right")
	runKind.Bind(actRunPrevCommand, "", "", "left")
	runKind.Bind(actRun, "enter", "run", "enter")
	runKind.BindQuiet(actWaitForRun, "enter")
	runKind.Bind(actRunCancel, "esc", "cancel", "esc", "ctrl+c")
	theRunForm = interact.List(runKind)

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
