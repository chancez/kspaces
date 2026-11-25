package spaces

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"time"

	"github.com/chancez/kspaces/internal/config"
	"github.com/chancez/kspaces/internal/git"
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
	// kitty's windows, as `kitten @ ls` lists them, and why they could not be listed.
	Windows  []kitty.OSWindow `json:"windows,omitempty"`
	KittyErr string           `json:"kitty_err,omitempty"`
	// What git and gh say of each worktree, by path, which only the sidebar asks.
	Git map[string]gitstate.State `json:"git,omitempty"`
	// What the session provider says, which only the sidebar asks; nil without one, see Poller.sessions.
	Provider *ProviderState `json:"provider,omitempty"`
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

// LoadShared reads the config, every space's worktrees and own file, the saved sessions and kitty's
// windows. Each read of a space's file is a stat while it has not changed, see statCache.
func LoadShared() Shared {
	sh := loadSpaces()
	if sh.Config != nil {
		sh = listWindows(sh, kitty.ListWindows)
	}
	return sh
}

// loadSpaces is LoadShared but kitty's windows.
func loadSpaces() Shared {
	cfg, err := config.Load()
	if err != nil {
		return Shared{Err: err.Error()}
	}
	// Each poll, so the sidebar follows an edit.
	git.SetTimeouts(cfg.Git.Timeout, cfg.Git.FetchTimeout)
	sh := Shared{Config: &cfg}
	for _, root := range cfg.Spaces {
		s := ReadSpace(cfg, root)
		ss := sharedSpace{Name: s.Name, Root: s.Root, Worktrees: s.Worktrees, Config: s.Config, Untrusted: s.Untrusted}
		if s.Err != nil {
			ss.Err = s.Err.Error()
		}
		if s.ConfigErr != nil {
			ss.ConfigErr = s.ConfigErr.Error()
		}
		sh.Spaces = append(sh.Spaces, ss)
	}
	sh.Saved = savedSessions(cfg.SessionsDir)
	sh.Suspended = suspendedSessions(cfg.SessionsDir)
	sh.Keeping = keepingSessions(cfg.SessionsDir)
	sh.Kept = slices.Sorted(maps.Keys(keptHandles(cfg.SessionsDir)))
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
	snap.Detached = detachedBySpace(sh.Provider, toSet(sh.Kept))
	if sh.Provider != nil {
		snap.Caps, snap.ProviderSpaces = sh.Provider.Caps, sh.Provider.Spaces
	}
	return snap
}

// Poller loads for the sidebar, with what git and gh say of each worktree, and writes those to a file for
// what does not poll itself, such as the picker, see GitStates. It is safe to call from several goroutines.
type Poller struct {
	// mu is held through a poll.
	mu sync.Mutex
	// path is the git states' file.
	path string
	// load is all of a poll's Shared but kitty's windows, which list lists after it, nil to keep load's.
	load func() Shared
	list func() ([]kitty.OSWindow, error)
	// lastMu is held while kitty's windows are listed, for last, the latest poll with the latest windows,
	// see Windows.
	lastMu sync.Mutex
	last   Shared
	have   bool
	// newWatcher makes the watcher of the worktrees' git states, nil for none.
	newWatcher func() *gitstate.Watcher
	git        *gitstate.Watcher
	// written is what was last written, so unchanged states are not written again.
	written []byte
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
}

// NewPoller is the poller of the kitty kspaces runs in.
func NewPoller() *Poller {
	p := newPoller(gitPath(), loadSpaces)
	p.list = kitty.ListWindows
	p.newWatcher = gitstate.NewWatcher
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
	return p
}

func newPoller(path string, load func() Shared) *Poller {
	return &Poller{path: path, load: load}
}

// gitPath is the git states' file of the kitty kspaces runs in, in the user's runtime directory, else a
// directory of the user's own in the temporary one, named after the kitty's remote control socket, which
// is what `kitten @` talks to, else its process.
func gitPath() string {
	dir := os.Getenv("XDG_RUNTIME_DIR")
	if dir != "" {
		dir = filepath.Join(dir, "kspaces")
	} else {
		dir = filepath.Join(os.TempDir(), fmt.Sprintf("kspaces-%d", os.Getuid()))
	}
	key := os.Getenv("KITTY_LISTEN_ON")
	if key == "" {
		key = "pid:" + os.Getenv("KITTY_PID")
	}
	sum := sha256.Sum256([]byte(key))
	return filepath.Join(dir, hex.EncodeToString(sum[:8])+".git.json")
}

// Poll loads, asks git and gh about what is due, and writes their states. kitty's windows are listed last,
// so they are as recent as they can be.
func (p *Poller) Poll() Shared {
	p.mu.Lock()
	defer p.mu.Unlock()
	defer logs.Took("poll", time.Now())
	sh := p.load()
	if w := p.watcher(); w != nil {
		sh.Git = w.Update(targets(sh))
		p.write(sh.Git)
	}
	p.lastMu.Lock()
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
	}
	p.followDock(sh)
	return sh
}

// Windows is the latest poll with kitty's windows listed again: a `kitten @ ls` alone, for when only they
// have changed, as when focus moves to another session. Before the first poll it polls.
func (p *Poller) Windows() Shared {
	p.lastMu.Lock()
	if !p.have {
		p.lastMu.Unlock()
		return p.Poll()
	}
	defer p.lastMu.Unlock()
	return p.listed(p.last)
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

// watcher is the git states' watcher, made at the first poll, nil for none.
func (p *Poller) watcher() *gitstate.Watcher {
	if p.git == nil && p.newWatcher != nil {
		p.git = p.newWatcher()
		// Carry on from the last sidebar, so gh is not asked about every branch again at once.
		if prev, ok := readGit(p.path); ok {
			p.git.Seed(prev)
		}
	}
	return p.git
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

// targets are the worktrees of the spaces in sh.
func targets(sh Shared) []gitstate.Target {
	var ans []gitstate.Target
	for _, s := range sh.Spaces {
		for _, wt := range s.Worktrees {
			ans = append(ans, gitstate.Target{Root: s.Root, Path: wt.Path, Branch: wt.Branch})
		}
	}
	return ans
}

// Refresh loads at once, and has git and gh asked again too.
func (p *Poller) Refresh() Shared {
	p.mu.Lock()
	if w := p.watcher(); w != nil {
		w.Refresh()
	}
	p.listAfter, p.listFailures, p.caps = time.Time{}, 0, nil
	p.mu.Unlock()
	return p.Poll()
}

// GitStates is the sidebar's latest git states, for what does not poll itself, such as the picker.
func GitStates() map[string]gitstate.State {
	states, _ := readGit(gitPath())
	return states
}

// write replaces the file with states, unless it already holds them. A rename, so a reader sees all of
// one poll's or all of the next's.
func (p *Poller) write(states map[string]gitstate.State) {
	data, err := json.Marshal(states)
	if err != nil {
		return
	}
	if bytes.Equal(data, p.written) {
		if _, err := os.Stat(p.path); err == nil {
			return
		}
	}
	if err := os.MkdirAll(filepath.Dir(p.path), 0o700); err != nil {
		return
	}
	tmp := fmt.Sprintf("%s.%d.tmp", p.path, os.Getpid())
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return
	}
	if err := os.Rename(tmp, p.path); err != nil {
		os.Remove(tmp)
		return
	}
	p.written = data
}

func readGit(path string) (map[string]gitstate.State, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, false
	}
	var states map[string]gitstate.State
	if err := json.Unmarshal(data, &states); err != nil {
		return nil, false
	}
	return states, true
}
