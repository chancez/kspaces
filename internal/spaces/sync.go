package spaces

import (
	"cmp"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"time"

	"github.com/chancez/kspaces/internal/config"
	"github.com/chancez/kspaces/internal/kitty"
	"github.com/chancez/kspaces/internal/logs"
)

// Sharing a copy's layout, see shared.go, is a pass at each poll: it captures the panes of every copy with
// windows here, takes what other machines saved on the copy's host since, and saves this machine's changes
// there. The host is the layout's only writer, see docs/server.md, so there is nothing to reconcile: what
// was saved since the pass last took or saved is merged into the panes here, keeping what changed here
// meanwhile, and the result saved, at the revision taken, after it. A pass runs at a time in a kitty,
// under a lock, and what it knows is in a file beside the lock.

// loadEvery is how often a pass asks a host for its layouts saved since, when it knows each it shows, and
// passEvery how often a pass runs at all.
const (
	loadEvery = 4 * time.Second
	passEvery = 1500 * time.Millisecond
	// settleAfterTake is how long a copy is not saved after taking another machine's layout opened or
	// closed its panes, or after the pass first sees it: a window just opened is not shown as its session's
	// until the provider lists it, and saving before would close it on the other machine.
	settleAfterTake = 10 * time.Second
)

// syncState is what passes know, kept between them in the kitty's layouts file.
type syncState struct {
	LastPass time.Time `json:"last_pass"`
	// LastLoad is when each host was last asked for its layouts.
	LastLoad map[string]time.Time `json:"last_load"`
	// By the copy's kitty session.
	Layouts map[string]*syncEntry `json:"layouts"`
}

// syncEntry is what a pass knows of a copy with windows: its host, empty for this machine, and its kitty
// session there, Key; the layout last saved or taken, Base, at Revision, -1 until the host has said; and
// Pending, the panes seen at the last pass, by Members, which a save waits to see twice so a change is
// saved once it settles. Its sessions are as the host names them.
type syncEntry struct {
	Host     string        `json:"host,omitempty"`
	Key      string        `json:"key"`
	Revision int64         `json:"revision"`
	Base     *SharedLayout `json:"base,omitempty"`
	Pending  string        `json:"pending,omitempty"`
	// Hold is when a take that changed the panes settles, see settleAfterTake.
	Hold time.Time `json:"hold,omitzero"`
}

// Members is which sessions l's tabs show, a tab's in no order and the tabs in none: what a pass saves on a
// change of and applies, so a split moved or resized is no change, as taking a layout only opens and
// closes panes and two screens size their splits apart.
func (l SharedLayout) Members() string {
	var tabs []string
	for _, t := range l.Tabs {
		var ids []string
		for _, p := range t.Panes {
			if id := l.session(p); id != "" {
				ids = append(ids, id)
			}
		}
		slices.Sort(ids)
		tabs = append(tabs, strings.Join(ids, ","))
	}
	slices.Sort(tabs)
	return strings.Join(tabs, ";")
}

// sharedKitty is what a pass does to kitty's windows.
type sharedKitty interface {
	// Launch runs launch args, opening one window in session, and is its id.
	Launch(session string, args []string) (int, error)
	GotoLayout(id int, layout string) error
	// Close closes window id, marked kept so a watcher ending its session on close leaves it.
	Close(id int) error
}

// layoutSync shares the layouts of a kitty's copies through their hosts' servers.
type layoutSync struct {
	path   string
	kitty  string
	writer string
	now    func() time.Time
	tabs   func() ([]kitty.FullTab, error)
	// host is the server of the host a copy is of, "" for this machine, nil when there is none to ask.
	host func(name string) Host
	open func(provider []string, s ProviderSession) ([]string, string, error)
	k    sharedKitty
}

// newLayoutSync is the layout sharing of the kitty kspaces runs in, its file beside the kitty's others,
// asking host for each copy's host's server.
func newLayoutSync(host func(name string) Host) *layoutSync {
	name, _ := os.Hostname()
	name, _, _ = strings.Cut(name, ".")
	return &layoutSync{
		path:   kittyPath() + ".layouts.json",
		kitty:  kittyID(),
		writer: name,
		now:    time.Now,
		tabs:   kitty.ListTabs,
		host:   host,
		open:   openCommand,
		k:      liveShared{},
	}
}

// kittyID names the kitty kspaces runs in, as kittyPath does, so a layout this kitty saved is told from
// one another kitty did, on this machine or another.
func kittyID() string {
	host, _ := os.Hostname()
	key := cmp.Or(os.Getenv("KITTY_LISTEN_ON"), "pid:"+os.Getenv("KITTY_PID"))
	sum := sha256.Sum256([]byte(host + "\x00" + key))
	return hex.EncodeToString(sum[:8])
}

