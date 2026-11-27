package spaces

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/chancez/kspaces/internal/config"
	"github.com/chancez/kspaces/internal/git"
	"github.com/chancez/kspaces/internal/gitstate"
	"github.com/chancez/kspaces/internal/logs"
)

// HostData is what a host's server holds of the host, see docs/server.md: its config, or why it could not
// be read, its spaces and their worktrees, those being created or removed, and what git and gh say of each.
type HostData struct {
	Config  *config.Config            `json:"config,omitempty"`
	Err     string                    `json:"err,omitempty"`
	Spaces  []sharedSpace             `json:"spaces,omitempty"`
	Pending []Pending                 `json:"pending,omitempty"`
	Git     map[string]gitstate.State `json:"git,omitempty"`
	// AdHoc are the host's ad hoc spaces, see AdHocSpace.
	AdHoc []AdHocSpace `json:"ad_hoc,omitempty"`
}

// Host is where this host's data comes from: its server, or LocalHost in the server itself.
type Host interface {
	// Load is the host data, loaded now when fresh, else the latest.
	Load(fresh bool) (HostData, error)
	// Refresh has git and gh asked again about every worktree at the next load.
	Refresh() error
	// Subscribe calls f with the host data whenever a load changes it, until stop.
	Subscribe(f func(HostData)) (stop func())
	// NewWorktree creates a worktree in the space at root on branch, from base when not empty, see
	// CreateOnHost, and is it.
	NewWorktree(root, branch, base string) (Worktree, error)
	// RemoveWorktree removes the worktree at path, see RemoveOnHost.
	RemoveWorktree(path string, opts RemoveOptions) error
	// LoadLayouts are the layouts of the host's copies known names at another revision than it gives, see
	// CopyLayout.
	LoadLayouts(known []CopyLayout) ([]CopyLayout, error)
	// SaveLayout saves l as the layout of the copy key names, if it is still at revision, and is its new
	// revision; ErrLayoutConflict when it is not.
	SaveLayout(key string, revision int64, l SharedLayout) (int64, error)
	// NewSpace makes an ad hoc space on the host, see AdHocSpace.
	NewSpace(name, dir string) error
	// RemoveSpace removes the host's ad hoc space name, ending the provider's sessions in it there.
	RemoveSpace(name string) error
}

var (
	hostMu   sync.Mutex
	thisHost Host
)

// UseHost makes h the host data's source for this process, as the command line does with the server.
func UseHost(h Host) {
	hostMu.Lock()
	defer hostMu.Unlock()
	thisHost = h
}

// ThisHost is the host data's source UseHost set, else a LocalHost of the process's own that leaves git
// and gh alone, as in tests.
func ThisHost() Host {
	hostMu.Lock()
	defer hostMu.Unlock()
	if thisHost == nil {
		thisHost = &LocalHost{load: loadHost}
	}
	return thisHost
}

// LocalHost loads the host data in this process, asking git and gh in the background, see
// gitstate.Watcher. It writes their states to a file, which a LocalHost started again carries on from, so
// gh is not asked about every branch again at once. It is safe to call from several goroutines.
type LocalHost struct {
	// mu is held through a load.
	mu sync.Mutex
	// path is the git states' file, "" for none.
	path string
	// load is the host data but git's states, read now.
	load func() HostData
	// newWatcher makes the watcher of the worktrees' git states, nil for none.
	newWatcher func() *gitstate.Watcher
	git        *gitstate.Watcher
	// written is what was last written, so unchanged states are not written again.
	written []byte
	last    HostData
	have    bool
	// sent is the last host data the subscribers were called with, as JSON.
	sent   []byte
	subsMu sync.Mutex
	subs   map[int]func(HostData)
	nextID int
	// layouts are the layouts of the host's copies it keeps, and adHoc its ad hoc spaces.
	layouts layoutStore
	adHoc   adHocStore
}

// NewLocalHost is the host of the machine kspaces runs on, keeping git's states in path, and in dir its
// copies' layouts and its ad hoc spaces; "" keeps either in memory alone.
func NewLocalHost(path, dir string) *LocalHost {
	h := &LocalHost{path: path, load: loadHost, newWatcher: gitstate.NewWatcher}
	if dir != "" {
		h.layouts.path, h.adHoc.path = filepath.Join(dir, "layouts.json"), filepath.Join(dir, "spaces.json")
	}
	return h
}

// loadHost reads the config, every space's worktrees and own file, and the worktrees being created or
// removed. Each read of a space's file is a stat while it has not changed, see statCache.
func loadHost() HostData {
	cfg, err := config.Load()
	if err != nil {
		return HostData{Err: err.Error()}
	}
	// Each load, so the host follows an edit.
	git.SetTimeouts(cfg.Git.Timeout, cfg.Git.FetchTimeout)
	hd := HostData{Config: &cfg}
	for _, root := range cfg.Spaces {
		s := ReadSpace(cfg, root)
		ss := sharedSpace{Name: s.Name, Root: s.Root, Worktrees: s.Worktrees, Config: s.Config, Untrusted: s.Untrusted}
		if s.Err != nil {
			ss.Err = s.Err.Error()
		}
		if s.ConfigErr != nil {
			ss.ConfigErr = s.ConfigErr.Error()
		}
		hd.Spaces = append(hd.Spaces, ss)
	}
	hd.Pending = pendingList()
	return hd
}

