package spaces

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/chancez/kspaces/internal/config"
	"github.com/chancez/kspaces/internal/gitstate"
	"github.com/chancez/kspaces/internal/kitty"
)

// One load for all the sidebars of a kitty. kitty docks a window to one tab only, so every tab has a
// sidebar of its own, and each loading kitty's state and every space's worktrees itself cost as many
// `kitten @ ls` and `git worktree list` runs. Instead the sidebars of one kitty elect a leader with a lock
// on a file: the leader loads and writes what it loaded to a file beside the lock, and the others read
// that file when it changes. When the leader exits its lock goes with it, and the next sidebar to poll
// takes the lead. What depends on the window a sidebar runs in, such as its tab's session, is worked out
// by each from the shared windows with View.

// Shared is what one load sees that is the same from every window of a kitty: all of a Snapshot but what
// depends on the window it is seen from, which View works out. The leader writes it as JSON, so errors
// are kept as their text.
type Shared struct {
	// Nil when the config could not be read, and Err says why.
	Config *config.Config `json:"config,omitempty"`
	Err    string         `json:"err,omitempty"`
	Spaces []sharedSpace  `json:"spaces,omitempty"`
	// Sessions with a file in the sessions directory.
	Saved []string `json:"saved,omitempty"`
	// kitty's windows, as `kitten @ ls` lists them, and why they could not be listed.
	Windows  []kitty.OSWindow `json:"windows,omitempty"`
	KittyErr string           `json:"kitty_err,omitempty"`
	// What git and gh say of each worktree, by path, which only the leader asks.
	Git map[string]gitstate.State `json:"git,omitempty"`
}

// sharedSpace is a Space with its error as text.
type sharedSpace struct {
	Name      string     `json:"name"`
	Root      string     `json:"root"`
	Worktrees []Worktree `json:"worktrees,omitempty"`
	Err       string     `json:"err,omitempty"`
}

// LoadShared reads the config, every space's worktrees, the saved sessions and kitty's windows.
func LoadShared() Shared {
	cfg, err := config.Load()
	if err != nil {
		return Shared{Err: err.Error()}
	}
	sh := Shared{Config: &cfg}
	for _, root := range cfg.Spaces {
		s := LoadSpace(root)
		ss := sharedSpace{Name: s.Name, Root: s.Root, Worktrees: s.Worktrees}
		if s.Err != nil {
			ss.Err = s.Err.Error()
		}
		sh.Spaces = append(sh.Spaces, ss)
	}
	sh.Saved = savedSessions(cfg.SessionsDir)
	windows, err := kitty.ListWindows()
	sh.Windows = windows
	if err != nil {
		sh.KittyErr = err.Error()
	}
	return sh
}

// View is the snapshot sh is from the window self, or kitty.FocusedWindow. The folded spaces, the sort
// mode and the worktrees' manual order are read here rather than shared, so a change shows in the sidebar
// that made it at once.
func (sh Shared) View(self int) Snapshot {
	if sh.Config == nil {
		return Snapshot{Err: errors.New(sh.Err)}
	}
	spaces := make([]Space, len(sh.Spaces))
	for i, s := range sh.Spaces {
		spaces[i] = Space{Name: s.Name, Root: s.Root, Worktrees: s.Worktrees}
		if s.Err != "" {
			spaces[i].Err = errors.New(s.Err)
		}
	}
	st := kitty.StateOf(sh.Windows, self)
	saved := make(map[string]bool, len(sh.Saved))
	for _, name := range sh.Saved {
		saved[name] = true
	}
	// The missing worktrees go in the spaces, which are this view's own copies: what the leader shares, and
	// asks git about in targets, is only what git lists.
	snap := Snapshot{Config: sh.Config, Spaces: spaces, Others: placeSessions(spaces, st.Sessions, sh.Saved), Folded: LoadFolded(), Saved: saved, Kitty: st}
	snap.Order = Order{Mode: LoadSort(), Worktrees: LoadOrder(), Focused: st.Focused, Statuses: st.Statuses}
	if sh.KittyErr != "" {
		snap.Err = errors.New(sh.KittyErr)
	}
	snap.Git = sh.Git
	return snap
}

