package spaces

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/chancez/kspaces/internal/config"
	"github.com/chancez/kspaces/internal/kitty"
)

// A detached session opens in its own place first: the worktree its tag names, else the deepest worktree
// holding its cwd, else the current session when it is the space's, else the space's main worktree. A
// missing worktree is never one but as the current session.
func TestOpenTarget(t *testing.T) {
	gr := Space{Name: "gr", Root: "/src/gr", Worktrees: []Worktree{
		{Path: "/src/gr", Session: "gr"}, {Path: "/src/gr/.worktrees/fix", Session: "gr@fix"}, {Path: "/src/gr/.worktrees/dev", Session: "gr@dev"},
	}, Missing: []Worktree{{Session: "gr@gone"}}}
	notes := Space{Name: "notes", Worktrees: []Worktree{{Session: "notes"}}}
	tagged := ProviderSession{ID: "a1", Tags: map[string]string{"worktree": "fix"}, Cwd: "/src/gr/.worktrees/dev"}
	inDev := ProviderSession{ID: "b2", Cwd: "/src/gr/.worktrees/dev/web"}
	inRoot := ProviderSession{ID: "c3", Cwd: "/src/gr/docs"}
	elsewhere := ProviderSession{ID: "d4", Cwd: "/tmp", Tags: map[string]string{"worktree": "nope"}}
	got := []Worktree{
		OpenTarget(kitty.State{Current: "gr@dev"}, gr, tagged),
		OpenTarget(kitty.State{Current: "gr@fix"}, gr, inDev),
		OpenTarget(kitty.State{Current: "gr@fix"}, gr, inRoot),
		OpenTarget(kitty.State{Current: "gr@dev"}, gr, elsewhere),
		OpenTarget(kitty.State{Current: "gr@gone"}, gr, elsewhere),
		OpenTarget(kitty.State{Current: "kitty"}, gr, elsewhere),
		OpenTarget(kitty.State{Current: "kitty"}, gr, ProviderSession{Tags: map[string]string{"worktree": "gone"}}),
		OpenTarget(kitty.State{Current: "gr"}, notes, tagged),
		OpenTarget(kitty.State{}, Space{Name: "broken"}, ProviderSession{}),
	}
	want := []Worktree{
		gr.Worktrees[1], // its tag, over its cwd and the current session
		gr.Worktrees[2], // the deepest worktree holding its cwd, over the current session
		gr.Worktrees[0],
		gr.Worktrees[2], // the current session, in the space
		gr.Missing[0],   // even a missing one, which has windows
		gr.Worktrees[0], // the main worktree
		gr.Worktrees[0], // a missing worktree's tag opens none
		notes.Worktrees[0],
		{Session: "broken"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got  %#v\nwant %#v", got, want)
	}
}

// The tab opening a session runs what the provider says, each word quoted for kitty, starting where it
// says, else in the worktree or the home directory. It goes by the session's window when it has windows,
// after restoring it from its file when that opens any, and is its first tab otherwise.
func TestOpenSessionActions(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "gr@fix.kitty-session"), []byte("launch\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := config.Config{Launch: []string{"launch"}, SessionsDir: dir}
	file := func(name string) string { return filepath.Join(dir, name+".kitty-session") }
	fix := Worktree{Path: "/src/gr/.worktrees/fix", Session: "gr@fix"}
	argv := []string{"mux", "attach", "a b"}
	got := [][][]string{
		openSessionActions(cfg, kitty.State{Sessions: map[string]int{"gr@fix": 7}}, fix, argv, "/src/gr/web dir"),
		openSessionActions(cfg, kitty.State{}, fix, argv, ""),
		openSessionActions(cfg, kitty.State{}, Worktree{Path: "/src/gr", Session: "gr"}, argv, ""),
		openSessionActions(cfg, kitty.State{}, Worktree{Session: "notes"}, argv, ""),
	}
	want := [][][]string{
		{{"goto_session", file("gr@fix")}, {"--match=id:7", "launch", "--type=tab", "'--cwd=/src/gr/web dir'", "--add-to-session=gr@fix",
			"--var=kspaces_space=gr", "--var=kspaces_worktree=fix", "mux", "attach", "'a b'"}},
		{{"goto_session", file("gr@fix")}, {"launch", "--type=tab", "--cwd=/src/gr/.worktrees/fix", "--add-to-session=gr@fix",
			"--var=kspaces_space=gr", "--var=kspaces_worktree=fix", "mux", "attach", "'a b'"}},
		{{"launch", "--type=tab", "--cwd=/src/gr", "--add-to-session=gr", "--var=kspaces_space=gr", "mux", "attach", "'a b'"}, {"goto_session", file("gr")}},
		{{"launch", "--type=tab", "--cwd=" + home, "--add-to-session=notes", "--var=kspaces_space=notes", "mux", "attach", "'a b'"}, {"goto_session", file("notes")}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got  %q\nwant %q", got, want)
	}
}
