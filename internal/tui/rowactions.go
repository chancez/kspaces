package tui

import (
	"fmt"

	tea "charm.land/bubbletea/v2"

	"github.com/chancez/kspaces/internal/gh"
	"github.com/chancez/kspaces/internal/spaces"
	"github.com/chancez/kspaces/internal/tui/interact"
)

// rowAction is what a row of a space or a session offers wherever it shows, the sidebar's spaces list and
// the picker alike: its menu title and section, whether it applies by what snap says, and what it does,
// which opens a form in a popup, run off the UI loop. Each host makes its own interact.Action of it.
type rowAction struct {
	title string
	// name is the menu's label when it says more than title, nil for title.
	name    func(snap spaces.Snapshot, r spaces.Row) string
	section string
	applies func(snap spaces.Snapshot, r spaces.Row) bool
	ask     func(snap spaces.Snapshot, r spaces.Row) error
	// key runs it on a row, wherever the row shows, see bindRow.
	key string
	// desc is what help says of key.
	desc string
}

// onEntry is a as the sidebar's spaces list offers it.
func onEntry(a *rowAction) *interact.Action[*sidebar, entrySubject] {
	act := &interact.Action[*sidebar, entrySubject]{
		Title:   a.title,
		Section: a.section,
		Applies: func(m *sidebar, s entrySubject) bool { return a.applies(m.view(), s.row) },
		Run: func(m *sidebar, s entrySubject) tea.Cmd {
			m.remote = false
			snap, r := m.view(), s.row
			return func() tea.Msg {
				if err := a.ask(snap, r); err != nil {
					return failedMsg{err}
				}
				return nil
			}
		},
	}
	if a.name != nil {
		act.Name = func(m *sidebar, s entrySubject) string { return a.name(m.view(), s.row) }
	}
	return act
}

// newWorktreeApplies is whether a worktree can be created in a row's space: one configured and read.
func newWorktreeApplies(_ spaces.Snapshot, r spaces.Row) bool {
	return !r.Other && !r.Missing && r.Space.Err == nil
}

// rowBranch is the branch of the linked worktree r is the row of, which a new worktree made from its row
// starts from; empty for the space's own row, whose branch is the default.
func rowBranch(r spaces.Row) string {
	if r.Main || r.Worktree == nil {
		return ""
	}
	return r.Worktree.Branch
}

// rowPR is the PR of r's worktree's branch the poller found, the one its row shows, nil for none.
func rowPR(snap spaces.Snapshot, r spaces.Row) *gh.PR {
	if r.Worktree == nil {
		return nil
	}
	return snap.Git[r.Worktree.Path].PR
}

// newWorktreeFrom opens the form that creates a worktree in the row's space, on the tab from says, a new
// branch starting from the row's: the dock is too narrow to type a name in and read back where it goes.
func newWorktreeFrom(from spaces.From, title, key, desc string) *rowAction {
	return &rowAction{
		title:   title,
		key:     key,
		desc:    desc,
		section: sectionSpace,
		applies: newWorktreeApplies,
		ask: func(snap spaces.Snapshot, r spaces.Row) error {
			base := rowBranch(r)
			if from == spaces.FromPR {
				base = ""
			}
			return spaces.NewWorktree(*snap.Config, snap.Kitty, r.Space.Root, from, base)
		},
	}
}

