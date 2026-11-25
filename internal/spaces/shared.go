package spaces

import (
	"cmp"
	"encoding/json"
	"slices"
	"time"

	"github.com/chancez/kspaces/internal/config"
	"github.com/chancez/kspaces/internal/kitty"
)

// A worktree's tabs are shared by every machine showing it, through the provider, as a multiplexer's windows
// are by its attached clients: see Shared layouts in docs/kitty.md. What is shared is, for each host, the panes
// that show the provider's sessions on it, which is all a layout names, so a pane's place is the session it
// shows and nothing on one machine needs meaning on another.

// SharedLayout is a worktree's shared panes, as the provider stores them. Its JSON is the provider's
// contract: it rewrites Sessions and leaves the rest as it is.
type SharedLayout struct {
	Version int `json:"version"`
	// Writer is the machine that saved it, and Kitty the kitty there, see kittyID, which tells a layout
	// this kitty saved from one another kitty did.
	Writer string `json:"writer,omitempty"`
	Kitty  string `json:"kitty,omitempty"`
	// Saved is when it was saved, by the writer's clock.
	Saved time.Time `json:"saved,omitzero"`
	// Sessions are the provider's ids of the sessions the panes show, each pane naming one by index.
	Sessions []string    `json:"sessions"`
	Tabs     []SharedTab `json:"tabs"`
}

// SharedTab is a tab of a SharedLayout: its title, set when the tab's was, and its kitty layout, empty for
// splits, as a layout's tab has them.
type SharedTab struct {
	Title  string       `json:"title,omitempty"`
	Layout string       `json:"layout,omitempty"`
	Panes  []SharedPane `json:"panes"`
}

// SharedPane is a pane of a SharedTab: the session it shows, by its index in Sessions, and in splits the
// side, the pane it splits, from 1, and its size, as a layout's pane has them.
type SharedPane struct {
	Session int    `json:"session"`
	Split   string `json:"split,omitempty"`
	Of      int    `json:"of,omitempty"`
	Size    int    `json:"size,omitempty"`
}

// sharedVersion is the SharedLayout version kspaces writes and reads.
const sharedVersion = 1

// session is the id of the session pane p of l shows, empty for an index out of range.
func (l SharedLayout) session(p SharedPane) string {
	if p.Session < 0 || p.Session >= len(l.Sessions) {
		return ""
	}
	return l.Sessions[p.Session]
}

// ids are the sessions l's panes show, in order.
func (l SharedLayout) ids() []string {
	var out []string
	for _, t := range l.Tabs {
		for _, p := range t.Panes {
			if id := l.session(p); id != "" {
				out = append(out, id)
			}
		}
	}
	return out
}

// Shape is l with its sessions named in place and who saved it left out, so two layouts of the same panes
// are the same string whichever order their sessions are listed in.
func (l SharedLayout) Shape() string {
	type pane struct {
		Session string `json:"session"`
		Split   string `json:"split,omitempty"`
		Of      int    `json:"of,omitempty"`
		Size    int    `json:"size,omitempty"`
	}
	type tab struct {
		Title  string `json:"title,omitempty"`
		Layout string `json:"layout,omitempty"`
		Panes  []pane `json:"panes"`
	}
	tabs := []tab{}
	for _, t := range l.Tabs {
		out := tab{Title: t.Title, Layout: t.Layout, Panes: []pane{}}
		for _, p := range t.Panes {
			out.Panes = append(out.Panes, pane{Session: l.session(p), Split: p.Split, Of: p.Of, Size: p.Size})
		}
		tabs = append(tabs, out)
	}
	b, _ := json.Marshal(tabs)
	return string(b)
}

// sharedLocal is where a captured layout's sessions are in kitty: the window showing each, and its tab.
type sharedLocal struct {
	Window map[string]int
	Tab    map[string]int
}

