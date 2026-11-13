package spaces

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/chancez/kspaces/internal/config"
	"github.com/chancez/kspaces/internal/gh"
	"github.com/chancez/kspaces/internal/git"
	"github.com/chancez/kspaces/internal/gittest"
)

// A worktree's branch is kept when HEAD is detached, it is the default branch or another worktree has it
// checked out; otherwise what deleting it loses is counted, and a merged PR makes the commits a squash left
// behind lose nothing.
func TestBranchOf(t *testing.T) {
	r := gittest.New(t)
	r.Commit("first")
	wt := func(name string) string { return filepath.Join(r.Dir, ".worktrees", name) }
	for _, name := range []string{"merged", "unmerged", "squashed"} {
		w := r.Worktree(name, wt(name))
		w.Commit(name + " 1")
		if name != "merged" {
			w.Commit(name + " 2")
		}
	}
	r.Git("merge", "-q", "--no-edit", "merged")
	space := LoadSpace(r.Dir)
	byBranch := map[string]Worktree{}
	for _, w := range space.Worktrees {
		byBranch[w.Branch] = w
	}
	detached := Worktree{Path: wt("detached")}
	onMain := Worktree{Path: wt("trunk"), Branch: "main"}
	twice := Worktree{Path: wt("twice"), Branch: "merged"}
	elsewhere := space
	elsewhere.Worktrees = append(slices.Clone(space.Worktrees), twice)
	merged := &gh.PR{Number: 12, State: "merged"}
	got := []Branch{
		BranchOf(space, "main", detached, nil),
		BranchOf(space, "main", onMain, nil),
		BranchOf(elsewhere, "main", twice, nil),
		BranchOf(space, "main", byBranch["merged"], merged),
		BranchOf(space, "main", byBranch["unmerged"], &gh.PR{Number: 13, State: "open"}),
		BranchOf(space, "main", byBranch["squashed"], merged),
	}
	want := []Branch{
		{Kept: "HEAD is detached"},
		{Name: "main", Kept: "it is the default branch"},
		{Name: "merged", Kept: "it is checked out in " + wt("merged")},
		{Name: "merged", Merge: git.Merge{Into: "main"}},
		{Name: "unmerged", Merge: git.Merge{Unmerged: 2}},
		{Name: "squashed", Merge: git.Merge{Unmerged: 2}, PR: 12},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got\n%#v\nwant\n%#v", got, want)
	}
}

// fakeRemoval puts on PATH a kitten that answers `kitten @ ls` with no windows and a git that runs the
// real one, and returns the branch deletions git was asked for, each its flag and branch.
func fakeRemoval(t *testing.T) func() []string {
	t.Helper()
	real, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	log := filepath.Join(dir, "deleted")
	scripts := map[string]string{
		"kitten": "#!/bin/sh\n[ \"$2\" = ls ] && echo '[]'\nexit 0\n",
		"git": "#!/bin/sh\ncase \"$*\" in *\" branch -\"[dD]\" \"*) echo \"$*\" | sed 's/.* branch //' >> " + log + ";; esac\n" +
			"exec " + real + " \"$@\"\n",
	}
	for name, script := range scripts {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(script), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("KITTY_INSTALLATION_DIR", "")
	return func() []string {
		data, err := os.ReadFile(log)
		if err != nil {
			return nil
		}
		ans := strings.Split(strings.TrimSpace(string(data)), "\n")
		os.Remove(log)
		return ans
	}
}

// Removing a worktree deletes its branch only when asked: with -d when merged, with -D only when forced
// past the commits it loses. One not merged and not forced keeps the worktree too, and one git refuses
// after the worktree is removed, as -d does a branch its upstream lacks, is a BranchError.
func TestRemoveWorktreeBranch(t *testing.T) {
	deleted := fakeRemoval(t)
	r := gittest.New(t)
	r.Commit("first")
	r.Git("remote", "add", "origin", r.Dir)
	wt := func(name string) string { return filepath.Join(r.Dir, ".worktrees", name) }
	for _, name := range []string{"kept", "merged", "unmerged", "behind"} {
		r.Worktree(name, wt(name)).Commit(name)
	}
	r.Git("merge", "-q", "--no-edit", "kept", "merged", "behind")
	// behind's upstream is where it started, so git's -d, which asks the upstream, refuses it.
	r.Git("update-ref", "refs/remotes/origin/behind", "main~1")
	r.Git("config", "branch.behind.remote", "origin")
	r.Git("config", "branch.behind.merge", "refs/heads/behind")

	type result struct {
		Err       string
		Deleted   []string
		Branches  string
		Worktrees []string
	}
	cfg := config.Config{SessionsDir: t.TempDir()}
	remove := func(name string, opts RemoveOptions) result {
		space := LoadSpace(r.Dir)
		var w Worktree
		for _, x := range space.Worktrees {
			if x.Branch == name {
				w = x
			}
		}
		res := result{}
		if err := RemoveWorktree(cfg, space, w, opts); err != nil {
			res.Err = err.Error()
			if _, ok := errors.AsType[*BranchError](err); ok {
				res.Err = "BranchError: " + res.Err
			}
		}
		res.Deleted = deleted()
		res.Branches = r.Git("branch", "--format=%(refname:short)")
		for _, x := range LoadSpace(r.Dir).Worktrees[1:] {
			res.Worktrees = append(res.Worktrees, x.Branch)
		}
		return res
	}
	got := []result{
		remove("kept", RemoveOptions{}),
		remove("unmerged", RemoveOptions{DeleteBranch: true}),
		remove("merged", RemoveOptions{DeleteBranch: true}),
		remove("unmerged", RemoveOptions{DeleteBranch: true, ForceBranch: true}),
		remove("behind", RemoveOptions{DeleteBranch: true}),
	}
	want := []result{
		{Branches: "behind\nkept\nmain\nmerged\nunmerged", Worktrees: []string{"behind", "merged", "unmerged"}},
		{
			Err:       "unmerged has 1 commit not merged, which deleting it would lose; the worktree is kept",
			Branches:  "behind\nkept\nmain\nmerged\nunmerged",
			Worktrees: []string{"behind", "merged", "unmerged"},
		},
		{Deleted: []string{"-d merged"}, Branches: "behind\nkept\nmain\nunmerged", Worktrees: []string{"behind", "unmerged"}},
		{Deleted: []string{"-D unmerged"}, Branches: "behind\nkept\nmain", Worktrees: []string{"behind"}},
		{
			Err: "BranchError: the worktree was removed, but not its branch behind: warning: not deleting branch 'behind' that is not yet merged to\n" +
				"         'refs/remotes/origin/behind', even though it is merged to HEAD\nerror: the branch 'behind' is not fully merged",
			Deleted:  []string{"-d behind"},
			Branches: "behind\nkept\nmain",
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got\n%#v\nwant\n%#v", got, want)
	}
}
