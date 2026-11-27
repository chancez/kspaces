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

// Row is one line of the sidebar's list. The zero row is the blank line between spaces.
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
	// On a space's own row: its linked worktrees are hidden. On a group's header: its spaces are.
	Folded bool
	// A group's header, which has no Space, heading its spaces' rows.
	Group *Group
	// Sessions are the provider's sessions no window shows that belong in the row's worktree, see homeOf,
	// all of its space's on a folded space's row: closed here, they still run, or are shelved to resume.
	// See WithSessions.
	Sessions []ProviderSession
	// A row of the hosts tab, see HostView: Host is the host whose part it is in, empty for this machine,
	// and HostHeader the host's own row, which heads that part and is nothing else.
	ByHost     bool
	Host       string
	HostHeader bool
}

// Group is a group of spaces, as its header has it: its name, and its spaces in the rows' order.
type Group struct {
	Name   string
	Spaces []*Space
}

// Blank reports whether r is the blank line between spaces, which is no entry.
func (r Row) Blank() bool {
	return r.Space == nil && r.Group == nil && !r.HostHeader
}

// Elsewhere reports whether r is a worktree only another host's sessions are tagged with, see
// Worktree.Host, with no directory known.
func (r Row) Elsewhere() bool {
	return r.Worktree != nil && r.Worktree.Host != "" && r.Worktree.Path == "" && !r.Other
}

// Remote reports whether r is another host's copy of a worktree, from that host's server, or of a space of
// its sessions in no space, see docs/stories.md: its directory is there, so nothing done in a directory
// here applies to it.
func (r Row) Remote() bool {
	return r.Worktree != nil && r.Worktree.Host != "" && (r.Worktree.Path != "" || r.Other)
}

// Absent reports whether r's worktree has no directory here: a missing one, or one only another host has.
// Nothing that works in the directory applies to it.
func (r Row) Absent() bool {
	return r.Missing || r.Elsewhere()
}

// Dir names a linked worktree's row: its directory, or for a missing one, or one only another host has, the
// directory its session is named after, as git has no path for it.
func (r Row) Dir() string {
	if r.Absent() {
		return strings.TrimPrefix(r.Worktree.Session, r.Space.Name+"@")
	}
	return filepath.Base(r.Worktree.Path)
}

// BuildRows lists the sessions that belong to no space first, then the spaces in no group, then each
// group's header followed by its spaces, each space followed by its linked worktrees and its missing ones,
// with a blank row between spaces and above each header, all in order's order. A space's own row opens
// its main worktree. The linked and missing worktrees of a space whose root is in folded are left out, as
// are the spaces of a group whose GroupKey is; the groups in hidden, by name, are left out whole. The
// rows point into copies of others and spaces.
func BuildRows(others, spaces []Space, folded, hidden map[string]bool, order Order) []Row {
	others, spaces = order.arrange(others, spaces)
	var rows []Row
	for i := range others {
		rows = append(rows, Row{Space: &others[i], Worktree: &others[i].Worktrees[0], Main: true, Other: true})
	}
	for _, g := range order.grouped(spaces) {
		if g.Name != "" {
			if hidden[g.Name] {
				continue
			}
			if len(rows) > 0 {
				rows = append(rows, Row{})
			}
			header := Row{Group: g, Folded: folded[GroupKey(g.Name)]}
			if rows = append(rows, header); header.Folded {
				continue
			}
		}
		for j, s := range g.Spaces {
			// A group's first space goes right under its header.
			if len(rows) > 0 && (j > 0 || g.Name == "") {
				rows = append(rows, Row{})
			}
			r := Row{Space: s, Main: true, Folded: folded[s.Root] && s.HasLinked()}
			if len(s.Worktrees) > 0 {
				r.Worktree = &s.Worktrees[0]
			}
			rows = append(rows, r)
			if r.Folded {
				continue
			}
			for k := 1; k < len(s.Worktrees); k++ {
				rows = append(rows, Row{Space: s, Worktree: &s.Worktrees[k]})
			}
			for k := range s.Missing {
				rows = append(rows, Row{Space: s, Worktree: &s.Missing[k], Missing: true})
			}
		}
	}
	return rows
}

