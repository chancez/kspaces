package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"
	"time"
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
	got := parseWorktrees("gr", porcelain)
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
func TestParseWorktreesBare(t *testing.T) {
	porcelain := `worktree /src/gr.git
bare

worktree /src/gr/main
HEAD 1111111111111111111111111111111111111111
branch refs/heads/main
`
	got := parseWorktrees("gr", porcelain)
	want := []Worktree{{Path: "/src/gr/main", Branch: "main", Session: "gr"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v\nwant %#v", got, want)
	}
}

func TestKittyStateFrom(t *testing.T) {
	dock := map[string]string{dockVar: "1"}
	data := []lsOSWindow{{Tabs: []lsTab{
		{ID: 1, Windows: []lsWindow{
			{ID: 10, SessionName: "gr", LastFocusedAt: 5},
			{ID: 11, SessionName: "gr", LastFocusedAt: 9},
			{ID: 12, UserVars: dock},
		}},
		{ID: 2, Windows: []lsWindow{
			{ID: 20, SessionName: "gr", LastFocusedAt: 7},
			{ID: 21, SessionName: "kitty@docks", LastFocusedAt: 1},
			// A dock never counts as a session's window, even if it was saved with one.
			{ID: 22, SessionName: "kitty@docks", LastFocusedAt: 99, UserVars: dock},
		}},
		{ID: 3, Windows: []lsWindow{{ID: 30}}},
	}}}

	got := kittyStateFrom(data, 12)
	// Home is the tab's most recently focused window that is not a dock.
	want := KittyState{Sessions: map[string]int{"gr": 11, "kitty@docks": 21}, Current: "gr", Home: 11}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v\nwant %#v", got, want)
	}

	// A sidebar in a tab with no session highlights nothing.
	got = kittyStateFrom(data, 30)
	want.Current, want.Home = "", 30
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v\nwant %#v", got, want)
	}
}

func TestParseConfig(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	in := `
spaces = ["~/projects/gr", "/src/kitty"]
new_tab = ["kitten", "my_launch.py", "--type=tab"]
sessions = "~/.local/share/kitty/sessions"

[worktrees]
branch_prefix = "pr/me"
`
	got, err := parseConfig(in)
	if err != nil {
		t.Fatal(err)
	}
	// worktrees.dir is left out, so it has its default.
	want := Config{
		Spaces:      []string{filepath.Join(home, "projects/gr"), "/src/kitty"},
		NewTab:      []string{"kitten", "my_launch.py", "--type=tab"},
		SessionsDir: filepath.Join(home, ".local/share/kitty/sessions"),
		Worktrees:   Worktrees{Dir: ".worktrees", BranchPrefix: "pr/me"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v\nwant %#v", got, want)
	}

	// Nothing set gives the defaults.
	got, err = parseConfig("")
	if err != nil {
		t.Fatal(err)
	}
	if want := (Config{NewTab: defaultNewTab, Worktrees: Worktrees{Dir: ".worktrees"}}); !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v\nwant %#v", got, want)
	}

	for _, bad := range []string{
		`space = ["~/x"]`,                 // a typo of spaces
		"[worktrees]\nprefix = \"pr/me\"", // a typo inside a table
		`new_tab = []`,
		`spaces = "~/x"`, // a string, not a list
	} {
		if _, err := parseConfig(bad); err == nil {
			t.Fatalf("%q was accepted", bad)
		}
	}
}

