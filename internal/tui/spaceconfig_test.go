package tui

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/chancez/kspaces/internal/config"
	"github.com/chancez/kspaces/internal/spaces"
)

// spaceFileFixture is gitFixture with gr's own config untrusted and kitty's broken.
func spaceFileFixture() spaces.Snapshot {
	snap := gitFixture()
	snap.Spaces[0].Untrusted = true
	snap.Spaces[1].ConfigErr = errors.New("~/src/kitty/.kspaces.toml: unknown keys: nope")
	return snap
}

// A space whose own config is untrusted or broken says so on its own row, where its git state would be,
// in fewer words, then only its mark, as the sidebar narrows.
func TestSpaceConfigOnRows(t *testing.T) {
	e := newSidebarEnv(t)
	e.m.Update(spaceFileFixture())
	rows := func() []string { return e.screen()[2:13] }
	pad := func(n int) string { return strings.Repeat(" ", n) }
	want := []string{
		"  " + termIcon + "default",
		"",
		"\u25bc " + repoIcon + "gr" + pad(3) + "\uf023  untrusted config",
		"  " + blocked + "dev pr/me/\u2026 \u25cf3 \u21912\u21931 #123",
		"  " + branchIcon + "fix fix" + pad(14) + "#98",
		"",
		"  " + repoIcon + "kitty m\u2026 \uf00d  config error",
		"",
		"\u25b6 \uf110  docs main",
		"",
		"  \uf00d  broken",
	}
	if got := rows(); !reflect.DeepEqual(got, want) {
		t.Errorf("rows at 30:\n%q\nwant\n%q", got, want)
	}
	e.m.Update(tea.WindowSizeMsg{Width: 22, Height: 40})
	want = []string{
		"  " + termIcon + "default",
		"",
		"\u25bc " + repoIcon + "gr" + pad(2) + "\uf023  untrusted",
		"  " + blocked + "dev \u25cf3 \u21912\u21931 #123",
		"  " + branchIcon + "fix fix" + pad(6) + "#98",
		"",
		"  " + repoIcon + "kitty master" + pad(3) + "\uf00d",
		"",
		"\u25b6 \uf110  docs main",
		"",
		"  \uf00d  broken",
	}
	if got := rows(); !reflect.DeepEqual(got, want) {
		t.Errorf("rows at 22:\n%q\nwant\n%q", got, want)
	}
	e.m.Update(tea.WindowSizeMsg{Width: 14, Height: 40})
	want = []string{
		"  " + termIcon + "default",
		"",
		"\u25bc " + repoIcon + "gr ma\u2026 \uf023",
		"  " + blocked + "dev   \u25cf3",
		"  " + branchIcon + "fix  #98",
		"",
		"  " + repoIcon + "kitty  \uf00d",
		"",
		"\u25b6 \uf110  docs ma\u2026",
		"",
		"  \uf00d  broken",
	}
	if got := rows(); !reflect.DeepEqual(got, want) {
		t.Errorf("rows at 14:\n%q\nwant\n%q", got, want)
	}
}

// Trust space config is in the Space section of the menu of every row of a space whose own config is
// untrusted, and opens the trust form for the space in an overlay.
func TestTrustMenu(t *testing.T) {
	e := newSidebarEnv(t)
	e.m.Update(spaceFileFixture())
	launched := fakeKitten(t)
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	offered := map[string]bool{}
	for _, session := range []string{"gr", "gr@dev", "gr@fix", "kitty", "default"} {
		e.m.cursor = e.row(session)
		target, _ := e.m.entryTarget(e.m.cursor)
		for _, g := range target.Groups(e.m) {
			for _, a := range g.Actions {
				if a.Is(actTrustConfig) {
					offered[session] = true
				}
			}
		}
	}
	if want := map[string]bool{"gr": true, "gr@dev": true, "gr@fix": true}; !reflect.DeepEqual(offered, want) {
		t.Errorf("offered on %v, want %v", offered, want)
	}
	e.m.cursor = e.row("gr@dev")
	e.press("m")
	_, y := e.zone(fmt.Sprintf(entryZone, e.m.cursor))
	if got, want := e.screen()[y+1:y+14], []string{
		menuRule("Session"), " Open", " Save as layout", " Close session", menuRule("Worktree"), " Open PR #123", " Remove worktree",
		menuRule("Space"), " New worktree", " Fold worktrees", " Trust space config", " Add space", menuRule(""),
	}; !reflect.DeepEqual(got, want) {
		t.Errorf("the menu:\n%q\nwant\n%q", got, want)
	}
	cmd := e.press("j", "j", "j", "j", "j", "j", "j", "enter")
	if cmd == nil {
		t.Fatal("Trust space config ran nothing")
	}
	if msg := cmd(); msg != nil {
		t.Errorf("Trust space config said %v", msg)
	}
	if got, want := launched(), []string{"@", "launch", "--type=overlay", "--var=kspaces_form=1", "--next-to", "id:11", exe, "trust", "/src/gr"}; !reflect.DeepEqual(got, want) {
		t.Errorf("launched %q, want %q", got, want)
	}
}

