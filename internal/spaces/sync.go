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

// Sharing a worktree's layout, see shared.go, is a pass at each poll: it captures the shared panes of every
// worktree with windows, takes the layouts other machines saved since, and saves this machine's changes.
// One pass runs at a time in a kitty, under a lock, whichever of its sidebars gets it, and what it knows
// is in a file all of them read, so a second sidebar neither opens a tab the first opened nor saves over
// what the first took.

// loadEvery is how often a pass asks the provider for layouts changed since, when it knows each it shows,
// and passEvery how often a pass runs at all, whichever of a kitty's sidebars polls, so several do not each
// list kitty's tabs at every poll.
const (
	loadEvery = 4 * time.Second
	passEvery = 1500 * time.Millisecond
	// settleAfterTake is how long a worktree is not saved after taking another machine's layout opened or
	// closed its panes: a window just opened is not shown as its session's until the provider lists it, and
	// saving before would close it on the other machine.
	settleAfterTake = 10 * time.Second
)

// syncState is what passes know, kept between them in the kitty's layouts file.
type syncState struct {
	LastPass time.Time `json:"last_pass"`
	LastLoad time.Time `json:"last_load"`
	// By the worktree's kitty session.
	Layouts map[string]*syncEntry `json:"layouts"`
}

// syncEntry is what a pass knows of a worktree's layout: where it is, the layout last saved or taken, Base,
// at Revision, -1 until the provider has said, and Pending, the panes seen at the last pass, by Members,
// which a save waits to see twice so a change is saved once it settles.
type syncEntry struct {
	Ref      LayoutRef     `json:"ref"`
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

// layoutSync shares the layouts of a kitty's worktrees through the provider.
type layoutSync struct {
	path   string
	kitty  string
	writer string
	now    func() time.Time
	tabs   func() ([]kitty.FullTab, error)
	load   func(provider []string, known []StoredLayout) ([]StoredLayout, error)
	save   func(provider []string, ref LayoutRef, revision int64, l SharedLayout) (int64, error)
	open   func(provider []string, s ProviderSession) ([]string, string, error)
	k      sharedKitty
}

// newLayoutSync is the layout sharing of the kitty kspaces runs in, its file beside the git states'.
func newLayoutSync() *layoutSync {
	host, _ := os.Hostname()
	host, _, _ = strings.Cut(host, ".")
	return &layoutSync{
		path:   strings.TrimSuffix(gitPath(), ".git.json") + ".layouts.json",
		kitty:  kittyID(),
		writer: host,
		now:    time.Now,
		tabs:   kitty.ListTabs,
		load:   loadLayouts,
		save:   saveLayout,
		open:   openCommand,
		k:      liveShared{},
	}
}

// kittyID names the kitty kspaces runs in, as gitPath does, so a layout this kitty saved is told from one
// another kitty did, on this machine or another.
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
	st := syncState{Layouts: map[string]*syncEntry{}}
	if b, err := os.ReadFile(ls.path); err == nil {
		if err := json.Unmarshal(b, &st); err != nil || st.Layouts == nil {
			st = syncState{Layouts: map[string]*syncEntry{}}
		}
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

// sharedWorktree is a worktree with windows whose layout a pass shares: its kitty session, where its
// layout is, and its panes now.
type sharedWorktree struct {
	session string
	ref     LayoutRef
	layout  SharedLayout
	at      sharedLocal
}

// sharedWorktrees are the worktrees of tabs whose panes show a provider session of their space, in
// sessions, by their kitty session. A pane showing a session of another space is not shared, which leaves
// out kitty's startup session whatever its file is named, its windows showing sessions in default, and
// default itself is this machine's alone.
func sharedWorktrees(tabs []kitty.FullTab, sessions []ProviderSession) map[string]sharedWorktree {
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
	out := map[string]sharedWorktree{}
	for name := range names {
		space, dir, _ := strings.Cut(name, "@")
		if space == "" || space == defaultSpace {
			continue
		}
		mine := map[int]ProviderSession{}
		for _, tab := range tabs {
			for _, w := range tab.Windows {
				if s, ok := shows[w.ID]; ok && w.SessionName == name && s.SpaceName() == space {
					mine[w.ID] = s
				}
			}
		}
		host, ok := sharedHost(tabs, name, mine)
		if !ok {
			continue
		}
		l, at := captureShared(tabs, name, host, mine)
		out[name] = sharedWorktree{session: name, ref: LayoutRef{Space: space, Host: host, Key: dir}, layout: l, at: at}
	}
	return out
}

// pass shares the layouts of the worktrees with windows: it forgets those with none, so one opened again
// is seen afresh rather than against panes it no longer has; takes the layouts other machines saved since
// it last asked, or every one it has not asked about; and saves the worktrees whose panes changed and
// settled. cwd is where a session opened without a directory of its own starts, by the worktree's kitty
// session, and anchor the window a new tab of a session goes by.
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
		shown := sharedWorktrees(tabs, sessions)
		for name, e := range st.Layouts {
			if w, ok := shown[name]; !ok || w.ref != e.Ref {
				delete(st.Layouts, name)
			}
		}
		for name, w := range shown {
			if st.Layouts[name] == nil {
				st.Layouts[name] = &syncEntry{Ref: w.ref, Revision: -1}
			}
		}
		taken := ls.take(cfg, st, shown, sessions, cwd, anchor)
		for _, name := range slices.Sorted(maps.Keys(shown)) {
			if !taken[name] {
				ls.saveIfChanged(cfg, st.Layouts[name], shown[name])
			}
		}
	})
	return cmp.Or(lockErr, err)
}

