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
	// A row of a space's provider sessions no window shows, which has no Space either, see WithDetached.
	Detached *DetachedRow
	// A row of the hosts tab, see HostView: Host is the host whose part it is in, empty for this machine,
	// and HostHeader the host's own row, which heads that part and is nothing else.
	ByHost     bool
	Host       string
	HostHeader bool
}

// DetachedRow is a row of a space's provider sessions no window shows: their group's head, standing for them all, a
// host's heading its sessions when they run on several, or one session.
type DetachedRow struct {
	// Of is the space the sessions are in, as its row has it, and Worktree the session of the worktree
	// they are in, whose row they follow, see detachedUnder.
	Of       *Space
	Worktree string
	// On the head: all of them, and whether they show under it.
	All      Detached
	Expanded bool
	// On a host's row, and on each of its sessions': the host, empty for this machine. HostRow is the
	// host's own row.
	Host    string
	HostRow bool
	// On a session's row: the session.
	Session *ProviderSession
}

// Head reports whether r is the head of a space's detached sessions.
func (r DetachedRow) Head() bool {
	return !r.HostRow && r.Session == nil
}

// Group is a group of spaces, as its header has it: its name, and its spaces in the rows' order.
type Group struct {
	Name   string
	Spaces []*Space
}

// Blank reports whether r is the blank line between spaces, which is no entry.
func (r Row) Blank() bool {
	return r.Space == nil && r.Group == nil && r.Detached == nil && !r.HostHeader
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

// WithDetached is rows with the provider's sessions no window shows, detached by space, each after the row
// of the worktree it is in, see detachedUnder: a head row standing for them, then, for a worktree in
// expanded, by its session's name, a row for each, under a row for its host when they run on several. A
// folded space's are left out with its worktrees, as are a folded group's, whose spaces have no rows. The
// rows point into copies of the sessions.
func WithDetached(rows []Row, detached map[string]Detached, expanded map[string]bool) []Row {
	if len(detached) == 0 {
		return rows
	}
	var out []Row
	var folded bool
	for _, r := range rows {
		out = append(out, r)
		if r.Space == nil {
			continue
		}
		if r.Main {
			folded = r.Folded
		}
		d, ok := detached[r.Space.Name]
		if folded || !ok {
			continue
		}
		wt := mainSession(*r.Space)
		if !r.Main {
			wt = r.Worktree.Session
		}
		d = detachedUnder(*r.Space, d)[wt]
		if d.Count() == 0 {
			continue
		}
		open := expanded[wt]
		out = append(out, Row{Detached: &DetachedRow{Of: r.Space, Worktree: wt, All: d, Expanded: open}})
		if !open {
			continue
		}
		for _, h := range d.Hosts {
			if len(d.Hosts) > 1 {
				out = append(out, Row{Detached: &DetachedRow{Of: r.Space, Worktree: wt, Host: h.Host, HostRow: true}})
			}
			for _, sess := range slices.Clone(h.Sessions) {
				out = append(out, Row{Detached: &DetachedRow{Of: r.Space, Worktree: wt, Host: h.Host, Session: &sess}})
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

// detachedUnder splits d, space's detached sessions, by the worktree each is in, by that worktree's
// session, see homeOf, else the main one's.
func detachedUnder(space Space, d Detached) map[string]Detached {
	out := map[string]Detached{}
	for _, h := range d.Hosts {
		by := map[string][]ProviderSession{}
		var order []string
		for _, s := range h.Sessions {
			wt := mainSession(space)
			if home, ok := homeOf(space, s); ok {
				wt = home.Session
			}
			if by[wt] == nil {
				order = append(order, wt)
			}
			by[wt] = append(by[wt], s)
		}
		for _, wt := range order {
			o := out[wt]
			o.Hosts = append(o.Hosts, HostSessions{Host: h.Host, Sessions: by[wt]})
			out[wt] = o
		}
	}
	return out
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