// Run and Open layout are offered on the rows of a space whose own config has commands and layouts, and
// on no other's.
func TestSpaceConfigMenus(t *testing.T) {
	e := newSidebarEnv(t)
	snap := fixture(time.Now())
	own := config.Config{
		Commands: []config.Command{{Name: "test", Run: []string{"make", "test"}, Open: "tab"}},
		Layouts:  []config.Layout{{Name: "tests", Tabs: []config.LayoutTab{{Panes: []config.Pane{{Command: "test"}}}}}},
	}
	snap.Spaces[1].Config = &own
	e.m.Update(snap)
	offered := map[string][]string{}
	for _, session := range []string{"gr", "gr@dev", "kitty"} {
		e.m.cursor = e.row(session)
		target, _ := e.m.entryTarget(e.m.cursor)
		for _, g := range target.Groups(e.m) {
			for _, a := range g.Actions {
				switch {
				case a.Is(actRunCommand):
					offered[session] = append(offered[session], "Run")
				case a.Is(actOpenLayout):
					offered[session] = append(offered[session], "Open layout")
				}
			}
		}
	}
	if want := map[string][]string{"kitty": {"Run", "Open layout"}}; !reflect.DeepEqual(offered, want) {
		t.Errorf("offered %v, want %v", offered, want)
	}
}

// trustEnv is the trust form for gr's file text, with a global config that has review, which the file's
// layout runs, and plain, which it replaces.
func trustEnv(t *testing.T, text string) (*trustForm, string) {
	t.Helper()
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	global, err := config.Parse(`spaces = ["/src/gr"]

[[commands]]
name = "review"
run = ["gr"]

[[layouts]]
name = "plain"
tabs = [{ panes = [{}] }]
`)
	if err != nil {
		t.Fatal(err)
	}
	space := spaces.Space{Name: "gr", Root: "/src/gr", Untrusted: true}
	f := newTrustForm(global, space, []byte(text))
	f.Update(tea.WindowSizeMsg{Width: 70, Height: 30})
	return f, filepath.Join(os.Getenv("XDG_STATE_HOME"), "kspaces", "trust")
}

// The trust form shows what the file adds: each command and what it runs, each layout and what each of
// its panes runs, quoted as a shell would split it, one in place of a global one saying so, and the
// settings. y trusts the content shown.
func TestTrustForm(t *testing.T) {
	text := `layout = "tests"
worktree_layout = "plain"

[worktrees]
branch_prefix = "cz"

[[commands]]
name = "test"
run = ["make", "test", "{pr}"]

[[commands]]
name = "review"
run = ["gr", "--watch", "a b"]

[[layouts]]
name = "tests"
tabs = [{ panes = [{ command = "test" }, { command = "review", split = "right" }] }, { panes = [{ run = ["tail", "-f", "log"] }, { split = "below" }] }]

[[layouts]]
name = "plain"
tabs = [{ panes = [{ command = "test" }] }]
`
	f, trustFile := trustEnv(t, text)
	want := []string{
		"", "  Trust gr's config", "",
		"  /src/gr/.kspaces.toml", "",
		"  Its commands run in gr's worktrees once trusted, until it changes:", "",
		"  Commands",
		"    test: make test '{pr}'",
		"    review: gr --watch 'a b'  in place of the global one",
		"  Layouts",
		"    tests",
		"      tab 1, pane 1: test: make test '{pr}'",
		"      tab 1, pane 2: review: gr --watch 'a b'",
		"      tab 2, pane 1: tail -f log",
		"      tab 2, pane 2: a shell",
		"    plain  in place of the global one",
		"      tab 1, pane 1: test: make test '{pr}'",
		"  Settings",
		"    layout = tests",
		"    worktree_layout = plain",
		"    worktrees.branch_prefix = \"cz\"",
		"",
		"   Trust y    Cancel n", "",
	}
	if got := formScreen(f); !reflect.DeepEqual(got, want) {
		t.Errorf("the form:\n%q\nwant\n%q", got, want)
	}
	_, cmd := f.Update(keyMsg("y"))
	if cmd == nil {
		t.Fatal("y ran nothing")
	}
	if _, quit := f.Update(cmd()); quit == nil || f.err != nil {
		t.Fatalf("the form stayed: %v", f.err)
	}
	data, err := os.ReadFile(trustFile)
	if err != nil {
		t.Fatal(err)
	}
	if !spaces.Trusted("/src/gr/.kspaces.toml", []byte(text)) || !strings.HasSuffix(string(data), "  /src/gr/.kspaces.toml\n") {
		t.Errorf("trust file %q", data)
	}
}

// A file with an error says so, and cannot be trusted until fixed.
func TestTrustFormBroken(t *testing.T) {
	f, _ := trustEnv(t, "[[layouts]]\nname = \"x\"\ntabs = [{ panes = [{ command = \"nope\" }] }]\n")
	want := []string{
		"", "  Trust gr's config", "",
		"  /src/gr/.kspaces.toml", "",
		`  layouts: x: tab 1 pane 1: command is "nope", no command has that`, "  name", "",
		"  Fix it, then trust it.", "",
		"   Trust    Cancel n", "",
	}
	if got := formScreen(f); !reflect.DeepEqual(got, want) {
		t.Errorf("the form:\n%q\nwant\n%q", got, want)
	}
	if _, cmd := f.Update(keyMsg("y")); cmd != nil {
		t.Error("y trusted a broken file")
	}
}