// take asks for the layouts changed since, when one is unknown or loadEvery has passed, and takes each:
// one this kitty saved is the base as it is, another's is merged into the worktree's panes. It is which
// worktrees it changed, whose panes the pass captured before it did, so they are not saved until the
// next.
func (ls *layoutSync) take(cfg config.Config, st *syncState, shown map[string]sharedWorktree, sessions []ProviderSession, cwd func(string) string, anchor map[string]int) map[string]bool {
	unknown := slices.ContainsFunc(slices.Collect(maps.Values(st.Layouts)), func(e *syncEntry) bool { return e.Revision < 0 })
	if len(st.Layouts) == 0 || !unknown && ls.now().Sub(st.LastLoad) < loadEvery {
		return nil
	}
	var known []StoredLayout
	byRef := map[LayoutRef]string{}
	for _, name := range slices.Sorted(maps.Keys(st.Layouts)) {
		e := st.Layouts[name]
		known = append(known, StoredLayout{LayoutRef: e.Ref, Revision: e.Revision})
		byRef[e.Ref] = name
	}
	got, err := ls.load(cfg.Provider, known)
	if err != nil {
		logs.Error("loading the shared layouts", err)
		return nil
	}
	st.LastLoad = ls.now()
	taken := map[string]bool{}
	for _, s := range got {
		name, ok := byRef[s.LayoutRef]
		if !ok {
			continue
		}
		e := st.Layouts[name]
		switch {
		case s.Revision == 0 || s.Layout == nil:
			// Nothing saved: this machine's panes are saved as the first.
			e.Revision, e.Base = 0, nil
		case s.Layout.Kitty == ls.kitty:
			e.Revision, e.Base = s.Revision, s.Layout
		default:
			w := shown[name]
			change := mergeShared(e.Base, w.layout, *s.Layout)
			if !change.Empty() {
				if err := ls.apply(cfg, w, change, sessions, cwd(name), anchor[name]); err != nil {
					logs.Error(fmt.Sprintf("taking the layout of %s saved on %s", name, s.Layout.Writer), err)
				}
				taken[name] = true
				e.Hold = ls.now().Add(settleAfterTake)
			}
			e.Revision, e.Base, e.Pending = s.Revision, s.Layout, ""
		}
	}
	return taken
}

// saveIfChanged saves w's panes as e's layout once they differ from its base and are as the last pass saw
// them. A conflict leaves the revision unknown, so the next pass takes what was saved and saves again
// after it.
func (ls *layoutSync) saveIfChanged(cfg config.Config, e *syncEntry, w sharedWorktree) {
	if e.Revision < 0 || ls.now().Before(e.Hold) {
		e.Pending = ""
		return
	}
	members := w.layout.Members()
	if e.Base != nil && e.Base.Members() == members || e.Base == nil && len(w.layout.Tabs) == 0 {
		e.Pending = ""
		return
	}
	if e.Pending != members {
		e.Pending = members
		return
	}
	l := w.layout
	l.Writer, l.Kitty = ls.writer, ls.kitty
	rev, err := ls.save(cfg.Provider, e.Ref, e.Revision, l)
	switch {
	case errors.Is(err, errLayoutConflict):
		e.Revision = -1
	case err != nil:
		logs.Error(fmt.Sprintf("saving the layout of %s", w.session), err)
	default:
		e.Revision, e.Base, e.Pending = rev, &l, ""
	}
}

// apply opens and closes w's panes as change says: each session to open in a split of the pane showing the
// one it splits, which it may have opened just before, else in a new tab of w's kitty session, by anchor,
// with --keep-focus so the tab that has focus keeps it; each to close by its window. One that fails is
// logged and the rest still go.
func (ls *layoutSync) apply(cfg config.Config, w sharedWorktree, change sharedChange, sessions []ProviderSession, cwd string, anchor int) error {
	var errs []error
	windows := maps.Clone(w.at.Window)
	for _, id := range change.Close {
		if win, ok := windows[id]; ok {
			if err := ls.k.Close(win); err != nil {
				errs = append(errs, fmt.Errorf("closing %s: %w", id, err))
			}
			delete(windows, id)
		}
	}
	for _, add := range change.Open {
		s := ProviderSession{ID: add.Session, Name: add.Session}
		if i := slices.IndexFunc(sessions, func(p ProviderSession) bool { return p.ID == add.Session }); i >= 0 {
			s = sessions[i]
		}
		argv, dir, err := ls.open(cfg.Provider, s)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		dir = cmp.Or(dir, s.Cwd, cwd)
		if dir == "" {
			dir, _ = os.UserHomeDir()
		}
		at := slices.Concat([]string{kitty.Quote("--cwd=" + dir)}, inSession(w.session), []string{"--keep-focus"})
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
		id, err := ls.k.Launch(w.session, args)
		if err != nil {
			errs = append(errs, fmt.Errorf("opening %s: %w", s.Name, err))
			continue
		}
		windows[add.Session] = id
		if add.SplitOf == "" && add.Layout != "" {
			if err := ls.k.GotoLayout(id, add.Layout); err != nil {
				errs = append(errs, err)
			}
		}
	}
	return errors.Join(errs...)
}

// shareLayouts runs a pass with sh's provider and windows, when the provider shares layouts. A pass another
// sidebar is running is left to it.
func shareLayouts(ls *layoutSync, sh Shared, self int) {
	if sh.Config == nil || sh.Provider == nil || !sh.Provider.Caps[CapLayouts] {
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