// liveShared is kitty itself.
type liveShared struct{}

func (liveShared) Launch(session string, args []string) (int, error) {
	return (&liveKitty{session: session}).Launch(args)
}

func (liveShared) GotoLayout(id int, layout string) error { return kitty.GotoLayout(id, layout) }

func (liveShared) Close(id int) error {
	if err := kitty.SetWindowVar(id, kitty.KeepVar, string(KeepClosed)); err != nil {
		return err
	}
	return kitty.CloseWindow(id)
}

// errBusy is a pass another sidebar of the kitty is running.
var errBusy = errors.New("another sidebar is sharing layouts")

// locked runs f under the layouts file's lock, with what the file holds, and writes back what f leaves.
// errBusy when another holds it.
func (ls *layoutSync) locked(f func(st *syncState)) error {
	if err := os.MkdirAll(filepath.Dir(ls.path), 0o700); err != nil {
		return err
	}
	lock, err := os.OpenFile(ls.path+".lock", os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return err
	}
	defer lock.Close()
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		if errors.Is(err, syscall.EWOULDBLOCK) {
			return errBusy
		}
		return err
	}
	st := syncState{}
	if b, err := os.ReadFile(ls.path); err == nil {
		if err := json.Unmarshal(b, &st); err != nil {
			st = syncState{}
		}
	}
	if st.Layouts == nil {
		st.Layouts = map[string]*syncEntry{}
	}
	if st.LastLoad == nil {
		st.LastLoad = map[string]time.Time{}
	}
	f(&st)
	b, err := json.Marshal(st)
	if err != nil {
		return err
	}
	tmp := ls.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, ls.path)
}

// sharedCopy is a copy with windows whose layout a pass shares: its host and key, see syncEntry, and its
// panes now, their sessions as the host names them.
type sharedCopy struct {
	host   string
	key    string
	layout SharedLayout
	// at is where each session's pane is here, by the session as the host names it.
	at sharedLocal
}

// onHost is id, a session as this machine's provider names it, as host names it: without the host before
// it, which the provider puts before another host's.
func onHost(host, id string) string {
	if host == "" {
		return id
	}
	return strings.TrimPrefix(id, host+":")
}

// fromHost is id, a session as host names it, as this machine's provider does.
func fromHost(host, id string) string {
	if host == "" {
		return id
	}
	return host + ":" + id
}

// sharedCopies are the copies with windows in tabs, by their kitty session, with the panes that show a
// session of the copy's host and space, of sessions, the provider's: a pane showing another's is not
// shared, see docs/stories.md. This machine's default is left out, which kitty's startup session's windows
// show; another host's default is a copy as any other. A copy of a host cfg does not name has nowhere to
// save, and is left out.
func sharedCopies(cfg config.Config, tabs []kitty.FullTab, sessions []ProviderSession) map[string]sharedCopy {
	shows := map[int]ProviderSession{}
	for _, s := range sessions {
		for _, id := range s.Windows {
			shows[id] = s
		}
	}
	names := map[string]bool{}
	for _, tab := range tabs {
		for _, w := range tab.Windows {
			if w.SessionName != "" && !w.Own() {
				names[w.SessionName] = true
			}
		}
	}
	out := map[string]sharedCopy{}
	for name := range names {
		host, key := SplitCopy(name)
		if _, ok := cfg.Hosts[host]; host != "" && !ok {
			continue
		}
		space, _, _ := strings.Cut(key, "@")
		if space == "" || host == "" && space == defaultSpace {
			continue
		}
		mine := map[int]ProviderSession{}
		for _, tab := range tabs {
			for _, w := range tab.Windows {
				if s, ok := shows[w.ID]; ok && w.SessionName == name && !w.Own() && s.Host == host && s.SpaceName() == space {
					mine[w.ID] = s
				}
			}
		}
		l, at := captureShared(tabs, name, host, mine)
		c := sharedCopy{host: host, key: key, layout: l, at: sharedLocal{Window: map[string]int{}, Tab: map[string]int{}}}
		for i, id := range l.Sessions {
			c.layout.Sessions[i] = onHost(host, id)
		}
		for id, w := range at.Window {
			c.at.Window[onHost(host, id)] = w
			c.at.Tab[onHost(host, id)] = at.Tab[id]
		}
		out[name] = c
	}
	return out
}

