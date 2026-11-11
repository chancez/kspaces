package spaces

import (
	"os"
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
	// A session that belongs to no space, see otherSessions.
	Other bool
	// On a space's own row: its linked worktrees are hidden.
	Folded bool
}

// BuildRows lists the sessions that belong to no space first, then each space followed by its linked
// worktrees, with a blank row between groups and between spaces. A space's own row opens its main worktree.
// The linked worktrees of a space whose root is in folded are left out.
func BuildRows(others, spaces []Space, folded map[string]bool) []Row {
	var rows []Row
	for i := range others {
		rows = append(rows, Row{Space: &others[i], Worktree: &others[i].Worktrees[0], Main: true, Other: true})
	}
	for i := range spaces {
		if len(rows) > 0 {
			rows = append(rows, Row{})
		}
		s := &spaces[i]
		r := Row{Space: s, Main: true, Folded: folded[s.Root] && len(s.Worktrees) > 1}
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
	}
	return rows
}

// otherSessions are the sessions that are open or have a saved file but belong to no worktree, such as the
// one kitty starts with. Each is a space of its own with one worktree that has no path, so one created
// afresh starts in the home directory.
func otherSessions(spaces []Space, open map[string]int, saved []string) []Space {
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
	var ans []Space
	for name := range names {
		if !inSpace[name] {
			ans = append(ans, Space{Name: name, Worktrees: []Worktree{{Session: name}}})
		}
	}
	slices.SortFunc(ans, func(a, b Space) int { return strings.Compare(a.Name, b.Name) })
	return ans
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
	// Sessions that belong to no space, see otherSessions.
	Others []Space
	// Roots of the spaces whose linked worktrees are hidden.
	Folded map[string]bool
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
