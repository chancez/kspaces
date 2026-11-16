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

// fakeGH puts a gh first on PATH that answers `gh pr list` with the PRs of open, a JSON list, and `gh pr
// view N` with prs[N], and fails as gh does for any other PR; fakeGHIssues does the same for `gh issue`.
func fakeGH(t *testing.T, open string, prs map[string]string) {
	t.Helper()
	writeGH(t, "pr", open, prs)
}

func fakeGHIssues(t *testing.T, open string, issues map[string]string) {
	t.Helper()
	writeGH(t, "issue", open, issues)
}

func writeGH(t *testing.T, kind, open string, views map[string]string) {
	t.Helper()
	dir := t.TempDir()
	files := map[string]string{kind + "-open.json": open}
	for n, js := range views {
		files[kind+"-"+n+".json"] = js
	}
	for name, js := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(js), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	script := "#!/bin/sh\n[ \"$2\" = list ] && exec cat " + dir + "/$1-open.json\n" +
		"case $1 in pr) n=${3##*/pull/} what='a PullRequest' field=pullRequest ;; *) n=${3##*/issues/} what='an issue or pull request' field=issue ;; esac\n" +
		"n=${n%%[/#?]*}\nf=" + dir + "/$1-$n.json\n" +
		"[ -f \"$f\" ] || { echo \"GraphQL: Could not resolve to $what with the number of $n. (repository.$field)\" >&2; exit 1; }\n" +
		"cat \"$f\"\n"
	if err := os.WriteFile(filepath.Join(dir, "gh"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

// openPRs are two open PRs, from the repository itself and from a fork, as gh pr list says them, and a
// closed one gh pr view knows.
const (
	feat3 = `{"author":{"login":"me"},"headRefName":"feat","headRepositoryOwner":{"login":"o"},"isCrossRepository":false,"isDraft":false,"number":3,"title":"Add feat","url":"https://github.com/o/r/pull/3"}`
	fork7 = `{"author":{"login":"forker"},"headRefName":"main","headRepositoryOwner":{"login":"forker"},"isCrossRepository":true,"isDraft":true,"number":7,"title":"Fix from a fork","url":"https://github.com/o/r/pull/7"}`
	old99 = `{"author":{"login":"me"},"headRefName":"old","headRepositoryOwner":{"login":"o"},"isCrossRepository":false,"isDraft":false,"number":99,"title":"Closed one","url":"https://github.com/o/r/pull/99"}`
)

// Two open issues, as gh issue list says them, and a closed one gh issue view knows.
const (
	crash12 = `{"author":{"login":"you"},"number":12,"title":"Crash on empty input","url":"https://github.com/o/r/issues/12"}`
	docs9   = `{"author":{"login":"me"},"number":9,"title":"Docs","url":"https://github.com/o/r/issues/9"}`
	old40   = `{"author":{"login":"me"},"number":40,"title":"Old one","url":"https://github.com/o/r/issues/40"}`
)

// mainRepo is a repository with a commit on main.
func mainRepo(t *testing.T) string {
	t.Helper()
	r := gittest.New(t)
	r.Commit("first")
	return r.Dir
}

// listPRs lists the open PRs as the PR tab does the first time it shows, and lookUpPR looks up the PR
// chosen, as the program would run what the form asks for; listOpen and lookUpChosen do the same for
// any picker.
func listPRs(f *newWorktreeForm)                { listOpen(f, f.prs) }
func lookUpPR(t *testing.T, f *newWorktreeForm) { t.Helper(); lookUpChosen(t, f, f.prs) }

func listOpen[V, L any](f *newWorktreeForm, p *ghPicker[V, L]) {
	p.open.asked = false
	f.Update(p.listOpen()())
}

func lookUpChosen[V, L any](t *testing.T, f *newWorktreeForm, p *ghPicker[V, L]) {
	t.Helper()
	delete(p.lookups, p.chosen.Arg)
	cmd := p.lookUp()
	if cmd == nil {
		t.Fatal("nothing chosen to look up")
	}
	f.Update(cmd())
}

// The PR tab's field takes a PR's number, URL or title: while it has focus the open PRs it matches show
// below it, the one of its number first, up and down going through them, and the one highlighted is the
// PR chosen, the first at first; a number no open PR has is looked up with gh. The form shows the PR's
// title and the branch it checks out, which the directory follows, or why it cannot; without focus the
// field shows the PR chosen. Typing # in the branch field goes to it, and a PR's URL pasted there chooses
// it. The fork's main clashes with the space's.
func TestNewWorktreeFormPR(t *testing.T) {
	root := mainRepo(t)
	fakeGH(t, "["+feat3+","+fork7+"]", map[string]string{"99": old99})
	space := spaces.Space{Name: "gr", Root: root, Worktrees: []spaces.Worktree{{Path: root, Branch: "main", Session: "gr"}}}
	f := newForm(config.Config{Worktrees: config.Worktrees{Dir: ".worktrees", BranchPrefix: "pr/me"}}, space)
	f.Update(tea.WindowSizeMsg{Width: 70, Height: 24})
	keys := "   Create enter    Cancel esc   tab next field"
	row := func(s string) string { return "              " + s }
	screen := func(pr []string, dir string, says ...string) []string {
		lines := append([]string{"", "  New worktree in gr", "", "   Branch   Pull request   Issue", ""}, pr...)
		lines = append(lines, "  directory  "+dir, "")
		for _, s := range says {
			if s != "" {
				s = "  " + s
			}
			lines = append(lines, s)
		}
		return append(lines, "", keys, "")
	}
	type result struct {
		Tab    int
		Branch string
		PR     int
		Screen []string
	}

	// # goes to the PR tab, the open PRs below its field once listed, the branch field back as it was.
	typeInto(f, "#")
	listPRs(f)
	lookUpPR(t, f)
	got := result{f.tab, f.branch.Value(), f.prs.chosen.Number, formScreen(f)[5:8]}
	want := result{tabPR, "pr/me/", 3, []string{"  PR         number, URL or title", row("#3 Add feat  me"), row("#7 Fix from a fork  forker, draft")}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("after #:\n%+v\nwant\n%+v", got, want)
	}
	if got, want := formScreen(f)[len(formScreen(f))-2], keys+"  up/down PR"; got != want {
		t.Errorf("keys %q, want %q", got, want)
	}
	typeInto(f, "7")
	lookUpPR(t, f)
	if got, want := formScreen(f), screen([]string{"  PR         7", row("#7 Fix from a fork  forker, draft")}, ".worktrees/pr-7-main",
		"PR #7 Fix from a fork", "Checks out forker:main from origin as pr-7-main"); !reflect.DeepEqual(got, want) {
		t.Errorf("a PR from a fork:\n%q\nwant\n%q", got, want)
	}
	if branch, dir := f.plan(); branch != "pr-7-main" || dir != filepath.Join(root, ".worktrees", "pr-7-main") {
		t.Errorf("plan %q in %q", branch, dir)
	}

	// down goes to the next of what the field matches, by title as well.
	f.Update(keyMsg("ctrl+u"))
	typeInto(f, "a")
	f.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	lookUpPR(t, f)
	if l, _ := f.prLookup(); f.prs.chosen.Number != 7 || l.found.Branch != "pr-7-main" {
		t.Errorf("down chose PR %d, its branch %q", f.prs.chosen.Number, l.found.Branch)
	}

	// A number no open PR has is looked up; once the field is left it shows the PR's title.
	f.Update(keyMsg("ctrl+u"))
	typeInto(f, "99")
	lookUpPR(t, f)
	f.Update(keyMsg("tab"))
	if got, want := formScreen(f), screen([]string{"  PR         #99 Closed one"}, ".worktrees/old", "PR #99 Closed one", "Checks out old from origin, tracking it"); !reflect.DeepEqual(got, want) {
		t.Errorf("a closed PR:\n%q\nwant\n%q", got, want)
	}

	// gh's refusal shows as git's does.
	f.Update(tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift})
	f.Update(keyMsg("ctrl+u"))
	typeInto(f, "404")
	lookUpPR(t, f)
	if got, want := formScreen(f)[5:12], []string{"  PR         404", "  directory  .worktrees", "", "  Could not look up PR #404", "",
		"  GraphQL: Could not resolve to a PullRequest with the number of", "  404. (repository.pullRequest)"}; !reflect.DeepEqual(got, want) {
		t.Errorf("a PR gh cannot find:\n%q\nwant\n%q", got, want)
	}

	// Back on the branch tab, a PR's URL pasted chooses it, the open one as listed.
	f.Update(tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift})
	f.Update(tea.KeyPressMsg{Code: tea.KeyLeft})
	f.Update(keyMsg("tab"))
	f.Update(tea.PasteMsg{Content: "https://github.com/o/r/pull/7/files"})
	lookUpPR(t, f)
	got = result{f.tab, f.branch.Value(), f.prs.chosen.Number, formScreen(f)[5:7]}
	want = result{tabPR, "pr/me/", 7, []string{"  PR         https://github.com/o/r/pull/7/files", row("#7 Fix from a fork  forker, draft")}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("a URL pasted:\n%+v\nwant\n%+v", got, want)
	}
}

// A click on one of the open PRs below the PR field chooses it, fills the field in with it and leaves the
// field for the directory, which closes the list.
func TestNewWorktreeFormClickPR(t *testing.T) {
	root := mainRepo(t)
	fakeGH(t, "["+feat3+","+fork7+"]", nil)
	space := spaces.Space{Name: "gr", Root: root, Worktrees: []spaces.Worktree{{Path: root, Branch: "main", Session: "gr"}}}
	f := newForm(config.Config{Worktrees: config.Worktrees{Dir: ".worktrees"}}, space)
	if err := startOn(f, tabPR, f.prs, ""); err != nil {
		t.Fatal(err)
	}
	f.Update(tea.WindowSizeMsg{Width: 70, Height: 24})
	listPRs(f)
	f.View()
	clickOn(t, f, "#7 Fix from a fork")
	lookUpPR(t, f)
	type result struct {
		PR     int
		Field  string
		OnPR   bool
		Screen []string
	}
	got := result{f.prs.chosen.Number, f.prs.field.Value(), f.onPicker(), formScreen(f)[5:7]}
	want := result{7, "#7", false, []string{"  PR         #7 Fix from a fork", "  directory  .worktrees/pr-7-main"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v\nwant %+v", got, want)
	}
}

// kspaces new --from-pr starts the form on the PR tab, choosing the first open PR once listed, and --pr on
// the PR it names, by number or URL; anything else is an error.
func TestNewWorktreeFormStartsOnPR(t *testing.T) {
	root := mainRepo(t)
	fakeGH(t, "["+feat3+"]", map[string]string{"99": old99})
	space := spaces.Space{Name: "gr", Root: root, Worktrees: []spaces.Worktree{{Path: root, Branch: "main", Session: "gr"}}}
	cfg := config.Config{Worktrees: config.Worktrees{Dir: ".worktrees", BranchPrefix: "pr/me"}}
	type result struct {
		Field, Branch, Err string
	}
	var got []result
	for _, pr := range []string{"", "99", "https://github.com/o/r/pull/99/files", "main"} {
		f := newForm(cfg, space)
		if err := startOn(f, tabPR, f.prs, pr); err != nil {
			got = append(got, result{Err: err.Error()})
			continue
		}
		listPRs(f)
		lookUpPR(t, f)
		l, _ := f.prLookup()
		got = append(got, result{f.prs.field.Value(), l.found.Branch, errString(l.err)})
	}
	want := []result{
		{"", "feat", "<nil>"},
		{"99", "old", "<nil>"},
		{"https://github.com/o/r/pull/99/files", "old", "<nil>"},
		{Err: "main is not a PR's number or URL"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v\nwant %+v", got, want)
	}
}

// An input's default is filled in with the built-ins of the worktree the form would create, and follows
// the branch field, or the PR chosen, while left as it was; an input typed in keeps what was typed. Each
// tab keeps its own.
func TestNewWorktreeFormDefaults(t *testing.T) {
	root := mainRepo(t)
	fakeGH(t, "["+feat3+","+fork7+"]", nil)
	cfg, err := config.Parse(`
[[commands]]
name = "claude"
run = ["claude", "{prompt}"]
inputs = [{ name = "prompt", label = "Prompt", default = "Review PR #{pr} on {branch}" }]
`)
	if err != nil {
		t.Fatal(err)
	}
	space := spaces.Space{Name: "gr", Root: root, Worktrees: []spaces.Worktree{{Path: root, Branch: "main", Session: "gr"}}}
	f := newForm(cfg, space)
	f.Update(tea.WindowSizeMsg{Width: 70, Height: 24})
	prompt := func(tab int) string { return f.tabs[tab].cmds.values()["prompt"] }
	shiftTab := tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift}
	// Each tab chooses claude, from its choice of command, below the directory: the branch tab's, then,
	// from the tab strip, the PR tab's, focus left on its PR field.
	for _, k := range []tea.KeyPressMsg{keyMsg("tab"), keyMsg("tab"), {Code: tea.KeyRight}, shiftTab, shiftTab, shiftTab,
		{Code: tea.KeyRight}, keyMsg("tab"), keyMsg("tab"), keyMsg("tab"), {Code: tea.KeyRight}, shiftTab, shiftTab} {
		f.Update(k)
	}
	listPRs(f)
	got := []string{prompt(tabBranch), prompt(tabPR)}
	lookUpPR(t, f)
	got = append(got, prompt(tabPR))
	f.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	lookUpPR(t, f)
	got = append(got, prompt(tabPR))
	// Typed in, it stays as the PR changes.
	f.Update(keyMsg("tab"))
	f.Update(keyMsg("tab"))
	f.Update(keyMsg("tab"))
	typeInto(f, "!")
	f.Update(tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift})
	f.Update(tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift})
	f.Update(tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift})
	f.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	lookUpPR(t, f)
	got = append(got, prompt(tabPR))
	// The branch tab follows its branch, its own.
	f.Update(tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift})
	f.Update(tea.KeyPressMsg{Code: tea.KeyLeft})
	f.Update(keyMsg("tab"))
	typeInto(f, "fix")
	got = append(got, prompt(tabBranch), prompt(tabPR))
	want := []string{"Review PR # on ", "Review PR #3 on ", "Review PR #3 on feat", "Review PR #7 on pr-7-main",
		"Review PR #7 on pr-7-main!", "Review PR # on fix", "Review PR #7 on pr-7-main!"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %q\nwant %q", got, want)
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
		lines := append([]string{"", "  New worktree in gr", "", "   Branch   Pull request   Issue", "", "  branch     pr/me/fix", "  directory  .worktrees/fix"}, fields...)
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
	hint := "  left/right command  space list"
	want := screen([]string{"  command    < claude >  left/right change  space list", "  Prompt     required"}, hint, "Runs claude in the first tab")
	if got := formScreen(f); !reflect.DeepEqual(got, want) {
		t.Errorf("claude chosen:\n%q\nwant\n%q", got, want)
	}
	// Round through gr and none, by key and from the list, back to claude.
	f.Update(tea.KeyPressMsg{Code: tea.KeyRight})
	if got, want := formScreen(f), screen([]string{"  command    < gr >  left/right change  space list"}, hint, "Runs gr in the first tab"); !reflect.DeepEqual(got, want) {
		t.Errorf("gr chosen:\n%q\nwant\n%q", got, want)
	}
	clickOn(t, f, "< gr >")
	clickOn(t, f, "none")
	f.Update(tea.KeyPressMsg{Code: tea.KeyRight})
	if c, _ := f.current().cmds.chosen(); c.Name != "claude" {
		t.Fatalf("chose %q, want claude", c.Name)
	}

	f.Update(keyMsg("tab"))
	if _, cmd := f.Update(keyMsg("enter")); cmd != nil || f.busy {
		t.Fatal("created with the prompt empty")
	}
	want = screen([]string{"  command    < claude >", "  Prompt     required"}, "", "Runs claude in the first tab", "", "claude needs Prompt")
	want[len(want)-2] += "  shift+enter newline"
	if got := formScreen(f); !reflect.DeepEqual(got, want) {
		t.Errorf("prompt missing:\n%q\nwant\n%q", got, want)
	}

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
		"--env=KSPACES_BASE=main", "--env=KSPACES_SESSION=gr@fix", "--env=KSPACES_PR=", "--env=KSPACES_ISSUE=",
		`'--env=KSPACES_INPUT_PROMPT=fix "it" in $$$$HOME'`, `'--env=TASK=fix "it" in $$$$HOME'`,
		"--hold", "claude", `'fix "it" in $$HOME'`, "--",
		"@", "action", "goto_session", "gr@fix.kitty-session", "--",
	}; !reflect.DeepEqual(got, want) {
		t.Errorf("kitten ran as\n%q\nwant\n%q", got, want)
	}
}

