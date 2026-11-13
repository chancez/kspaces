package spaces

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/chancez/kspaces/internal/config"
	"github.com/chancez/kspaces/internal/git"
	"github.com/chancez/kspaces/internal/gittest"
)

// fakeGH puts a gh first on PATH that answers `gh pr view N` with prs[N], fails as gh does for a PR that
// is not there, and records where it ran and with what. It returns the last call's directory and arguments.
func fakeGH(t *testing.T, prs map[string]string) func() []string {
	t.Helper()
	dir := t.TempDir()
	for n, js := range prs {
		if err := os.WriteFile(filepath.Join(dir, "pr-"+n+".json"), []byte(js), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	log := filepath.Join(dir, "args")
	script := "#!/bin/sh\n{ pwd -P; for a; do printf '%s\\n' \"$a\"; done; } > " + log + "\n" +
		"n=${3##*/pull/}; n=${n%%/*}\n" +
		"f=" + dir + "/pr-$n.json\n" +
		"[ -f \"$f\" ] || { echo \"GraphQL: Could not resolve to a PullRequest with the number of $n. (repository.pullRequest)\" >&2; exit 1; }\n" +
		"cat \"$f\"\n"
	if err := os.WriteFile(filepath.Join(dir, "gh"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return func() []string {
		data, err := os.ReadFile(log)
		if err != nil {
			return nil
		}
		return strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
	}
}

// prRepos is a repository standing in for GitHub's, with a branch feat for a PR from itself and a
// pull/7/head for one from a fork, and a clone of it as the space, which has main and 123 of its own.
func prRepos(t *testing.T) (space *gittest.Repo, pullHead string) {
	t.Helper()
	remote := gittest.New(t)
	remote.Commit("first")
	remote.Git("switch", "-q", "-c", "feat")
	remote.Commit("feat")
	remote.Git("switch", "-q", "-c", "fork", "main")
	pullHead = remote.Commit("from a fork")
	remote.Git("update-ref", "refs/pull/7/head", pullHead)
	remote.Git("switch", "-q", "main")
	remote.Git("branch", "-q", "-D", "fork")
	tmp, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	space = remote.Clone(filepath.Join(tmp, "space"))
	space.Git("branch", "123")
	return space, pullHead
}

// The branch field names a PR as #N, a URL or a number that is no branch, after the prefix or not.
func TestParsePRRef(t *testing.T) {
	r, _ := prRepos(t)
	root := r.Dir
	cfg := config.Config{Worktrees: config.Worktrees{BranchPrefix: "pr/me"}}
	space := Space{Name: "space", Root: root}
	type result struct {
		Ref PRRef
		OK  bool
	}
	var got []result
	for _, field := range []string{
		"#7", " pr/me/#7 ", "7", "pr/me/7", "007",
		"https://github.com/o/r/pull/7", "pr/me/https://github.com/o/r/pull/7/files",
		// A branch, or nothing to look up.
		"123", "#123", "feat", "pr/me/", "#", "#x", "7a", "0", "-7", "https://github.com/o/r/issues/7",
	} {
		ref, ok := ParsePRRef(cfg, space, field)
		got = append(got, result{ref, ok})
	}
	seven := result{PRRef{Arg: "7", Number: 7}, true}
	want := []result{
		seven, seven, seven, seven, seven,
		{PRRef{Arg: "https://github.com/o/r/pull/7", Number: 7}, true},
		{PRRef{Arg: "https://github.com/o/r/pull/7/files", Number: 7}, true},
		{}, {PRRef{Arg: "123", Number: 123}, true}, {}, {}, {}, {}, {}, {}, {}, {},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v\nwant %+v", got, want)
	}
}

// A PR from the repository itself is its branch, tracking the remote's; one from a fork is pull/N/head in
// a branch of the fork's name, or pr-N-NAME when the space has that name, which pulls from pull/N/head.
func TestPRWorktree(t *testing.T) {
	r, pullHead := prRepos(t)
	root := r.Dir
	args := fakeGH(t, map[string]string{
		"3": `{"headRefName":"feat","headRepositoryOwner":{"id":"U_1","login":"o"},"isCrossRepository":false,"number":3,"title":"Add feat","url":"https://github.com/o/r/pull/3"}`,
		"7": `{"headRefName":"main","headRepositoryOwner":{"id":"U_2","login":"forker"},"isCrossRepository":true,"number":7,"title":"Fix from a fork","url":"https://github.com/o/r/pull/7"}`,
	})
	space := Space{Name: "space", Root: root}

	same, err := ResolvePR(space, PRRef{Arg: "3", Number: 3})
	if err != nil {
		t.Fatal(err)
	}
	if want := (PR{Number: 3, Title: "Add feat", HeadRef: "feat", Owner: "o", Remote: "origin", Branch: "feat"}); same != want {
		t.Errorf("same repository: got %+v\nwant %+v", same, want)
	}
	if got, want := args(), []string{root, "pr", "view", "3", "--json", "number,headRefName,headRepositoryOwner,isCrossRepository,title,url"}; !reflect.DeepEqual(got, want) {
		t.Errorf("gh ran as %q\nwant %q", got, want)
	}
	fork, err := ResolvePR(space, PRRef{Arg: "https://github.com/o/r/pull/7", Number: 7})
	if err != nil {
		t.Fatal(err)
	}
	if want := (PR{Number: 7, Title: "Fix from a fork", HeadRef: "main", Owner: "forker", Cross: true, Remote: "origin", Branch: "pr-7-main"}); fork != want {
		t.Errorf("fork: got %+v\nwant %+v", fork, want)
	}

	type made struct {
		Worktree             Worktree
		Head, Remote, Merges string
	}
	check := func(pr PR, dir string) made {
		t.Helper()
		wt, err := CreatePRWorktree(space, pr, filepath.Join(root, ".worktrees", dir))
		if err != nil {
			t.Fatal(err)
		}
		return made{wt, r.Git("-C", wt.Path, "rev-parse", "HEAD"), r.Git("config", "branch."+pr.Branch+".remote"), git.BranchMerge(root, pr.Branch)}
	}
	got := []made{check(same, "feat"), check(fork, "pr-7")}
	want := []made{
		{Worktree{Path: filepath.Join(root, ".worktrees", "feat"), Branch: "feat", Session: "space@feat"}, r.Git("rev-parse", "origin/feat"), "origin", "refs/heads/feat"},
		{Worktree{Path: filepath.Join(root, ".worktrees", "pr-7"), Branch: "pr-7-main", Session: "space@pr-7"}, pullHead, "origin", "refs/pull/7/head"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("worktrees: got %+v\nwant %+v", got, want)
	}

	// Both branches are there now, and checked out as they are the next time.
	var again []PR
	for _, ref := range []PRRef{{Arg: "3", Number: 3}, {Arg: "7", Number: 7}} {
		pr, err := ResolvePR(space, ref)
		if err != nil {
			t.Fatal(err)
		}
		again = append(again, pr)
	}
	same.Exists, fork.Exists = true, true
	if want := []PR{same, fork}; !reflect.DeepEqual(again, want) {
		t.Errorf("again: got %+v\nwant %+v", again, want)
	}
	// Another PR from a fork's main finds both names taken.
	r.Git("config", "branch.pr-7-main.merge", "refs/heads/main")
	if _, err := ResolvePR(space, PRRef{Arg: "7", Number: 7}); err == nil || err.Error() != "the branches main and pr-7-main are taken, so PR #7 needs a branch of its own" {
		t.Errorf("both names taken: %v", err)
	}
}

// A PR is fetched from the remote for the repository it is to, whichever way its URL is written, origin
// first, and from origin when no remote is.
func TestPRRemote(t *testing.T) {
	remotes := map[string]string{
		"origin":   "git@github.com:me/r.git",
		"upstream": "https://github.com/O/R.git",
		"ent":      "ssh://git@ghe.example.com:2222/team/tool",
		"mirror":   "git@github.com:me/r",
		"local":    "/src/r",
	}
	var got []string
	for _, url := range []string{
		"https://github.com/o/r/pull/3",
		"https://github.com/me/r/pull/3",
		"https://ghe.example.com/team/tool/pull/9",
		"https://github.com/other/r/pull/3",
		"",
	} {
		got = append(got, prRemote(remotes, url))
	}
	if want := []string{"upstream", "origin", "ent", "origin", "origin"}; !reflect.DeepEqual(got, want) {
		t.Errorf("got %q, want %q", got, want)
	}
}
