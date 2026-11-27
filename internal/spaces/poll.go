package spaces

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"time"

	"github.com/chancez/kspaces/internal/config"
	"github.com/chancez/kspaces/internal/gitstate"
	"github.com/chancez/kspaces/internal/kitty"
	"github.com/chancez/kspaces/internal/logs"
	"github.com/chancez/kspaces/internal/state"
)

// Shared is what one load sees that is the same from every window of a kitty: all of a Snapshot but what
// depends on the window it is seen from, which View works out. Errors are kept as their text.
type Shared struct {
	// Nil when the config could not be read, and Err says why.
	Config *config.Config `json:"config,omitempty"`
	Err    string         `json:"err,omitempty"`
	Spaces []sharedSpace  `json:"spaces,omitempty"`
	// Sessions with a file in the sessions directory.
	Saved []string `json:"saved,omitempty"`
	// Saved sessions kept with KeepSuspended.
	Suspended []string `json:"suspended,omitempty"`
	// Sessions the provider is keeping, with the mode each is listed as, see KeepClosing.
	Keeping map[string]KeepMode `json:"keeping,omitempty"`
	// What the provider kept of the kept sessions, see keptHandles.
	Kept []string `json:"kept,omitempty"`
	// The worktrees being created or removed, see MarkPending.
	Pending []Pending `json:"pending,omitempty"`
	// kitty's windows, as `kitten @ ls` lists them, and why they could not be listed.
	Windows  []kitty.OSWindow `json:"windows,omitempty"`
	KittyErr string           `json:"kitty_err,omitempty"`
	// What git and gh say of each worktree, by path, from the host data.
	Git map[string]gitstate.State `json:"git,omitempty"`
	// What the session provider says, which only the sidebar asks; nil without one, see Poller.sessions.
	Provider *ProviderState `json:"provider,omitempty"`
	// The worktrees' layouts diverged from their hosts', see layoutSync.
	Diverged []Divergence `json:"diverged,omitempty"`
}

// sharedSpace is a Space with its errors as text.
type sharedSpace struct {
	Name      string     `json:"name"`
	Root      string     `json:"root"`
	Worktrees []Worktree `json:"worktrees,omitempty"`
	Err       string     `json:"err,omitempty"`
	// The space's own config, see Space.Config.
	Config    *config.Config `json:"config,omitempty"`
	Untrusted bool           `json:"untrusted,omitempty"`
	ConfigErr string         `json:"config_err,omitempty"`
}

// LoadShared is this host's data, loaded now, with the session files and kitty's windows.
func LoadShared() Shared {
	sh := loadShared(ThisHost(), nil)
	if sh.Config != nil {
		sh = listWindows(sh, kitty.ListWindows)
	}
	return sh
}

// loadShared is all of LoadShared but kitty's windows: the host data pushed, or else h's loaded now, and
// the session files of the sessions directory it configures.
func loadShared(h Host, pushed *HostData) Shared {
	var hd HostData
	if pushed != nil {
		hd = *pushed
	} else {
		var err error
		if hd, err = h.Load(true); err != nil {
			return Shared{Err: err.Error()}
		}
	}
	sh := Shared{Config: hd.Config, Err: hd.Err, Spaces: hd.Spaces, Pending: hd.Pending, Git: hd.Git}
	if cfg := sh.Config; cfg != nil {
		sh.Saved = savedSessions(cfg.SessionsDir)
		sh.Suspended = suspendedSessions(cfg.SessionsDir)
		sh.Keeping = keepingSessions(cfg.SessionsDir)
		sh.Kept = slices.Sorted(maps.Keys(keptHandles(cfg.SessionsDir)))
	}
	return sh
}

// listWindows is sh with kitty's windows as list says now.
func listWindows(sh Shared, list func() ([]kitty.OSWindow, error)) Shared {
	windows, err := list()
	sh.Windows, sh.KittyErr = windows, ""
	if err != nil {
		sh.KittyErr = err.Error()
	}
	return sh
}

