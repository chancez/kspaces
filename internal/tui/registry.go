package tui

import (
	tea "charm.land/bubbletea/v2"

	"github.com/chancez/kspaces/internal/config"
	"github.com/chancez/kspaces/internal/kitty"
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

// detachedSubject is a row of a space's provider sessions that no window shows, by its index in the rows: their head,
// a host's or a session's.
type detachedSubject struct {
	index int
	row   spaces.DetachedRow
}

// hostSubject is a host's row on the hosts tab, by the host, empty for this machine.
type hostSubject struct{ host string }

// groupFilterSubject is a group in the spaces list's menu, which shows or hides it.
type groupFilterSubject struct{ name string }

// statusSubject is an entry of the sidebar's status list, by its index in the statuses.
type statusSubject struct {
	index  int
	status status.Status
}

// paneSubject is the sidebar itself, for the moves through it and what acts on no entry.
type paneSubject struct{}

// spacesSubject is the spaces list as a whole, for what adds to it rather than acting on one row: from a
// group's header or one of its spaces' rows, in that group.
type spacesSubject struct{ group string }

// menuSubject is the sidebar's open context menu.
type menuSubject struct{}

// pickerSubject is the picker, formSubject the new worktree form, removeSubject the remove worktree form,
// closeSubject the close session form and addSubject the add space form.
type pickerSubject struct{}
type formSubject struct{}
type removeSubject struct{}
type closeSubject struct{}
type addSubject struct{}

// endSessionsSubject is the form that ends the provider's sessions.
type endSessionsSubject struct{}

// moveSubject is the form that moves the provider's sessions to another space.
type moveSubject struct{}

// removeSpaceSubject is the remove space form.
type removeSpaceSubject struct{}

// forgetSubject is the forget session form.
type forgetSubject struct{}

// resumeSubject is the resume session form.
type resumeSubject struct{}

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

// pickRowSubject is a session or worktree in the picker, as its row in the sidebar's spaces list.
type pickRowSubject struct{ row spaces.Row }

// pickStatusSubject is a status entry in the picker.
type pickStatusSubject struct{ status status.Status }

// pickMenuSubject is the picker's open menu.
type pickMenuSubject struct{}

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

// helpSubject is the list of keys ? opens, see keyhelp.go.
type helpSubject struct{}

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
		// One only another host has while one of its sessions there can be opened, or its kitty session has
		// windows, as it has no directory to start one in.
		Applies: func(m *sidebar, s entrySubject) bool {
			switch {
			case s.row.Worktree == nil:
				return false
			case s.row.Remote():
				return true
			case s.row.Elsewhere():
				return m.isOpen(s.row.Worktree) || m.caps[spaces.CapOpen] && len(m.hostSessionsOf(s.row.Worktree.Host, *s.row.Space, s.row.Worktree.Session)) > 0
			}
			return !s.row.Missing || m.isOpen(s.row.Worktree)
		},
		Run: func(m *sidebar, s entrySubject) tea.Cmd {
			m.remote = false
			if wt := *s.row.Worktree; s.row.Elsewhere() && !m.isOpen(&wt) {
				return m.openDetachedCmd(spaces.DetachedRow{Of: s.row.Space, Session: &m.hostSessionsOf(wt.Host, *s.row.Space, wt.Session)[0]})
			}
			return m.openOrResumeCmd(*s.row.Worktree, spaces.OpenOptions{})
		},
	}
	actOpenPlain = &interact.Action[*sidebar, entrySubject]{
		Title:   "Open without layout",
		Section: sectionSession,
		// Only where the default layout would open: elsewhere it opens just as Open does.
		Applies: func(m *sidebar, s entrySubject) bool {
			return !s.row.Other && !s.row.Absent() && !s.row.Remote() && s.row.Worktree != nil &&
				spaces.OpensDefaultLayout(m.cfg, m.kitty, *s.row.Space, *s.row.Worktree)
		},
		Run: func(m *sidebar, s entrySubject) tea.Cmd {
			m.remote = false
			return m.openOrResumeCmd(*s.row.Worktree, spaces.OpenOptions{NoLayout: true})
		},
	}
	actFold = &interact.Action[*sidebar, entrySubject]{
		Title:   "Fold worktrees",
		Section: sectionSpace,
		Applies: func(m *sidebar, s entrySubject) bool {
			return foldable(m, s) && !m.folded[spaces.FoldKey(s.row.Host, s.row.Space.Root)]
		},
		Run: func(m *sidebar, s entrySubject) tea.Cmd { m.toggleFold(s.row.Host, s.row.Space.Root); return nil },
	}
	actUnfold = &interact.Action[*sidebar, entrySubject]{
		Title:   "Unfold worktrees",
		Section: sectionSpace,
		Applies: func(m *sidebar, s entrySubject) bool {
			return foldable(m, s) && m.folded[spaces.FoldKey(s.row.Host, s.row.Space.Root)]
		},
		Run: func(m *sidebar, s entrySubject) tea.Cmd { m.toggleFold(s.row.Host, s.row.Space.Root); return nil },
	}
	actNewWorktree          = onEntry(rowNewWorktree)
	actNewWorktreeFromPR    = onEntry(rowNewWorktreeFromPR)
	actNewWorktreeFromIssue = onEntry(rowNewWorktreeFromIssue)
	actRemoveWorktree       = onEntry(rowRemoveWorktree)
	actOpenPR               = onEntry(rowOpenPR)
	actRunCommand           = onEntry(rowRunCommand)
	actOpenLayout           = onEntry(rowOpenLayout)
	actSaveAsLayout         = onEntry(rowSaveAsLayout)
	actCloseSession         = onEntry(rowCloseSession)
	actSuspendSession       = onEntry(rowSuspendSession)
	actForgetSession        = onEntry(rowForgetSession)
	actCleanup              = onEntry(rowCleanup)
	actTrustConfig          = onEntry(rowTrustConfig)
	actRemoveSpaceRow       = onEntry(rowRemoveSpace)
	// actSelectRow turns the select mode on with the row ticked.
	actSelectRow = &interact.Action[*sidebar, entrySubject]{
		Title:   "Select",
		Section: sectionSpace,
		Applies: func(m *sidebar, s entrySubject) bool { return !m.selecting && s.row.Worktree != nil && !s.row.Remote() },
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

// A host's row on the hosts tab'.
var (
	actFoldHost = &interact.Action[*sidebar, hostSubject]{
		Title:   "Fold host",
		Section: "Host",
		Applies: func(m *sidebar, s hostSubject) bool { return !m.filtering && !m.hostsFolded[s.host] },
		Run:     func(m *sidebar, s hostSubject) tea.Cmd { m.toggleHost(s.host); return nil },
	}
	actUnfoldHost = &interact.Action[*sidebar, hostSubject]{
		Title:   "Unfold host",
		Section: "Host",
		Applies: func(m *sidebar, s hostSubject) bool { return !m.filtering && m.hostsFolded[s.host] },
		Run:     func(m *sidebar, s hostSubject) tea.Cmd { m.toggleHost(s.host); return nil },
	}
	// actOpenHostShell opens a host the config names' default copy: a shell there, see docs/stories.md.
	actOpenHostShell = &interact.Action[*sidebar, hostSubject]{
		Title:   "Open shell",
		Section: "Host",
		Applies: func(m *sidebar, s hostSubject) bool { _, ok := m.remotes[s.host]; return ok },
		Run: func(m *sidebar, s hostSubject) tea.Cmd {
			m.remote = false
			wt := spaces.Worktree{Session: spaces.CopyName(s.host, "default"), Host: s.host}
			return m.openWorktreeCmd(wt, spaces.OpenOptions{})
		},
	}
)

// A space's detached sessions' rows'. A click on the head does what space does, as there is nothing to open.
var (
	actOpenDetached = &interact.Action[*sidebar, detachedSubject]{
		Title:   "Open",
		Section: sectionSession,
		Applies: func(m *sidebar, s detachedSubject) bool { return s.row.Session != nil && m.caps[spaces.CapOpen] },
		Run: func(m *sidebar, s detachedSubject) tea.Cmd {
			m.remote = false
			return m.openDetachedCmd(s.row)
		},
	}
	actKillDetached = &interact.Action[*sidebar, detachedSubject]{
		Title:   "Kill",
		Section: sectionSession,
		Applies: func(m *sidebar, s detachedSubject) bool {
			return s.row.Session != nil && !s.row.Session.Ended() && m.caps[spaces.CapKill]
		},
		Run: func(m *sidebar, s detachedSubject) tea.Cmd {
			return m.askDetached(func(cfg config.Config, st kitty.State) error {
				return spaces.AskKillProviderSessions(cfg, st, s.row.Session.ID)
			})
		},
	}
	actShelveDetached = &interact.Action[*sidebar, detachedSubject]{
		Title:   "Shelve",
		Section: sectionSession,
		Applies: func(m *sidebar, s detachedSubject) bool {
			return s.row.Session != nil && !s.row.Session.Ended() && m.caps[spaces.CapShelve]
		},
		Run: func(m *sidebar, s detachedSubject) tea.Cmd { return m.shelveDetachedCmd(*s.row.Session) },
	}
	actRemoveDetached = &interact.Action[*sidebar, detachedSubject]{
		Title:   "Remove",
		Section: sectionSession,
		Applies: func(m *sidebar, s detachedSubject) bool {
			return s.row.Session != nil && s.row.Session.Ended() && m.caps[spaces.CapRemove]
		},
		Run: func(m *sidebar, s detachedSubject) tea.Cmd {
			return m.askDetached(func(cfg config.Config, st kitty.State) error {
				return spaces.AskRemoveProviderSessions(cfg, st, s.row.Session.ID)
			})
		},
	}
	actMoveDetached = &interact.Action[*sidebar, detachedSubject]{
		Title:   "Move to space",
		Section: sectionSession,
		Applies: func(m *sidebar, s detachedSubject) bool { return s.row.Session != nil && m.caps[spaces.CapMove] },
		Run: func(m *sidebar, s detachedSubject) tea.Cmd {
			return m.askDetached(func(cfg config.Config, st kitty.State) error {
				return spaces.AskMoveProviderSessions(cfg, st, s.row.Session.ID)
			})
		},
	}
	actCleanupDetached = &interact.Action[*sidebar, detachedSubject]{
		Title:   "Clean up sessions",
		Section: sectionSpace,
		Applies: func(m *sidebar, s detachedSubject) bool {
			return s.row.Head() && (m.caps[spaces.CapKill] || m.caps[spaces.CapRemove])
		},
		Run: func(m *sidebar, s detachedSubject) tea.Cmd {
			return m.askDetached(func(cfg config.Config, st kitty.State) error {
				return spaces.AskCleanupProviderSessions(cfg, st, s.row.Of.Name)
			})
		},
	}
	actShowDetached = &interact.Action[*sidebar, detachedSubject]{
		Title:   "Show sessions",
		Section: sectionSpace,
		Applies: func(_ *sidebar, s detachedSubject) bool { return s.row.Head() && !s.row.Expanded },
		Run:     func(m *sidebar, s detachedSubject) tea.Cmd { m.toggleDetachedAt(s.index); return nil },
	}
	actHideDetached = &interact.Action[*sidebar, detachedSubject]{
		Title:   "Hide sessions",
		Section: sectionSpace,
		Applies: func(_ *sidebar, s detachedSubject) bool { return s.row.Head() && s.row.Expanded },
		Run:     func(m *sidebar, s detachedSubject) tea.Cmd { m.toggleDetachedAt(s.index); return nil },
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
	// actClearStatus removes the done and failed records of the entry's window, leaving the focus where it
	// is. The others end by themselves, at the next prompt.
	actClearStatus = &interact.Action[*sidebar, statusSubject]{
		Title:   "Clear",
		Section: sectionSession,
		Applies: func(_ *sidebar, s statusSubject) bool { return hasFinished(s.status) },
		Run: func(m *sidebar, s statusSubject) tea.Cmd {
			return m.clearCmd([]int{s.status.Window}, finishedStates...)
		},
	}
)

// actClearFinished removes the done and failed records of the status list's windows, leaving the focus
// where it is, wherever the cursor is.
var actClearFinished = &interact.Action[*sidebar, paneSubject]{
	Title:   "Clear done and failed",
	Applies: func(m *sidebar, _ paneSubject) bool { return len(finished(m.kitty.Statuses)) > 0 },
	Run: func(m *sidebar, _ paneSubject) tea.Cmd {
		return m.clearCmd(finished(m.kitty.Statuses), finishedStates...)
	},
}

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
	Run: func(m *sidebar, s spacesSubject) tea.Cmd {
		m.remote = false
		return m.addSpace(s.group)
	},
}

// paneAction is an action on the sidebar.
func paneAction(title string, run func(m *sidebar) tea.Cmd) *interact.Action[*sidebar, paneSubject] {
	return &interact.Action[*sidebar, paneSubject]{Title: title, Run: cmd[*sidebar, paneSubject](run)}
}

// The sidebar's own: moves, leaving, refreshing, and the cursor's menu.
var (
	actDown = paneAction("down", func(m *sidebar) tea.Cmd { m.moveCursor(1); return nil })
	actUp   = paneAction("up", func(m *sidebar) tea.Cmd { m.moveCursor(-1); return nil })
	// The page moves go by the list the cursor is in, as less and vim's do.
	actHalfDown = paneAction("half page down", func(m *sidebar) tea.Cmd { m.moveCursor(max(m.pageRows()/2, 1)); return nil })
	actHalfUp   = paneAction("half page up", func(m *sidebar) tea.Cmd { m.moveCursor(-max(m.pageRows()/2, 1)); return nil })
	actPageDown = paneAction("page down", func(m *sidebar) tea.Cmd { m.moveCursor(m.pageRows()); return nil })
	actPageUp   = paneAction("page up", func(m *sidebar) tea.Cmd { m.moveCursor(-m.pageRows()); return nil })
	actTop      = paneAction("top", func(m *sidebar) tea.Cmd { m.cursor = 0; m.scrollToCursor(); return nil })
	actBottom   = paneAction("bottom", func(m *sidebar) tea.Cmd {
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

// actOpenOnly toggles the open filter, see toggleOpen.
var actOpenOnly = paneAction("open filter", (*sidebar).toggleOpen)

// actAwakeOnly toggles the suspended filter, see toggleAwake.
var actAwakeOnly = paneAction("suspended filter", (*sidebar).toggleAwake)

// actOpenPicker opens the picker in a popup, as kitty.conf's kspaces_pick mapping does.
var actOpenPicker = paneAction("picker", (*sidebar).openPicker)

// actSwitchTab shows the other tab, see hosts.go, as a click on its title does.
var actSwitchTab = &interact.Action[*sidebar, paneSubject]{
	Title:   "other tab",
	Applies: func(m *sidebar, _ paneSubject) bool { return m.hasHosts() },
	Run:     func(m *sidebar, _ paneSubject) tea.Cmd { m.switchTab(); return nil },
}

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

// The select mode's: ticking rows, acting on the ticked ones, and leaving. Each applies throughout the
// mode, so a key that does nothing here, as x with nothing to remove ticked, says why rather than reaching
// the cursor's row, where it would remove its one worktree.
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
	// The bar's, see selectbar.go, each opening its form with the ticked rows it applies to.
	actSelRemove = &interact.Action[*sidebar, selectSubject]{
		Title: "Remove",
		Run:   cmd[*sidebar, selectSubject]((*sidebar).removeSelected),
	}
	actSelClose = &interact.Action[*sidebar, selectSubject]{
		Title: "Close",
		Run:   cmd[*sidebar, selectSubject]((*sidebar).closeSelected),
	}
	actSelSuspend = &interact.Action[*sidebar, selectSubject]{
		Title:   "Suspend",
		Applies: func(m *sidebar, _ selectSubject) bool { return spaces.CanKeep(m.cfg) },
		Run:     cmd[*sidebar, selectSubject]((*sidebar).suspendSelected),
	}
	actSelForget = &interact.Action[*sidebar, selectSubject]{
		Title: "Forget",
		Run:   cmd[*sidebar, selectSubject]((*sidebar).forgetSelected),
	}
	actSelRun = &interact.Action[*sidebar, selectSubject]{
		Title: "Run",
		Run:   cmd[*sidebar, selectSubject]((*sidebar).runSelected),
	}
	actSelMove = &interact.Action[*sidebar, selectSubject]{
		Title:   "Move",
		Applies: func(m *sidebar, _ selectSubject) bool { return m.caps[spaces.CapMove] },
		Run:     cmd[*sidebar, selectSubject]((*sidebar).moveSelected),
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

// actOpenItem is the open filter in the spaces list's menu, beside the attention filter.
var actOpenItem = &interact.Action[*sidebar, sortSubject]{
	Title:   "Only open sessions",
	Section: "Filter",
	Run:     func(m *sidebar, _ sortSubject) tea.Cmd { return m.toggleOpen() },
}

// actAwakeItem is the suspended filter in the spaces list's menu, beside the open filter.
var actAwakeItem = &interact.Action[*sidebar, sortSubject]{
	Title:   "Hide suspended sessions",
	Section: "Filter",
	Run:     func(m *sidebar, _ sortSubject) tea.Cmd { return m.toggleAwake() },
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
	actFilterHelp = &interact.Action[*sidebar, filterSubject]{
		Title: "keys",
		Run:   do[*sidebar, filterSubject](func(m *sidebar) { m.openHelp(theFilter.Target(filterSubject{})) }),
	}
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
	// actMenuHelp lists the menu's keys and its items'.
	actMenuHelp = &interact.Action[*sidebar, menuSubject]{
		Title: "keys",
		Run: do[*sidebar, menuSubject](func(m *sidebar) {
			m.openHelp(append([]interact.Target[*sidebar]{m.menu.own}, m.menu.of...)...)
		}),
	}
)

// actHelp lists the keys where the cursor is.
var actHelp = paneAction("keys", func(m *sidebar) tea.Cmd { m.openHelp(m.chain()...); return nil })

// The list of keys': scrolling it and closing it.
var (
	actHelpDown = &interact.Action[*keyHelp, helpSubject]{
		Title: "down",
		Run:   do[*keyHelp, helpSubject](func(h *keyHelp) { h.scroll(1) }),
	}
	actHelpUp = &interact.Action[*keyHelp, helpSubject]{
		Title: "up",
		Run:   do[*keyHelp, helpSubject](func(h *keyHelp) { h.scroll(-1) }),
	}
	actHelpTop = &interact.Action[*keyHelp, helpSubject]{
		Title: "top",
		Run:   do[*keyHelp, helpSubject](func(h *keyHelp) { h.offset = 0 }),
	}
	actHelpBottom = &interact.Action[*keyHelp, helpSubject]{
		Title: "bottom",
		Run:   do[*keyHelp, helpSubject](func(h *keyHelp) { h.offset = h.last }),
	}
	actHelpClose = &interact.Action[*keyHelp, helpSubject]{
		Title: "close",
		Run:   do[*keyHelp, helpSubject](func(h *keyHelp) { h.closed = true }),
	}
	actHelpQuit = quit[*keyHelp, helpSubject]()
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
	// actPickScope lists the next scope.
	actPickScope = &interact.Action[*picker, pickerSubject]{
		Title: "next scope",
		Run:   do[*picker, pickerSubject]((*picker).nextScope),
	}
	// actPickMenu opens the menu of the entry under the cursor.
	actPickMenu = &interact.Action[*picker, pickerSubject]{
		Title: "actions",
		Run:   do[*picker, pickerSubject]((*picker).openPickMenu),
	}
)

// The picker's menu's, on its entries: opening them, as enter and shift+enter do, and the forms they open,
// as their rows in the sidebar do.
var (
	actPickOpen = &interact.Action[*picker, pickRowSubject]{
		Title:   "Open",
		Section: sectionSession,
		// A missing worktree only while its session has windows, as the sidebar's Open.
		Applies: func(m *picker, s pickRowSubject) bool {
			return !s.row.Missing || m.snap.Kitty.Windows[s.row.Worktree.Session] > 0
		},
		Run: func(m *picker, s pickRowSubject) tea.Cmd {
			return m.chooseItem(pickItem{row: s.row}, spaces.OpenOptions{})
		},
	}
	actPickOpenPlain = &interact.Action[*picker, pickRowSubject]{
		Title:   "Open without layout",
		Section: sectionSession,
		Applies: func(m *picker, s pickRowSubject) bool {
			return !s.row.Other && !s.row.Missing && spaces.OpensDefaultLayout(*m.snap.Config, m.snap.Kitty, *s.row.Space, *s.row.Worktree)
		},
		Run: func(m *picker, s pickRowSubject) tea.Cmd {
			return m.chooseItem(pickItem{row: s.row}, spaces.OpenOptions{NoLayout: true})
		},
	}
	actPickOpenStatus = &interact.Action[*picker, pickStatusSubject]{
		Title:   "Open",
		Section: sectionSession,
		Run: func(m *picker, s pickStatusSubject) tea.Cmd {
			return m.chooseItem(pickItem{status: &s.status}, spaces.OpenOptions{})
		},
	}
	actPickReply = &interact.Action[*picker, pickStatusSubject]{
		Title:   "Reply",
		Section: sectionSession,
		Run: func(m *picker, s pickStatusSubject) tea.Cmd {
			return m.ask(func(snap spaces.Snapshot) error { return spaces.AskReply(*snap.Config, snap.Kitty, s.status.Window) })
		},
	}
	actPickMenuDown = &interact.Action[*picker, pickMenuSubject]{
		Title: "down",
		Run:   do[*picker, pickMenuSubject](func(m *picker) { m.menu.sel = min(m.menu.sel+1, len(m.menu.items)-1) }),
	}
	actPickMenuUp = &interact.Action[*picker, pickMenuSubject]{
		Title: "up",
		Run:   do[*picker, pickMenuSubject](func(m *picker) { m.menu.sel = max(m.menu.sel-1, 0) }),
	}
	actPickMenuRun = &interact.Action[*picker, pickMenuSubject]{
		Title: "run",
		Run:   cmd[*picker, pickMenuSubject](func(m *picker) tea.Cmd { return m.runPickMenuItem(m.menu.sel) }),
	}
	actPickMenuClose = &interact.Action[*picker, pickMenuSubject]{
		Title: "close",
		Run:   do[*picker, pickMenuSubject](func(m *picker) { m.menu = nil }),
	}
	actPickMenuQuit = quit[*picker, pickMenuSubject]()
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
	// On a choice, the tab strip, layout or command, only, so left and right move the cursor in a text
	// field.
	actNextChoice = &interact.Action[*newWorktreeForm, formSubject]{
		Title:   "next choice",
		Applies: func(f *newWorktreeForm, _ formSubject) bool { return f.onChoice() },
		Run:     func(f *newWorktreeForm, _ formSubject) tea.Cmd { return f.cycleChoice(1) },
	}
	actPrevChoice = &interact.Action[*newWorktreeForm, formSubject]{
		Title:   "previous choice",
		Applies: func(f *newWorktreeForm, _ formSubject) bool { return f.onChoice() },
		Run:     func(f *newWorktreeForm, _ formSubject) tea.Cmd { return f.cycleChoice(-1) },
	}
	actFormList = &interact.Action[*newWorktreeForm, formSubject]{
		Title: "list",
		Applies: func(f *newWorktreeForm, _ formSubject) bool {
			return !f.busy && (f.onSpace() || f.onBase() || f.onOpen())
		},
		Run: do[*newWorktreeForm, formSubject]((*newWorktreeForm).openList),
	}
	actFormBackground = &interact.Action[*newWorktreeForm, formSubject]{
		Title:   "open in background",
		Applies: func(f *newWorktreeForm, _ formSubject) bool { return !f.busy && f.onBackground() },
		Run:     do[*newWorktreeForm, formSubject]((*newWorktreeForm).toggleBackground),
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
	actCancel    = quit[*newWorktreeForm, formSubject]()
	actFormClear = clearDraft[*newWorktreeForm, formSubject]()
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
	actCloseNextFate = &interact.Action[*closeSessionForm, closeSubject]{
		Title:   "next for sessions",
		Applies: func(f *closeSessionForm, _ closeSubject) bool { return !f.busy && f.fate.offered() },
		Run:     do[*closeSessionForm, closeSubject](func(f *closeSessionForm) { f.fate.cycle(1) }),
	}
	actClosePrevFate = &interact.Action[*closeSessionForm, closeSubject]{
		Title:   "previous for sessions",
		Applies: func(f *closeSessionForm, _ closeSubject) bool { return !f.busy && f.fate.offered() },
		Run:     do[*closeSessionForm, closeSubject](func(f *closeSessionForm) { f.fate.cycle(-1) }),
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

// The resume session form's.
var (
	actResume = &interact.Action[*resumeSessionForm, resumeSubject]{
		Title:   "resume",
		Applies: func(f *resumeSessionForm, _ resumeSubject) bool { return !f.busy },
		Run:     cmd[*resumeSessionForm, resumeSubject]((*resumeSessionForm).resume),
	}
	actResumeCancel = &interact.Action[*resumeSessionForm, resumeSubject]{
		Title:   "cancel",
		Applies: func(f *resumeSessionForm, _ resumeSubject) bool { return !f.busy },
		Run:     func(*resumeSessionForm, resumeSubject) tea.Cmd { return tea.Quit },
	}
	actResumeQuit = quit[*resumeSessionForm, resumeSubject]()
)

// The form that ends the provider's sessions'.
var (
	actEndSessionsDown = &interact.Action[*endSessionsForm, endSessionsSubject]{
		Title:   "down",
		Applies: func(f *endSessionsForm, _ endSessionsSubject) bool { return f.cleaning() },
		Run:     do[*endSessionsForm, endSessionsSubject](func(f *endSessionsForm) { f.cursor = min(f.cursor+1, max(len(f.sessions)-1, 0)) }),
	}
	actEndSessionsUp = &interact.Action[*endSessionsForm, endSessionsSubject]{
		Title:   "up",
		Applies: func(f *endSessionsForm, _ endSessionsSubject) bool { return f.cleaning() },
		Run:     do[*endSessionsForm, endSessionsSubject](func(f *endSessionsForm) { f.cursor = max(f.cursor-1, 0) }),
	}
	actEndSessionsToggle = &interact.Action[*endSessionsForm, endSessionsSubject]{
		Title: "tick",
		Applies: func(f *endSessionsForm, _ endSessionsSubject) bool {
			return !f.busy && f.cleaning() && len(f.sessions) > 0
		},
		Run: do[*endSessionsForm, endSessionsSubject](func(f *endSessionsForm) { f.toggle(f.cursor) }),
	}
	actEndSessionsTickAll = &interact.Action[*endSessionsForm, endSessionsSubject]{
		Title: "tick all",
		Applies: func(f *endSessionsForm, _ endSessionsSubject) bool {
			return !f.busy && f.cleaning() && len(f.sessions) > 0
		},
		Run: do[*endSessionsForm, endSessionsSubject]((*endSessionsForm).tickAll),
	}
	actEndSessions = &interact.Action[*endSessionsForm, endSessionsSubject]{
		Title:   "end",
		Applies: func(f *endSessionsForm, _ endSessionsSubject) bool { return !f.busy && len(f.chosen()) > 0 },
		Run:     cmd[*endSessionsForm, endSessionsSubject]((*endSessionsForm).end),
	}
	actEndSessionsCancel = &interact.Action[*endSessionsForm, endSessionsSubject]{
		Title:   "cancel",
		Applies: func(f *endSessionsForm, _ endSessionsSubject) bool { return !f.busy },
		Run:     func(*endSessionsForm, endSessionsSubject) tea.Cmd { return tea.Quit },
	}
	actEndSessionsQuit = quit[*endSessionsForm, endSessionsSubject]()
)

// The move form's. Every other key types.
var (
	actMoveNext = &interact.Action[*moveSessionsForm, moveSubject]{
		Title: "next space",
		Run:   do[*moveSessionsForm, moveSubject](func(f *moveSessionsForm) { f.choose(1) }),
	}
	actMovePrev = &interact.Action[*moveSessionsForm, moveSubject]{
		Title: "previous space",
		Run:   do[*moveSessionsForm, moveSubject](func(f *moveSessionsForm) { f.choose(-1) }),
	}
	actMove = &interact.Action[*moveSessionsForm, moveSubject]{
		Title:   "move",
		Applies: func(f *moveSessionsForm, _ moveSubject) bool { return !f.busy && f.target() != "" },
		Run:     cmd[*moveSessionsForm, moveSubject]((*moveSessionsForm).move),
	}
	actMoveCancel = quit[*moveSessionsForm, moveSubject]()
)

// The remove space form's.
var (
	actRemoveSpace = &interact.Action[*removeSpaceForm, removeSpaceSubject]{
		Title:   "remove",
		Applies: func(f *removeSpaceForm, _ removeSpaceSubject) bool { return !f.busy && f.refused() == nil },
		Run:     cmd[*removeSpaceForm, removeSpaceSubject]((*removeSpaceForm).remove),
	}
	actRemoveSpaceCancel = &interact.Action[*removeSpaceForm, removeSpaceSubject]{
		Title:   "cancel",
		Applies: func(f *removeSpaceForm, _ removeSpaceSubject) bool { return !f.busy },
		Run:     func(*removeSpaceForm, removeSpaceSubject) tea.Cmd { return tea.Quit },
	}
	actRemoveSpaceNextFate = &interact.Action[*removeSpaceForm, removeSpaceSubject]{
		Title:   "next for sessions",
		Applies: func(f *removeSpaceForm, _ removeSpaceSubject) bool { return !f.busy && f.fate.offered() },
		Run:     do[*removeSpaceForm, removeSpaceSubject](func(f *removeSpaceForm) { f.fate.cycle(1) }),
	}
	actRemoveSpacePrevFate = &interact.Action[*removeSpaceForm, removeSpaceSubject]{
		Title:   "previous for sessions",
		Applies: func(f *removeSpaceForm, _ removeSpaceSubject) bool { return !f.busy && f.fate.offered() },
		Run:     do[*removeSpaceForm, removeSpaceSubject](func(f *removeSpaceForm) { f.fate.cycle(-1) }),
	}
	actRemoveSpaceQuit = quit[*removeSpaceForm, removeSpaceSubject]()
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
	actRunClear  = clearDraft[*runForm, runSubject]()
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
	actLayoutClear  = clearDraft[*layoutForm, layoutSubject]()
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
	actSaveClear  = clearDraft[*saveLayoutForm, saveSubject]()
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
	actAddCancel    = quit[*addSpaceForm, addSubject]()
	actAddClear     = clearDraft[*addSpaceForm, addSubject]()
	actAddNextField = &interact.Action[*addSpaceForm, addSubject]{
		Title: "next field",
		Run:   cmd[*addSpaceForm, addSubject]((*addSpaceForm).nextField),
	}
	actAddPrevField = &interact.Action[*addSpaceForm, addSubject]{
		Title: "previous field",
		Run:   cmd[*addSpaceForm, addSubject]((*addSpaceForm).prevField),
	}
	actAddNextGroup = &interact.Action[*addSpaceForm, addSubject]{
		Title:   "next group",
		Applies: func(f *addSpaceForm, _ addSubject) bool { return f.onGroup() },
		Run:     func(f *addSpaceForm, _ addSubject) tea.Cmd { return f.cycleGroup(1) },
	}
	actAddPrevGroup = &interact.Action[*addSpaceForm, addSubject]{
		Title:   "previous group",
		Applies: func(f *addSpaceForm, _ addSubject) bool { return f.onGroup() },
		Run:     func(f *addSpaceForm, _ addSubject) tea.Cmd { return f.cycleGroup(-1) },
	}
	actAddList = &interact.Action[*addSpaceForm, addSubject]{
		Title:   "list",
		Applies: func(f *addSpaceForm, _ addSubject) bool { return !f.busy && f.onGroup() },
		Run:     do[*addSpaceForm, addSubject](func(f *addSpaceForm) { f.group.openList() }),
	}
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
	actReplyClear  = clearDraft[*replyForm, replySubject]()
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
	entryKind       *interact.Kind[*sidebar, entrySubject]
	groupKind       *interact.Kind[*sidebar, groupSubject]
	detachedKind    *interact.Kind[*sidebar, detachedSubject]
	groupShowKind   *interact.Kind[*sidebar, groupFilterSubject]
	statusKind      *interact.Kind[*sidebar, statusSubject]
	paneKind        *interact.Kind[*sidebar, paneSubject]
	selectKind      *interact.Kind[*sidebar, selectSubject]
	spacesKind      *interact.Kind[*sidebar, spacesSubject]
	menuKind        *interact.Kind[*sidebar, menuSubject]
	helpKind        *interact.Kind[*keyHelp, helpSubject]
	pickerKind      *interact.Kind[*picker, pickerSubject]
	formKind        *interact.Kind[*newWorktreeForm, formSubject]
	removeKind      *interact.Kind[*removeWorktreeForm, removeSubject]
	removeSpaceKind *interact.Kind[*removeSpaceForm, removeSpaceSubject]
	closeKind       *interact.Kind[*closeSessionForm, closeSubject]
	forgetKind      *interact.Kind[*forgetSessionForm, forgetSubject]
	resumeKind      *interact.Kind[*resumeSessionForm, resumeSubject]
	endSessionsKind *interact.Kind[*endSessionsForm, endSessionsSubject]
	moveKind        *interact.Kind[*moveSessionsForm, moveSubject]
	addKind         *interact.Kind[*addSpaceForm, addSubject]
	cleanKind       *interact.Kind[*cleanupForm, cleanupSubject]
	runKind         *interact.Kind[*runForm, runSubject]
	layoutKind      *interact.Kind[*layoutForm, layoutSubject]
	saveKind        *interact.Kind[*saveLayoutForm, saveSubject]
	trustKind       *interact.Kind[*trustForm, trustSubject]
	replyKind       *interact.Kind[*replyForm, replySubject]
	errorKind       *interact.Kind[*sidebar, errorSubject]
	showKind        *interact.Kind[*errorForm, showSubject]
	filterKind      *interact.Kind[*sidebar, filterSubject]
	listKind        *interact.Kind[*choiceList, listSubject]
	sortKind        *interact.Kind[*sidebar, sortSubject]

	spacesList         *interact.Place[*sidebar, entrySubject]
	groupHeaders       *interact.Place[*sidebar, groupSubject]
	hostHeaders        *interact.Place[*sidebar, hostSubject]
	detachedRows       *interact.Place[*sidebar, detachedSubject]
	theGroups          *interact.Place[*sidebar, groupFilterSubject]
	statusList         *interact.Place[*sidebar, statusSubject]
	thePane            *interact.Place[*sidebar, paneSubject]
	theMenu            *interact.Place[*sidebar, menuSubject]
	theHelp            *interact.Place[*keyHelp, helpSubject]
	thePicker          *interact.Place[*picker, pickerSubject]
	pickRowKind        *interact.Kind[*picker, pickRowSubject]
	pickStatusKind     *interact.Kind[*picker, pickStatusSubject]
	pickMenuKind       *interact.Kind[*picker, pickMenuSubject]
	thePickRows        *interact.Place[*picker, pickRowSubject]
	thePickStatuses    *interact.Place[*picker, pickStatusSubject]
	thePickMenu        *interact.Place[*picker, pickMenuSubject]
	theForm            *interact.Place[*newWorktreeForm, formSubject]
	theRemoveForm      *interact.Place[*removeWorktreeForm, removeSubject]
	theRemoveSpaceForm *interact.Place[*removeSpaceForm, removeSpaceSubject]
	theCloseForm       *interact.Place[*closeSessionForm, closeSubject]
	theForgetForm      *interact.Place[*forgetSessionForm, forgetSubject]
	theResumeForm      *interact.Place[*resumeSessionForm, resumeSubject]
	theEndSessionsForm *interact.Place[*endSessionsForm, endSessionsSubject]
	theMoveForm        *interact.Place[*moveSessionsForm, moveSubject]
	theAddForm         *interact.Place[*addSpaceForm, addSubject]
	theCleanup         *interact.Place[*cleanupForm, cleanupSubject]
	theRunForm         *interact.Place[*runForm, runSubject]
	theLayoutForm      *interact.Place[*layoutForm, layoutSubject]
	theSaveForm        *interact.Place[*saveLayoutForm, saveSubject]
	theTrustForm       *interact.Place[*trustForm, trustSubject]
	theReplyForm       *interact.Place[*replyForm, replySubject]
	theErrorLine       *interact.Place[*sidebar, errorSubject]
	theShowForm        *interact.Place[*errorForm, showSubject]
	theFilter          *interact.Place[*sidebar, filterSubject]
	theSort            *interact.Place[*sidebar, sortSubject]
	theChoiceList      *interact.Place[*choiceList, listSubject]
	theSelection       *interact.Place[*sidebar, selectSubject]
)

// Kinds and places are made in init, as their actions run code that reads them.
func init() {
	// Every row and the sidebar carry the list's own, so a row's menu offers them, even a row that has
	// nothing else, and their keys work wherever the cursor is.
	spacesKind = interact.NewKind(func(spacesSubject) string { return "Spaces" }, actAddSpace)
	spacesKind.Bind(actAddSpace, "a", "add space", "a")

	entryKind = interact.NewKind(func(s entrySubject) string { return s.row.Space.Name },
		actOpenWorktree, actOpenPlain, actRunCommand, actOpenLayout, actSaveAsLayout, actCloseSession, actSuspendSession, actOpenPR, actRemoveWorktree, actForgetSession, actNewWorktree, actNewWorktreeFromPR, actNewWorktreeFromIssue, actFold, actUnfold, actCleanup, actTrustConfig, actRemoveSpaceRow, actSelectRow)
	entryKind.Bind(actFold, "space", "fold or unfold the space's worktrees", "space")
	entryKind.Bind(actUnfold, "", "", "space")
	bindRow(entryKind, actNewWorktree, rowNewWorktree)
	bindRow(entryKind, actNewWorktreeFromPR, rowNewWorktreeFromPR)
	bindRow(entryKind, actNewWorktreeFromIssue, rowNewWorktreeFromIssue)
	bindRow(entryKind, actRunCommand, rowRunCommand)
	bindRow(entryKind, actOpenLayout, rowOpenLayout)
	entryKind.Bind(actOpenPlain, "o", "open without the default layout", "o")
	bindRow(entryKind, actCloseSession, rowCloseSession)
	bindRow(entryKind, actSuspendSession, rowSuspendSession)
	bindRow(entryKind, actOpenPR, rowOpenPR)
	bindRow(entryKind, actRemoveWorktree, rowRemoveWorktree)
	bindRow(entryKind, actForgetSession, rowForgetSession)
	// F too, as x removes a linked worktree before forgetting its closed or suspended session.
	entryKind.Bind(actForgetSession, "F", "forget the saved session", "F")
	bindRow(entryKind, actCleanup, rowCleanup)
	bindRow(entryKind, actRemoveSpaceRow, rowRemoveSpace)
	interact.Via(entryKind, func(s entrySubject) (spacesSubject, bool) {
		if s.row.Space == nil || s.row.Other {
			return spacesSubject{}, true
		}
		return spacesSubject{group: s.row.Space.Group}, true
	}, spacesKind)
	spacesList = interact.List(entryKind)
	spacesList.Bind(actOpenWorktree, "enter/l", "open", "enter", "l")

	groupKind = interact.NewKind(func(s groupSubject) string { return s.name }, actFoldGroup, actUnfoldGroup, actSelectGroup)
	groupKind.Bind(actFoldGroup, "space", "fold or unfold the group", "space")
	groupKind.Bind(actUnfoldGroup, "", "", "space")
	interact.Via(groupKind, func(s groupSubject) (spacesSubject, bool) { return spacesSubject{group: s.name}, true }, spacesKind)
	groupHeaders = interact.List(groupKind)

	hostKind := interact.NewKind(func(s hostSubject) string { return hostName(s.host) }, actOpenHostShell, actFoldHost, actUnfoldHost)
	hostKind.Bind(actFoldHost, "space", "fold or unfold the host", "space")
	hostKind.Bind(actUnfoldHost, "", "", "space")
	hostKind.Bind(actOpenHostShell, "o", "open a shell on the host", "o")
	interact.Via(hostKind, func(hostSubject) (spacesSubject, bool) { return spacesSubject{}, true }, spacesKind)
	hostHeaders = interact.List(hostKind)
	hostHeaders.Bind(actFoldHost, "enter/l", "fold or unfold", "enter", "l")
	hostHeaders.Bind(actUnfoldHost, "", "", "enter", "l")

	detachedKind = interact.NewKind(func(s detachedSubject) string {
		if s.row.Session != nil {
			return s.row.Session.Name
		}
		return s.row.Of.Name
	}, actOpenDetached, actKillDetached, actShelveDetached, actRemoveDetached, actMoveDetached, actShowDetached, actHideDetached, actCleanupDetached)
	detachedKind.Bind(actShowDetached, "space", "show or hide the space's detached sessions", "space")
	detachedKind.Bind(actHideDetached, "", "", "space")
	// x as on a worktree's row: kill a running one, remove an ended one, which F does too, as forgetting.
	detachedKind.Bind(actKillDetached, "x", "kill the session", "x")
	detachedKind.Bind(actRemoveDetached, "F/x", "remove the ended session", "F", "x")
	detachedKind.Bind(actShelveDetached, "z", "shelve the session", "z")
	detachedKind.Bind(actMoveDetached, "M", "move the session to another space", "M")
	detachedKind.Bind(actCleanupDetached, "p", "clean up the space's detached sessions", "p")
	interact.Via(detachedKind, func(s detachedSubject) (spacesSubject, bool) { return spacesSubject{group: s.row.Of.Group}, true }, spacesKind)
	detachedRows = interact.List(detachedKind)
	detachedRows.Bind(actOpenDetached, "enter/l", "open", "enter", "l")
	detachedRows.Bind(actShowDetached, "", "", "enter", "l")
	detachedRows.Bind(actHideDetached, "", "", "enter", "l")

	statusKind = interact.NewKind(func(s statusSubject) string { return s.status.Session }, actOpenWindow, actReply, actClearStatus)
	statusKind.Bind(actReply, "t", "reply", "t")
	statusKind.Bind(actClearStatus, "x", "clear", "x")
	statusList = interact.List(statusKind)
	statusList.Bind(actOpenWindow, "enter/l", "open", "enter", "l")

	paneKind = interact.NewKind(func(paneSubject) string { return "Sidebar" },
		actDown, actUp, actHalfDown, actHalfUp, actPageDown, actPageUp, actTop, actBottom, actLeave, actRefresh, actMenuHere, actQuit,
		actAttend, actClearFinished, actFilter, actAttention, actOpenOnly, actAwakeOnly, actOpenPicker, actCycleSort, actMoveUp, actMoveDown, actSelect, actSwitchTab, actHelp)
	paneKind.Bind(actDown, "j/k", "down/up", "j", "down")
	paneKind.Bind(actUp, "", "", "k", "up")
	paneKind.Bind(actHalfDown, "ctrl+d/u", "half page down/up", "ctrl+d")
	paneKind.Bind(actHalfUp, "", "", "ctrl+u")
	paneKind.Bind(actPageDown, "ctrl+f/b", "page down/up", "ctrl+f", "pgdown")
	paneKind.Bind(actPageUp, "", "", "ctrl+b", "pgup")
	paneKind.Bind(actTop, "g/G", "top/bottom", "g", "home")
	paneKind.Bind(actBottom, "", "", "G", "end")
	paneKind.Bind(actLeave, "esc/q", "leave", "esc", "q")
	paneKind.Bind(actRefresh, "r", "refresh", "r")
	paneKind.Bind(actMenuHere, "m", "menu", "m")
	paneKind.BindQuiet(actEndRemote, "enter", "l")
	paneKind.Bind(actQuit, "ctrl+c", "quit", "ctrl+c")
	paneKind.Bind(actAttend, "!", "go to what needs you", "!")
	paneKind.Bind(actClearFinished, "X", "clear the done and failed statuses", "X")
	paneKind.Bind(actFilter, "/", "filter the spaces", "/")
	paneKind.Bind(actAttention, "f", "show only what is not idle", "f")
	paneKind.Bind(actOpenOnly, "O", "show only open sessions", "O")
	paneKind.Bind(actAwakeOnly, "Z", "hide suspended sessions", "Z")
	paneKind.Bind(actOpenPicker, "P", "open the picker", "P")
	paneKind.Bind(actCycleSort, "s", "change the sort mode", "s")
	paneKind.Bind(actMoveUp, "K/J", "move the row up/down", "K")
	paneKind.Bind(actMoveDown, "", "", "J")
	paneKind.Bind(actSelect, "v", "select rows to act on", "v")
	paneKind.Bind(actSwitchTab, "tab", "switch between the spaces and hosts tabs", "tab")
	paneKind.Bind(actHelp, "?", "list keys", append([]string{"?"}, helpKeys...)...)
	interact.Via(paneKind, func(paneSubject) (spacesSubject, bool) { return spacesSubject{}, true }, spacesKind)
	thePane = interact.List(paneKind)

	selectKind = interact.NewKind(func(selectSubject) string { return "Select" },
		actTick, actTickAll, actSelRemove, actSelClose, actSelSuspend, actSelForget, actSelRun, actSelMove, actEndSelect)
	selectKind.Bind(actTick, "space", "tick the row", "space")
	selectKind.Bind(actTickAll, "A", "tick every row", "A")
	selectKind.Bind(actSelRemove, "x", "remove the ticked worktrees", "x")
	selectKind.Bind(actSelClose, "c", "close the ticked sessions", "c")
	selectKind.Bind(actSelSuspend, "z", "suspend the ticked sessions", "z")
	selectKind.Bind(actSelForget, "F", "forget the ticked sessions' saved files", "F")
	selectKind.Bind(actSelRun, "R", "run a command in the ticked worktrees", "R")
	selectKind.Bind(actSelMove, "M", "move the ticked sessions to another space", "M")
	selectKind.Bind(actEndSelect, "esc", "leave the select mode", "esc")
	theSelection = interact.List(selectKind)

	menuKind = interact.NewKind(func(menuSubject) string { return "Menu" },
		actMenuDown, actMenuUp, actMenuRun, actMenuClose, actMenuHelp, actMenuQuit)
	menuKind.Bind(actMenuDown, "j/k", "down/up", "j", "down")
	menuKind.Bind(actMenuUp, "", "", "k", "up")
	menuKind.Bind(actMenuRun, "enter/l", "run", "enter", "l")
	menuKind.Bind(actMenuClose, "esc/q/m", "close", "esc", "q", "m")
	menuKind.Bind(actMenuHelp, "?", "list keys", append([]string{"?"}, helpKeys...)...)
	menuKind.Bind(actMenuQuit, "ctrl+c", "quit", "ctrl+c")
	theMenu = interact.List(menuKind)

	helpKind = interact.NewKind(func(helpSubject) string { return "Keys" }, actHelpDown, actHelpUp, actHelpTop, actHelpBottom, actHelpClose, actHelpQuit)
	helpKind.Bind(actHelpDown, "j/k", "scroll", "j", "down")
	helpKind.Bind(actHelpUp, "", "", "k", "up")
	helpKind.Bind(actHelpTop, "g/G", "top/bottom", "g", "home")
	helpKind.Bind(actHelpBottom, "", "", "G", "end")
	helpKind.Bind(actHelpClose, "esc/q/?", "close", append([]string{"esc", "q", "?"}, helpKeys...)...)
	helpKind.Bind(actHelpQuit, "ctrl+c", "quit", "ctrl+c")
	theHelp = interact.List(helpKind)

	pickerKind = interact.NewKind(func(pickerSubject) string { return "Picker" }, actPickDown, actPickUp, actPick, actPickPlain, actPickMenu, actPickScope, actPickQuit)
	pickerKind.Bind(actPickDown, "down/up", "down/up", "down", "ctrl+n", "ctrl+j")
	pickerKind.Bind(actPickUp, "", "", "up", "ctrl+p", "ctrl+k")
	pickerKind.Bind(actPick, "enter", "switch", "enter")
	// alt+enter for a terminal that sends shift+enter as enter, as a form's newline takes it.
	pickerKind.Bind(actPickPlain, "shift+enter", "switch without layout", "shift+enter", "alt+enter")
	pickerKind.Bind(actPickMenu, "tab", "actions", "tab")
	pickerKind.Bind(actPickScope, "shift+tab", "next scope", "shift+tab")
	pickerKind.Bind(actPickQuit, "esc", "cancel", "esc", "ctrl+c")
	thePicker = interact.List(pickerKind)

	// The same forms as the sidebar's rows, in the same order, on the same keys, which the menu takes as
	// the query does not see them while it is open.
	pickRows := []*rowAction{rowRunCommand, rowOpenLayout, rowSaveAsLayout, rowCloseSession, rowSuspendSession,
		rowOpenPR, rowRemoveWorktree, rowForgetSession, rowNewWorktree, rowNewWorktreeFromPR, rowNewWorktreeFromIssue,
		rowCleanup, rowTrustConfig}
	pickActs := []*interact.Action[*picker, pickRowSubject]{actPickOpen, actPickOpenPlain}
	for _, r := range pickRows {
		pickActs = append(pickActs, onPick(r))
	}
	pickRowKind = interact.NewKind(func(s pickRowSubject) string { return s.row.Space.Name }, pickActs...)
	pickRowKind.Bind(actPickOpenPlain, "o", "open without the default layout", "o")
	for i, r := range pickRows {
		bindRow(pickRowKind, pickActs[2+i], r)
	}
	thePickRows = interact.List(pickRowKind)
	pickStatusKind = interact.NewKind(func(s pickStatusSubject) string { return s.status.Session }, actPickOpenStatus, actPickReply)
	pickStatusKind.Bind(actPickReply, "t", "reply", "t")
	thePickStatuses = interact.List(pickStatusKind)
	pickMenuKind = interact.NewKind(func(pickMenuSubject) string { return "Menu" },
		actPickMenuDown, actPickMenuUp, actPickMenuRun, actPickMenuClose, actPickMenuQuit)
	pickMenuKind.Bind(actPickMenuDown, "down/up", "down/up", "down", "ctrl+n", "ctrl+j")
	pickMenuKind.Bind(actPickMenuUp, "", "", "up", "ctrl+p", "ctrl+k")
	pickMenuKind.Bind(actPickMenuRun, "enter", "run", "enter")
	pickMenuKind.Bind(actPickMenuClose, "esc/tab", "close", "esc", "tab")
	pickMenuKind.Bind(actPickMenuQuit, "ctrl+c", "quit", "ctrl+c")
	thePickMenu = interact.List(pickMenuKind)

	formKind = interact.NewKind(func(formSubject) string { return "New worktree" },
		actSwitchField, actPrevField, actNextChoice, actPrevChoice, actFormList, actFormBackground, actCreate, actCancel, actFormClear)
	formKind.Bind(actSwitchField, "tab", "next field", "tab", "down")
	formKind.Bind(actPrevField, "shift+tab", "previous field", "shift+tab", "up")
	formKind.Bind(actNextChoice, "left/right", "choose", "right", "l")
	formKind.Bind(actPrevChoice, "", "", "left", "h")
	formKind.Bind(actFormList, "space", "list", "space")
	formKind.Bind(actFormBackground, "space", "tick", "space")
	formKind.Bind(actCreate, "enter", "create", "enter")
	formKind.BindQuiet(actWaitForCreate, "enter")
	formKind.Bind(actCancel, "esc", "cancel", "esc", "ctrl+c")
	formKind.Bind(actFormClear, "ctrl+r", "clear what was typed", "ctrl+r")
	theForm = interact.List(formKind)

	removeKind = interact.NewKind(func(removeSubject) string { return "Remove worktree" },
		actRemove, actRemoveCancel, actRemoveDeleteBranch, actRemoveQuit)
	removeKind.Bind(actRemove, "y/enter", "remove", "y", "enter")
	removeKind.Bind(actRemoveCancel, "n/esc", "cancel", "n", "esc")
	removeKind.Bind(actRemoveDeleteBranch, "d", "delete branch", "d")
	removeKind.Bind(actRemoveQuit, "ctrl+c", "quit", "ctrl+c")
	theRemoveForm = interact.List(removeKind)

	removeSpaceKind = interact.NewKind(func(removeSpaceSubject) string { return "Remove space" },
		actRemoveSpace, actRemoveSpaceCancel, actRemoveSpaceNextFate, actRemoveSpacePrevFate, actRemoveSpaceQuit)
	removeSpaceKind.Bind(actRemoveSpace, "y/enter", "remove", "y", "enter")
	removeSpaceKind.Bind(actRemoveSpaceCancel, "n/esc", "cancel", "n", "esc")
	removeSpaceKind.Bind(actRemoveSpaceNextFate, "left/right", "what becomes of the sessions", "right", "l")
	removeSpaceKind.Bind(actRemoveSpacePrevFate, "", "", "left", "h")
	removeSpaceKind.Bind(actRemoveSpaceQuit, "ctrl+c", "quit", "ctrl+c")
	theRemoveSpaceForm = interact.List(removeSpaceKind)

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

	closeKind = interact.NewKind(func(closeSubject) string { return "Close session" }, actClose, actCloseCancel, actCloseNextFate, actClosePrevFate, actCloseQuit)
	closeKind.Bind(actClose, "y/enter", "close", "y", "enter")
	closeKind.Bind(actCloseCancel, "n/esc", "cancel", "n", "esc")
	closeKind.Bind(actCloseNextFate, "left/right", "what becomes of the sessions", "right", "l")
	closeKind.Bind(actClosePrevFate, "", "", "left", "h")
	closeKind.Bind(actCloseQuit, "ctrl+c", "quit", "ctrl+c")
	theCloseForm = interact.List(closeKind)

	forgetKind = interact.NewKind(func(forgetSubject) string { return "Forget session" }, actForget, actForgetCancel, actForgetQuit)
	forgetKind.Bind(actForget, "y/enter", "delete", "y", "enter")
	forgetKind.Bind(actForgetCancel, "n/esc", "cancel", "n", "esc")
	forgetKind.Bind(actForgetQuit, "ctrl+c", "quit", "ctrl+c")
	theForgetForm = interact.List(forgetKind)
	resumeKind = interact.NewKind(func(resumeSubject) string { return "Resume session" }, actResume, actResumeCancel, actResumeQuit)
	resumeKind.Bind(actResume, "y/enter", "resume", "y", "enter")
	resumeKind.Bind(actResumeCancel, "n/esc", "cancel", "n", "esc")
	resumeKind.Bind(actResumeQuit, "ctrl+c", "quit", "ctrl+c")
	theResumeForm = interact.List(resumeKind)

	endSessionsKind = interact.NewKind(func(endSessionsSubject) string { return "End sessions" },
		actEndSessionsDown, actEndSessionsUp, actEndSessionsToggle, actEndSessionsTickAll, actEndSessions, actEndSessionsCancel, actEndSessionsQuit)
	endSessionsKind.Bind(actEndSessionsDown, "j/k", "down/up", "j", "down")
	endSessionsKind.Bind(actEndSessionsUp, "", "", "k", "up")
	endSessionsKind.Bind(actEndSessionsToggle, "space", "tick", "space")
	endSessionsKind.Bind(actEndSessionsTickAll, "A", "tick all", "A")
	endSessionsKind.Bind(actEndSessions, "y/enter", "end", "y", "enter")
	endSessionsKind.Bind(actEndSessionsCancel, "n/esc", "cancel", "n", "esc")
	endSessionsKind.Bind(actEndSessionsQuit, "ctrl+c", "quit", "ctrl+c")
	theEndSessionsForm = interact.List(endSessionsKind)

	moveKind = interact.NewKind(func(moveSubject) string { return "Move sessions" }, actMoveNext, actMovePrev, actMove, actMoveCancel)
	moveKind.Bind(actMoveNext, "down/up", "choose a space", "down", "ctrl+n", "ctrl+j")
	moveKind.Bind(actMovePrev, "", "", "up", "ctrl+p", "ctrl+k")
	moveKind.Bind(actMove, "enter", "move", "enter")
	moveKind.Bind(actMoveCancel, "esc", "cancel", "esc", "ctrl+c")
	theMoveForm = interact.List(moveKind)

	trustKind = interact.NewKind(func(trustSubject) string { return "Trust space config" }, actTrust, actTrustCancel, actTrustQuit)
	trustKind.Bind(actTrust, "y/enter", "trust", "y", "enter")
	trustKind.Bind(actTrustCancel, "n/esc", "cancel", "n", "esc")
	trustKind.Bind(actTrustQuit, "ctrl+c", "quit", "ctrl+c")
	theTrustForm = interact.List(trustKind)

	addKind = interact.NewKind(func(addSubject) string { return "Add space" },
		actAddNextField, actAddPrevField, actAddNextGroup, actAddPrevGroup, actAddList, actAdd, actAddCancel, actAddClear)
	addKind.Bind(actAddNextField, "tab", "next field", "tab", "down")
	addKind.Bind(actAddPrevField, "shift+tab", "previous field", "shift+tab", "up")
	addKind.Bind(actAddNextGroup, "left/right", "group", "right", "l")
	addKind.Bind(actAddPrevGroup, "", "", "left", "h")
	addKind.Bind(actAddList, "space", "list", "space")
	addKind.Bind(actAdd, "enter", "add", "enter")
	addKind.Bind(actAddCancel, "esc", "cancel", "esc", "ctrl+c")
	addKind.Bind(actAddClear, "ctrl+r", "clear what was typed", "ctrl+r")
	theAddForm = interact.List(addKind)

	replyKind = interact.NewKind(func(replySubject) string { return "Reply" }, actReplySend, actReplyType, actReplyCancel, actReplyClear)
	replyKind.Bind(actReplySend, "enter", "send", "enter")
	replyKind.Bind(actReplyType, "ctrl+s", "send without enter", "ctrl+s")
	replyKind.Bind(actReplyCancel, "esc", "cancel", "esc", "ctrl+c")
	replyKind.Bind(actReplyClear, "ctrl+r", "clear what was typed", "ctrl+r")
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
		actRunNextField, actRunPrevField, actRunNextCommand, actRunPrevCommand, actRunList, actRun, actRunCancel, actRunClear)
	runKind.Bind(actRunNextField, "tab", "next field", "tab", "down")
	runKind.Bind(actRunPrevField, "shift+tab", "previous field", "shift+tab", "up")
	runKind.Bind(actRunNextCommand, "left/right", "command", "right", "l")
	runKind.Bind(actRunPrevCommand, "", "", "left", "h")
	runKind.Bind(actRunList, "space", "list", "space")
	runKind.Bind(actRun, "enter", "run", "enter")
	runKind.BindQuiet(actWaitForRun, "enter")
	runKind.Bind(actRunCancel, "esc", "cancel", "esc", "ctrl+c")
	runKind.Bind(actRunClear, "ctrl+r", "clear what was typed", "ctrl+r")
	theRunForm = interact.List(runKind)

	layoutKind = interact.NewKind(func(layoutSubject) string { return "Open layout" },
		actLayoutNextField, actLayoutPrevField, actNextLayout, actPrevLayout, actLayoutList, actOpenLayoutNow, actLayoutCancel, actLayoutClear)
	layoutKind.Bind(actLayoutNextField, "tab", "next field", "tab", "down")
	layoutKind.Bind(actLayoutPrevField, "shift+tab", "previous field", "shift+tab", "up")
	layoutKind.Bind(actNextLayout, "left/right", "layout", "right", "l")
	layoutKind.Bind(actPrevLayout, "", "", "left", "h")
	layoutKind.Bind(actLayoutList, "space", "list", "space")
	layoutKind.Bind(actOpenLayoutNow, "enter", "open", "enter")
	layoutKind.BindQuiet(actWaitForLayout, "enter")
	layoutKind.Bind(actLayoutCancel, "esc", "cancel", "esc", "ctrl+c")
	layoutKind.Bind(actLayoutClear, "ctrl+r", "clear what was typed", "ctrl+r")
	theLayoutForm = interact.List(layoutKind)

	saveKind = interact.NewKind(func(saveSubject) string { return "Save layout" },
		actSaveNextField, actSavePrevField, actSaveNextChoice, actSavePrevChoice, actSaveList, actSaveTick, actSaveLayout, actSaveReplace, actSaveKeep, actSaveCancel, actSaveClear)
	saveKind.Bind(actSaveNextField, "tab", "next field", "tab", "down")
	saveKind.Bind(actSavePrevField, "shift+tab", "previous field", "shift+tab", "up")
	saveKind.Bind(actSaveNextChoice, "left/right", "choose", "right", "l")
	saveKind.Bind(actSavePrevChoice, "", "", "left", "h")
	saveKind.Bind(actSaveList, "space", "list", "space")
	saveKind.Bind(actSaveTick, "space", "tick", "space")
	saveKind.Bind(actSaveLayout, "enter", "save", "enter")
	saveKind.Bind(actSaveReplace, "y", "replace", "y")
	saveKind.Bind(actSaveKeep, "n/esc", "keep it", "n", "esc")
	saveKind.BindQuiet(actWaitForSave, "enter")
	saveKind.Bind(actSaveCancel, "esc", "cancel", "esc", "ctrl+c")
	saveKind.Bind(actSaveClear, "ctrl+r", "clear what was typed", "ctrl+r")
	theSaveForm = interact.List(saveKind)

	// j and k type, as worktrees are named with them, so only arrows and the picker's ctrl keys move.
	filterKind = interact.NewKind(func(filterSubject) string { return "Filter" },
		actFilterDown, actFilterUp, actFilterOpen, actFilterClear, actFilterHelp, actFilterQuit)
	filterKind.Bind(actFilterDown, "down/up", "down/up", "down", "ctrl+n", "ctrl+j")
	filterKind.Bind(actFilterUp, "", "", "up", "ctrl+p", "ctrl+k")
	filterKind.Bind(actFilterOpen, "enter", "open", "enter")
	filterKind.Bind(actFilterClear, "esc", "clear", "esc")
	filterKind.Bind(actFilterHelp, "ctrl+/", "list keys", helpKeys...)
	filterKind.Bind(actFilterQuit, "ctrl+c", "quit", "ctrl+c")
	theFilter = interact.List(filterKind)

	sortKind = interact.NewKind(func(sortSubject) string { return "Sort" },
		sortActions[spaces.SortManual], sortActions[spaces.SortActivity], sortActions[spaces.SortStatus],
		actAttentionItem, actOpenItem, actAwakeItem, actSelectList)
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
	case i >= 0 && i < len(m.rows) && m.rows[i].HostHeader:
		return hostHeaders.Target(hostSubject{host: m.rows[i].Host}), true
	case i >= 0 && i < len(m.rows) && m.rows[i].Group != nil:
		return groupHeaders.Target(groupSubject{name: m.rows[i].Group.Name}), true
	case i >= 0 && i < len(m.rows) && m.rows[i].Detached != nil:
		return detachedRows.Target(detachedSubject{index: i, row: *m.rows[i].Detached}), true
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
