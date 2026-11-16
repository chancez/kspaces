package spaces

import (
	"path/filepath"
	"reflect"
	"testing"

	"github.com/chancez/kspaces/internal/config"
	"github.com/chancez/kspaces/internal/gittest"
)

// A worktree created for an issue knows it from then on, as git config records it with its branch; the
// branch deleted, a new one of its name is for no issue.
func TestCreateIssueWorktree(t *testing.T) {
	r := gittest.New(t)
	r.Commit("first")
	space := LoadSpace(r.Dir)
	dir := filepath.Join(r.Dir, ".worktrees", "42-fix")
	wt, err := CreateIssueWorktree(space, 42, "pr/me/42-fix", dir)
	if err != nil {
		t.Fatal(err)
	}
	got := []Links{WorktreeLinks(space, wt)}
	r.Git("worktree", "remove", dir)
	r.Git("branch", "-q", "-D", "pr/me/42-fix")
	again, err := CreateWorktree(space, "pr/me/42-fix", dir)
	if err != nil {
		t.Fatal(err)
	}
	got = append(got, WorktreeLinks(space, again))
	if want := []Links{{Issue: 42}, {}}; !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v, want %+v", got, want)
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
