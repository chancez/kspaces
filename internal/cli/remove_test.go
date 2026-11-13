package cli

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/chancez/kspaces/internal/gittest"
	"github.com/chancez/kspaces/internal/spaces"
)

// The flags for every path come first, then those for one path each, as the clean up form passes them.
func TestRemoveOptions(t *testing.T) {
	got := removeOptions([]string{"a", "b", "c"}, false, false, false, []string{"b"}, []string{"a", "b"}, []string{"b"})
	want := map[string]spaces.RemoveOptions{
		"a": {DeleteBranch: true},
		"b": {Force: true, DeleteBranch: true, ForceBranch: true},
		"c": {},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("per path: got %#v\nwant %#v", got, want)
	}
	got = removeOptions([]string{"a"}, true, true, true, nil, nil, nil)
	if want := map[string]spaces.RemoveOptions{"a": {Force: true, DeleteBranch: true, ForceBranch: true}}; !reflect.DeepEqual(got, want) {
		t.Errorf("for all: got %#v\nwant %#v", got, want)
	}
}

// remove --yes --delete-branch deletes a merged branch with git branch -d once its worktree is removed,
// refuses an unmerged one, keeping its worktree and saying so in kitty, unless --force-branch, which
// deletes it with -D; --delete-branch-path and --force-branch-path do the same for one path each.
func TestRemoveDeleteBranch(t *testing.T) {
	r := gittest.New(t)
	r.Commit("first")
	wt := func(name string) string { return filepath.Join(r.Dir, ".worktrees", name) }
	for _, name := range []string{"merged", "unmerged", "merged2", "unmerged2", "kept"} {
		w := r.Worktree(name, wt(name))
		if name != "kept" {
			w.Commit(name)
		}
	}
	r.Git("merge", "-q", "--no-edit", "merged", "merged2")
	calls, _ := fakeRemoval(t, r.Dir)

	type result struct {
		Err      string
		Calls    []string
		Branches string
	}
	run := func(args ...string) result {
		var out bytes.Buffer
		a := &app{stdout: &out, stderr: &out}
		cmd := a.rootCommand()
		cmd.SetArgs(append([]string{"remove", "--yes"}, args...))
		res := result{}
		if err := cmd.Execute(); err != nil {
			res.Err = err.Error()
		}
		res.Calls = calls()
		res.Branches = strings.ReplaceAll(r.Git("branch", "--format=%(refname:short)"), "\n", " ")
		return res
	}
	got := []result{
		run("--delete-branch", wt("merged")),
		run("--delete-branch", wt("unmerged")),
		run("--delete-branch", "--force-branch", wt("unmerged")),
		run("--delete-branch-path", wt("merged2"), "--delete-branch-path", wt("unmerged2"), "--force-branch-path", wt("unmerged2"),
			"--", wt("merged2"), wt("unmerged2"), wt("kept")),
	}
	refused := fmt.Sprintf("%s: unmerged has 1 commit not merged, which deleting it would lose; the worktree is kept", wt("unmerged"))
	want := []result{
		{Calls: []string{"git -d merged"}, Branches: "kept main merged2 unmerged unmerged2"},
		{Err: refused, Calls: []string{"kitten show_error Could not remove the worktree"}, Branches: "kept main merged2 unmerged unmerged2"},
		{Calls: []string{"git -D unmerged"}, Branches: "kept main merged2 unmerged2"},
		{Calls: []string{"git -d merged2", "git -D unmerged2"}, Branches: "kept main"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got\n%#v\nwant\n%#v", got, want)
	}
	if wts := r.Git("worktree", "list", "--porcelain"); strings.Count(wts, "worktree ") != 1 {
		t.Errorf("worktrees left:\n%s", wts)
	}
}

// remove --yes --forget-session ends a missing worktree's session, deleting its saved file, with no PATH
// or after the worktrees; one with no saved file is fine, and one that cannot be deleted is shown in kitty.
func TestRemoveForgetSession(t *testing.T) {
	r := gittest.New(t)
	r.Commit("first")
	dev := filepath.Join(r.Dir, ".worktrees", "dev")
	r.Worktree("dev", dev)
	calls, sessions := fakeRemoval(t, r.Dir)
	name := filepath.Base(r.Dir)
	for _, s := range []string{"@gone", "@dev", "@kept"} {
		if err := os.WriteFile(filepath.Join(sessions, name+s+".kitty-session"), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	// A directory with something in it is a saved file os.Remove cannot delete.
	stuck := filepath.Join(sessions, name+"@stuck.kitty-session")
	if err := os.MkdirAll(filepath.Join(stuck, "x"), 0o755); err != nil {
		t.Fatal(err)
	}
	run := func(args ...string) []string {
		var out bytes.Buffer
		a := &app{stdout: &out, stderr: &out}
		cmd := a.rootCommand()
		cmd.SetArgs(append([]string{"remove"}, args...))
		res := []string{"<nil>"}
		if err := cmd.Execute(); err != nil {
			res[0] = err.Error()
		}
		res = append(res, calls()...)
		entries, err := os.ReadDir(sessions)
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range entries {
			res = append(res, strings.TrimPrefix(e.Name(), name))
		}
		return res
	}
	got := [][]string{
		run("--forget-session", name+"@gone"),
		run("--yes"),
		run("--yes", "--forget-session", name+"@gone", "--forget-session", name+"@none", dev),
		run("--yes", "--forget-session", name+"@stuck"),
	}
	all := []string{"@dev.kitty-session", "@gone.kitty-session", "@kept.kitty-session", "@stuck.kitty-session"}
	want := [][]string{
		append([]string{"--forget-session needs --yes"}, all...),
		append([]string{"give the PATH of a worktree to remove"}, all...),
		{"<nil>", "@kept.kitty-session", "@stuck.kitty-session"},
		{fmt.Sprintf("%s@stuck: remove %s: directory not empty", name, stuck), "kitten show_error Could not remove the worktree", "@kept.kitty-session", "@stuck.kitty-session"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got\n%q\nwant\n%q", got, want)
	}
}

// fakeRemoval configures root as the only space, with sessions saved in the directory it returns, and puts
// on PATH a kitten that answers `kitten @ ls` with no windows and a git that runs the real one. Its func
// returns the branch deletions git was asked for and the errors kitty was asked to show since it was last
// called.
func fakeRemoval(t *testing.T, root string) (func() []string, string) {
	t.Helper()
	real, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	dir, config, sessions := t.TempDir(), t.TempDir(), t.TempDir()
	log := filepath.Join(dir, "calls")
	scripts := map[string]string{
		"kitten": "#!/bin/sh\n[ \"$2\" = ls ] && echo '[]'\n" +
			"[ \"$3\" = show_error ] && echo \"kitten $3 $4\" >> " + log + "\nexit 0\n",
		"git": "#!/bin/sh\ncase \"$*\" in *\" branch -\"[dD]\" \"*) echo \"git $*\" | sed 's/ .* branch / /' >> " + log + ";; esac\n" +
			"exec " + real + " \"$@\"\n",
	}
	for name, script := range scripts {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(script), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	toml := fmt.Sprintf("spaces = [%q]\nsessions = %q\n", root, sessions)
	if err := os.MkdirAll(filepath.Join(config, "kspaces"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(config, "kspaces", "config.toml"), []byte(toml), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_CONFIG_HOME", config)
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("KITTY_INSTALLATION_DIR", "")
	return func() []string {
		data, err := os.ReadFile(log)
		if err != nil {
			return nil
		}
		os.Remove(log)
		return strings.Split(strings.TrimSpace(string(data)), "\n")
	}, sessions
}