// Load loads, unless it is not fresh and has loaded before, and asks git and gh about what is due.
func (h *LocalHost) Load(fresh bool) (HostData, error) {
	h.mu.Lock()
	if h.have && !fresh {
		defer h.mu.Unlock()
		return h.last, nil
	}
	start := time.Now()
	hd := h.load()
	hd.AdHoc = h.adHoc.list()
	if w := h.watcher(); w != nil {
		hd.Git = w.Update(hostTargets(hd))
		h.write(hd.Git)
	}
	logs.Took("load", start)
	h.last, h.have = hd, true
	h.mu.Unlock()
	h.publish(hd)
	return hd, nil
}

// Refresh has git and gh asked again at the next load.
func (h *LocalHost) Refresh() error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if w := h.watcher(); w != nil {
		w.Refresh()
	}
	return nil
}

// NewWorktree creates the worktree on this host, see CreateOnHost, and loads.
func (h *LocalHost) NewWorktree(root, branch, base string) (Worktree, error) {
	cfg, err := config.Load()
	if err != nil {
		return Worktree{}, err
	}
	wt, err := CreateOnHost(cfg, root, branch, base)
	h.Load(true)
	return wt, err
}

// RemoveWorktree removes the worktree from this host, see RemoveOnHost, and loads.
func (h *LocalHost) RemoveWorktree(path string, opts RemoveOptions) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	err = RemoveOnHost(cfg, path, opts)
	h.Load(true)
	return err
}

// LoadLayouts are the layouts of known kept here at another revision, see CopyLayout.
func (h *LocalHost) LoadLayouts(known []CopyLayout) ([]CopyLayout, error) {
	return h.layouts.changed(known), nil
}

// SaveLayout keeps l as the layout of key, if it is still at revision.
func (h *LocalHost) SaveLayout(key string, revision int64, l SharedLayout) (int64, error) {
	return h.layouts.save(key, revision, l)
}

// NewSpace keeps the ad hoc space name, starting in dir, and loads, so it shows at once.
func (h *LocalHost) NewSpace(name, dir string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if err := CheckAdHocName(cfg, name); err != nil {
		return err
	}
	if err := h.adHoc.add(AdHocSpace{Name: name, Dir: dir}); err != nil {
		return err
	}
	h.Load(true)
	return nil
}

// RemoveSpace ends the provider's sessions in the ad hoc space name here, has the provider remove its
// space, and forgets it, and loads.
func (h *LocalHost) RemoveSpace(name string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	err = endSpaceOnHost(cfg, name)
	if _, ferr := h.adHoc.remove(name); ferr != nil {
		err = errors.Join(err, ferr)
	}
	h.Load(true)
	return err
}

// Subscribe calls f with every load that changes the host data.
func (h *LocalHost) Subscribe(f func(HostData)) (stop func()) {
	h.subsMu.Lock()
	defer h.subsMu.Unlock()
	if h.subs == nil {
		h.subs = map[int]func(HostData){}
	}
	id := h.nextID
	h.nextID++
	h.subs[id] = f
	return func() {
		h.subsMu.Lock()
		defer h.subsMu.Unlock()
		delete(h.subs, id)
	}
}

// publish calls the subscribers with hd, unless it is what they were last called with.
func (h *LocalHost) publish(hd HostData) {
	data, err := json.Marshal(hd)
	if err != nil {
		return
	}
	h.subsMu.Lock()
	defer h.subsMu.Unlock()
	if bytes.Equal(data, h.sent) {
		return
	}
	h.sent = data
	for _, f := range h.subs {
		f(hd)
	}
}

// watcher is the git states' watcher, made at the first load, nil for none.
func (h *LocalHost) watcher() *gitstate.Watcher {
	if h.git == nil && h.newWatcher != nil {
		h.git = h.newWatcher()
		if prev, ok := readGit(h.path); ok {
			h.git.Seed(prev)
		}
	}
	return h.git
}

// hostTargets are the worktrees of the spaces in hd.
func hostTargets(hd HostData) []gitstate.Target {
	var ans []gitstate.Target
	for _, s := range hd.Spaces {
		for _, wt := range s.Worktrees {
			ans = append(ans, gitstate.Target{Root: s.Root, Path: wt.Path, Branch: wt.Branch})
		}
	}
	return ans
}

// write replaces the file with states, unless it already holds them. A rename, so a reader sees all of
// one load's or all of the next's.
func (h *LocalHost) write(states map[string]gitstate.State) {
	if h.path == "" {
		return
	}
	data, err := json.Marshal(states)
	if err != nil {
		return
	}
	if bytes.Equal(data, h.written) {
		if _, err := os.Stat(h.path); err == nil {
			return
		}
	}
	if err := os.MkdirAll(filepath.Dir(h.path), 0o700); err != nil {
		return
	}
	tmp := fmt.Sprintf("%s.%d.tmp", h.path, os.Getpid())
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return
	}
	if err := os.Rename(tmp, h.path); err != nil {
		os.Remove(tmp)
		return
	}
	h.written = data
}

func readGit(path string) (map[string]gitstate.State, bool) {
	if path == "" {
		return nil, false
	}
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