// pass shares the layouts of the copies with windows. It forgets a copy with none, so one opened again is
// seen afresh rather than against panes it no longer has. It takes the layouts other machines saved since
// it last asked, or every one it has not asked about, and saves those whose panes changed and settled.
// cwd is where a session opened without a directory of its own starts, by the copy's kitty session, and
// anchor the window a new tab of a session goes by.
func (ls *layoutSync) pass(cfg config.Config, sessions []ProviderSession, cwd func(session string) string, anchor map[string]int) error {
	var err error
	lockErr := ls.locked(func(st *syncState) {
		if now := ls.now(); now.Sub(st.LastPass) < passEvery {
			return
		} else {
			st.LastPass = now
		}
		var tabs []kitty.FullTab
		if tabs, err = ls.tabs(); err != nil {
			return
		}
		shown := sharedCopies(cfg, tabs, sessions)
		maps.DeleteFunc(st.Layouts, func(name string, _ *syncEntry) bool { _, ok := shown[name]; return !ok })
		for name, c := range shown {
			if st.Layouts[name] == nil {
				// Seen afresh, as at its opening: its panes are listed a few passes apart.
				st.Layouts[name] = &syncEntry{Host: c.host, Key: c.key, Revision: -1, Hold: ls.now().Add(settleAfterTake)}
			}
		}
		taken := ls.take(cfg, st, shown, sessions, cwd, anchor)
		for _, name := range slices.Sorted(maps.Keys(shown)) {
			if !taken[name] {
				ls.saveIfChanged(st.Layouts[name], name, shown[name])
			}
		}
	})
	return cmp.Or(lockErr, err)
}

// take asks each host for its layouts saved since, when one is unknown or loadEvery has passed, and takes
// each. One this kitty saved is the base as it is; another's is merged into the panes here against the
// base, see mergeShared, or opened alongside them without one. A host that does not answer is asked again
// at the next pass, its copies left as they are. It is which copies it changed, by kitty session, whose
// panes the pass captured before it did, so they are not saved until the next.
func (ls *layoutSync) take(cfg config.Config, st *syncState, shown map[string]sharedCopy, sessions []ProviderSession, cwd func(string) string, anchor map[string]int) map[string]bool {
	byHost := map[string]map[string]string{}
	for name, e := range st.Layouts {
		if ls.now().Before(e.Hold) {
			continue
		}
		if byHost[e.Host] == nil {
			byHost[e.Host] = map[string]string{}
		}
		byHost[e.Host][e.Key] = name
	}
	taken := map[string]bool{}
	for _, host := range slices.Sorted(maps.Keys(byHost)) {
		names := byHost[host]
		var known []CopyLayout
		unknown := false
		for _, key := range slices.Sorted(maps.Keys(names)) {
			e := st.Layouts[names[key]]
			known = append(known, CopyLayout{Key: key, Revision: e.Revision})
			unknown = unknown || e.Revision < 0
		}
		h := ls.host(host)
		if h == nil || !unknown && ls.now().Sub(st.LastLoad[host]) < loadEvery {
			continue
		}
		got, err := h.LoadLayouts(known)
		if err != nil {
			logs.Debug("loading the layouts", "host", host, "err", err)
			continue
		}
		st.LastLoad[host] = ls.now()
		for _, g := range got {
			name, ok := names[g.Key]
			if !ok {
				continue
			}
			e, c := st.Layouts[name], shown[name]
			var change sharedChange
			switch {
			case g.Revision == 0 || g.Layout == nil:
				// Nothing saved: this machine's panes are saved as the first.
				e.Base = nil
			case g.Layout.Kitty == ls.kitty:
				e.Base = g.Layout
			default:
				change = mergeShared(e.Base, c.layout, *g.Layout)
				e.Base = g.Layout
			}
			e.Revision, e.Pending = g.Revision, ""
			if !change.Empty() {
				if err := ls.apply(cfg, name, c, change, sessions, cwd(name), anchor[name]); err != nil {
					logs.Error(fmt.Sprintf("taking the layout of %s saved on %s", name, g.Layout.Writer), err)
				}
				taken[name] = true
				e.Hold = ls.now().Add(settleAfterTake)
			}
		}
	}
	return taken
}

