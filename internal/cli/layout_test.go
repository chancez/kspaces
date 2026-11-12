package cli

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/chancez/kspaces/internal/gittest"
)

// layout save takes a name and the path of a worktree, and saves its session's tabs, which kitty must
// have: one with none is an error. It takes at most the two.
func TestLayoutSave(t *testing.T) {
	r := gittest.New(t)
	r.Commit("first")
	cfgDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", cfgDir)
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	if err := os.MkdirAll(filepath.Join(cfgDir, "kspaces"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cfgDir, "kspaces", "config.toml"), []byte(fmt.Sprintf("spaces = [%q]\n", r.Dir)), 0o644); err != nil {
		t.Fatal(err)
	}
	// A kitten whose kitty has no windows.
	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "kitten"), []byte("#!/bin/sh\necho '[]'\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("KITTY_INSTALLATION_DIR", "")
	t.Setenv("KITTY_WINDOW_ID", "")
	var got []string
	for _, args := range [][]string{
		{"agent", r.Dir},
		{"--tab", "agent", r.Dir},
		{"a", "b", "c"},
	} {
		var out bytes.Buffer
		a := &app{stdout: &out, stderr: &out}
		cmd := a.rootCommand()
		cmd.SetArgs(append([]string{"layout", "save"}, args...))
		got = append(got, fmt.Sprint(cmd.Execute()))
	}
	name := filepath.Base(r.Dir)
	want := []string{name + " has no windows to save", name + " has no windows to save", "accepts at most 2 arg(s), received 3"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %q\nwant %q", got, want)
	}
}
