package tui

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/chancez/kspaces/internal/config"
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
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"init", "-q", "-b", "main"}, {"commit", "-q", "--allow-empty", "-m", "first"}} {
		git := exec.Command("git", append([]string{"-C", root}, args...)...)
		git.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t",
			"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
		if out, err := git.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
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
