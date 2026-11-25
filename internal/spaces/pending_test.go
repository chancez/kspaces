package spaces

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/chancez/kspaces/internal/config"
	"github.com/chancez/kspaces/internal/kitty"
)

// pendingEnv gives the test a state directory of its own and a directory to mark worktrees in, symlinks
// resolved as the marks keep paths.
func pendingEnv(t *testing.T) (dir, root string) {
	t.Helper()
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	dir, err = pendingDir()
	if err != nil {
		t.Fatal(err)
	}
	return dir, root
}

// markOther writes a mark as another process would have: its pid, and when it marked.
func markOther(t *testing.T, dir string, p Pending) {
	t.Helper()
	if err := writePending(dir, p); err != nil {
		t.Fatal(err)
	}
}

func TestMarkPending(t *testing.T) {
	_, root := pendingEnv(t)
	path := filepath.Join(root, ".worktrees", "fix")
	before := time.Now()
	unmark, err := MarkPending(PendingCreate, root, path, "pr/me/fix")
	if err != nil {
		t.Fatal(err)
	}
	got := LoadPending()
	mark := got[path]
	if mark.Since.Before(before.Truncate(time.Second)) || mark.Since.After(time.Now()) {
		t.Fatalf("marked at %v, want between %v and now", mark.Since, before)
	}
	want := map[string]Pending{path: {Op: PendingCreate, Root: root, Path: path, Branch: "pr/me/fix", PID: os.Getpid(), Since: mark.Since}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("LoadPending() = %+v, want %+v", got, want)
	}
	// This process's own mark does not stop it opening the worktree it creates.
	if err := CheckPending(path); err != nil {
		t.Fatalf("CheckPending(own mark) = %v, want nil", err)
	}
	unmark()
	if got := LoadPending(); len(got) != 0 {
		t.Fatalf("LoadPending() after unmark = %+v, want none", got)
	}
}

func TestCheckPendingOther(t *testing.T) {
	dir, root := pendingEnv(t)
	path := filepath.Join(root, "wt")
	// The test's parent runs for as long as the test does.
	markOther(t, dir, Pending{Op: PendingRemove, Root: root, Path: path, PID: os.Getppid(), Since: time.Now().Add(-time.Hour)})
	err := CheckPending(path)
	if err == nil || err.Error() != path+" is being removed" {
		t.Fatalf("CheckPending() = %v, want %s is being removed", err, path)
	}
	// Creating a worktree being removed is refused; removing it again takes the mark over.
	if _, err := MarkPending(PendingCreate, root, path, ""); err == nil || err.Error() != path+" is being removed" {
		t.Fatalf("MarkPending(create) = %v, want %s is being removed", err, path)
	}
	unmark, err := MarkPending(PendingRemove, root, path, "")
	if err != nil {
		t.Fatalf("MarkPending(remove) = %v, want nil", err)
	}
	unmark()
	if got := LoadPending(); len(got) != 0 {
		t.Fatalf("LoadPending() after unmark = %+v, want none", got)
	}
}

func TestUnmarkTakenOver(t *testing.T) {
	dir, root := pendingEnv(t)
	path := filepath.Join(root, "wt")
	unmark, err := MarkPending(PendingRemove, root, path, "")
	if err != nil {
		t.Fatal(err)
	}
	// A background process takes the mark over, as from the form that handed the removal to it.
	other := Pending{Op: PendingRemove, Root: root, Path: path, PID: os.Getppid(), Since: time.Unix(1000, 0)}
	markOther(t, dir, other)
	unmark()
	got, want := LoadPending(), map[string]Pending{path: other}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("LoadPending() = %+v, want %+v", got, want)
	}
}