// pollWait is how long a sidebar waits for the leader's first file before loading itself: every tab's
// sidebar starts at once with kitty, and the leader takes a moment to write.
const pollWait = time.Second

// Poller shares loads between the sidebars of one kitty, see Shared. It is safe to call from several
// goroutines.
type Poller struct {
	mu sync.Mutex
	// path is the shared file; the lock is path.lock.
	path string
	load func() Shared
	wait time.Duration
	// newWatcher makes the leader's watcher of the worktrees' git states, nil for none.
	newWatcher func() *gitstate.Watcher
	// lock is the open lock file, held while leader.
	lock   *os.File
	leader bool
	git    *gitstate.Watcher
	// refreshAt is when the refresh file was last touched, as the leader last saw it.
	refreshAt time.Time
	// written is what the leader last wrote, so an unchanged load is not written again.
	written []byte
	// seen is the file last read, so a follower reads it again only when the leader has replaced it.
	seen os.FileInfo
	last Shared
	have bool
}

// NewPoller is the poller of the kitty kspaces runs in.
func NewPoller() *Poller {
	p := newPoller(filepath.Join(pollDir(), pollKey()+".json"), LoadShared)
	p.newWatcher = gitstate.NewWatcher
	return p
}

func newPoller(path string, load func() Shared) *Poller {
	return &Poller{path: path, load: load, wait: pollWait}
}

// pollDir is where the shared files go: the user's runtime directory, else a directory of the user's own
// in the temporary one.
func pollDir() string {
	if dir := os.Getenv("XDG_RUNTIME_DIR"); dir != "" {
		return filepath.Join(dir, "kspaces")
	}
	return filepath.Join(os.TempDir(), fmt.Sprintf("kspaces-%d", os.Getuid()))
}

// pollKey names the kitty the sidebars run in: its remote control socket, which is what `kitten @` talks
// to, else its process.
func pollKey() string {
	key := os.Getenv("KITTY_LISTEN_ON")
	if key == "" {
		key = "pid:" + os.Getenv("KITTY_PID")
	}
	sum := sha256.Sum256([]byte(key))
	return hex.EncodeToString(sum[:8])
}

// Poll is the latest load. The leader loads and writes it; a follower reads what the leader wrote, and
// takes the lead when the leader has gone. fresh loads in a follower too, for what has to show a change
// at once, such as the refresh key or a switch just made.
func (p *Poller) Poll(fresh bool) Shared {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.leader {
		lead, ok := p.tryLead()
		if !ok {
			// No lock to share through, so every sidebar loads for itself, as without a poller.
			return p.load()
		}
		p.leader = lead
	} else if !p.holdsLock() {
		// The lock file was removed, by a cleaner of the temporary directory, and another may lead now.
		p.dropLock()
		return p.pollUnlocked(fresh)
	}
	if p.leader {
		return p.lead()
	}
	return p.follow(fresh)
}

// pollUnlocked is Poll once the lock it held is gone.
func (p *Poller) pollUnlocked(fresh bool) Shared {
	lead, ok := p.tryLead()
	if !ok {
		return p.load()
	}
	if p.leader = lead; lead {
		return p.lead()
	}
	return p.follow(fresh)
}