// SessionCounts is how many of sessions, a row's Sessions, still run and how many are shelved to resume.
func SessionCounts(sessions []ProviderSession) (running, shelved int) {
	for _, s := range sessions {
		switch s.State {
		case "running":
			running++
		case "shelved":
			shelved++
		}
	}
	return running, shelved
}

// WithSessions is rows with each space's and worktree's Sessions, from detached, by space, see homeOf: a
// folded space's row has all of its space's, as it stands for its worktrees.
func WithSessions(rows []Row, detached map[string]Detached) []Row {
	if len(detached) == 0 {
		return rows
	}
	out := slices.Clone(rows)
	for i, r := range out {
		d, ok := Detached{}, false
		if r.Space != nil {
			d, ok = detached[r.Space.Name]
		}
		if !ok {
			continue
		}
		wt := mainSession(*r.Space)
		if !r.Main && r.Worktree != nil {
			wt = r.Worktree.Session
		}
		for _, h := range d.Hosts {
			for _, s := range h.Sessions {
				home := mainSession(*r.Space)
				if w, ok := homeOf(*r.Space, s); ok {
					home = w.Session
				}
				if home == wt || r.Main && r.Folded {
					out[i].Sessions = append(out[i].Sessions, s)
				}
			}
		}
	}
	return out
}

// mainSession is the session of space's main worktree, empty for a space whose worktrees could not be
// listed.
func mainSession(space Space) string {
	if len(space.Worktrees) == 0 {
		return ""
	}
	return space.Worktrees[0].Session
}

// placeSessions sorts the sessions that are open or have a saved file but belong to no worktree, and the
// sessions of topics, the provider's spaces that are no configured space's, whether or not they are either. One
// named SPACE@DIR after a space whose worktrees were listed is a missing worktree of that space, added to
// its Missing in name order: its directory is gone, as when a worktree is removed while its session has
// windows. The rest are returned as the others, such as the one kitty starts with, each a space of its
// own with one worktree that has no path, so one created afresh starts in the home directory.
func placeSessions(spaces []Space, open map[string]int, saved []string, topics []string) (others []Space) {
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
	for _, name := range slices.Concat(saved, topics) {
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
	// Roots of the spaces whose linked worktrees are hidden, and the GroupKey of each group whose spaces
	// are.
	Folded map[string]bool
	// The groups left out of the sidebar, by name, see LoadHidden.
	Hidden map[string]bool
	// How the rows are ordered: the sort mode and the worktrees' manual order, saved as the folded spaces
	// are, with kitty's focus times and statuses.
	Order Order
	// Sessions with a file in the sessions directory, to be restored from.
	Saved map[string]bool
	// Saved sessions that were suspended, see KeepSuspended.
	Suspended map[string]bool
	// Sessions the provider is keeping, with the mode each is listed as, see KeepClosing.
	Keeping map[string]KeepMode
	Kitty   kitty.State
	// What git and gh say of each worktree, by GitKey.
	Git map[string]gitstate.State
	// The other hosts the config names, by name, see RemoteView.
	Remotes map[string]RemoteView
	// This machine's ad hoc spaces, see AdHocSpace.
	AdHoc []AdHocSpace
	// The worktrees being created or removed, by path, see placePending.
	Pending map[string]PendingOp
	// The provider's sessions no kitty window shows, by space, which is a configured space's name or a
	// session's in no space; nil without a provider that lists them. Only the sidebar asks.
	Detached map[string]Detached
	// Every session the provider lists, those windows show too, for the hosts tab, see HostRows.
	Sessions []ProviderSession
	// What the session provider does, see Capabilities; nil without one.
	Caps Capabilities
	// The provider's spaces, nil without a provider that lists them. Only the sidebar asks.
	ProviderSpaces []string
	Err            error
}

// Load reads the config on every call, so edits show up without restarting every dock, and a broken
// config is returned as the snapshot's error rather than ending the sidebar. kspaces_watcher.py would
// otherwise keep replacing a dock that exits the moment it starts. kitty's state is seen from the window
// self, or kitty.FocusedWindow.
func Load(self int) Snapshot {
	return LoadShared().View(self)
}
