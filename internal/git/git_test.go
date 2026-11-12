package git

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"testing"

	"github.com/chancez/kspaces/internal/gittest"
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

// testRepo makes a repository with one commit and a linked worktree at .worktrees/x on branch x, and
// returns the repository's root and the worktree's path.
func testRepo(t *testing.T) (root, wt string) {
	t.Helper()
	r := gittest.New(t)
	r.Commit("first")
	return r.Dir, r.Worktree("x", ".worktrees/x").Dir
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

// TopLevel is the top of the work tree from anywhere in it, a linked worktree's own, and an error outside
// any repository.
func TestTopLevel(t *testing.T) {
	root, wt := testRepo(t)
	sub := filepath.Join(root, "sub")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	type result struct {
		Path string
		Err  bool
	}
	var got []result
	for _, dir := range []string{root, sub, wt, t.TempDir()} {
		p, err := TopLevel(dir)
		got = append(got, result{p, err != nil})
	}
	want := []result{{root, false}, {root, false}, {wt, false}, {"", true}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

// DefaultBranch picks gr's base remote and takes its HEAD, else its branch of a default name, else a local
// one. The cases follow gr's, plus the one that put a closed PR on hubble-timescape's main worktree: a
// remote not called origin, with an origin/HEAD left by a remote since removed.
func TestDefaultBranch(t *testing.T) {
	type remote struct {
		name     string
		branches []string
		// head is the branch the remote's HEAD points at, or "" for none, as after git remote add.
		head string
	}
	for name, tc := range map[string]struct {
		remotes []remote
		local   []string
		config  map[string]string
		// stale leaves refs/remotes/origin/HEAD pointing at origin/master with no origin remote.
		stale bool
		want  string
	}{
		"no remote, main":               {local: []string{"main"}, want: "main"},
		"no remote, master":             {local: []string{"master"}, want: "master"},
		"no remote, init.defaultBranch": {local: []string{"main", "trunk"}, config: map[string]string{"init.defaultBranch": "trunk"}, want: "trunk"},
		"no remote, no default name":    {local: []string{"feature"}, want: ""},
		"origin's HEAD": {
			remotes: []remote{{name: "origin", branches: []string{"main", "dev"}, head: "dev"}},
			want:    "dev",
		},
		"the only remote": {
			remotes: []remote{{name: "work", branches: []string{"trunk"}, head: "trunk"}},
			want:    "trunk",
		},
		"origin before another remote": {
			remotes: []remote{{name: "fork", branches: []string{"mine"}, head: "mine"}, {name: "origin", branches: []string{"main"}, head: "main"}},
			want:    "main",
		},
		"origin before upstream": {
			remotes: []remote{{name: "origin", branches: []string{"fork-main"}, head: "fork-main"}, {name: "upstream", branches: []string{"main"}, head: "main"}},
			want:    "fork-main",
		},
		"checkout.defaultRemote before origin": {
			remotes: []remote{{name: "origin", branches: []string{"fork-main"}, head: "fork-main"}, {name: "upstream", branches: []string{"main"}, head: "main"}},
			config:  map[string]string{"checkout.defaultRemote": "upstream"},
			want:    "main",
		},
		"clone.defaultRemoteName": {
			remotes: []remote{{name: "a", branches: []string{"a-main"}, head: "a-main"}, {name: "b", branches: []string{"b-main"}, head: "b-main"}},
			config:  map[string]string{"clone.defaultRemoteName": "b"},
			want:    "b-main",
		},
		"the remote that is not pushDefault": {
			remotes: []remote{{name: "mine", branches: []string{"x"}, head: "x"}, {name: "theirs", branches: []string{"main"}, head: "main"}},
			config:  map[string]string{"remote.pushDefault": "mine"},
			want:    "main",
		},
		"no HEAD, a branch of a default name": {
			remotes: []remote{{name: "origin", branches: []string{"dev", "master"}}},
			want:    "master",
		},
		"a remote not called origin, a stale origin/HEAD": {
			remotes: []remote{{name: "isovalent", branches: []string{"main"}, head: "main"}},
			local:   []string{"main"},
			stale:   true,
			want:    "main",
		},
		"two remotes, none to pick, a local default": {
			remotes: []remote{{name: "a", branches: []string{"x"}, head: "x"}, {name: "b", branches: []string{"y"}, head: "y"}},
			local:   []string{"main"},
			want:    "main",
		},
	} {
		t.Run(name, func(t *testing.T) {
			r := gittest.New(t)
			r.Commit("first")
			for _, b := range tc.local {
				if b != "main" {
					r.Git("branch", b)
				}
			}
			if !slices.Contains(tc.local, "main") {
				r.Git("checkout", "-q", "--detach")
				r.Git("branch", "-D", "main")
			}
			for _, rm := range tc.remotes {
				r.Git("remote", "add", rm.name, r.Dir)
				for _, b := range rm.branches {
					r.Git("update-ref", "refs/remotes/"+rm.name+"/"+b, "HEAD")
				}
				if rm.head != "" {
					r.Git("symbolic-ref", "refs/remotes/"+rm.name+"/HEAD", "refs/remotes/"+rm.name+"/"+rm.head)
				}
			}
			if tc.stale {
				r.Git("update-ref", "refs/remotes/origin/master", "HEAD")
				r.Git("symbolic-ref", "refs/remotes/origin/HEAD", "refs/remotes/origin/master")
			}
			for k, v := range tc.config {
				r.Git("config", k, v)
			}
			if got := DefaultBranch(r.Dir); got != tc.want {
				t.Errorf("DefaultBranch = %q, want %q", got, tc.want)
			}
		})
	}
}

// A branch is merged when the default branch, local or the base remote's, or its upstream has all its
// commits, and says which; otherwise the commits neither has are counted. A gone upstream counts as none.
func TestMergeOf(t *testing.T) {
	r := gittest.New(t)
	r.Commit("first")
	r.Git("remote", "add", "origin", r.Dir)
	branch := func(name string, commits int) {
		r.Git("checkout", "-q", "-b", name, "main")
		for i := range commits {
			r.Commit(fmt.Sprintf("%s %d", name, i))
		}
		r.Git("checkout", "-q", "main")
	}
	upstream := func(name string) {
		r.Git("config", "branch."+name+".remote", "origin")
		r.Git("config", "branch."+name+".merge", "refs/heads/"+name)
	}
	branch("fresh", 0)
	branch("merged", 2)
	r.Git("merge", "-q", "--no-edit", "--no-ff", "merged")
	branch("remote", 1)
	r.Git("update-ref", "refs/remotes/origin/main", "remote")
	branch("pushed", 2)
	upstream("pushed")
	r.Git("update-ref", "refs/remotes/origin/pushed", "pushed")
	branch("ahead", 2)
	upstream("ahead")
	r.Git("update-ref", "refs/remotes/origin/ahead", "ahead~1")
	branch("gone", 1)
	upstream("gone")

	got := map[string]Merge{}
	for _, b := range []string{"fresh", "merged", "remote", "pushed", "ahead", "gone"} {
		m, err := MergeOf(r.Dir, b, "main")
		if err != nil {
			t.Fatal(err)
		}
		got[b] = m
	}
	want := map[string]Merge{
		"fresh":  {Into: "main"},
		"merged": {Into: "main"},
		"remote": {Into: "main"},
		"pushed": {Into: "origin/pushed"},
		"ahead":  {Unmerged: 1},
		"gone":   {Unmerged: 1},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %#v\nwant %#v", got, want)
	}
}

// git branch -d refuses a branch not merged, and -D deletes it.
func TestDeleteBranch(t *testing.T) {
	r := gittest.New(t)
	r.Commit("first")
	r.Git("branch", "merged")
	r.Git("checkout", "-q", "-b", "unmerged")
	r.Commit("work")
	r.Git("checkout", "-q", "main")
	type result struct {
		Err      bool
		Branches string
	}
	var got []result
	for _, step := range []struct {
		branch string
		force  bool
	}{{"merged", false}, {"unmerged", false}, {"unmerged", true}} {
		err := DeleteBranch(r.Dir, step.branch, step.force)
		got = append(got, result{err != nil, r.Git("branch", "--format=%(refname:short)")})
	}
	want := []result{{false, "main\nunmerged"}, {true, "main\nunmerged"}, {false, "main"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %#v\nwant %#v", got, want)
	}
}