// View is the snapshot sh is from the window self, or kitty.FocusedWindow. The folded spaces and groups,
// the hidden groups, the sort mode and the worktrees' manual order are read here rather than shared, so a
// change shows in the sidebar that made it at once.
func (sh Shared) View(self int) Snapshot {
	if sh.Config == nil {
		return Snapshot{Err: errors.New(sh.Err)}
	}
	spaces := make([]Space, len(sh.Spaces))
	for i, s := range sh.Spaces {
		spaces[i] = Space{Name: s.Name, Root: s.Root, Worktrees: s.Worktrees, Group: sh.Config.Space[s.Name].Group, Config: s.Config, Untrusted: s.Untrusted}
		if s.Err != "" {
			spaces[i].Err = errors.New(s.Err)
		}
		if s.ConfigErr != "" {
			spaces[i].ConfigErr = errors.New(s.ConfigErr)
		}
	}
	st := kitty.StateOf(sh.Windows, self)
	topics := topicSpaces(sh.Provider, spaces)
	saved := make(map[string]bool, len(sh.Saved))
	for _, name := range sh.Saved {
		saved[name] = true
	}
	suspended := make(map[string]bool, len(sh.Suspended))
	for _, name := range sh.Suspended {
		suspended[name] = true
	}
	// The missing worktrees go in the spaces, which are this view's own copies: what the load holds, and
	// git is asked about in targets, is only what git lists.
	kept := state.Load().Sidebar
	snap := Snapshot{Config: sh.Config, Spaces: spaces, Others: placeSessions(spaces, st.Sessions, sh.Saved, topics), Folded: toSet(kept.Folded), Hidden: toSet(kept.Hidden), Saved: saved, Suspended: suspended, Keeping: sh.Keeping, Kitty: st}
	snap.Order = Order{Mode: sortMode(kept.Sort), Worktrees: worktreeOrder(kept.Order), Groups: sh.Config.Groups, Focused: st.Focused, Statuses: st.Statuses}
	if sh.KittyErr != "" {
		snap.Err = errors.New(sh.KittyErr)
	}
	snap.Git = sh.Git
	snap.Pending = placePending(snap.Spaces, sh.Pending)
	snap.Diverged = sh.Diverged
	snap.Detached = detachedBySpace(sh.Provider, toSet(sh.Kept))
	if sh.Provider != nil {
		snap.Caps, snap.ProviderSpaces, snap.Sessions = sh.Provider.Caps, sh.Provider.Spaces, sh.Provider.Sessions
	}
	return snap
}

// Poller loads for the sidebar: this host's data, from its server, and what is kitty's. It is safe to
// call from several goroutines.
type Poller struct {
	// mu is held through a poll.
	mu sync.Mutex
	// load is all of a poll's Shared but kitty's windows, which list lists after it, nil to keep load's:
	// with the host data pushed, as a subscription has it, else loaded now.
	load func(pushed *HostData) Shared
	// refresh has git and gh asked again at the next load, nil for nothing to ask.
	refresh func() error
	list    func() ([]kitty.OSWindow, error)
	// lastMu is held while kitty's windows are listed, for last, the latest poll with the latest windows,
	// see Windows.
	lastMu sync.Mutex
	last   Shared
	have   bool
	// capabilities asks the session provider what it does, once per sidebar and again at a refresh; caps is
	// what it said, nil until asked. listSessions is its list, nil for never. listed is the last list that
	// worked, which one that fails leaves showing, as a provider restarting under the sidebar should not
	// blank its rows; after listTries failures in a row it is not asked again until listAfter, so one that
	// cannot say does not fail into the log at every poll.
	capabilities func(provider []string) Capabilities
	caps         Capabilities
	listSessions func(provider []string, windows []ProviderWindow) (*ProviderState, error)
	listedLast   *ProviderState
	listFailures int
	listAfter    time.Time
	// saveWidth saves the width the sidebar's dock was dragged to from the config's width cfg, nil for
	// none; drag follows the dock for it.
	saveWidth func(width, cfg int) error
	drag      dockDrag
	// settle settles the sessions left listed as keeping, see settleKept, given the provider's sessions,
	// nil when it has not listed, and kitty's windows; nil for never.
	settle func(cfg config.Config, st *ProviderState, data []kitty.OSWindow)
	// follow moves the windows of sessions whose space changed, see followSpaces, given the poll; nil for
	// never.
	follow func(sh Shared)
	// share shares the worktrees' layouts through the provider, see layoutSync, given the poll; nil for
	// never.
	share func(sh Shared)
	// diverged are the layouts the last pass found diverged, see Divergences; nil for never.
	diverged func() []Divergence
}

// NewPoller is the poller of the kitty kspaces runs in.
func NewPoller() *Poller {
	h := ThisHost()
	p := newPoller(func(pushed *HostData) Shared { return loadShared(h, pushed) })
	p.refresh = h.Refresh
	p.list = kitty.ListWindows
	p.capabilities = ProviderCapabilities
	p.listSessions = listSessions
	p.saveWidth = saveDockWidth
	p.settle = func(cfg config.Config, st *ProviderState, data []kitty.OSWindow) {
		if err := settleKept(cfg, st, data, time.Now()); err != nil {
			logs.Error("settling the sessions left keeping", err)
		}
	}
	watch, self := &spaceWatch{}, kitty.SelfWindowID()
	p.follow = func(sh Shared) { followSpaces(watch, sh, self) }
	ls := newLayoutSync()
	p.share = func(sh Shared) { shareLayouts(ls, sh, self) }
	p.diverged = ls.divergences
	return p
}

func newPoller(load func(pushed *HostData) Shared) *Poller {
	return &Poller{load: load}
}

// kittyPath is the file of the kitty kspaces runs in, of what lasts as long as it does, in the runtime
// directory, named after the kitty's remote control socket, which is what `kitten @` talks to, else its
// process.
func kittyPath() string {
	key := os.Getenv("KITTY_LISTEN_ON")
	if key == "" {
		key = "pid:" + os.Getenv("KITTY_PID")
	}
	sum := sha256.Sum256([]byte(key))
	return filepath.Join(state.RuntimeDir(), hex.EncodeToString(sum[:8]))
}