// The Issue tab's field picks an issue as the PR tab's picks a PR, and an issue's URL pasted in the branch
// field goes to it. Choosing one fills the branch in with the prefix, the issue's number and its title,
// again as the issue changes until the branch is typed in; the directory and {issue} follow. Created, the
// worktree records its issue, and the command run in it gets it.
func TestNewWorktreeFormIssue(t *testing.T) {
	root := mainRepo(t)
	fakeGHIssues(t, "["+crash12+","+docs9+"]", map[string]string{"40": old40})
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
inputs = [{ name = "prompt", label = "Prompt", default = "Fix issue #{issue}" }]
`)
	if err != nil {
		t.Fatal(err)
	}
	space := spaces.Space{Name: "gr", Root: root, Worktrees: []spaces.Worktree{{Path: root, Branch: "main", Session: "gr"}}}
	f := newForm(cfg, space)
	f.Update(tea.WindowSizeMsg{Width: 70, Height: 24})
	type result struct {
		Tab, Issue     int
		Branch, Prompt string
	}
	now := func() result {
		return result{f.tab, f.issues.chosen.Number, f.issueBranch.Value(), f.tabs[tabIssue].cmds.values()["prompt"]}
	}
	shiftTab := tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift}

	f.Update(tea.PasteMsg{Content: "https://github.com/o/r/issues/12"})
	listOpen(f, f.issues)
	lookUpChosen(t, f, f.issues)
	// The command, claude, below the directory.
	for _, k := range []tea.KeyPressMsg{keyMsg("tab"), keyMsg("tab"), keyMsg("tab"), {Code: tea.KeyRight}, shiftTab, shiftTab, shiftTab} {
		f.Update(k)
	}
	got := []result{now()}
	// # matches every open issue, down goes to the next.
	f.Update(keyMsg("ctrl+u"))
	typeInto(f, "#")
	f.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	got = append(got, now())
	// One that is not open is looked up.
	f.Update(keyMsg("ctrl+u"))
	typeInto(f, "40")
	lookUpChosen(t, f, f.issues)
	got = append(got, now())
	// Typed in, the branch stays as the issue changes.
	f.Update(keyMsg("tab"))
	typeInto(f, "-x")
	f.Update(shiftTab)
	f.Update(keyMsg("ctrl+u"))
	typeInto(f, "12")
	f.Update(keyMsg("tab"))
	got = append(got, now())
	want := []result{
		{tabIssue, 12, "pr/me/12-crash-on-empty-input", "Fix issue #12"},
		{tabIssue, 9, "pr/me/9-docs", "Fix issue #9"},
		{tabIssue, 40, "pr/me/40-old-one", "Fix issue #40"},
		{tabIssue, 12, "pr/me/40-old-one-x", "Fix issue #12"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v\nwant %+v", got, want)
	}
	if got, want := formScreen(f), []string{"", "  New worktree in gr", "", "   Branch   Pull request   Issue", "",
		"  issue      #12 Crash on empty input", "  branch     pr/me/40-old-one-x", "  directory  .worktrees/40-old-one-x",
		"  command    < claude >", "  Prompt     Fix issue #12", "",
		"  Issue #12 Crash on empty input", "  Creates pr/me/40-old-one-x from main", "  Runs claude in the first tab", "",
		"   Create enter    Cancel esc   tab next field", ""}; !reflect.DeepEqual(got, want) {
		t.Errorf("form:\n%q\nwant\n%q", got, want)
	}

	_, cmd := f.Update(keyMsg("enter"))
	if cmd == nil {
		t.Fatal("enter created nothing")
	}
	if _, quit := f.Update(cmd()); quit == nil || f.err != nil {
		t.Fatalf("the form stayed: %v", f.err)
	}
	wt := filepath.Join(root, ".worktrees", "40-old-one-x")
	links := spaces.WorktreeLinks(spaces.LoadSpace(root), spaces.Worktree{Path: wt, Branch: "pr/me/40-old-one-x"})
	data, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	type created struct {
		Links spaces.Links
		Ran   []string
	}
	if got, want := (created{links, strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")}), (created{spaces.Links{Issue: 12}, []string{
		"@", "action", "launch", "--type=tab", "--cwd=" + wt, "--add-to-session=gr@40-old-one-x",
		"--env=KSPACES_SPACE=gr", "--env=KSPACES_ROOT=" + root, "--env=KSPACES_WORKTREE=" + wt, "--env=KSPACES_BRANCH=pr/me/40-old-one-x",
		"--env=KSPACES_BASE=main", "--env=KSPACES_SESSION=gr@40-old-one-x", "--env=KSPACES_PR=", "--env=KSPACES_ISSUE=12",
		"'--env=KSPACES_INPUT_PROMPT=Fix issue #12'", "--hold", "claude", "'Fix issue #12'", "--",
		"@", "action", "goto_session", "gr@40-old-one-x.kitty-session", "--",
	}}); !reflect.DeepEqual(got, want) {
		t.Errorf("created\n%+v\nwant\n%+v", got, want)
	}
}

// kspaces new --from-issue starts the form on the Issue tab, and --issue on the issue it names, by number or
// URL; anything else is an error. With no issue chosen it creates nothing.
func TestNewWorktreeFormStartsOnIssue(t *testing.T) {
	root := mainRepo(t)
	fakeGHIssues(t, "[]", map[string]string{"40": old40})
	space := spaces.Space{Name: "gr", Root: root, Worktrees: []spaces.Worktree{{Path: root, Branch: "main", Session: "gr"}}}
	cfg := config.Config{Worktrees: config.Worktrees{Dir: ".worktrees", BranchPrefix: "pr/me"}}
	type result struct {
		Branch, Err string
	}
	var got []result
	for _, issue := range []string{"", "40", "https://github.com/o/r/issues/40", "main"} {
		f := newForm(cfg, space)
		if err := startOn(f, tabIssue, f.issues, issue); err != nil {
			got = append(got, result{Err: err.Error()})
			continue
		}
		listOpen(f, f.issues)
		if f.issues.chosen.Number == 0 {
			f.create()
			got = append(got, result{f.issueBranch.Value(), errString(f.err)})
			continue
		}
		lookUpChosen(t, f, f.issues)
		got = append(got, result{f.issueBranch.Value(), errString(f.err)})
	}
	want := []result{
		{"pr/me/", "choose an issue to create a branch for"},
		{"pr/me/40-old-one", "<nil>"},
		{"pr/me/40-old-one", "<nil>"},
		{Err: "main is not an issue's number or URL"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v\nwant %+v", got, want)
	}
}

// With other local branches, the branch tab chooses the new branch's base, the main worktree's at first,
// the others the last committed to first, from its list filtered by what is typed too. {base} follows it.
// Created, the branch starts at the base, which the worktree knows from then on.
func TestNewWorktreeFormBase(t *testing.T) {
	r := gittest.New(t)
	r.Commit("first")
	r.Git("branch", "old")
	r.Git("switch", "-q", "-c", "feat")
	feat := r.Commit("feat")
	r.Git("switch", "-q", "main")
	root := r.Dir
	dir := t.TempDir()
	script := "#!/bin/sh\nexit 0\n"
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
inputs = [{ name = "prompt", label = "Prompt", default = "Diff against {base}" }]
`)
	if err != nil {
		t.Fatal(err)
	}
	space := spaces.LoadSpace(root)
	f := newForm(cfg, space)
	f.Update(tea.WindowSizeMsg{Width: 70, Height: 24})
	typeInto(f, "stack")
	shiftTab := tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift}
	for _, k := range []tea.KeyPressMsg{keyMsg("tab"), keyMsg("tab"), keyMsg("tab"), {Code: tea.KeyRight}, shiftTab, shiftTab} {
		f.Update(k)
	}
	screen := func(base string) []string {
		return []string{"", "  New worktree in " + space.Name, "", "   Branch   Pull request   Issue", "",
			"  branch     pr/me/stack", "  from       < " + base + " >  left/right change  space list", "  directory  .worktrees/stack",
			"  command    < claude >", "  Prompt     Diff against " + base, "",
			"  Creates pr/me/stack from " + base, "  Runs claude in the first tab", "",
			"   Create enter    Cancel esc   tab next field  left/right base  space list", ""}
	}
	got := [][]string{formScreen(f)}
	f.Update(keyMsg("space"))
	typeInto(f, "fe")
	f.Update(keyMsg("enter"))
	got = append(got, formScreen(f))
	if want := [][]string{screen("main"), screen("feat")}; !reflect.DeepEqual(got, want) {
		t.Errorf("got\n%q\nwant\n%q", got, want)
	}

	_, cmd := f.Update(keyMsg("enter"))
	if cmd == nil {
		t.Fatal("enter created nothing")
	}
	if _, quit := f.Update(cmd()); quit == nil || f.err != nil {
		t.Fatalf("the form stayed: %v", f.err)
	}
	wt := spaces.Worktree{Path: filepath.Join(root, ".worktrees", "stack"), Branch: "pr/me/stack"}
	type created struct {
		Links spaces.Links
		Tip   string
	}
	if got, want := (created{spaces.WorktreeLinks(space, wt), strings.TrimSpace(r.Git("rev-parse", "pr/me/stack"))}), (created{spaces.Links{Base: "feat"}, feat}); got != want {
		t.Errorf("created %+v, want %+v", got, want)
	}
}

// kspaces new --base, as a worktree's row opens it with its branch, starts the branch and Issue tabs' base
// on that branch; the main worktree's is the default, and anything but a local branch is an error.
func TestNewWorktreeFormStartsFrom(t *testing.T) {
	r := gittest.New(t)
	r.Commit("first")
	r.Git("branch", "feat")
	space := spaces.LoadSpace(r.Dir)
	type result struct {
		Branch, Issue, Err string
	}
	var got []result
	for _, base := range []string{"feat", "main", "", "nope"} {
		f := newForm(config.Config{}, space)
		if err := f.startFrom(base); err != nil {
			got = append(got, result{Err: err.Error()})
			continue
		}
		var bases [3]string
		for _, tab := range []int{tabBranch, tabIssue} {
			f.tab = tab
			bases[tab] = f.base()
		}
		got = append(got, result{Branch: bases[tabBranch], Issue: bases[tabIssue]})
	}
	want := []result{{"feat", "feat", ""}, {}, {}, {Err: "nope is not a local branch"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v\nwant %+v", got, want)
	}
}
