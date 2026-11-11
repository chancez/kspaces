package tui

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/chancez/kspaces/internal/config"
	"github.com/chancez/kspaces/internal/gittest"
	"github.com/chancez/kspaces/internal/spaces"
)

// fakeGH puts a gh first on PATH that answers `gh pr view N` with prs[N] and fails as gh does for any other
// PR.
func fakeGH(t *testing.T, prs map[string]string) {
	t.Helper()
	dir := t.TempDir()
	for n, js := range prs {
		if err := os.WriteFile(filepath.Join(dir, "pr-"+n+".json"), []byte(js), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	script := "#!/bin/sh\nn=${3##*/pull/}; n=${n%%/*}\nf=" + dir + "/pr-$n.json\n" +
		"[ -f \"$f\" ] || { echo \"GraphQL: Could not resolve to a PullRequest with the number of $n. (repository.pullRequest)\" >&2; exit 1; }\n" +
		"cat \"$f\"\n"
	if err := os.WriteFile(filepath.Join(dir, "gh"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

// mainRepo is a repository with a commit on main.
func mainRepo(t *testing.T) string {
	t.Helper()
	r := gittest.New(t)
	r.Commit("first")
	return r.Dir
}

// lookUpPR lets the form's debounce run out and its lookup finish, as the program would.
func lookUpPR(t *testing.T, f *newWorktreeForm) {
	t.Helper()
	_, cmd := f.Update(prDueMsg{f.prSeq, f.pr})
	if cmd == nil {
		t.Fatal("the PR was not looked up")
	}
	f.Update(cmd())
}

// The branch field takes a PR, after the prefix too: the form looks it up once the field rests on it, and
// shows its title and the branch it checks out, which the directory follows, or why it cannot. The fork's
// main clashes with the space's.
func TestNewWorktreeFormPR(t *testing.T) {
	root := mainRepo(t)
	fakeGH(t, map[string]string{
		"3": `{"headRefName":"feat","headRepositoryOwner":{"login":"o"},"isCrossRepository":false,"number":3,"title":"Add feat","url":"https://github.com/o/r/pull/3"}`,
		"7": `{"headRefName":"main","headRepositoryOwner":{"login":"forker"},"isCrossRepository":true,"number":7,"title":"Fix from a fork","url":"https://github.com/o/r/pull/7"}`,
	})
	space := spaces.Space{Name: "gr", Root: root, Worktrees: []spaces.Worktree{{Path: root, Branch: "main", Session: "gr"}}}
	f := newForm(config.Config{Worktrees: config.Worktrees{Dir: ".worktrees", BranchPrefix: "pr/me"}}, space)
	f.Update(tea.WindowSizeMsg{Width: 60, Height: 20})
	screen := func(branch, dir string, says ...string) []string {
		lines := []string{"", "  New worktree in gr", "", "  branch     " + branch, "  directory  " + dir, ""}
		for _, s := range says {
			if s != "" {
				s = "  " + s
			}
			lines = append(lines, s)
		}
		return append(lines, "", "   Create enter    Cancel esc   tab next field", "")
	}

	typeInto(f, "#7")
	if got, want := formScreen(f), screen("pr/me/#7", ".worktrees", "Looking up PR #7..."); !reflect.DeepEqual(got, want) {
		t.Errorf("before the lookup:\n%q\nwant\n%q", got, want)
	}
	// A lookup the field did not rest on, as typing went on, starts nothing.
	if _, cmd := f.Update(prDueMsg{f.prSeq - 1, f.pr}); cmd != nil {
		t.Error("a lookup of a PR typed past started")
	}
	lookUpPR(t, f)
	if got, want := formScreen(f), screen("pr/me/#7", ".worktrees/pr-7-main", "PR #7 Fix from a fork", "Checks out forker:main from origin as pr-7-main"); !reflect.DeepEqual(got, want) {
		t.Errorf("a PR from a fork:\n%q\nwant\n%q", got, want)
	}
	if branch, dir := f.plan(); branch != "pr-7-main" || dir != filepath.Join(root, ".worktrees", "pr-7-main") {
		t.Errorf("plan %q in %q", branch, dir)
	}

	// A number alone, from the repository itself.
	f.Update(keyMsg("ctrl+u"))
	typeInto(f, "3")
	lookUpPR(t, f)
	if got, want := formScreen(f), screen("3", ".worktrees/feat", "PR #3 Add feat", "Checks out feat from origin, tracking it"); !reflect.DeepEqual(got, want) {
		t.Errorf("a PR from the repository:\n%q\nwant\n%q", got, want)
	}

	// gh's refusal shows as git's does.
	f.Update(keyMsg("ctrl+u"))
	typeInto(f, "https://github.com/o/r/pull/404")
	lookUpPR(t, f)
	want := screen("https://github.com/o/r/pull/404", ".worktrees", "Could not look up PR #404", "",
		"GraphQL: Could not resolve to a PullRequest with the", "number of 404. (repository.pullRequest)")
	if got := formScreen(f); !reflect.DeepEqual(got, want) {
		t.Errorf("a PR gh cannot find:\n%q\nwant\n%q", got, want)
	}

	// Back to a PR looked up already: shown at once, with no second lookup.
	f.Update(keyMsg("ctrl+u"))
	typeInto(f, "#7")
	if _, cmd := f.Update(prDueMsg{f.prSeq, f.pr}); cmd != nil {
		t.Error("a PR was looked up twice")
	}
	if got := formScreen(f); !strings.Contains(strings.Join(got, "\n"), "PR #7 Fix from a fork") {
		t.Errorf("a PR looked up already:\n%s", strings.Join(got, "\n"))
	}
}

// With commands configured the form chooses one, none at first, and has a field for each of its inputs;
// tab goes through them all, left and right or a click change the command. A required input left empty
// refuses to create. Created, the worktree's first tab runs the command: kspaces sends kitty launch with
// the KSPACES_ and the command's env, --hold and the command, each word quoted, then switches to it.
func TestNewWorktreeFormCommand(t *testing.T) {
	root := mainRepo(t)
	dir := t.TempDir()
	log := filepath.Join(dir, "calls")
	script := "#!/bin/sh\n{ for a; do printf '%s\\n' \"$a\"; done; echo --; } >> " + log + "\n"
	if err := os.WriteFile(filepath.Join(dir, "kitten"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("KITTY_INSTALLATION_DIR", "")

	cfg, err := config.Parse(`
[worktrees]
branch_prefix = "pr/me"
[[commands]]
name = "claude"
run = ["claude", "{prompt}"]
env = { TASK = "{prompt}" }
[[commands.inputs]]
name = "prompt"
label = "Prompt"
required = true
[[commands]]
name = "gr"
run = ["gr"]
`)
	if err != nil {
		t.Fatal(err)
	}
	space := spaces.Space{Name: "gr", Root: root, Worktrees: []spaces.Worktree{{Path: root, Branch: "main", Session: "gr"}}}
	f := newForm(cfg, space)
	f.Update(tea.WindowSizeMsg{Width: 70, Height: 20})
	screen := func(fields []string, hint string, says ...string) []string {
		lines := append([]string{"", "  New worktree in gr", "", "  branch     pr/me/fix", "  directory  .worktrees/fix"}, fields...)
		lines = append(lines, "", "  Creates pr/me/fix from main")
		for _, s := range says {
			if s != "" {
				s = "  " + s
			}
			lines = append(lines, s)
		}
		return append(lines, "", "   Create enter    Cancel esc   tab next field"+hint, "")
	}
	typeInto(f, "fix")
	if got, want := formScreen(f), screen([]string{"  command    < none >"}, ""); !reflect.DeepEqual(got, want) {
		t.Errorf("form:\n%q\nwant\n%q", got, want)
	}

	f.Update(keyMsg("tab"))
	f.Update(keyMsg("tab"))
	f.Update(tea.KeyPressMsg{Code: tea.KeyRight})
	want := screen([]string{"  command    < claude >", "  Prompt     required"}, "  left/right command", "Runs claude in the first tab")
	if got := formScreen(f); !reflect.DeepEqual(got, want) {
		t.Errorf("claude chosen:\n%q\nwant\n%q", got, want)
	}
	// Round through gr and none, by key and by click, back to claude.
	f.Update(tea.KeyPressMsg{Code: tea.KeyRight})
	if got, want := formScreen(f), screen([]string{"  command    < gr >"}, "  left/right command", "Runs gr in the first tab"); !reflect.DeepEqual(got, want) {
		t.Errorf("gr chosen:\n%q\nwant\n%q", got, want)
	}
	clickOn(t, f, "< gr >")
	f.Update(tea.KeyPressMsg{Code: tea.KeyLeft})
	f.Update(tea.KeyPressMsg{Code: tea.KeyLeft})
	if c, _ := f.cmds.chosen(); c.Name != "claude" {
		t.Fatalf("chose %q, want claude", c.Name)
	}

	if _, cmd := f.Update(keyMsg("enter")); cmd != nil || f.busy {
		t.Fatal("created with the prompt empty")
	}
	want = screen([]string{"  command    < claude >", "  Prompt     required"}, "  left/right command", "Runs claude in the first tab", "", "claude needs Prompt")
	if got := formScreen(f); !reflect.DeepEqual(got, want) {
		t.Errorf("prompt missing:\n%q\nwant\n%q", got, want)
	}

	f.Update(keyMsg("tab"))
	typeInto(f, `fix "it" in $HOME`)
	_, cmd := f.Update(keyMsg("enter"))
	if cmd == nil {
		t.Fatal("enter created nothing")
	}
	if _, quit := f.Update(cmd()); quit == nil || f.err != nil {
		t.Fatalf("the form stayed: %v", f.err)
	}
	data, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	wt := filepath.Join(root, ".worktrees", "fix")
	if got, want := strings.Split(strings.TrimSuffix(string(data), "\n"), "\n"), []string{
		"@", "action", "launch", "--type=tab", "--cwd=" + wt, "--add-to-session=gr@fix",
		"--env=KSPACES_SPACE=gr", "--env=KSPACES_ROOT=" + root, "--env=KSPACES_WORKTREE=" + wt, "--env=KSPACES_BRANCH=pr/me/fix",
		"--env=KSPACES_BASE=main", "--env=KSPACES_SESSION=gr@fix", "--env=KSPACES_PR=",
		`'--env=KSPACES_INPUT_PROMPT=fix "it" in $$$$HOME'`, `'--env=TASK=fix "it" in $$$$HOME'`,
		"--hold", "claude", `'fix "it" in $$HOME'`, "--",
		"@", "action", "goto_session", "gr@fix.kitty-session", "--",
	}; !reflect.DeepEqual(got, want) {
		t.Errorf("kitten ran as\n%q\nwant\n%q", got, want)
	}
}