// lead loads as the leader, with the git states, and writes what it loaded for the others.
func (p *Poller) lead() Shared {
	sh := p.load()
	if p.newWatcher != nil {
		if p.git == nil {
			p.git = p.newWatcher()
			// Carry on from the last leader, so gh is not asked about every branch again at once.
			if prev, ok := p.read(); ok {
				p.git.Seed(prev.Git)
			}
			p.refreshAt = p.refreshTime()
		}
		if at := p.refreshTime(); at.After(p.refreshAt) {
			p.refreshAt = at
			p.git.Refresh()
		}
		sh.Git = p.git.Update(targets(sh))
	}
	p.write(sh)
	return sh
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

// follow is the leader's latest file, or a load of its own when fresh or when there is no file to read in
// time. Its own load keeps the git states it had, which only the leader asks for.
func (p *Poller) follow(fresh bool) Shared {
	if !fresh {
		for deadline := time.Now().Add(p.wait); ; time.Sleep(50 * time.Millisecond) {
			if sh, ok := p.read(); ok {
				return sh
			}
			if time.Now().After(deadline) {
				break
			}
		}
	}
	// Kept as the latest, until the leader writes a newer one.
	sh := p.load()
	sh.Git = p.last.Git
	p.last, p.have = sh, true
	return sh
}

// Refresh loads at once, and has the leader ask git and gh again too: itself, or through the refresh
// file, whose time the leader checks at each poll.
func (p *Poller) Refresh() Shared {
	p.mu.Lock()
	if p.leader && p.git != nil {
		p.git.Refresh()
	} else {
		now := time.Now()
		if f, err := os.OpenFile(p.path+".refresh", os.O_CREATE|os.O_WRONLY, 0o600); err == nil {
			f.Close()
			os.Chtimes(p.path+".refresh", now, now)
		}
	}
	p.mu.Unlock()
	return p.Poll(true)
}

// refreshTime is when the refresh file was last touched, zero when it never was.
func (p *Poller) refreshTime() time.Time {
	fi, err := os.Stat(p.path + ".refresh")
	if err != nil {
		return time.Time{}
	}
	return fi.ModTime()
}

// Git is the leader's latest git states, for what does not poll itself, such as the picker.
func (p *Poller) Git() map[string]gitstate.State {
	p.mu.Lock()
	defer p.mu.Unlock()
	sh, _ := p.read()
	return sh.Git
}

// tryLead takes the lock if no other process holds it. ok is false when there is no lock file to take.
func (p *Poller) tryLead() (lead, ok bool) {
	if p.lock == nil {
		if err := os.MkdirAll(filepath.Dir(p.path), 0o700); err != nil {
			return false, false
		}
		f, err := os.OpenFile(p.path+".lock", os.O_CREATE|os.O_RDWR, 0o600)
		if err != nil {
			return false, false
		}
		p.lock = f
	}
	return syscall.Flock(int(p.lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB) == nil, true
}

// holdsLock reports whether the lock file the leader holds is still the one at its path.
func (p *Poller) holdsLock() bool {
	held, err := p.lock.Stat()
	if err != nil {
		return false
	}
	there, err := os.Stat(p.path + ".lock")
	return err == nil && os.SameFile(held, there)
}

func (p *Poller) dropLock() {
	if p.lock != nil {
		p.lock.Close()
	}
	p.lock, p.leader, p.written, p.git = nil, false, nil, nil
}

// Close gives up the lead, for another sidebar to take.
func (p *Poller) Close() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.dropLock()
}

// write replaces the shared file with sh, unless it already holds it. A rename, so a reader sees all of
// one load or all of the next.
func (p *Poller) write(sh Shared) {
	data, err := json.Marshal(sh)
	if err != nil {
		return
	}
	if bytes.Equal(data, p.written) {
		if _, err := os.Stat(p.path); err == nil {
			return
		}
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

// read is the shared file, read again only when the leader has replaced it since it was last read.
func (p *Poller) read() (Shared, bool) {
	f, err := os.Open(p.path)
	if err != nil {
		return Shared{}, false
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		return Shared{}, false
	}
	// The same file, size and time: a replaced file can get the inode of one removed before it.
	if p.have && p.seen != nil && os.SameFile(fi, p.seen) && fi.Size() == p.seen.Size() && fi.ModTime().Equal(p.seen.ModTime()) {
		return p.last, true
	}
	var sh Shared
	if err := json.NewDecoder(f).Decode(&sh); err != nil {
		return Shared{}, false
	}
	p.seen, p.last, p.have = fi, sh, true
	return sh, true
}
