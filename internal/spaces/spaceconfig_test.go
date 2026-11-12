package spaces

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/chancez/kspaces/internal/config"
	"github.com/chancez/kspaces/internal/gittest"
)

// A space file is used only once trusted, and only for the content trusted: a change makes it untrusted
// again until trusted anew, and revoking forgets it. The trust file has a line per file trusted.
func TestSpaceFileTrust(t *testing.T) {
	state := t.TempDir()
	t.Setenv("XDG_STATE_HOME", state)
	r := gittest.New(t)
	r.Commit("first")
	cfg, err := config.Parse(fmt.Sprintf("spaces = [%q]\n", r.Dir))
	if err != nil {
		t.Fatal(err)
	}
	name, file := filepath.Base(r.Dir), filepath.Join(r.Dir, config.SpaceFileName)
	worktrees := LoadSpace(r.Dir).Worktrees
	space := func(c *config.Config, untrusted bool) Space {
		return Space{Name: name, Root: r.Dir, Worktrees: worktrees, Config: c, Untrusted: untrusted}
	}
	write := func(text string) []byte {
		if err := os.WriteFile(file, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
		return []byte(text)
	}
	check := func(when string, want Space) {
		t.Helper()
		if got := ReadSpace(cfg, r.Dir); !reflect.DeepEqual(got, want) {
			t.Errorf("%s:\n%#v\nwant\n%#v", when, got, want)
		}
	}
	check("no file", space(nil, false))
	data := write("[[commands]]\nname = \"test\"\nrun = [\"make\", \"test\"]\n")
	check("untrusted", space(nil, true))
	if err := Trust(file, data); err != nil {
		t.Fatal(err)
	}
	merged := cfg
	merged.Commands = []config.Command{{Name: "test", Run: []string{"make", "test"}, Open: "tab"}}
	check("trusted", space(&merged, false))
	trust, err := os.ReadFile(filepath.Join(state, "kspaces", "trust"))
	if err != nil {
		t.Fatal(err)
	}
	if want := contentSum(data) + "  " + file + "\n"; string(trust) != want {
		t.Errorf("trust file %q, want %q", trust, want)
	}
	write("[[commands]]\nname = \"test\"\nrun = [\"make\", \"test\", \"-j\"]\n")
	check("changed", space(nil, true))
	if err := Trust(file, data); err != nil {
		t.Fatal(err)
	}
	write(string(data))
	check("changed back", space(&merged, false))
	if err := Revoke(file); err != nil {
		t.Fatal(err)
	}
	check("revoked", space(nil, true))
	if trust, err := os.ReadFile(filepath.Join(state, "kspaces", "trust")); err != nil || string(trust) != "" {
		t.Errorf("trust file after revoking %q, %v", trust, err)
	}
}

// A broken space file, once trusted, is left out with what is wrong with it, and the other spaces and
// the global config are as they were; a sidebar sees both from the leader's load.
func TestBrokenSpaceFile(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	broken, good := gittest.New(t), gittest.New(t)
	broken.Commit("first")
	good.Commit("first")
	text := fmt.Sprintf("spaces = [%q, %q]\n", broken.Dir, good.Dir)
	if err := os.MkdirAll(filepath.Join(os.Getenv("XDG_CONFIG_HOME"), "kspaces"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(os.Getenv("XDG_CONFIG_HOME"), "kspaces", "config.toml"), []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(broken.Dir, config.SpaceFileName)
	data := []byte("layout = \"nope\"\n")
	if err := os.WriteFile(file, data, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Trust(file, data); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Parse(text)
	if err != nil {
		t.Fatal(err)
	}
	// A kitten that fails, so no kitty is asked for its windows, an error of their own.
	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "kitten"), []byte("#!/bin/sh\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("KITTY_INSTALLATION_DIR", "")
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	sh := LoadShared()
	if !reflect.DeepEqual(*sh.Config, cfg) {
		t.Errorf("config %#v, want %#v", *sh.Config, cfg)
	}
	want := []sharedSpace{
		{Name: filepath.Base(broken.Dir), Root: broken.Dir, Worktrees: LoadSpace(broken.Dir).Worktrees,
			ConfigErr: config.Tilde(file) + `: layout is "nope", no layout has that name`},
		{Name: filepath.Base(good.Dir), Root: good.Dir, Worktrees: LoadSpace(good.Dir).Worktrees},
	}
	if !reflect.DeepEqual(sh.Spaces, want) {
		t.Errorf("spaces:\n%#v\nwant\n%#v", sh.Spaces, want)
	}
	if got := sh.View(0).Spaces[0].ConfigErr; !reflect.DeepEqual(got, errors.New(want[0].ConfigErr)) {
		t.Errorf("seen from a sidebar: %v", got)
	}
}

// Saving a layout to a space's file creates the file, trusted as written, and keeps it out of git once in
// info/exclude; then appends, and replaces in place. A file changed since it was trusted is refused, and
// so is saving in place of a layout without saying to replace it.
func TestSaveSpaceLayout(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	r := gittest.New(t)
	r.Commit("first")
	linked := r.Worktree("dev", ".worktrees/dev")
	global, err := config.Parse(fmt.Sprintf("spaces = [%q]\n\n[[commands]]\nname = \"review\"\nrun = [\"gr\"]\n", r.Dir))
	if err != nil {
		t.Fatal(err)
	}
	space := ReadSpace(global, r.Dir)
	file := filepath.Join(r.Dir, config.SpaceFileName)
	exclude := filepath.Join(r.Dir, ".git", "info", "exclude")
	if err := os.WriteFile(exclude, []byte("# mine\n*.log"), 0o644); err != nil {
		t.Fatal(err)
	}
	read := func(path string) string {
		t.Helper()
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		return string(data)
	}
	review := config.Layout{Name: "review", Tabs: []config.LayoutTab{{Panes: []config.Pane{{Command: "review"}}}}}
	shell := config.Layout{Name: "shell", Tabs: []config.LayoutTab{{Panes: []config.Pane{{}}}}}
	if err := SaveSpaceLayout(global, space, review, false, true); err != nil {
		t.Fatal(err)
	}
	block := "[[layouts]]\nname = \"review\"\n[[layouts.tabs]]\npanes = [\n  { command = \"review\" },\n]\n"
	if got := read(file); got != block {
		t.Errorf("created:\n%s\nwant\n%s", got, block)
	}
	if got, want := read(exclude), "# mine\n*.log\n/.kspaces.toml\n"; got != want {
		t.Errorf("exclude %q, want %q", got, want)
	}
	// In use as written, by the linked worktrees too.
	if got, _, err := FindWorktree(global, linked.Dir); err != nil || got.Untrusted || got.Config == nil {
		t.Errorf("not in use as written: %v, untrusted %v, %v", err, got.Untrusted, got.ConfigErr)
	}
	// Made again, the line is there already.
	if err := os.Remove(file); err != nil {
		t.Fatal(err)
	}
	if err := SaveSpaceLayout(global, space, review, false, true); err != nil {
		t.Fatal(err)
	}
	if got, want := read(exclude), "# mine\n*.log\n/.kspaces.toml\n"; got != want {
		t.Errorf("exclude made again %q, want %q", got, want)
	}
	if err := SaveSpaceLayout(global, space, shell, false, true); err != nil {
		t.Fatal(err)
	}
	shellBlock := "[[layouts]]\nname = \"shell\"\n[[layouts.tabs]]\npanes = [\n  {},\n]\n"
	if got, want := read(file), block+"\n"+shellBlock; got != want {
		t.Errorf("appended:\n%s\nwant\n%s", got, want)
	}
	other := config.Layout{Name: "review", Tabs: []config.LayoutTab{{Panes: []config.Pane{{Run: []string{"gr", "--watch"}}}}}}
	err = SaveSpaceLayout(global, space, other, false, true)
	if want := file + ": a layout called review exists"; err == nil || err.Error() != want {
		t.Errorf("without replacing: %v, want %s", err, want)
	}
	if err := SaveSpaceLayout(global, space, other, true, true); err != nil {
		t.Fatal(err)
	}
	replaced := "[[layouts]]\nname = \"review\"\n[[layouts.tabs]]\npanes = [\n  { run = [\"gr\", \"--watch\"] },\n]\n\n" + shellBlock
	if got := read(file); got != replaced {
		t.Errorf("replaced:\n%s\nwant\n%s", got, replaced)
	}
	if err := os.WriteFile(file, []byte(replaced+"# edited\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	err = SaveSpaceLayout(global, space, shell, true, true)
	if want := file + ": " + ErrUntrusted.Error(); err == nil || err.Error() != want {
		t.Errorf("untrusted: %v, want %s", err, want)
	}
	if got := read(file); got != replaced+"# edited\n" {
		t.Errorf("untrusted, the file became:\n%s", got)
	}
}
