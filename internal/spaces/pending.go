package spaces

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"syscall"
	"time"

	"github.com/chancez/kspaces/internal/config"
	"github.com/chancez/kspaces/internal/state"
)

// Creating and removing a worktree take a while, git checking out or deleting its files, and its row can be
// clicked meanwhile: opening a session in a directory half made or half gone. Whatever creates or removes a
// worktree marks its directory pending first, a file each in the state directory, so the sidebar shows it
// and opening it waits until the mark goes. A mark names the process that holds it, and goes stale when
// that process is gone, as when kspaces stopped part way.

// PendingOp is what is under way in a pending worktree.
type PendingOp string

const (
	PendingCreate PendingOp = "creating"
	PendingRemove PendingOp = "removing"
)

// Pending is a worktree being created or removed, see MarkPending.
type Pending struct {
	Op PendingOp `json:"op"`
	// Root is the space's root, Path the worktree's directory, symlinks resolved as far as it exists.
	Root   string `json:"root"`
	Path   string `json:"path"`
	Branch string `json:"branch,omitempty"`
	// PID is the process that holds the mark, Since when it was marked.
	PID   int       `json:"pid"`
	Since time.Time `json:"since"`
}

// pendingHandoff is how long a mark outlives the process that made it: a form marks the worktree it hands
// to a background process to remove, and exits before that process marks it as its own.
const pendingHandoff = 10 * time.Second

// pendingDir is where the marks are, a file each, so marking one never rewrites another's.
func pendingDir() (string, error) {
	dir, err := state.Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "pending"), nil
}

// pendingPath is the canonical form of path that marks are kept by: git lists worktrees with symlinks
// resolved, and a worktree being created does not exist yet, so the nearest directory that does is.
func pendingPath(path string) string {
	path = filepath.Clean(path)
	rest := ""
	for dir := path; ; dir = filepath.Dir(dir) {
		if resolved, err := filepath.EvalSymlinks(dir); err == nil {
			return filepath.Join(resolved, rest)
		}
		if filepath.Dir(dir) == dir {
			return path
		}
		rest = filepath.Join(filepath.Base(dir), rest)
	}
}

func pendingFile(dir, path string) string {
	sum := sha256.Sum256([]byte(path))
	return filepath.Join(dir, hex.EncodeToString(sum[:8])+".json")
}

// live says whether p still stands at now: its process runs, or it was handed off moments ago.
func (p Pending) live(now time.Time) bool {
	return now.Sub(p.Since) < pendingHandoff || processRuns(p.PID)
}

// processRuns says whether a process with pid runs. One owned by another user, which signalling is refused,
// runs too.
func processRuns(pid int) bool {
	if pid <= 0 {
		return false
	}
	err := syscall.Kill(pid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}

// pendingError says what is under way in a worktree, for what it stops.
func pendingError(p Pending) error {
	return fmt.Errorf("%s is being %s", config.Tilde(p.Path), map[PendingOp]string{PendingCreate: "created", PendingRemove: "removed"}[p.Op])
}

// MarkPending marks the worktree of the space at root, at path on branch, as op says, held by this process,
// and returns what unmarks it, which leaves a mark another process has taken since alone. It refuses a
// worktree marked for another op: one being removed cannot be created, nor one being created removed. A
// mark for the same op is taken over, as a background process removing a worktree takes the form's.
func MarkPending(op PendingOp, root, path, branch string) (unmark func(), err error) {
	dir, err := pendingDir()
	if err != nil {
		return nil, err
	}
	path = pendingPath(path)
	if p, ok := readPending(pendingFile(dir, path)); ok && p.Op != op && p.PID != os.Getpid() && p.live(time.Now()) {
		return nil, pendingError(p)
	}
	p := Pending{Op: op, Root: root, Path: path, Branch: branch, PID: os.Getpid(), Since: time.Now()}
	if err := writePending(dir, p); err != nil {
		return nil, err
	}
	return func() {
		file := pendingFile(dir, path)
		if q, ok := readPending(file); ok && q.PID == p.PID && q.Op == p.Op {
			os.Remove(file)
		}
	}, nil
}

// PendingOf is the mark another process holds on the worktree at path, if one stands. This process's own
// is left out: creating a worktree opens its session while it is marked.
func PendingOf(path string) (Pending, bool) {
	dir, err := pendingDir()
	if err != nil {
		return Pending{}, false
	}
	p, ok := readPending(pendingFile(dir, pendingPath(path)))
	if !ok || p.PID == os.Getpid() || !p.live(time.Now()) {
		return Pending{}, false
	}
	return p, true
}

// CheckPending is the error opening the worktree at path meets while another process creates or removes
// it, nil when none does.
func CheckPending(path string) error {
	if p, ok := PendingOf(path); ok {
		return pendingError(p)
	}
	return nil
}

// LoadPending is the marks that stand, by path. Stale ones are deleted.
func LoadPending() map[string]Pending {
	out := map[string]Pending{}
	dir, err := pendingDir()
	if err != nil {
		return out
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return out
	}
	now := time.Now()
	for _, e := range entries {
		if filepath.Ext(e.Name()) != ".json" {
			continue
		}
		file := filepath.Join(dir, e.Name())
		p, ok := readPending(file)
		if !ok || !p.live(now) {
			os.Remove(file)
			continue
		}
		out[p.Path] = p
	}
	return out
}

// pendingList is LoadPending in order of path, as a poll keeps it.
func pendingList() []Pending {
	pending := LoadPending()
	var out []Pending
	for _, path := range slices.Sorted(maps.Keys(pending)) {
		out = append(out, pending[path])
	}
	return out
}

func readPending(file string) (Pending, bool) {
	data, err := os.ReadFile(file)
	if err != nil {
		return Pending{}, false
	}
	var p Pending
	if err := json.Unmarshal(data, &p); err != nil || p.Path == "" {
		return Pending{}, false
	}
	return p, true
}

// writePending writes p's mark whole, by a rename, so a reader never sees half of one.
func writePending(dir string, p Pending) error {
	data, err := json.Marshal(p)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, "mark.*.tmp")
	if err != nil {
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	return os.Rename(tmp.Name(), pendingFile(dir, p.Path))
}

// placePending is what is under way in the worktrees of spaces, by their paths as git lists them, from
// marks. A worktree being created that git does not list yet is added to its space, so its row shows from
// the start. The spaces are the view's own copies, see Shared.View.
func placePending(spaces []Space, marks []Pending) map[string]PendingOp {
	if len(marks) == 0 {
		return nil
	}
	out := map[string]PendingOp{}
	for _, p := range marks {
		listed := false
		for i := range spaces {
			for _, wt := range spaces[i].Worktrees {
				if wt.Path != "" && (wt.Path == p.Path || pendingPath(wt.Path) == p.Path) {
					out[wt.Path], listed = p.Op, true
				}
			}
		}
		if listed || p.Op != PendingCreate {
			continue
		}
		for i, s := range spaces {
			if s.Root == p.Root && s.Err == nil {
				spaces[i].Worktrees = append(slices.Clone(s.Worktrees), LinkedWorktree(s, p.Branch, p.Path))
				out[p.Path] = p.Op
				break
			}
		}
	}
	return out
}