func TestPendingStale(t *testing.T) {
	dir, root := pendingEnv(t)
	// No process has pid 0's mark, so it stands only while it is handed off.
	gone := Pending{Op: PendingRemove, Root: root, Path: filepath.Join(root, "gone"), Since: time.Now().Add(-time.Minute)}
	handed := Pending{Op: PendingRemove, Root: root, Path: filepath.Join(root, "handed"), Since: time.Now().Truncate(time.Second)}
	markOther(t, dir, gone)
	markOther(t, dir, handed)
	got := LoadPending()
	handed.Since = got[handed.Path].Since
	want := map[string]Pending{handed.Path: handed}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("LoadPending() = %+v, want %+v", got, want)
	}
	if _, err := os.Stat(pendingFile(dir, gone.Path)); !os.IsNotExist(err) {
		t.Fatalf("stale mark's file: %v, want it deleted", err)
	}
}

func TestPendingPath(t *testing.T) {
	_, root := pendingEnv(t)
	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(root, link); err != nil {
		t.Fatal(err)
	}
	// A worktree not made yet is kept by its nearest directory that is, symlinks resolved.
	if got, want := pendingPath(filepath.Join(link, ".worktrees", "fix")), filepath.Join(root, ".worktrees", "fix"); got != want {
		t.Fatalf("pendingPath() = %q, want %q", got, want)
	}
}

// Opening a worktree another process creates, or removing it, is refused before kitty or git is asked.
func TestPendingRefused(t *testing.T) {
	dir, root := pendingEnv(t)
	path := filepath.Join(root, ".worktrees", "fix")
	markOther(t, dir, Pending{Op: PendingCreate, Root: root, Path: path, PID: os.Getppid(), Since: time.Now()})
	wt := Worktree{Path: path, Branch: "fix", Session: "gr@fix"}
	want := path + " is being created"
	if err := OpenWorktree(config.Config{}, kitty.State{}, wt, OpenOptions{}); err == nil || err.Error() != want {
		t.Fatalf("OpenWorktree() = %v, want %s", err, want)
	}
	space := Space{Name: "gr", Root: root, Worktrees: []Worktree{{Path: root}, wt}}
	if err := RemoveWorktree(config.Config{}, space, wt, RemoveOptions{}); err == nil || err.Error() != want {
		t.Fatalf("RemoveWorktree() = %v, want %s", err, want)
	}
}

// A mark is placed on the worktree git lists at its path, and a worktree being created that git does not
// list yet is added to its space.
func TestPlacePending(t *testing.T) {
	spaces := []Space{
		{Name: "gr", Root: "/src/gr", Worktrees: []Worktree{{Path: "/src/gr", Branch: "main", Session: "gr"}, {Path: "/src/gr/.worktrees/dev", Branch: "dev", Session: "gr@dev"}}},
		{Name: "kitty", Root: "/src/kitty", Worktrees: []Worktree{{Path: "/src/kitty", Branch: "master", Session: "kitty"}}},
	}
	listed := spaces[0].Worktrees
	marks := []Pending{
		{Op: PendingRemove, Path: "/src/gr/.worktrees/dev"},
		{Op: PendingCreate, Root: "/src/kitty", Path: "/src/kitty/.worktrees/fix", Branch: "fix"},
		// Removing what git no longer lists shows nothing.
		{Op: PendingRemove, Path: "/src/gr/.worktrees/gone"},
	}
	got := placePending(spaces, marks)
	want := map[string]PendingOp{"/src/gr/.worktrees/dev": PendingRemove, "/src/kitty/.worktrees/fix": PendingCreate}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("placePending() = %v, want %v", got, want)
	}
	wantSpaces := []Space{
		{Name: "gr", Root: "/src/gr", Worktrees: listed},
		{Name: "kitty", Root: "/src/kitty", Worktrees: []Worktree{{Path: "/src/kitty", Branch: "master", Session: "kitty"}, {Path: "/src/kitty/.worktrees/fix", Branch: "fix", Session: "kitty@fix"}}},
	}
	if !reflect.DeepEqual(spaces, wantSpaces) {
		t.Fatalf("spaces = %+v, want %+v", spaces, wantSpaces)
	}
}
