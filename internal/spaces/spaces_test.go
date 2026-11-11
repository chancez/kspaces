package spaces

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/chancez/kspaces/internal/config"
	"github.com/chancez/kspaces/internal/git"
	"github.com/chancez/kspaces/internal/kitty"
	"github.com/chancez/kspaces/internal/status"
)

func TestWorktreeSessions(t *testing.T) {
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
	got := fromGit("gr", git.ParseWorktrees(porcelain))
	want := []Worktree{
		{Path: "/src/gr", Branch: "main", Session: "gr"},
		{Path: "/src/gr/.worktrees/dev", Branch: "pr/chancez/dev", Session: "gr@dev"},
		{Path: "/src/gr/.worktrees/bisect", Branch: "", Session: "gr@bisect"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v\nwant %#v", got, want)
	}
}

// A bare repository has no files to open, so its first checked out worktree is the space's main one.
func TestWorktreeSessionsBare(t *testing.T) {
	porcelain := `worktree /src/gr.git
bare

worktree /src/gr/main
HEAD 1111111111111111111111111111111111111111
branch refs/heads/main
`
	got := fromGit("gr", git.ParseWorktrees(porcelain))
	want := []Worktree{{Path: "/src/gr/main", Branch: "main", Session: "gr"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v\nwant %#v", got, want)
	}
}

// rowNames shows rows as their worktree's session, with "" for the blank rows between groups.
func rowNames(rows []Row) []string {
	var ans []string
	for _, r := range rows {
		switch {
		case r.Space == nil:
			ans = append(ans, "")
		case r.Folded:
			ans = append(ans, r.Worktree.Session+" (folded)")
		default:
			ans = append(ans, r.Worktree.Session)
		}
	}
	return ans
}

func TestBuildRows(t *testing.T) {
	others := []Space{{Name: "default", Worktrees: []Worktree{{Session: "default"}}}}
	spaces := []Space{
		{Name: "gr", Root: "/src/gr", Worktrees: []Worktree{{Session: "gr"}, {Session: "gr@dev"}, {Session: "gr@fix"}}},
		{Name: "kitty", Root: "/src/kitty", Worktrees: []Worktree{{Session: "kitty"}}},
		{Name: "docs", Root: "/src/docs", Worktrees: []Worktree{{Session: "docs"}, {Session: "docs@notes"}}},
	}
	// Folding a space with no linked worktrees changes nothing.
	got := rowNames(BuildRows(others, spaces, map[string]bool{"/src/docs": true, "/src/kitty": true}))
	want := []string{"default", "", "gr", "gr@dev", "gr@fix", "", "kitty", "", "docs (folded)"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %q\nwant %q", got, want)
	}
}

func TestWorktreeDir(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	cfg := config.Config{Worktrees: config.Worktrees{Dir: ".worktrees", BranchPrefix: "pr/me"}}
	got := []string{
		// The default drops the prefix and turns / into -.
		WorktreeDir(cfg, "/src/gr", "pr/me/fix/bug", ""),
		// A branch without the prefix keeps all of its name.
		WorktreeDir(cfg, "/src/gr", "release/1.0", ""),
		// A given directory: relative to the space, absolute, or under the home directory.
		WorktreeDir(cfg, "/src/gr", "pr/me/x", "../gr-x"),
		WorktreeDir(cfg, "/src/gr", "pr/me/x", "/tmp/x/"),
		WorktreeDir(cfg, "/src/gr", "pr/me/x", "~/wt/x"),
	}
	want := []string{"/src/gr/.worktrees/fix-bug", "/src/gr/.worktrees/release-1.0", "/src/gr-x", "/tmp/x", filepath.Join(home, "wt/x")}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %q\nwant %q", got, want)
	}
	// A trailing / in the prefix is the same prefix.
	starts := []string{
		BranchStart(cfg),
		BranchStart(config.Config{Worktrees: config.Worktrees{BranchPrefix: "pr/me/"}}),
		BranchStart(config.Config{}),
	}
	if want := []string{"pr/me/", "pr/me/", ""}; !reflect.DeepEqual(starts, want) {
		t.Fatalf("got %q, want %q", starts, want)
	}
}

func TestCreateWorktree(t *testing.T) {
	run := func(dir string, args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t",
			"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	run(root, "init", "-q", "-b", "main")
	run(root, "commit", "-q", "--allow-empty", "-m", "first")
	run(root, "branch", "old")
	space := Space{Name: "repo", Root: root}

	// A new branch.
	got, err := CreateWorktree(space, "pr/me/fix", filepath.Join(root, ".worktrees", "fix"))
	if err != nil {
		t.Fatal(err)
	}
	want := Worktree{Path: filepath.Join(root, ".worktrees", "fix"), Branch: "pr/me/fix", Session: "repo@fix"}
	if got != want {
		t.Fatalf("got %#v\nwant %#v", got, want)
	}
	// An existing branch is checked out rather than created, wherever it is put.
	if got, err = CreateWorktree(space, "old", filepath.Join(root, "elsewhere")); err != nil {
		t.Fatal(err)
	}
	if want := (Worktree{Path: filepath.Join(root, "elsewhere"), Branch: "old", Session: "repo@elsewhere"}); got != want {
		t.Fatalf("got %#v\nwant %#v", got, want)
	}
	if ws := LoadSpace(root).Worktrees; len(ws) != 3 || ws[1].Branch != "pr/me/fix" || ws[2].Branch != "old" {
		t.Fatalf("git lists %#v", ws)
	}
	// git's own refusal comes back as the error: the directory is taken.
	if _, err := CreateWorktree(space, "new", filepath.Join(root, "elsewhere")); err == nil {
		t.Fatal("a second worktree in the same directory was created")
	}
	for _, branch := range []string{"two words", "pr/me/", " "} {
		if _, err := CreateWorktree(space, branch, filepath.Join(root, "bad")); err == nil {
			t.Fatalf("branch %q was accepted", branch)
		}
	}
}

func TestOtherSessions(t *testing.T) {
	spaces := []Space{{Name: "gr", Worktrees: []Worktree{
		{Path: "/src/gr", Session: "gr"},
		{Path: "/src/gr/.worktrees/dev", Session: "gr@dev"},
	}}}
	open := map[string]int{"gr": 1, "default": 2, "scratch": 3}
	saved := []string{"default", "gr@dev", "old"}

	got := otherSessions(spaces, open, saved)
	want := []Space{
		{Name: "default", Worktrees: []Worktree{{Session: "default"}}},
		{Name: "old", Worktrees: []Worktree{{Session: "old"}}},
		{Name: "scratch", Worktrees: []Worktree{{Session: "scratch"}}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v\nwant %#v", got, want)
	}
}

func TestOpenActions(t *testing.T) {
	home, dir := t.TempDir(), t.TempDir()
	t.Setenv("HOME", home)
	files := map[string]string{
		"gr":     "new_tab\ncd /src/gr\nlaunch\n",
		"gr@dev": "",
		// What save_sessions.py leaves for the default session once its last window has closed.
		"default": "",
		"old":     "new_tab\nlayout tall\n",
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name+".kitty-session"), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	cfg := config.Config{NewTab: []string{"launch", "--type=tab"}, SessionsDir: dir}
	st := kitty.State{Sessions: map[string]int{"gr@fix": 7}}
	file := func(name string) string { return filepath.Join(dir, name+".kitty-session") }
	newTab := func(cwd, session string) []string {
		return []string{"launch", "--type=tab", "--cwd=" + cwd, "--add-to-session=" + session}
	}
	for _, c := range []struct {
		cfg  config.Config
		wt   Worktree
		want [][]string
	}{
		{cfg, Worktree{Path: "/src/gr/.worktrees/fix", Session: "gr@fix"}, [][]string{{"goto_session", file("gr@fix")}}},
		{cfg, Worktree{Path: "/src/gr", Session: "gr"}, [][]string{{"goto_session", file("gr")}}},
		// A file kitty would open no window from is not restored: goto_session does nothing with it.
		{cfg, Worktree{Path: "/src/gr/.worktrees/dev", Session: "gr@dev"}, [][]string{newTab("/src/gr/.worktrees/dev", "gr@dev"), {"goto_session", file("gr@dev")}}},
		{cfg, Worktree{Path: "/src/gr/.worktrees/new", Session: "gr@new"}, [][]string{newTab("/src/gr/.worktrees/new", "gr@new"), {"goto_session", file("gr@new")}}},
		// A session in no space starts in the home directory.
		{cfg, Worktree{Session: "default"}, [][]string{newTab(home, "default"), {"goto_session", file("default")}}},
		{cfg, Worktree{Session: "old"}, [][]string{newTab(home, "old"), {"goto_session", file("old")}}},
		// Without a sessions directory nothing is restored, and the file only names the session.
		{config.Config{NewTab: cfg.NewTab}, Worktree{Path: "/src/gr", Session: "gr"}, [][]string{newTab("/src/gr", "gr"), {"goto_session", "gr.kitty-session"}}},
	} {
		got, err := openActions(c.cfg, st, c.wt)
		if err != nil {
			t.Errorf("%s: %v", c.wt.Session, err)
		} else if !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: got %q\nwant %q", c.wt.Session, got, c.want)
		}
	}
}

func TestCycleIndex(t *testing.T) {
	got := []int{
		cycleIndex(3, 0, 1),
		cycleIndex(3, 2, 1),
		cycleIndex(3, 0, -1),
		cycleIndex(3, -1, 1),
		cycleIndex(3, -1, -1),
		cycleIndex(1, 0, 1),
	}
	if want := []int{1, 0, 2, 0, 2, 0}; !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

// Stepping through what needs me goes blocked, failed, then done, the most recent change first in each,
// past working and idle, wrapping around; from a window that is none of them it starts at either end.
func TestAttentionStep(t *testing.T) {
	at := func(min int) time.Time { return time.Unix(int64(min)*60, 0) }
	statuses := []status.Status{
		{Window: 1, Record: status.Record{State: "done", Since: at(5)}},
		{Window: 2, Record: status.Record{State: "working", Since: at(9)}},
		{Window: 3, Record: status.Record{State: "error", Since: at(1)}},
		{Window: 4, Record: status.Record{State: "blocked", Since: at(2)}},
		{Window: 5, Record: status.Record{State: "idle", Since: at(9)}},
		{Window: 6, Record: status.Record{State: "done", Since: at(7)}},
		{Window: 7, Record: status.Record{State: "blocked", Since: at(3)}},
	}
	var order []int
	for _, s := range Attention(statuses) {
		order = append(order, s.Window)
	}
	if want := []int{7, 4, 3, 6, 1}; !reflect.DeepEqual(order, want) {
		t.Errorf("order %v, want %v", order, want)
	}
	step := func(current, delta int) int {
		s, ok := attentionStep(statuses, current, delta)
		if !ok {
			return 0
		}
		return s.Window
	}
	got := []int{step(7, 1), step(3, 1), step(1, 1), step(7, -1), step(2, 1), step(2, -1), step(0, 1)}
	if want := []int{4, 6, 7, 1, 7, 1, 7}; !reflect.DeepEqual(got, want) {
		t.Errorf("steps %v, want %v", got, want)
	}
	// Nothing to attend to: working and idle only, or no statuses at all.
	if _, ok := attentionStep(statuses[1:2], 2, 1); ok {
		t.Error("a working window needs me")
	}
	if _, ok := attentionStep(nil, 0, -1); ok {
		t.Error("no statuses gave a window")
	}
}

// NewSpaceRoot adds the top of the work tree a path is in, ~/ expanded, and says why it cannot add a path
// that is no directory, in no repository, or a space already, by any name.
func TestNewSpaceRoot(t *testing.T) {
	home, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	repo, plain := filepath.Join(home, "repo"), filepath.Join(home, "plain")
	for _, dir := range []string{filepath.Join(repo, "sub"), plain} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(home, "file"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	initRepo := exec.Command("git", "-C", repo, "init", "-q")
	initRepo.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null")
	if out, err := initRepo.CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
	link := filepath.Join(home, "link")
	if err := os.Symlink(repo, link); err != nil {
		t.Fatal(err)
	}
	// A linked worktree, outside the repository, as one in another directory would be.
	wt := filepath.Join(home, "wt")
	for _, args := range [][]string{{"commit", "-q", "--allow-empty", "-m", "first"}, {"worktree", "add", "-q", "-b", "dev", wt}} {
		cmd := exec.Command("git", append([]string{"-C", repo}, args...)...)
		cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t",
			"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %s: %v\n%s", args[0], err, out)
		}
	}

	type result struct{ Root, Err string }
	check := func(roots []string, paths ...string) []result {
		var out []result
		for _, p := range paths {
			root, err := NewSpaceRoot(roots, p)
			r := result{Root: root}
			if err != nil {
				r.Err = err.Error()
			}
			out = append(out, r)
		}
		return out
	}
	// A linked worktree adds its repository: spaces are repositories, and their worktrees come with them.
	got := check([]string{plain}, repo, " ~/repo/sub ", link, "~/wt", "", "~/nothing", "~/file", plain)
	want := []result{
		{Root: repo},
		{Root: repo},
		{Root: repo},
		{Root: repo},
		{Err: "name a repository to add"},
		{Err: "~/nothing is not a directory"},
		{Err: "~/file is not a directory"},
		{Err: plain + " is not in a git repository"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %q\nwant %q", got, want)
	}
	// A space configured through a symlink is the same repository.
	got = check([]string{link}, filepath.Join(repo, "sub"), wt)
	if want := []result{{Err: "~/repo is already a space"}, {Err: "~/repo is already a space"}}; !reflect.DeepEqual(got, want) {
		t.Errorf("got %q, want %q", got, want)
	}
}
