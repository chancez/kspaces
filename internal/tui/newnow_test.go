package tui

import (
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"testing"

	"github.com/chancez/kspaces/internal/spaces"
)

// kspaces new --yes creates the worktree on the branch given, after the prefix, and opens its session
// with the command or command line given, in the background without switching to it. What it opens with
// is checked first: a required input not given, or no branch, creates nothing.
func TestNewWorktreeNow(t *testing.T) {
	root := mainRepo(t)
	cfg := testCommands(t)
	cfg.Spaces = []string{root}
	cfg.Worktrees.Dir, cfg.Worktrees.BranchPrefix = ".worktrees", "pr/me"
	calls := fakeKittenLs(t, lsWith("other"))
	space := spaces.LoadSpace(root)

	_, errNoBranch := NewWorktreeNow(cfg, space, NewWorktree{Command: "claude"})
	_, errNoPrompt := NewWorktreeNow(cfg, space, NewWorktree{Branch: "nope", Command: "claude"})
	_, errNoLayout := NewWorktreeNow(cfg, space, NewWorktree{Branch: "nope", Layout: "claude"})
	got := []string{errString(errNoBranch), errString(errNoPrompt), errString(errNoLayout)}
	if want := []string{"give a branch, a PR or an issue", "claude needs Prompt", "no layout claude in the config"}; !reflect.DeepEqual(got, want) {
		t.Errorf("got %q\nwant %q", got, want)
	}
	if _, err := os.Stat(filepath.Join(root, ".worktrees", "nope")); !os.IsNotExist(err) {
		t.Errorf("a worktree was created: %v", err)
	}

	fix, err := NewWorktreeNow(cfg, space, NewWorktree{Branch: "fix", Command: "claude",
		Inputs: map[string]string{"prompt": "fix {it}"}, Stay: true})
	if err != nil {
		t.Fatal(err)
	}
	dev, err := NewWorktreeNow(cfg, spaces.LoadSpace(root), NewWorktree{Branch: "pr/me/dev", Args: []string{"go", "test", "{}"}})
	if err != nil {
		t.Fatal(err)
	}
	session := func(dir string) string { return filepath.Base(root) + "@" + dir }
	want := []spaces.Worktree{
		{Path: filepath.Join(root, ".worktrees", "fix"), Branch: "pr/me/fix", Session: session("fix")},
		{Path: filepath.Join(root, ".worktrees", "dev"), Branch: "pr/me/dev", Session: session("dev")},
	}
	if got := []spaces.Worktree{fix, dev}; !reflect.DeepEqual(got, want) {
		t.Errorf("created\n%+v\nwant\n%+v", got, want)
	}
	env := func(wt spaces.Worktree) []string {
		return []string{"--env=KSPACES_SPACE=" + filepath.Base(root), "--env=KSPACES_ROOT=" + root, "--env=KSPACES_WORKTREE=" + wt.Path,
			"--env=KSPACES_BRANCH=" + wt.Branch, "--env=KSPACES_BASE=main", "--env=KSPACES_SESSION=" + wt.Session, "--env=KSPACES_PR=",
			"--env=KSPACES_ISSUE="}
	}
	if got, want := calls(), slices.Concat(
		[]string{"@", "action", "launch", "--type=tab", "--cwd=" + fix.Path}, inSession(fix.Session), []string{"--keep-focus"},
		env(fix), []string{"'--env=KSPACES_INPUT_PROMPT=fix {it}'", "--hold", "claude", "'fix {it}'", "--"},
		[]string{"@", "action", "launch", "--type=tab", "--cwd=" + dev.Path}, inSession(dev.Session),
		env(dev), []string{"--hold", "go", "test", "{}", "--"},
		[]string{"@", "action", "goto_session", dev.Session + ".kitty-session", "--"},
	); !reflect.DeepEqual(got, want) {
		t.Errorf("kitten ran as\n%q\nwant\n%q", got, want)
	}
}