// Poll loads, then lists kitty's windows, last, so they are as recent as they can be.
func (p *Poller) Poll() Shared {
	return p.poll(nil)
}

// Pushed polls with the host data a subscription pushed, rather than loading it again.
func (p *Poller) Pushed(hd HostData) Shared {
	return p.poll(&hd)
}

func (p *Poller) poll(pushed *HostData) Shared {
	p.mu.Lock()
	defer p.mu.Unlock()
	defer logs.Took("poll", time.Now())
	sh := p.load(pushed)
	p.lastMu.Lock()
	// The provider's last list until it lists again below, so windows listed in between, see Windows, do
	// not drop its sessions.
	sh.Provider = p.last.Provider
	sh = p.listed(sh)
	p.lastMu.Unlock()
	// After kitty's windows, which the provider says which of its sessions show.
	if sh.Config != nil {
		sh.Provider = p.sessions(sh.Config.Provider, sh.Windows)
		p.lastMu.Lock()
		p.last.Provider = sh.Provider
		p.lastMu.Unlock()
		if len(sh.Keeping) > 0 && p.settle != nil {
			var listed *ProviderState
			if p.listedLast != nil {
				listed = sh.Provider
			}
			p.settle(*sh.Config, listed, sh.Windows)
		}
		if p.follow != nil {
			p.follow(sh)
		}
		if p.share != nil {
			p.share(sh)
		}
		if p.diverged != nil {
			sh.Diverged = p.diverged()
			p.lastMu.Lock()
			p.last.Diverged = sh.Diverged
			p.lastMu.Unlock()
		}
	}
	p.followDock(sh)
	return sh
}

// Windows is the latest poll with kitty's windows listed again: a `kitten @ ls` alone, for when only they
// have changed, as when focus moves to another session, and the pending worktrees read again, as when a
// form that closed has started removing one. Before the first poll it polls.
func (p *Poller) Windows() Shared {
	p.lastMu.Lock()
	if !p.have {
		p.lastMu.Unlock()
		return p.Poll()
	}
	defer p.lastMu.Unlock()
	sh := p.last
	if sh.Config != nil {
		sh.Pending = pendingList()
	}
	return p.listed(sh)
}

// listed lists kitty's windows into sh, and keeps it as the latest. lastMu is held.
func (p *Poller) listed(sh Shared) Shared {
	if p.list != nil && sh.Config != nil {
		sh = listWindows(sh, p.list)
	}
	p.last, p.have = sh, true
	return sh
}

// listTries is how many lists in a row may fail before the poller waits listRetry to ask again.
const (
	listTries = 3
	listRetry = time.Minute
)

// sessions is what provider says now of its sessions, for windows: its capabilities, and its sessions and
// spaces when it lists them, or the last it listed when listing fails or waits, see listTries. Nil without
// a provider.
func (p *Poller) sessions(provider []string, data []kitty.OSWindow) *ProviderState {
	if len(provider) == 0 || p.capabilities == nil {
		return nil
	}
	if p.caps == nil {
		p.caps = p.capabilities(provider)
	}
	if !p.caps[CapList] || p.listSessions == nil {
		return &ProviderState{Caps: p.caps}
	}
	if !time.Now().Before(p.listAfter) {
		if st, err := p.listSessions(provider, allWindows(data)); err == nil {
			p.listedLast, p.listFailures = st, 0
		} else if p.listFailures++; p.listFailures >= listTries {
			p.listAfter, p.listFailures = time.Now().Add(listRetry), 0
		}
	}
	if p.listedLast == nil {
		return &ProviderState{Caps: p.caps}
	}
	st := *p.listedLast
	st.Caps = p.caps
	return &st
}

// followDock saves the width the sidebar dock was dragged to, see dockDrag.
func (p *Poller) followDock(sh Shared) {
	if p.saveWidth == nil {
		return
	}
	if sh.Config == nil || sh.KittyErr != "" {
		p.drag = dockDrag{}
		return
	}
	cfg := sh.Config.Sidebar.Width
	width := SidebarWidth(cfg, state.Load().Sidebar)
	if w := p.drag.observe(dockWidth(kitty.SidebarWidths(sh.Windows), width), width); w != 0 {
		if err := p.saveWidth(w, cfg); err != nil {
			logs.Error("saving the dragged sidebar width", err)
		}
	}
}

// Refresh loads at once, and has git and gh asked again too.
func (p *Poller) Refresh() Shared {
	p.mu.Lock()
	if p.refresh != nil {
		if err := p.refresh(); err != nil {
			logs.Error("refreshing git and gh", err)
		}
	}
	p.listAfter, p.listFailures, p.caps = time.Time{}, 0, nil
	p.mu.Unlock()
	return p.Poll()
}

// GitStates is the sidebar's latest git states, for what does not poll itself, such as the picker.
func GitStates() map[string]gitstate.State {
	states, _ := readGit(kittyPath() + ".git.json")
	return states
}
