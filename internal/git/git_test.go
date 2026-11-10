package git

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"
)

func TestParseWorktrees(t *testing.T) {
	porcelain := `worktree /src/gr
HEAD 1111111111111111111111111111111111111111
branch refs/heads/main

worktree /src/gr/.worktrees/dev
HEAD 2222222222222222222222222222222222222222
branch refs/heads/pr/chancez/dev

worktree /src/gr/.worktrees/gone
HEAD 3333333333333333333333333333333333333333
branch refs/heads/old
prunable gitdir file points to non-existent location

worktree /src/gr/.worktrees/bisect
HEAD 4444444444444444444444444444444444444444
detached
`
	got := ParseWorktrees(porcelain)
	want := []Worktree{
		{Path: "/src/gr", Branch: "main"},
		{Path: "/src/gr/.worktrees/dev", Branch: "pr/chancez/dev"},
		{Path: "/src/gr/.worktrees/bisect", Branch: ""},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v\nwant %#v", got, want)
	}
}

// A bare repository has no files to open, so its first checked out worktree comes first.
func TestParseWorktreesBare(t *testing.T) {
	porcelain := `worktree /src/gr.git
bare

worktree /src/gr/main
HEAD 1111111111111111111111111111111111111111
branch refs/heads/main
`
	got := ParseWorktrees(porcelain)
	want := []Worktree{{Path: "/src/gr/main", Branch: "main"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v\nwant %#v", got, want)
	}
}

// testRepo makes a repository with one commit and a linked worktree at .worktrees/x on branch x, without
// the user's git config, and returns the repository's root and the worktree's path.
func testRepo(t *testing.T) (root, wt string) {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"init", "-q", "-b", "main"},
		{"commit", "-q", "--allow-empty", "-m", "first"},
		{"worktree", "add", "-q", "-b", "x", ".worktrees/x"},
	} {
		cmd := exec.Command("git", append([]string{"-C", root}, args...)...)
		cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t",
			"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	return root, filepath.Join(root, ".worktrees", "x")
}

// git refuses to remove a worktree with changes unless forced, and the branch stays either way.
func TestRemoveWorktree(t *testing.T) {
	root, wt := testRepo(t)
	if err := os.WriteFile(filepath.Join(wt, "new"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if n, err := Changes(wt); n != 1 || err != nil {
		t.Fatalf("changes = %d, %v", n, err)
	}
	if err := RemoveWorktree(root, wt, false); err == nil {
		t.Fatal("a worktree with changes was removed without force")
	}
	if _, err := os.Stat(wt); err != nil {
		t.Fatalf("the refused worktree is gone: %v", err)
	}
	if err := RemoveWorktree(root, wt, true); err != nil {
		t.Fatal(err)
	}
	wts, err := ListWorktrees(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(wt); !os.IsNotExist(err) || len(wts) != 1 || !BranchExists(root, "x") {
		t.Fatalf("after removing: stat %v, worktrees %#v, branch kept %v", err, wts, BranchExists(root, "x"))
	}
	if n, err := Changes(root); n != 0 || err != nil {
		t.Fatalf("changes in a clean worktree = %d, %v", n, err)
	}
}

// A locked worktree is listed as locked, with or without a reason.
func TestParseLockedWorktrees(t *testing.T) {
	porcelain := "worktree /src/gr\nHEAD 1\nbranch refs/heads/main\n\n" +
		"worktree /src/gr/a\nHEAD 2\nbranch refs/heads/a\nlocked\n\n" +
		"worktree /src/gr/b\nHEAD 3\nbranch refs/heads/b\nlocked on a usb drive\n"
	want := []Worktree{
		{Path: "/src/gr", Branch: "main"},
		{Path: "/src/gr/a", Branch: "a", Locked: true},
		{Path: "/src/gr/b", Branch: "b", Locked: true},
	}
	if got := ParseWorktrees(porcelain); !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v\nwant %#v", got, want)
	}
}