// captureShared is the layout of session's panes that show a provider session on host, in tabs, by shows,
// the provider's sessions by the windows that show them, and where each is. A tab with none is left out, and
// in splits the tree is the one the panes kept would have, as a layout saved from the tab is.
func captureShared(tabs []kitty.FullTab, session, host string, shows map[int]ProviderSession) (SharedLayout, sharedLocal) {
	l := SharedLayout{Version: sharedVersion, Sessions: []string{}, Tabs: []SharedTab{}}
	at := sharedLocal{Window: map[string]int{}, Tab: map[string]int{}}
	index := map[string]int{}
	mine, _ := sessionTabs(tabs, session, 0)
	for _, tab := range mine {
		windows := map[int]kitty.FullWindow{}
		for _, w := range tab.Windows {
			windows[w.ID] = w
		}
		// A group's window is its first that is not kspaces' own, as captureTab has it.
		groups := map[int]kitty.FullWindow{}
		var order []int
		for _, g := range tab.LayoutState.AllWindows.WindowGroups {
			for _, id := range g.WindowIDs {
				w, ok := windows[id]
				if !ok || w.Own() {
					continue
				}
				if s, shown := shows[w.ID]; shown && s.Host == host && w.SessionName == session {
					groups[g.ID] = w
					order = append(order, g.ID)
				}
				break
			}
		}
		if len(order) == 0 {
			continue
		}
		var panes []config.Pane
		var ids []int
		out := SharedTab{}
		if tab.Layout == config.SplitsLayout && tab.LayoutState.Pairs != nil {
			root := prune(unmaximize(tab.LayoutState), func(g int) bool { _, ok := groups[g]; return ok })
			ids, panes = splitPanes(root)
		} else {
			if tab.Layout != config.SplitsLayout {
				out.Layout = tab.Layout
			}
			ids, panes = order, make([]config.Pane, len(order))
		}
		if tab.TitleOverridden {
			out.Title = tab.Title
		}
		for i, g := range ids {
			w := groups[g]
			id := shows[w.ID].ID
			n, seen := index[id]
			if !seen {
				n = len(l.Sessions)
				index[id] = n
				l.Sessions = append(l.Sessions, id)
			}
			out.Panes = append(out.Panes, SharedPane{Session: n, Split: panes[i].Split, Of: panes[i].Of, Size: panes[i].Size})
			at.Window[id], at.Tab[id] = w.ID, tab.ID
		}
		l.Tabs = append(l.Tabs, out)
	}
	return l, at
}

// sharedAdd is a session a merge opens: as a split of the pane showing SplitOf, which a pane the merge
// opens before it may be, else as the first pane of a new tab with the tab's title and layout.
type sharedAdd struct {
	Session string
	SplitOf string
	Split   string
	Size    int
	Title   string
	Layout  string
}

// sharedChange is what taking another machine's layout does to this one's: the sessions it opens, in the
// order they open, and those it closes.
type sharedChange struct {
	Open  []sharedAdd
	Close []string
}

// Empty reports whether c does nothing.
func (c sharedChange) Empty() bool {
	return len(c.Open) == 0 && len(c.Close) == 0
}

// mergeShared is what taking remote does to local, a three-way merge of their panes by session against
// base, the layout last saved or taken, nil for none: a session remote has and base did not opens, unless
// local has it; a session base had that remote does not closes, unless local had already closed it. A
// session local added or removed since base stays as local has it. Without a base nothing closes, as
// nothing says remote removed it rather than never had it, and everything remote has that local does not
// opens. A session opens as a split of the pane remote has it split from when that pane is open, or opens
// before it, else of the pane before it in its tab, else as a new tab.
func mergeShared(base *SharedLayout, local, remote SharedLayout) sharedChange {
	has := func(l SharedLayout) map[string]bool {
		out := map[string]bool{}
		for _, id := range l.ids() {
			out[id] = true
		}
		return out
	}
	inLocal, inRemote := has(local), has(remote)
	inBase := map[string]bool{}
	if base != nil {
		inBase = has(*base)
	}
	var c sharedChange
	for _, id := range local.ids() {
		if inBase[id] && !inRemote[id] {
			c.Close = append(c.Close, id)
		}
	}
	open := map[string]bool{}
	for id := range inLocal {
		open[id] = !slices.Contains(c.Close, id)
	}
	for _, t := range remote.Tabs {
		for i, p := range t.Panes {
			id := remote.session(p)
			if id == "" || inLocal[id] || inBase[id] {
				continue
			}
			add := sharedAdd{Session: id, Split: p.Split, Size: p.Size}
			if p.Of > 0 && p.Of <= len(t.Panes) && open[remote.session(t.Panes[p.Of-1])] {
				add.SplitOf = remote.session(t.Panes[p.Of-1])
			} else if prev := slices.IndexFunc(t.Panes[:i], func(q SharedPane) bool { return open[remote.session(q)] }); prev >= 0 {
				add.SplitOf = remote.session(t.Panes[prev])
				add.Split = cmp.Or(add.Split, "right")
			} else {
				add.Split, add.Size, add.Title, add.Layout = "", 0, t.Title, t.Layout
			}
			c.Open = append(c.Open, add)
			open[id] = true
		}
	}
	return c
}
