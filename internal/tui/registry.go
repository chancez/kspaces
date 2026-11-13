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

// groupSubject is a group's header in the spaces list, by the group's name.
type groupSubject struct{ name string }

// groupFilterSubject is a group in the spaces list's menu, which shows or hides it.
type groupFilterSubject struct{ name string }

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

// layoutSubject is the layout form.
type layoutSubject struct{}

// saveSubject is the save layout form.
type saveSubject struct{}

// trustSubject is the trust space config form.
type trustSubject struct{}

// replySubject is the reply form.
type replySubject struct{}

// listSubject is a choice field's open list.
type listSubject struct{}

// filterSubject is the spaces list's filter, while it is open.
type filterSubject struct{}

// sortSubject is the spaces list as a whole, whose menu, of the sort button, the SPACES title and the
// list's empty rows, chooses the sort mode and turns the attention filter on and off.
type sortSubject struct{}

// errorSubject is the error on the sidebar's last line, as it read when the mouse came to it.
type errorSubject struct{ text string }

// showSubject is the show error form.
type showSubject struct{}

// selectSubject is the select mode, while it is on: the ticked rows, and what acts on them.
type selectSubject struct{}

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
			return m.openWorktreeCmd(*s.row.Worktree, spaces.OpenOptions{})
		},
	}
	actOpenPlain = &interact.Action[*sidebar, entrySubject]{
		Title:   "Open without layout",
		Section: sectionSession,
		// Only where the default layout would open: elsewhere it opens just as Open does.
		Applies: func(m *sidebar, s entrySubject) bool {
			return !s.row.Other && !s.row.Missing && s.row.Worktree != nil &&
				spaces.OpensDefaultLayout(m.cfg, m.kitty, *s.row.Space, *s.row.Worktree)
		},
		Run: func(m *sidebar, s entrySubject) tea.Cmd {
			m.remote = false
			return m.openWorktreeCmd(*s.row.Worktree, spaces.OpenOptions{NoLayout: true})
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
			return !s.row.Other && !s.row.Missing && s.row.Worktree != nil && len(s.row.Space.ConfigOf(m.cfg).Commands) > 0
		},
		Run: func(m *sidebar, s entrySubject) tea.Cmd {
			m.remote = false
			return m.runCommand(s.row)
		},
	}
	actOpenLayout = &interact.Action[*sidebar, entrySubject]{
		Title:   "Open layout",
		Section: sectionSession,
		Applies: func(m *sidebar, s entrySubject) bool {
			return !s.row.Other && !s.row.Missing && s.row.Worktree != nil && len(s.row.Space.ConfigOf(m.cfg).Layouts) > 0
		},
		Run: func(m *sidebar, s entrySubject) tea.Cmd {
			m.remote = false
			return m.openLayout(s.row)
		},
	}
	actSaveAsLayout = &interact.Action[*sidebar, entrySubject]{
		Title:   "Save as layout",
		Section: sectionSession,
		// A worktree's session with windows to save, whose cwds are templates of the worktree's.
		Applies: func(m *sidebar, s entrySubject) bool {
			return !s.row.Other && !s.row.Missing && s.row.Worktree != nil && m.kitty.Windows[s.row.Worktree.Session] > 0
		},
		Run: func(m *sidebar, s entrySubject) tea.Cmd {
			m.remote = false
			return m.saveLayout(s.row.Worktree.Path)
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
	actTrustConfig = &interact.Action[*sidebar, entrySubject]{
		Title:   "Trust space config",
		Section: sectionSpace,
		// Its file is there, but not trusted as it is: never, or not since it changed.
		Applies: func(_ *sidebar, s entrySubject) bool { return !s.row.Other && s.row.Space.Untrusted },
		Run: func(m *sidebar, s entrySubject) tea.Cmd {
			m.remote = false
			return m.trustConfig(s.row.Space.Root)
		},
	}
	// actSelectRow turns the select mode on with the row ticked.
	actSelectRow = &interact.Action[*sidebar, entrySubject]{
		Title:   "Select",
		Section: sectionSpace,
		Applies: func(m *sidebar, s entrySubject) bool { return !m.selecting && s.row.Worktree != nil },
		Run:     func(m *sidebar, s entrySubject) tea.Cmd { m.tickRow(rowKey(s.row)); return nil },
	}
)

// A group's header's. A click on it does what space does, as there is nothing to open.
var (
	actFoldGroup = &interact.Action[*sidebar, groupSubject]{
		Title:   "Fold group",
		Section: "Group",
		Applies: func(m *sidebar, s groupSubject) bool { return !m.filtering && !m.folded[spaces.GroupKey(s.name)] },
		Run:     func(m *sidebar, s groupSubject) tea.Cmd { m.toggleGroupFold(s.name); return nil },
	}
	actUnfoldGroup = &interact.Action[*sidebar, groupSubject]{
		Title:   "Unfold group",
		Section: "Group",
		Applies: func(m *sidebar, s groupSubject) bool { return !m.filtering && m.folded[spaces.GroupKey(s.name)] },
		Run:     func(m *sidebar, s groupSubject) tea.Cmd { m.toggleGroupFold(s.name); return nil },
	}
	actSelectGroup = &interact.Action[*sidebar, groupSubject]{
		Title:   "Select",
		Section: sectionSpace,
		Applies: func(m *sidebar, _ groupSubject) bool { return !m.selecting },
		Run:     func(m *sidebar, s groupSubject) tea.Cmd { m.tickRow(spaces.GroupKey(s.name)); return nil },
	}
)

// The status list's entries.
var (
	actOpenWindow = &interact.Action[*sidebar, statusSubject]{
		Title:   "Open",
		Section: sectionSession,
		Run: func(m *sidebar, s statusSubject) tea.Cmd {
			m.remote = false
			return m.openWindowCmd(s.status)
		},
	}
	actReply = &interact.Action[*sidebar, statusSubject]{
		Title:   "Reply",
		Section: sectionSession,
		Run: func(m *sidebar, s statusSubject) tea.Cmd {
			m.remote = false
			return m.reply(s.status.Window)
		},
	}
)

// The sidebar's last line, while it shows an error.
var actShowError = &interact.Action[*sidebar, errorSubject]{
	Title: "Show error",
	Run: func(m *sidebar, s errorSubject) tea.Cmd {
		m.remote = false
		return m.showError(s.text)
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

// actAttention toggles the attention filter, see toggleAttention.
var actAttention = paneAction("attention filter", (*sidebar).toggleAttention)

// actCycleSort goes on to the next sort mode, as a left click on the sort button does.
var actCycleSort = paneAction("sort", func(m *sidebar) tea.Cmd { m.setSort(m.sortMode().Next()); return nil })

// actMoveUp and actMoveDown move the cursor's space, or linked worktree within its space, as dragging it
// does.
var (
	actMoveUp   = paneAction("move up", func(m *sidebar) tea.Cmd { return m.moveRow(-1) })
	actMoveDown = paneAction("move down", func(m *sidebar) tea.Cmd { return m.moveRow(1) })
)

// actSelect turns the select mode on, see select.go.
var actSelect = paneAction("select", func(m *sidebar) tea.Cmd { m.startSelect(); return nil })

// The select mode's: ticking rows, and leaving. Each applies throughout the mode, so a key that does
// nothing here does not reach the cursor's row, where x would remove its one worktree.
var (
	actTick = &interact.Action[*sidebar, selectSubject]{
		Title: "tick",
		Run:   do[*sidebar, selectSubject]((*sidebar).tickCursor),
	}
	actTickAll = &interact.Action[*sidebar, selectSubject]{
		Title: "tick all",
		Run:   do[*sidebar, selectSubject]((*sidebar).tickAll),
	}
	actEndSelect = &interact.Action[*sidebar, selectSubject]{
		Title: "Cancel",
		Run:   do[*sidebar, selectSubject]((*sidebar).endSelect),
	}
)

// sortAction chooses mode from the sort button's menu.
func sortAction(title string, mode spaces.SortMode) *interact.Action[*sidebar, sortSubject] {
	return &interact.Action[*sidebar, sortSubject]{
		Title:   title,
		Section: "Sort",
		Run:     do[*sidebar, sortSubject](func(m *sidebar) { m.setSort(mode) }),
	}
}

// The sort button's menu, by mode.
var sortActions = map[spaces.SortMode]*interact.Action[*sidebar, sortSubject]{
	spaces.SortManual:   sortAction("Manual", spaces.SortManual),
	spaces.SortActivity: sortAction("Activity", spaces.SortActivity),
	spaces.SortStatus:   sortAction("Status", spaces.SortStatus),
}

// actAttentionItem is the attention filter in the spaces list's menu, in a section of its own as it is a
// switch rather than one of the sort modes.
var actAttentionItem = &interact.Action[*sidebar, sortSubject]{
	Title:   "Only what is not idle",
	Section: "Filter",
	Run:     func(m *sidebar, _ sortSubject) tea.Cmd { return m.toggleAttention() },
}

// actSelectList turns the select mode on from the spaces list's menu, with nothing ticked.
var actSelectList = &interact.Action[*sidebar, sortSubject]{
	Title:   "Select",
	Section: "Rows",
	Applies: func(m *sidebar, _ sortSubject) bool { return !m.selecting },
	Run:     func(m *sidebar, _ sortSubject) tea.Cmd { m.startSelect(); return nil },
}

// actShowGroup shows or hides a group, in the spaces list's menu, which marks it while it is shown.
var actShowGroup = &interact.Action[*sidebar, groupFilterSubject]{
	Title:   "group",
	Name:    func(_ *sidebar, s groupFilterSubject) string { return s.name },
	Section: "Groups",
	Run:     func(m *sidebar, s groupFilterSubject) tea.Cmd { m.toggleHidden(s.name); return nil },
}

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

// A choice field's open list's. Typing filters it, so j and k move only before anything is typed.
var (
	actListDown = &interact.Action[*choiceList, listSubject]{
		Title: "down",
		Run:   do[*choiceList, listSubject](func(l *choiceList) { l.move(1) }),
	}
	actListUp = &interact.Action[*choiceList, listSubject]{
		Title: "up",
		Run:   do[*choiceList, listSubject](func(l *choiceList) { l.move(-1) }),
	}
	actListJ = &interact.Action[*choiceList, listSubject]{
		Title:   "down",
		Applies: func(l *choiceList, _ listSubject) bool { return l.filter == "" },
		Run:     do[*choiceList, listSubject](func(l *choiceList) { l.move(1) }),
	}
	actListK = &interact.Action[*choiceList, listSubject]{
		Title:   "up",
		Applies: func(l *choiceList, _ listSubject) bool { return l.filter == "" },
		Run:     do[*choiceList, listSubject](func(l *choiceList) { l.move(-1) }),
	}
	actListPick = &interact.Action[*choiceList, listSubject]{
		Title: "choose",
		Run:   do[*choiceList, listSubject]((*choiceList).pick),
	}
	actListErase = &interact.Action[*choiceList, listSubject]{
		Title: "erase",
		Run:   do[*choiceList, listSubject]((*choiceList).erase),
	}
	actListClose = &interact.Action[*choiceList, listSubject]{
		Title: "close",
		Run:   do[*choiceList, listSubject]((*choiceList).close),
	}
	actListQuit = quit[*choiceList, listSubject]()
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
		Run:   func(m *picker, _ pickerSubject) tea.Cmd { return m.choose(spaces.OpenOptions{}) },
	}
	actPickPlain = &interact.Action[*picker, pickerSubject]{
		Title:   "switch without layout",
		Applies: func(m *picker, _ pickerSubject) bool { return m.opensLayout() },
		Run:     func(m *picker, _ pickerSubject) tea.Cmd { return m.choose(spaces.OpenOptions{NoLayout: true}) },
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
	// On a choice, of layout or command, only, so left and right move the cursor in a text field.
	actNextChoice = &interact.Action[*newWorktreeForm, formSubject]{
		Title:   "next choice",
		Applies: func(f *newWorktreeForm, _ formSubject) bool { return f.onLayout() || f.onCommand() },
		Run:     func(f *newWorktreeForm, _ formSubject) tea.Cmd { return f.cycleChoice(1) },
	}
	actPrevChoice = &interact.Action[*newWorktreeForm, formSubject]{
		Title:   "previous choice",
		Applies: func(f *newWorktreeForm, _ formSubject) bool { return f.onLayout() || f.onCommand() },
		Run:     func(f *newWorktreeForm, _ formSubject) tea.Cmd { return f.cycleChoice(-1) },
	}
	actFormList = &interact.Action[*newWorktreeForm, formSubject]{
		Title:   "list",
		Applies: func(f *newWorktreeForm, _ formSubject) bool { return !f.busy && (f.onLayout() || f.onCommand()) },
		Run:     do[*newWorktreeForm, formSubject]((*newWorktreeForm).openList),
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
	actCleanupTickAll = &interact.Action[*cleanupForm, cleanupSubject]{
		Title:   "tick all",
		Applies: func(f *cleanupForm, _ cleanupSubject) bool { return !f.busy && len(f.items) > 0 },
		Run:     do[*cleanupForm, cleanupSubject]((*cleanupForm).tickAll),
	}
	actCleanupBranches = &interact.Action[*cleanupForm, cleanupSubject]{
		Title:   "delete branches",
		Applies: func(f *cleanupForm, _ cleanupSubject) bool { return !f.busy && f.branches() },
		Run:     do[*cleanupForm, cleanupSubject](func(f *cleanupForm) { f.deleteBranches = !f.deleteBranches }),
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
	actRunList = &interact.Action[*runForm, runSubject]{
		Title:   "list",
		Applies: func(f *runForm, _ runSubject) bool { return !f.busy && f.onCommand() },
		Run:     do[*runForm, runSubject](func(f *runForm) { f.cmds.openList() }),
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

// The layout form's.
var (
	actLayoutNextField = &interact.Action[*layoutForm, layoutSubject]{
		Title: "next field",
		Run:   cmd[*layoutForm, layoutSubject]((*layoutForm).nextField),
	}
	actLayoutPrevField = &interact.Action[*layoutForm, layoutSubject]{
		Title: "previous field",
		Run:   cmd[*layoutForm, layoutSubject]((*layoutForm).prevField),
	}
	actNextLayout = &interact.Action[*layoutForm, layoutSubject]{
		Title:   "next layout",
		Applies: func(f *layoutForm, _ layoutSubject) bool { return f.onLayout() },
		Run:     func(f *layoutForm, _ layoutSubject) tea.Cmd { return f.cycleLayout(1) },
	}
	actPrevLayout = &interact.Action[*layoutForm, layoutSubject]{
		Title:   "previous layout",
		Applies: func(f *layoutForm, _ layoutSubject) bool { return f.onLayout() },
		Run:     func(f *layoutForm, _ layoutSubject) tea.Cmd { return f.cycleLayout(-1) },
	}
	actLayoutList = &interact.Action[*layoutForm, layoutSubject]{
		Title:   "list",
		Applies: func(f *layoutForm, _ layoutSubject) bool { return !f.busy && f.onLayout() },
		Run:     do[*layoutForm, layoutSubject](func(f *layoutForm) { f.layouts.openList() }),
	}
	actOpenLayoutNow = &interact.Action[*layoutForm, layoutSubject]{
		Title:   "open",
		Applies: func(f *layoutForm, _ layoutSubject) bool { return !f.busy },
		Run:     cmd[*layoutForm, layoutSubject]((*layoutForm).open),
	}
	// actWaitForLayout is enter while the layout is being opened, which does nothing.
	actWaitForLayout = &interact.Action[*layoutForm, layoutSubject]{
		Title: "opening",
		Run:   func(*layoutForm, layoutSubject) tea.Cmd { return nil },
	}
	actLayoutCancel = quit[*layoutForm, layoutSubject]()
)

// The save layout form's. Typing goes to the name and run boxes, so y and n only answer the question
// whether to replace a layout, while it is asked.
var (
	actSaveNextField = &interact.Action[*saveLayoutForm, saveSubject]{
		Title:   "next field",
		Applies: func(f *saveLayoutForm, _ saveSubject) bool { return !f.confirm },
		Run:     cmd[*saveLayoutForm, saveSubject]((*saveLayoutForm).nextField),
	}
	actSavePrevField = &interact.Action[*saveLayoutForm, saveSubject]{
		Title:   "previous field",
		Applies: func(f *saveLayoutForm, _ saveSubject) bool { return !f.confirm },
		Run:     cmd[*saveLayoutForm, saveSubject]((*saveLayoutForm).prevField),
	}
	actSaveNextChoice = &interact.Action[*saveLayoutForm, saveSubject]{
		Title:   "next choice",
		Applies: func(f *saveLayoutForm, _ saveSubject) bool { return !f.confirm && f.onChoice() },
		Run:     func(f *saveLayoutForm, _ saveSubject) tea.Cmd { return f.cycleChoice(1) },
	}
	actSavePrevChoice = &interact.Action[*saveLayoutForm, saveSubject]{
		Title:   "previous choice",
		Applies: func(f *saveLayoutForm, _ saveSubject) bool { return !f.confirm && f.onChoice() },
		Run:     func(f *saveLayoutForm, _ saveSubject) tea.Cmd { return f.cycleChoice(-1) },
	}
	actSaveList = &interact.Action[*saveLayoutForm, saveSubject]{
		Title:   "list",
		Applies: func(f *saveLayoutForm, _ saveSubject) bool { return !f.busy && !f.confirm && f.onChoice() },
		Run:     do[*saveLayoutForm, saveSubject](func(f *saveLayoutForm) { f.focused().choice.openList() }),
	}
	actSaveTick = &interact.Action[*saveLayoutForm, saveSubject]{
		Title:   "tick",
		Applies: func(f *saveLayoutForm, _ saveSubject) bool { return !f.busy && !f.confirm && f.onTick() },
		Run:     do[*saveLayoutForm, saveSubject]((*saveLayoutForm).tick),
	}
	actSaveLayout = &interact.Action[*saveLayoutForm, saveSubject]{
		Title:   "save",
		Applies: func(f *saveLayoutForm, _ saveSubject) bool { return !f.busy && !f.confirm },
		Run:     func(f *saveLayoutForm, _ saveSubject) tea.Cmd { return f.save(false) },
	}
	// actWaitForSave is enter while the layout is being saved, or the question asked, which does nothing.
	actWaitForSave = &interact.Action[*saveLayoutForm, saveSubject]{
		Title: "saving",
		Run:   func(*saveLayoutForm, saveSubject) tea.Cmd { return nil },
	}
	actSaveReplace = &interact.Action[*saveLayoutForm, saveSubject]{
		Title:   "replace",
		Applies: func(f *saveLayoutForm, _ saveSubject) bool { return f.confirm && !f.busy },
		Run:     func(f *saveLayoutForm, _ saveSubject) tea.Cmd { return f.save(true) },
	}
	actSaveKeep = &interact.Action[*saveLayoutForm, saveSubject]{
		Title:   "keep it",
		Applies: func(f *saveLayoutForm, _ saveSubject) bool { return f.confirm && !f.busy },
		Run:     do[*saveLayoutForm, saveSubject](func(f *saveLayoutForm) { f.confirm = false }),
	}
	actSaveCancel = quit[*saveLayoutForm, saveSubject]()
)

// The trust space config form's.
var (
	actTrust = &interact.Action[*trustForm, trustSubject]{
		Title:   "trust",
		Applies: func(f *trustForm, _ trustSubject) bool { return !f.busy && f.bad == nil },
		Run:     cmd[*trustForm, trustSubject]((*trustForm).trust),
	}
	actTrustCancel = &interact.Action[*trustForm, trustSubject]{
		Title:   "cancel",
		Applies: func(f *trustForm, _ trustSubject) bool { return !f.busy },
		Run:     func(*trustForm, trustSubject) tea.Cmd { return tea.Quit },
	}
	actTrustQuit = quit[*trustForm, trustSubject]()
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

// The reply form's. Every other key types.
var (
	actReplySend = &interact.Action[*replyForm, replySubject]{
		Title:   "send",
		Applies: func(f *replyForm, _ replySubject) bool { return !f.busy },
		Run:     func(f *replyForm, _ replySubject) tea.Cmd { return f.reply(true) },
	}
	// actReplyType sends the text with no Enter after it, as a choice menu takes a single key.
	actReplyType = &interact.Action[*replyForm, replySubject]{
		Title:   "send without enter",
		Applies: func(f *replyForm, _ replySubject) bool { return !f.busy && f.in.Value() != "" },
		Run:     func(f *replyForm, _ replySubject) tea.Cmd { return f.reply(false) },
	}
	actReplyCancel = quit[*replyForm, replySubject]()
)

// The show error form's.
var (
	actShowDown     = &interact.Action[*errorForm, showSubject]{Title: "down", Run: do[*errorForm, showSubject](func(f *errorForm) { f.scroll(1) })}
	actShowUp       = &interact.Action[*errorForm, showSubject]{Title: "up", Run: do[*errorForm, showSubject](func(f *errorForm) { f.scroll(-1) })}
	actShowPageDown = &interact.Action[*errorForm, showSubject]{Title: "page down", Run: do[*errorForm, showSubject](func(f *errorForm) { f.scroll(f.shown()) })}
	actShowPageUp   = &interact.Action[*errorForm, showSubject]{Title: "page up", Run: do[*errorForm, showSubject](func(f *errorForm) { f.scroll(-f.shown()) })}
	actShowCopy     = &interact.Action[*errorForm, showSubject]{Title: "copy", Run: cmd[*errorForm, showSubject]((*errorForm).copyText)}
	actShowClose    = quit[*errorForm, showSubject]()
)

var (
	entryKind     *interact.Kind[*sidebar, entrySubject]
	groupKind     *interact.Kind[*sidebar, groupSubject]
	groupShowKind *interact.Kind[*sidebar, groupFilterSubject]
	statusKind    *interact.Kind[*sidebar, statusSubject]
	paneKind      *interact.Kind[*sidebar, paneSubject]
	selectKind    *interact.Kind[*sidebar, selectSubject]
	spacesKind    *interact.Kind[*sidebar, spacesSubject]
	menuKind      *interact.Kind[*sidebar, menuSubject]
	pickerKind    *interact.Kind[*picker, pickerSubject]
	formKind      *interact.Kind[*newWorktreeForm, formSubject]
	removeKind    *interact.Kind[*removeWorktreeForm, removeSubject]
	closeKind     *interact.Kind[*closeSessionForm, closeSubject]
	forgetKind    *interact.Kind[*forgetSessionForm, forgetSubject]
	addKind       *interact.Kind[*addSpaceForm, addSubject]
	cleanKind     *interact.Kind[*cleanupForm, cleanupSubject]
	runKind       *interact.Kind[*runForm, runSubject]
	layoutKind    *interact.Kind[*layoutForm, layoutSubject]
	saveKind      *interact.Kind[*saveLayoutForm, saveSubject]
	trustKind     *interact.Kind[*trustForm, trustSubject]
	replyKind     *interact.Kind[*replyForm, replySubject]
	errorKind     *interact.Kind[*sidebar, errorSubject]
	showKind      *interact.Kind[*errorForm, showSubject]
	filterKind    *interact.Kind[*sidebar, filterSubject]
	listKind      *interact.Kind[*choiceList, listSubject]
	sortKind      *interact.Kind[*sidebar, sortSubject]

	spacesList    *interact.Place[*sidebar, entrySubject]
	groupHeaders  *interact.Place[*sidebar, groupSubject]
	theGroups     *interact.Place[*sidebar, groupFilterSubject]
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
	theLayoutForm *interact.Place[*layoutForm, layoutSubject]
	theSaveForm   *interact.Place[*saveLayoutForm, saveSubject]
	theTrustForm  *interact.Place[*trustForm, trustSubject]
	theReplyForm  *interact.Place[*replyForm, replySubject]
	theErrorLine  *interact.Place[*sidebar, errorSubject]
	theShowForm   *interact.Place[*errorForm, showSubject]
	theFilter     *interact.Place[*sidebar, filterSubject]
	theSort       *interact.Place[*sidebar, sortSubject]
	theChoiceList *interact.Place[*choiceList, listSubject]
	theSelection  *interact.Place[*sidebar, selectSubject]
)

// Kinds and places are made in init, as their actions run code that reads them.
func init() {
	// Every row and the sidebar carry the list's own, so a row's menu offers them, even a row that has
	// nothing else, and their keys work wherever the cursor is.
	spacesKind = interact.NewKind(func(spacesSubject) string { return "Spaces" }, actAddSpace)
	spacesKind.Bind(actAddSpace, "a", "add space", "a")

	entryKind = interact.NewKind(func(s entrySubject) string { return s.row.Space.Name },
		actOpenWorktree, actOpenPlain, actRunCommand, actOpenLayout, actSaveAsLayout, actCloseSession, actOpenPR, actRemoveWorktree, actForgetSession, actNewWorktree, actFold, actUnfold, actCleanup, actTrustConfig, actSelectRow)
	entryKind.Bind(actFold, "space", "fold or unfold the space's worktrees", "space")
	entryKind.Bind(actUnfold, "", "", "space")
	entryKind.Bind(actNewWorktree, "n", "new worktree", "n")
	entryKind.Bind(actRunCommand, "R", "run a command", "R")
	entryKind.Bind(actOpenLayout, "L", "open a layout", "L")
	entryKind.Bind(actOpenPlain, "o", "open without the default layout", "o")
	entryKind.Bind(actCloseSession, "c", "close session", "c")
	entryKind.Bind(actOpenPR, "b", "open the PR in the browser", "b")
	entryKind.Bind(actRemoveWorktree, "x", "remove worktree", "x")
	// x on a missing worktree, which has nothing to remove but its saved file.
	entryKind.Bind(actForgetSession, "", "", "x")
	entryKind.Bind(actCleanup, "p", "clean up worktrees", "p")
	interact.Via(entryKind, func(entrySubject) (spacesSubject, bool) { return spacesSubject{}, true }, spacesKind)
	spacesList = interact.List(entryKind)
	spacesList.Bind(actOpenWorktree, "enter/l", "open", "enter", "l")

	groupKind = interact.NewKind(func(s groupSubject) string { return s.name }, actFoldGroup, actUnfoldGroup, actSelectGroup)
	groupKind.Bind(actFoldGroup, "space", "fold or unfold the group", "space")
	groupKind.Bind(actUnfoldGroup, "", "", "space")
	interact.Via(groupKind, func(groupSubject) (spacesSubject, bool) { return spacesSubject{}, true }, spacesKind)
	groupHeaders = interact.List(groupKind)

	statusKind = interact.NewKind(func(s statusSubject) string { return s.status.Session }, actOpenWindow, actReply)
	statusKind.Bind(actReply, "t", "reply", "t")
	statusList = interact.List(statusKind)
	statusList.Bind(actOpenWindow, "enter/l", "open", "enter", "l")

	paneKind = interact.NewKind(func(paneSubject) string { return "Sidebar" },
		actDown, actUp, actTop, actBottom, actLeave, actRefresh, actMenuHere, actQuit,
		actAttend, actFilter, actAttention, actCycleSort, actMoveUp, actMoveDown, actSelect)
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
	paneKind.Bind(actAttention, "f", "show only what is not idle", "f")
	paneKind.Bind(actCycleSort, "s", "change the sort mode", "s")
	paneKind.Bind(actMoveUp, "K/J", "move the row up/down", "K")
	paneKind.Bind(actMoveDown, "", "", "J")
	paneKind.Bind(actSelect, "v", "select rows to act on", "v")
	interact.Via(paneKind, func(paneSubject) (spacesSubject, bool) { return spacesSubject{}, true }, spacesKind)
	thePane = interact.List(paneKind)

	selectKind = interact.NewKind(func(selectSubject) string { return "Select" }, actTick, actTickAll, actEndSelect)
	selectKind.Bind(actTick, "space", "tick the row", "space")
	selectKind.Bind(actTickAll, "A", "tick every row", "A")
	selectKind.Bind(actEndSelect, "esc", "leave the select mode", "esc")
	theSelection = interact.List(selectKind)

	menuKind = interact.NewKind(func(menuSubject) string { return "Menu" },
		actMenuDown, actMenuUp, actMenuRun, actMenuClose, actMenuQuit)
	menuKind.Bind(actMenuDown, "j/k", "down/up", "j", "down")
	menuKind.Bind(actMenuUp, "", "", "k", "up")
	menuKind.Bind(actMenuRun, "enter/l", "run", "enter", "l")
	menuKind.Bind(actMenuClose, "esc/q/m", "close", "esc", "q", "m")
	menuKind.Bind(actMenuQuit, "ctrl+c", "quit", "ctrl+c")
	theMenu = interact.List(menuKind)

	pickerKind = interact.NewKind(func(pickerSubject) string { return "Picker" }, actPickDown, actPickUp, actPick, actPickPlain, actPickQuit)
	pickerKind.Bind(actPickDown, "down/up", "down/up", "down", "ctrl+n", "ctrl+j")
	pickerKind.Bind(actPickUp, "", "", "up", "ctrl+p", "ctrl+k")
	pickerKind.Bind(actPick, "enter", "switch", "enter")
	// alt+enter for a terminal that sends shift+enter as enter, as a form's newline takes it.
	pickerKind.Bind(actPickPlain, "shift+enter", "switch without layout", "shift+enter", "alt+enter")
	pickerKind.Bind(actPickQuit, "esc", "cancel", "esc", "ctrl+c")
	thePicker = interact.List(pickerKind)

	formKind = interact.NewKind(func(formSubject) string { return "New worktree" },
		actSwitchField, actPrevField, actNextChoice, actPrevChoice, actFormList, actCreate, actCancel)
	formKind.Bind(actSwitchField, "tab", "next field", "tab", "down")
	formKind.Bind(actPrevField, "shift+tab", "previous field", "shift+tab", "up")
	formKind.Bind(actNextChoice, "left/right", "choose", "right")
	formKind.Bind(actPrevChoice, "", "", "left")
	formKind.Bind(actFormList, "space", "list", "space")
	formKind.Bind(actCreate, "enter", "create", "enter")
	formKind.BindQuiet(actWaitForCreate, "enter")
	formKind.Bind(actCancel, "esc", "cancel", "esc", "ctrl+c")
	theForm = interact.List(formKind)

	removeKind = interact.NewKind(func(removeSubject) string { return "Remove worktree" },
		actRemove, actRemoveCancel, actRemoveDeleteBranch, actRemoveQuit)
	removeKind.Bind(actRemove, "y/enter", "remove", "y", "enter")
	removeKind.Bind(actRemoveCancel, "n/esc", "cancel", "n", "esc")
	removeKind.Bind(actRemoveDeleteBranch, "d", "delete branch", "d")
	removeKind.Bind(actRemoveQuit, "ctrl+c", "quit", "ctrl+c")
	theRemoveForm = interact.List(removeKind)

	cleanKind = interact.NewKind(func(cleanupSubject) string { return "Clean up worktrees" },
		actCleanupDown, actCleanupUp, actCleanupToggle, actCleanupTickAll, actCleanupBranches, actCleanupRemove, actCleanupCancel, actCleanupQuit)
	cleanKind.Bind(actCleanupDown, "j/k", "down/up", "j", "down")
	cleanKind.Bind(actCleanupUp, "", "", "k", "up")
	cleanKind.Bind(actCleanupToggle, "space", "tick", "space")
	cleanKind.Bind(actCleanupTickAll, "A", "tick all", "A")
	cleanKind.Bind(actCleanupBranches, "d", "delete branches", "d")
	cleanKind.Bind(actCleanupRemove, "y/enter", "remove", "y", "enter")
	cleanKind.Bind(actCleanupCancel, "n/esc", "cancel", "n", "esc")
	cleanKind.Bind(actCleanupQuit, "ctrl+c", "quit", "ctrl+c")
	theCleanup = interact.List(cleanKind)

	closeKind = interact.NewKind(func(closeSubject) string { return "Close session" }, actClose, actCloseCancel, actCloseQuit)
	closeKind.Bind(actClose, "y/enter", "close", "y", "enter")
	closeKind.Bind(actCloseCancel, "n/esc", "cancel", "n", "esc")
	closeKind.Bind(actCloseQuit, "ctrl+c", "quit", "ctrl+c")
	theCloseForm = interact.List(closeKind)

	forgetKind = interact.NewKind(func(forgetSubject) string { return "Forget session" }, actForget, actForgetCancel, actForgetQuit)
	forgetKind.Bind(actForget, "y/enter", "delete", "y", "enter")
	forgetKind.Bind(actForgetCancel, "n/esc", "cancel", "n", "esc")
	forgetKind.Bind(actForgetQuit, "ctrl+c", "quit", "ctrl+c")
	theForgetForm = interact.List(forgetKind)

	trustKind = interact.NewKind(func(trustSubject) string { return "Trust space config" }, actTrust, actTrustCancel, actTrustQuit)
	trustKind.Bind(actTrust, "y/enter", "trust", "y", "enter")
	trustKind.Bind(actTrustCancel, "n/esc", "cancel", "n", "esc")
	trustKind.Bind(actTrustQuit, "ctrl+c", "quit", "ctrl+c")
	theTrustForm = interact.List(trustKind)

	addKind = interact.NewKind(func(addSubject) string { return "Add space" }, actAdd, actAddCancel)
	addKind.Bind(actAdd, "enter", "add", "enter")
	addKind.Bind(actAddCancel, "esc", "cancel", "esc", "ctrl+c")
	theAddForm = interact.List(addKind)

	replyKind = interact.NewKind(func(replySubject) string { return "Reply" }, actReplySend, actReplyType, actReplyCancel)
	replyKind.Bind(actReplySend, "enter", "send", "enter")
	replyKind.Bind(actReplyType, "ctrl+s", "send without enter", "ctrl+s")
	replyKind.Bind(actReplyCancel, "esc", "cancel", "esc", "ctrl+c")
	theReplyForm = interact.List(replyKind)

	errorKind = interact.NewKind(func(errorSubject) string { return "Error" }, actShowError)
	theErrorLine = interact.List(errorKind)

	showKind = interact.NewKind(func(showSubject) string { return "Error" },
		actShowDown, actShowUp, actShowPageDown, actShowPageUp, actShowCopy, actShowClose)
	showKind.Bind(actShowDown, "up/down", "scroll", "down", "j")
	showKind.Bind(actShowUp, "", "", "up", "k")
	showKind.Bind(actShowPageDown, "pgup/pgdown", "page", "pgdown", "space")
	showKind.Bind(actShowPageUp, "", "", "pgup")
	showKind.Bind(actShowCopy, "y", "copy", "y")
	showKind.Bind(actShowClose, "esc/q/enter", "close", "esc", "q", "enter", "ctrl+c")
	theShowForm = interact.List(showKind)

	runKind = interact.NewKind(func(runSubject) string { return "Run" },
		actRunNextField, actRunPrevField, actRunNextCommand, actRunPrevCommand, actRunList, actRun, actRunCancel)
	runKind.Bind(actRunNextField, "tab", "next field", "tab", "down")
	runKind.Bind(actRunPrevField, "shift+tab", "previous field", "shift+tab", "up")
	runKind.Bind(actRunNextCommand, "left/right", "command", "right")
	runKind.Bind(actRunPrevCommand, "", "", "left")
	runKind.Bind(actRunList, "space", "list", "space")
	runKind.Bind(actRun, "enter", "run", "enter")
	runKind.BindQuiet(actWaitForRun, "enter")
	runKind.Bind(actRunCancel, "esc", "cancel", "esc", "ctrl+c")
	theRunForm = interact.List(runKind)

	layoutKind = interact.NewKind(func(layoutSubject) string { return "Open layout" },
		actLayoutNextField, actLayoutPrevField, actNextLayout, actPrevLayout, actLayoutList, actOpenLayoutNow, actLayoutCancel)
	layoutKind.Bind(actLayoutNextField, "tab", "next field", "tab", "down")
	layoutKind.Bind(actLayoutPrevField, "shift+tab", "previous field", "shift+tab", "up")
	layoutKind.Bind(actNextLayout, "left/right", "layout", "right")
	layoutKind.Bind(actPrevLayout, "", "", "left")
	layoutKind.Bind(actLayoutList, "space", "list", "space")
	layoutKind.Bind(actOpenLayoutNow, "enter", "open", "enter")
	layoutKind.BindQuiet(actWaitForLayout, "enter")
	layoutKind.Bind(actLayoutCancel, "esc", "cancel", "esc", "ctrl+c")
	theLayoutForm = interact.List(layoutKind)

	saveKind = interact.NewKind(func(saveSubject) string { return "Save layout" },
		actSaveNextField, actSavePrevField, actSaveNextChoice, actSavePrevChoice, actSaveList, actSaveTick, actSaveLayout, actSaveReplace, actSaveKeep, actSaveCancel)
	saveKind.Bind(actSaveNextField, "tab", "next field", "tab", "down")
	saveKind.Bind(actSavePrevField, "shift+tab", "previous field", "shift+tab", "up")
	saveKind.Bind(actSaveNextChoice, "left/right", "choose", "right")
	saveKind.Bind(actSavePrevChoice, "", "", "left")
	saveKind.Bind(actSaveList, "space", "list", "space")
	saveKind.Bind(actSaveTick, "space", "tick", "space")
	saveKind.Bind(actSaveLayout, "enter", "save", "enter")
	saveKind.Bind(actSaveReplace, "y", "replace", "y")
	saveKind.Bind(actSaveKeep, "n/esc", "keep it", "n", "esc")
	saveKind.BindQuiet(actWaitForSave, "enter")
	saveKind.Bind(actSaveCancel, "esc", "cancel", "esc", "ctrl+c")
	theSaveForm = interact.List(saveKind)

	// j and k type, as worktrees are named with them, so only arrows and the picker's ctrl keys move.
	filterKind = interact.NewKind(func(filterSubject) string { return "Filter" },
		actFilterDown, actFilterUp, actFilterOpen, actFilterClear, actFilterQuit)
	filterKind.Bind(actFilterDown, "down/up", "down/up", "down", "ctrl+n", "ctrl+j")
	filterKind.Bind(actFilterUp, "", "", "up", "ctrl+p", "ctrl+k")
	filterKind.Bind(actFilterOpen, "enter", "open", "enter")
	filterKind.Bind(actFilterClear, "esc", "clear", "esc")
	filterKind.Bind(actFilterQuit, "ctrl+c", "quit", "ctrl+c")
	theFilter = interact.List(filterKind)

	sortKind = interact.NewKind(func(sortSubject) string { return "Sort" },
		sortActions[spaces.SortManual], sortActions[spaces.SortActivity], sortActions[spaces.SortStatus],
		actAttentionItem, actSelectList)
	theSort = interact.List(sortKind)
	groupShowKind = interact.NewKind(func(groupFilterSubject) string { return "Groups" }, actShowGroup)
	theGroups = interact.List(groupShowKind)

	listKind = interact.NewKind(func(listSubject) string { return "Choices" },
		actListDown, actListUp, actListPick, actListErase, actListClose, actListQuit)
	listKind.Bind(actListDown, "up/down", "move", "down", "ctrl+n", "ctrl+j")
	listKind.Bind(actListUp, "", "", "up", "ctrl+p", "ctrl+k")
	listKind.BindQuiet(actListJ, "j")
	listKind.BindQuiet(actListK, "k")
	listKind.Bind(actListPick, "enter", "choose", "enter")
	listKind.Bind(actListErase, "backspace", "erase", "backspace")
	listKind.Bind(actListClose, "esc", "close", "esc")
	listKind.Bind(actListQuit, "ctrl+c", "quit", "ctrl+c")
	theChoiceList = interact.List(listKind)
}

// Targets: the subjects on screen, each in its place.

// errorTarget is the error on the last line, if it shows one.
func (m *sidebar) errorTarget() (interact.Target[*sidebar], bool) {
	err := m.lastError()
	if err == nil {
		return interact.Target[*sidebar]{}, false
	}
	return theErrorLine.Target(errorSubject{text: err.Error()}), true
}

// entryTarget is entry i of the sidebar, a row of the spaces list or a status entry, if there is one
// there: a blank row between groups is none.
func (m *sidebar) entryTarget(i int) (interact.Target[*sidebar], bool) {
	switch {
	case i >= 0 && i < len(m.rows) && m.rows[i].Group != nil:
		return groupHeaders.Target(groupSubject{name: m.rows[i].Group.Name}), true
	case i >= 0 && i < len(m.rows) && m.rows[i].Space != nil:
		return spacesList.Target(entrySubject{index: i, row: m.rows[i]}), true
	case i >= len(m.rows) && i < m.numEntries():
		j := i - len(m.rows)
		return statusList.Target(statusSubject{index: j, status: m.kitty.Statuses[j]}), true
	}
	return interact.Target[*sidebar]{}, false
}

// chain is what a key reaches: the select mode while it is on, the cursor's entry, then the sidebar.
func (m *sidebar) chain() []interact.Target[*sidebar] {
	var out []interact.Target[*sidebar]
	if m.selecting {
		out = append(out, theSelection.Target(selectSubject{}))
	}
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