// The forms a row opens, each in a popup for the tab st is seen from.
var (
	rowNewWorktree          = newWorktreeFrom(spaces.FromBranch, "New worktree", "n", "new worktree")
	rowNewWorktreeFromPR    = newWorktreeFrom(spaces.FromPR, "New worktree from PR", "N", "new worktree from PR")
	rowNewWorktreeFromIssue = newWorktreeFrom(spaces.FromIssue, "New worktree from issue", "I", "new worktree from issue")
	rowRemoveWorktree       = &rowAction{
		title:   "Remove worktree",
		key:     "x",
		desc:    "remove worktree",
		section: sectionWorktree,
		// Only a linked worktree: the main one is the repository itself.
		applies: func(_ spaces.Snapshot, r spaces.Row) bool { return !r.Main && !r.Missing && r.Worktree != nil },
		ask: func(snap spaces.Snapshot, r spaces.Row) error {
			return spaces.AskRemoveWorktree(*snap.Config, snap.Kitty, r.Worktree.Path)
		},
	}
	// rowOpenPR opens the PR in the browser with gh, in the worktree for its repository, rather than a form.
	rowOpenPR = &rowAction{
		title: "Open PR",
		key:   "b",
		desc:  "open the PR in the browser",
		name: func(snap spaces.Snapshot, r spaces.Row) string {
			return fmt.Sprintf("Open PR #%d", rowPR(snap, r).Number)
		},
		section: sectionWorktree,
		// Not on a missing worktree: its directory, where gh would run, is gone.
		applies: func(snap spaces.Snapshot, r spaces.Row) bool { return !r.Missing && rowPR(snap, r) != nil },
		ask: func(snap spaces.Snapshot, r spaces.Row) error {
			return gh.Browse(r.Worktree.Path, rowPR(snap, r).Number)
		},
	}
	// rowRunCommand runs a command in the worktree, the first of its space's chosen.
	rowRunCommand = &rowAction{
		title:   "Run",
		key:     "R",
		desc:    "run a command",
		section: sectionSession,
		applies: func(snap spaces.Snapshot, r spaces.Row) bool {
			return !r.Other && !r.Missing && r.Worktree != nil && len(r.Space.ConfigOf(*snap.Config).Commands) > 0
		},
		ask: func(snap spaces.Snapshot, r spaces.Row) error {
			name := r.Space.ConfigOf(*snap.Config).Commands[0].Name
			return spaces.AskRunCommand(*snap.Config, snap.Kitty, name, r.Worktree.Path)
		},
	}
	// rowOpenLayout opens a layout in the worktree's session, its default layout chosen, else the first.
	rowOpenLayout = &rowAction{
		title:   "Open layout",
		key:     "L",
		desc:    "open a layout",
		section: sectionSession,
		applies: func(snap spaces.Snapshot, r spaces.Row) bool {
			return !r.Other && !r.Missing && r.Worktree != nil && len(r.Space.ConfigOf(*snap.Config).Layouts) > 0
		},
		ask: func(snap spaces.Snapshot, r spaces.Row) error {
			cfg := r.Space.ConfigOf(*snap.Config)
			name := cfg.LayoutFor(r.Space.Name, !r.Main)
			if name == "" {
				name = cfg.Layouts[0].Name
			}
			return spaces.AskOpenLayout(*snap.Config, snap.Kitty, name, r.Worktree.Path)
		},
	}
	rowSaveAsLayout = &rowAction{
		title:   "Save as layout",
		section: sectionSession,
		// A worktree's session with windows to save, whose cwds are templates of the worktree's.
		applies: func(snap spaces.Snapshot, r spaces.Row) bool {
			return !r.Other && !r.Missing && r.Worktree != nil && snap.Kitty.Windows[r.Worktree.Session] > 0
		},
		ask: func(snap spaces.Snapshot, r spaces.Row) error {
			return spaces.AskSaveLayout(*snap.Config, snap.Kitty, r.Worktree.Path)
		},
	}
	rowCloseSession = &rowAction{
		title:   "Close session",
		key:     "c",
		desc:    "close session",
		section: sectionSession,
		applies: func(snap spaces.Snapshot, r spaces.Row) bool {
			return r.Worktree != nil && snap.Kitty.Windows[r.Worktree.Session] > 0
		},
		ask: func(snap spaces.Snapshot, r spaces.Row) error {
			return spaces.AskCloseSession(*snap.Config, snap.Kitty, r.Worktree.Session)
		},
	}
	rowSuspendSession = &rowAction{
		title:   "Suspend session",
		key:     "z",
		desc:    "suspend session",
		section: sectionSession,
		// Suspending keeps the session's file, which only the session saver writes.
		applies: func(snap spaces.Snapshot, r spaces.Row) bool {
			return r.Worktree != nil && snap.Kitty.Windows[r.Worktree.Session] > 0 && spaces.CanKeep(*snap.Config)
		},
		ask: func(snap spaces.Snapshot, r spaces.Row) error {
			return spaces.AskSuspendSession(*snap.Config, snap.Kitty, r.Worktree.Session)
		},
	}
	rowForgetSession = &rowAction{
		title: "Forget",
		// x, as a missing worktree has nothing to remove but its saved file.
		key:     "x",
		desc:    "forget the saved session",
		section: sectionWorktree,
		// A session with only its saved file left, which kitty would restore it from: a missing worktree's,
		// or a closed or suspended one's.
		applies: func(snap spaces.Snapshot, r spaces.Row) bool {
			return r.Worktree != nil && snap.Kitty.Windows[r.Worktree.Session] == 0 && snap.Saved[r.Worktree.Session]
		},
		ask: func(snap spaces.Snapshot, r spaces.Row) error {
			return spaces.AskForgetSession(*snap.Config, snap.Kitty, r.Worktree.Session)
		},
	}
	rowCleanup = &rowAction{
		title:   "Clean up worktrees",
		key:     "p",
		desc:    "clean up worktrees",
		section: sectionSpace,
		// On the space's own row: it offers removing any of the space's linked worktrees.
		applies: func(_ spaces.Snapshot, r spaces.Row) bool {
			return r.Main && !r.Other && r.Space.Err == nil && len(r.Space.Worktrees) > 1
		},
		ask: func(snap spaces.Snapshot, r spaces.Row) error {
			return spaces.AskCleanup(*snap.Config, snap.Kitty, r.Space.Root)
		},
	}
	// rowTrustConfig shows what the space's own config adds, and trusts it.
	rowTrustConfig = &rowAction{
		title:   "Trust space config",
		section: sectionSpace,
		// Its file is there, but not trusted as it is: never, or not since it changed.
		applies: func(_ spaces.Snapshot, r spaces.Row) bool { return !r.Other && r.Space.Untrusted },
		ask: func(snap spaces.Snapshot, r spaces.Row) error {
			return spaces.AskTrust(*snap.Config, snap.Kitty, r.Space.Root)
		},
	}
)

// bindRow binds r's key on k to a, the action k's host made of r, so a row takes the same key in every
// place it shows.
func bindRow[M, S any](k *interact.Kind[M, S], a *interact.Action[M, S], r *rowAction) {
	if r.key != "" {
		k.Bind(a, r.key, r.desc, r.key)
	}
}
