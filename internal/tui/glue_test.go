package tui

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/chancez/kspaces/internal/config"
	"github.com/chancez/kspaces/internal/kittyglue"
	"github.com/chancez/kspaces/internal/tui/look"
)

// A sidebar started by an installed watcher checks that install's files: up to date says nothing, out of
// date says so, and with auto_install rewrites them and says to restart kitty.
func TestCheckGlue(t *testing.T) {
	cfgHome, dir := t.TempDir(), filepath.Join(t.TempDir(), "kspaces")
	t.Setenv("XDG_CONFIG_HOME", cfgHome)
	writeConfig := func(s string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Join(cfgHome, "kspaces"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(cfgHome, "kspaces", "config.toml"), []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	writeConfig("")

	t.Setenv(glueDirEnv, "")
	var got []tea.Msg
	got = append(got, checkGlue())
	t.Setenv(glueDirEnv, dir)
	got = append(got, checkGlue())
	if err := kittyglue.Install(config.Config{}, dir); err != nil {
		t.Fatal(err)
	}
	got = append(got, checkGlue())
	// Turning the session saver on makes the installed files out of date.
	writeConfig("[kitty]\nsave_sessions = true\nauto_install = true\n")
	got = append(got, checkGlue(), checkGlue())
	want := []tea.Msg{
		nil, // not started by an installed watcher
		glueMsg{look.Error("kitty files out of date")},
		nil,
		glueMsg{look.Busy("restart kitty for new files")},
		nil, // the first check installed them
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("checks = %#v\nwant %#v", got, want)
	}
}