// rowNames shows rows as their worktree's session, with "" for the blank rows between groups.
func rowNames(rows []row) []string {
	var ans []string
	for _, r := range rows {
		switch {
		case r.space == nil:
			ans = append(ans, "")
		case r.folded:
			ans = append(ans, r.worktree.Session+" (folded)")
		default:
			ans = append(ans, r.worktree.Session)
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
	got := rowNames(buildRows(others, spaces, map[string]bool{"/src/docs": true, "/src/kitty": true}))
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
	cfg := Config{Worktrees: Worktrees{Dir: ".worktrees", BranchPrefix: "pr/me"}}
	got := []string{
		// The default drops the prefix and turns / into -.
		worktreeDir(cfg, "/src/gr", "pr/me/fix/bug", ""),
		// A branch without the prefix keeps all of its name.
		worktreeDir(cfg, "/src/gr", "release/1.0", ""),
		// A given directory: relative to the space, absolute, or under the home directory.
		worktreeDir(cfg, "/src/gr", "pr/me/x", "../gr-x"),
		worktreeDir(cfg, "/src/gr", "pr/me/x", "/tmp/x/"),
		worktreeDir(cfg, "/src/gr", "pr/me/x", "~/wt/x"),
	}
	want := []string{"/src/gr/.worktrees/fix-bug", "/src/gr/.worktrees/release-1.0", "/src/gr-x", "/tmp/x", filepath.Join(home, "wt/x")}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %q\nwant %q", got, want)
	}
	// A trailing / in the prefix is the same prefix.
	starts := []string{branchStart(cfg), branchStart(Config{Worktrees: Worktrees{BranchPrefix: "pr/me/"}}), branchStart(Config{})}
	if want := []string{"pr/me/", "pr/me/", ""}; !reflect.DeepEqual(starts, want) {
		t.Fatalf("got %q, want %q", starts, want)
	}
}

func TestCreateWorktree(t *testing.T) {
	git := func(dir string, args ...string) {
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
	git(root, "init", "-q", "-b", "main")
	git(root, "commit", "-q", "--allow-empty", "-m", "first")
	git(root, "branch", "old")
	space := Space{Name: "repo", Root: root}

	// A new branch.
	got, err := createWorktree(space, "pr/me/fix", filepath.Join(root, ".worktrees", "fix"))
	if err != nil {
		t.Fatal(err)
	}
	want := Worktree{Path: filepath.Join(root, ".worktrees", "fix"), Branch: "pr/me/fix", Session: "repo@fix"}
	if got != want {
		t.Fatalf("got %#v\nwant %#v", got, want)
	}
	// An existing branch is checked out rather than created, wherever it is put.
	if got, err = createWorktree(space, "old", filepath.Join(root, "elsewhere")); err != nil {
		t.Fatal(err)
	}
	if want := (Worktree{Path: filepath.Join(root, "elsewhere"), Branch: "old", Session: "repo@elsewhere"}); got != want {
		t.Fatalf("got %#v\nwant %#v", got, want)
	}
	if ws := loadSpace(root).Worktrees; len(ws) != 3 || ws[1].Branch != "pr/me/fix" || ws[2].Branch != "old" {
		t.Fatalf("git lists %#v", ws)
	}
	// git's own refusal comes back as the error: the directory is taken.
	if _, err := createWorktree(space, "new", filepath.Join(root, "elsewhere")); err == nil {
		t.Fatal("a second worktree in the same directory was created")
	}
	for _, branch := range []string{"two words", "pr/me/", " "} {
		if _, err := createWorktree(space, branch, filepath.Join(root, "bad")); err == nil {
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

func TestStatuses(t *testing.T) {
	at := func(sec int64) time.Time { return time.Unix(sec, 0) }
	data := []lsOSWindow{{Tabs: []lsTab{
		{ID: 1, Windows: []lsWindow{
			// Several records: the most urgent one stands for the window. A noted time counts only for the
			// state it was noted for: build has changed to working since.
			{
				ID: 10, SessionName: "gr",
				UserVars: map[string]string{statusAtVar: `{"pid": 1, "records": {"deploy": ["blocked", 100], "build": ["blocked", 50]}}`},
				ProgramStatus: []programRecord{
					{ID: "build", State: "working"},
					{ID: "deploy", State: "blocked", App: "claude", Title: "approve"},
				},
			},
			// Idle is listed: the program is waiting for its next task.
			{ID: 11, SessionName: "gr", ProgramStatus: []programRecord{{State: "idle", App: "claude"}}},
			// A broken var only loses the times.
			{ID: 12, SessionName: "gr", UserVars: map[string]string{statusAtVar: "{"}, ProgramStatus: []programRecord{{State: "done", App: "go"}}},
		}},
		{ID: 2, Windows: []lsWindow{
			{ID: 20, SessionName: "kitty", UserVars: map[string]string{statusAtVar: `{"records": {"": ["done", 300]}}`}, ProgramStatus: []programRecord{{State: "done", App: "make"}}},
			{ID: 21, Title: "scratch", ProgramStatus: []programRecord{{State: "error", Msg: "exit 1"}}},
			{ID: 22, SessionName: "kitty", UserVars: map[string]string{dockVar: "1"}, ProgramStatus: []programRecord{{State: "blocked"}}},
		}},
	}}}

	got := kittyStateFrom(data, 0).Statuses
	// Grouped by state, most urgent first, and within a state the most recent change first: the done
	// record noted at 300 comes before the one with no time.
	want := []Status{
		{
			Window: 10, Session: "gr",
			programRecord: programRecord{ID: "deploy", State: "blocked", App: "claude", Title: "approve", Since: at(100)},
			Others:        []programRecord{{ID: "build", State: "working"}},
		},
		{Window: 21, WindowTitle: "scratch", programRecord: programRecord{State: "error", Msg: "exit 1"}},
		{Window: 20, Session: "kitty", programRecord: programRecord{State: "done", App: "make", Since: at(300)}},
		{Window: 12, Session: "gr", programRecord: programRecord{State: "done", App: "go"}},
		{Window: 11, Session: "gr", programRecord: programRecord{State: "idle", App: "claude"}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v\nwant %#v", got, want)
	}

	gotStates := []string{sessionStatus(got, "gr"), sessionStatus(got, "kitty"), sessionStatus(got, "docs")}
	if wantStates := []string{"blocked", "done", ""}; !reflect.DeepEqual(gotStates, wantStates) {
		t.Fatalf("session states: got %q, want %q", gotStates, wantStates)
	}
}

func TestAge(t *testing.T) {
	now := time.Unix(1_000_000, 0)
	got := []string{
		age(time.Time{}, now),
		age(now.Add(-59*time.Second), now),
		age(now.Add(-61*time.Second), now),
		age(now.Add(-5*time.Hour-59*time.Minute), now),
		age(now.Add(-49*time.Hour), now),
	}
	want := []string{"", "<1m", "1m", "5h", "2d"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestWrap(t *testing.T) {
	got := wrap("run the full test suite\nthen deploy supercalifragilistic", 10)
	want := []string{"run the", "full test", "suite", "then", "deploy", "supercalif", "ragilistic"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %q\nwant %q", got, want)
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

func TestKeyboardWindow(t *testing.T) {
	tabs := func(active int) []lsTab {
		return []lsTab{
			{ID: 1, IsActive: active == 1, Windows: []lsWindow{{ID: 10, IsActive: true}, {ID: 11}}},
			{ID: 2, IsActive: active == 2, Windows: []lsWindow{{ID: 20}, {ID: 21, IsActive: true}}},
		}
	}
	unfocused := []lsOSWindow{{Tabs: tabs(1)}, {LastFocused: true, Tabs: tabs(2)}}
	focused := []lsOSWindow{{IsFocused: true, Tabs: tabs(1)}, {LastFocused: true, Tabs: tabs(2)}}
	got := []int{keyboardWindow(unfocused), keyboardWindow(focused), keyboardWindow(nil)}
	// Without a focused OS window, the one focused last stands in.
	if want := []int{21, 10, 0}; !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}
