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
