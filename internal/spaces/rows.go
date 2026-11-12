package spaces

import (
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/chancez/kspaces/internal/config"
	"github.com/chancez/kspaces/internal/gitstate"
	"github.com/chancez/kspaces/internal/kitty"
)

// Row is one line of the sidebar's list. The zero row is the blank line between groups.
type Row struct {
	Space *Space
	// Nil when the space's worktrees could not be listed.
	Worktree *Worktree
	// The space's own row, which opens its main worktree.
	Main bool
	// A session that belongs to no space, see placeSessions.
	Other bool
	// A missing worktree of the space, see Space.Missing. Its Worktree has no path.
	Missing bool
	// On a space's own row: its linked worktrees are hidden.
	Folded bool
}

// Dir names a linked worktree's row: its directory, or for a missing one the directory its session is
// named after, as git no longer has its path.
func (r Row) Dir() string {
	if r.Missing {
		return strings.TrimPrefix(r.Worktree.Session, r.Space.Name+"@")
	}
	return filepath.Base(r.Worktree.Path)
}

// BuildRows lists the sessions that belong to no space first, then each space followed by its linked
// worktrees and its missing ones, with a blank row between groups and between spaces, all in order's
// order. A space's own row opens its main worktree. The linked and missing worktrees of a space whose root
// is in folded are left out. The rows point into copies of others and spaces.
func BuildRows(others, spaces []Space, folded map[string]bool, order Order) []Row {
	others, spaces = order.arrange(others, spaces)
	var rows []Row
	for i := range others {
		rows = append(rows, Row{Space: &others[i], Worktree: &others[i].Worktrees[0], Main: true, Other: true})
	}
	for i := range spaces {
		if len(rows) > 0 {
			rows = append(rows, Row{})
		}
		s := &spaces[i]
		r := Row{Space: s, Main: true, Folded: folded[s.Root] && s.HasLinked()}
		if len(s.Worktrees) > 0 {
			r.Worktree = &s.Worktrees[0]
		}
		rows = append(rows, r)
		if r.Folded {
			continue
		}
		for j := 1; j < len(s.Worktrees); j++ {
			rows = append(rows, Row{Space: s, Worktree: &s.Worktrees[j]})
		}
		for j := range s.Missing {
			rows = append(rows, Row{Space: s, Worktree: &s.Missing[j], Missing: true})
		}
	}
	return rows
}

// placeSessions sorts the sessions that are open or have a saved file but belong to no worktree. One named
// SPACE@DIR after a space whose worktrees were listed is a missing worktree of that space, added to its
// Missing in name order: its directory is gone, as when a worktree is removed while its session has
// windows. The rest are returned as the others, such as the one kitty starts with, each a space of its
// own with one worktree that has no path, so one created afresh starts in the home directory.
func placeSessions(spaces []Space, open map[string]int, saved []string) (others []Space) {
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
	var sorted []string
	for name := range names {
		if !inSpace[name] {
			sorted = append(sorted, name)
		}
	}
	slices.Sort(sorted)
	for _, name := range sorted {
		if s := missingOf(spaces, name); s != nil {
			s.Missing = append(s.Missing, Worktree{Session: name})
			continue
		}
		others = append(others, Space{Name: name, Worktrees: []Worktree{{Session: name}}})
	}
	return others
}

// missingOf is the space session would be a linked worktree of, by its name, if its worktrees were listed.
// The name is matched whole rather than cut at an @, which a space's or a directory's name may have.
func missingOf(spaces []Space, session string) *Space {
	for i := range spaces {
		s := &spaces[i]
		if dir, ok := strings.CutPrefix(session, s.Name+"@"); ok && dir != "" && s.Err == nil {
			return s
		}
	}
	return nil
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

// Snapshot is everything the sidebar and the picker show, read at once.
type Snapshot struct {
	// Nil when nothing was loaded, for example because opening a worktree failed.
	Config *config.Config
	Spaces []Space
	// Sessions that belong to no space, see placeSessions.
	Others []Space
	// Roots of the spaces whose linked worktrees are hidden.
	Folded map[string]bool
	// How the rows are ordered: the sort mode and the worktrees' manual order, saved as the folded spaces
	// are, with kitty's focus times and statuses.
	Order Order
	// Sessions with a file in the sessions directory, to be restored from.
	Saved map[string]bool
	Kitty kitty.State
	// What git and gh say of each worktree, by path, as far as the leading sidebar knows.
	Git map[string]gitstate.State
	Err error
}

// Load reads the config on every call, so edits show up without restarting every dock, and a broken
// config is returned as the snapshot's error rather than ending the sidebar. kspaces_watcher.py would
// otherwise keep replacing a dock that exits the moment it starts. kitty's state is seen from the window
// self, or kitty.FocusedWindow.
func Load(self int) Snapshot {
	return LoadShared().View(self)
}