// saveIfChanged saves c, the panes of the copy whose kitty session is name, as e's layout on its host
// once they differ from its base and are as the last pass saw them. A conflict leaves the revision
// unknown, so the next pass takes what was saved and saves again after it.
func (ls *layoutSync) saveIfChanged(e *syncEntry, name string, c sharedCopy) {
	if e.Revision < 0 || ls.now().Before(e.Hold) {
		e.Pending = ""
		return
	}
	members := c.layout.Members()
	if e.Base != nil && e.Base.Members() == members || e.Base == nil && len(c.layout.Tabs) == 0 {
		e.Pending = ""
		return
	}
	if e.Pending != members {
		e.Pending = members
		return
	}
	h := ls.host(e.Host)
	if h == nil {
		return
	}
	l := c.layout
	l.Writer, l.Kitty, l.Saved = ls.writer, ls.kitty, ls.now()
	rev, err := h.SaveLayout(e.Key, e.Revision, l)
	switch {
	case errors.Is(err, ErrLayoutConflict):
		e.Revision = -1
	case err != nil:
		logs.Error(fmt.Sprintf("saving the layout of %s on %s", name, cmp.Or(e.Host, "this machine")), err)
	default:
		e.Revision, e.Pending, e.Base = rev, "", &l
	}
}

// apply opens and closes c's panes, of kitty session session's, as change says, its sessions as c's host
// names them: each session to open in a split of the pane showing the one it splits, which it may have
// opened just before, else in a new tab of session, by anchor, with --keep-focus so the tab that has focus
// keeps it; each to close by its window. A pane of another host's copy starts here in the home directory,
// as the copy's directory is the host's. One that fails is logged and the rest still go.
func (ls *layoutSync) apply(cfg config.Config, session string, c sharedCopy, change sharedChange, sessions []ProviderSession, cwd string, anchor int) error {
	var errs []error
	windows := map[string]int{}
	maps.Copy(windows, c.at.Window)
	for _, id := range change.Close {
		if win, ok := windows[id]; ok {
			if err := ls.k.Close(win); err != nil {
				errs = append(errs, fmt.Errorf("closing %s: %w", id, err))
			}
			delete(windows, id)
		}
	}
	home, _ := os.UserHomeDir()
	for _, add := range change.Open {
		id := fromHost(c.host, add.Session)
		s := ProviderSession{ID: id, Name: id}
		if i := slices.IndexFunc(sessions, func(p ProviderSession) bool { return p.ID == id }); i >= 0 {
			s = sessions[i]
		}
		argv, dir, err := ls.open(cfg.Provider, s)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		dir = cmp.Or(dir, s.Cwd, cwd, home)
		if c.host != "" {
			dir = home
		}
		at := slices.Concat([]string{kitty.Quote("--cwd=" + dir)}, inSession(session), []string{"--keep-focus"})
		var args []string
		if of, ok := windows[add.SplitOf]; ok && add.SplitOf != "" {
			args = append(args, fmt.Sprintf("--match=id:%d", of))
			args = append(args, cfg.Launch...)
			args = append(args, "--type=window", "--location="+cmp.Or(locations[add.Split], "vsplit"), fmt.Sprintf("--next-to=id:%d", of))
			if add.Size > 0 {
				args = append(args, fmt.Sprintf("--bias=%d", add.Size))
			}
		} else {
			if anchor != 0 {
				args = append(args, fmt.Sprintf("--match=id:%d", anchor))
			}
			args = append(args, cfg.Launch...)
			args = append(args, "--type=tab")
			if add.Title != "" {
				args = append(args, kitty.Quote("--tab-title="+add.Title))
			}
		}
		args = append(args, at...)
		for _, a := range argv {
			args = append(args, kitty.Quote(a))
		}
		win, err := ls.k.Launch(session, args)
		if err != nil {
			errs = append(errs, fmt.Errorf("opening %s: %w", s.Name, err))
			continue
		}
		windows[add.Session] = win
		if add.SplitOf == "" && add.Layout != "" {
			if err := ls.k.GotoLayout(win, add.Layout); err != nil {
				errs = append(errs, err)
			}
		}
	}
	return errors.Join(errs...)
}

// shareLayouts runs a pass with sh's provider and windows, when the provider lists its sessions and opens
// them, which is what a layout names. A pass another sidebar is running is left to it.
func shareLayouts(ls *layoutSync, sh Shared, self int) {
	if sh.Config == nil || sh.Provider == nil || !sh.Provider.Caps[CapList] || !sh.Provider.Caps[CapOpen] {
		return
	}
	snap := sh.View(self)
	paths := map[string]string{}
	for _, s := range snap.Spaces {
		for _, wt := range s.Worktrees {
			paths[wt.Session] = wt.Path
		}
	}
	cwd := func(session string) string { return paths[session] }
	if err := ls.pass(*sh.Config, sh.Provider.Sessions, cwd, snap.Kitty.Sessions); err != nil && !errors.Is(err, errBusy) {
		logs.Error("sharing layouts", err)
	}
}
