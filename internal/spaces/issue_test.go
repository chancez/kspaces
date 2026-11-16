package spaces

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/chancez/kspaces/internal/config"
	"github.com/chancez/kspaces/internal/gittest"
)

// A worktree created for an issue, from a base, knows both from then on, as git config records them with
// its branch, which starts at the base; one from the main worktree's branch records no base. The branch
// deleted, a new one of its name is for no issue and has no base.
func TestCreateIssueWorktree(t *testing.T) {
	r := gittest.New(t)
	main := r.Commit("first")
	r.Git("switch", "-q", "-c", "feat")
	feat := r.Commit("feat")
	r.Git("switch", "-q", "main")
	space := LoadSpace(r.Dir)
	type result struct {
		Links Links
		Tip   string
	}
	var got []result
	created := func(wt Worktree, err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
		got = append(got, result{WorktreeLinks(space, wt), strings.TrimSpace(r.Git("rev-parse", wt.Branch))})
	}
	dir := filepath.Join(r.Dir, ".worktrees", "42-fix")
	created(CreateIssueWorktree(space, 42, "pr/me/42-fix", "feat", dir))
	created(CreateWorktreeFrom(space, "plain", "main", filepath.Join(r.Dir, ".worktrees", "plain")))
	r.Git("worktree", "remove", dir)
	r.Git("branch", "-q", "-D", "pr/me/42-fix")
	created(CreateWorktree(space, "pr/me/42-fix", dir))
	want := []result{{Links{Issue: 42, Base: "feat"}, feat}, {Links{}, main}, {Links{}, main}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v\nwant %+v", got, want)
	}
}

// An issue is named by its number, #N or its URL, as a PR is; a PR's URL is not an issue's.
func TestParseIssueRef(t *testing.T) {
	type result struct {
		Ref Ref
		OK  bool
	}
	var got []result
	for _, s := range []string{"12", " #12 ", "https://github.com/o/r/issues/12#issuecomment-1", "https://github.com/o/r/pull/12", "#", "fix"} {
		ref, ok := ParseIssueRef(s)
		got = append(got, result{ref, ok})
	}
	twelve := result{Ref{Arg: "12", Number: 12}, true}
	want := []result{twelve, twelve, {Ref{Arg: "https://github.com/o/r/issues/12#issuecomment-1", Number: 12}, true}, {}, {}, {}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v\nwant %+v", got, want)
	}
}

// An issue's branch is the prefix, its number and its title's words, letters and digits only, as many
// whole words as fit.
func TestIssueBranch(t *testing.T) {
	cfg := config.Config{Worktrees: config.Worktrees{BranchPrefix: "pr/me"}}
	got := []string{
		IssueBranch(cfg, 12, "Crash on empty input!"),
		IssueBranch(cfg, 12, "kitty's `launch --cwd` loses the shell's directory, again and again"),
		IssueBranch(cfg, 12, "\u65e5\u672c"),
		IssueBranch(config.Config{}, 3, "Docs"),
	}
	want := []string{"pr/me/12-crash-on-empty-input", "pr/me/12-kitty-s-launch-cwd-loses-the-shell-s", "pr/me/12", "3-docs"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %q\nwant %q", got, want)
	}
}
