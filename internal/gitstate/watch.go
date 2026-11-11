package gitstate

import (
	"maps"
	"sync"
	"time"
)

// How often a worktree's status is read again, and its branch's pull request looked up. gh goes over the
// network and counts against GitHub's rate limit, so much less often.
const (
	StatusEvery = 10 * time.Second
	PREvery     = 3 * time.Minute
)

// Target is a worktree to keep the state of.
type Target struct {
	Root   string
	Path   string
	Branch string
}

// Watcher keeps the state of worktrees, asking git and gh again when it is due, in the background, so
// that whoever asks it for the states never waits on either. Failures leave nothing known, as without gh,
// without a GitHub remote or offline.
type Watcher struct {
	mu     sync.Mutex
	states map[string]State
	// When each worktree's status was last read. Not shared: reading it is cheap, so a new leader reads
	// every status once.
	statusAt map[string]time.Time
	busy     bool

	readStatus func(dir string) (Status, error)
	lookupPR   func(root, branch string) (*PR, error)
	now        func() time.Time
	start      func(func())
}

// NewWatcher is a watcher that asks git and gh.
func NewWatcher() *Watcher {
	return WatcherOf(ReadStatus, LookupPR, time.Now, func(f func()) { go f() })
}

// WatcherOf is a watcher that asks readStatus and lookupPR, on the clock now, and starts each pass with
// start, for tests to run them at once.
func WatcherOf(readStatus func(string) (Status, error), lookupPR func(string, string) (*PR, error), now func() time.Time, start func(func())) *Watcher {
	return &Watcher{states: map[string]State{}, statusAt: map[string]time.Time{}, readStatus: readStatus, lookupPR: lookupPR, now: now, start: start}
}

// Seed starts from states another watcher had, so pull requests are not all looked up again at once.
func (w *Watcher) Seed(states map[string]State) {
	w.mu.Lock()
	defer w.mu.Unlock()
	maps.Copy(w.states, states)
}

// Refresh makes every worktree due, for the next Update.
func (w *Watcher) Refresh() {
	w.mu.Lock()
	defer w.mu.Unlock()
	clear(w.statusAt)
	for path, st := range w.states {
		st.PRAt = time.Time{}
		w.states[path] = st
	}
}

// job is what is due for a worktree.
type job struct {
	Target
	status, pr bool
}

// Update says which worktrees there are now, starts asking about those due unless a pass is already under
// way, and returns what is known of them, by path.
func (w *Watcher) Update(targets []Target) map[string]State {
	w.mu.Lock()
	now := w.now()
	paths := make(map[string]bool, len(targets))
	var due []job
	for _, t := range targets {
		paths[t.Path] = true
		st := w.states[t.Path]
		j := job{Target: t, status: now.Sub(w.statusAt[t.Path]) >= StatusEvery, pr: st.PRBranch != t.Branch || now.Sub(st.PRAt) >= PREvery}
		if j.status || j.pr {
			due = append(due, j)
		}
	}
	// Worktrees that went: removed, or their space with them.
	maps.DeleteFunc(w.states, func(path string, _ State) bool { return !paths[path] })
	maps.DeleteFunc(w.statusAt, func(path string, _ time.Time) bool { return !paths[path] })
	start := len(due) > 0 && !w.busy
	w.busy = w.busy || start
	w.mu.Unlock()
	if start {
		w.start(func() { w.run(due) })
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	return maps.Clone(w.states)
}

// run asks about what is due: every status first, as gh is slow.
func (w *Watcher) run(due []job) {
	for _, j := range due {
		if !j.status {
			continue
		}
		s, err := w.readStatus(j.Path)
		w.mu.Lock()
		st := w.states[j.Path]
		st.Status = nil
		if err == nil {
			st.Status = &s
		}
		w.states[j.Path], w.statusAt[j.Path] = st, w.now()
		w.mu.Unlock()
	}
	for _, j := range due {
		if !j.pr {
			continue
		}
		pr, err := w.lookupPR(j.Root, j.Branch)
		w.mu.Lock()
		st := w.states[j.Path]
		// A failure keeps what was known of the same branch: gh offline for a moment should not hide it.
		if err == nil || st.PRBranch != j.Branch {
			st.PR = pr
		}
		st.PRAt, st.PRBranch = w.now(), j.Branch
		w.states[j.Path] = st
		w.mu.Unlock()
	}
	w.mu.Lock()
	w.busy = false
	w.mu.Unlock()
}
