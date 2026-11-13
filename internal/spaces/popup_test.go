package spaces

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/chancez/kspaces/internal/config"
	"github.com/chancez/kspaces/internal/kitty"
)

// fakeLaunch puts a kitten on PATH that records its arguments, and returns them as the last call had them.
func fakeLaunch(t *testing.T) func() []string {
	t.Helper()
	dir := t.TempDir()
	log := filepath.Join(dir, "args")
	script := "#!/bin/sh\nfor a; do printf '%s\\n' \"$a\"; done > " + log + "\n"
	if err := os.WriteFile(filepath.Join(dir, "kitten"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	t.Setenv("KITTY_INSTALLATION_DIR", "")
	return func() []string {
		data, err := os.ReadFile(log)
		if err != nil {
			return nil
		}
		return strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
	}
}

// With popup = "panel" each form opens in a panel of its size for the window beside the sidebar, and the
// picker for the window it was asked for; with the default, in an overlay over that window.
func TestPopups(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	st := kitty.State{Current: "gr", Home: 11}
	panel := func(columns, lines, v string, args []string) []string {
		return append([]string{
			"@", "launch", "--type=os-panel", "--os-panel", "edge=center-sized", "--os-panel", "columns=" + columns,
			"--os-panel", "lines=" + lines, "--os-panel", "focus-policy=exclusive", "--os-panel", "layer=overlay",
			"--var=" + v + "=1", "--var=kspaces_popup=1", "--env", "KSPACES_FROM=11", exe,
		}, args...)
	}
	overlay := func(v string, args []string) []string {
		return append([]string{"@", "launch", "--type=overlay", "--var=" + v + "=1", "--next-to", "id:11", exe}, args...)
	}
	const form, picker = "kspaces_form", "kspaces_picker"
	for _, c := range []struct {
		open           func(config.Config) error
		columns, lines string
		v              string
		args           []string
	}{
		{func(cfg config.Config) error { return NewWorktree(cfg, st, "/src/gr") }, "90", "24", form, []string{"new", "/src/gr"}},
		{func(cfg config.Config) error { return AskRemoveWorktree(cfg, st, "/src/gr/dev") }, "90", "24", form, []string{"remove", "/src/gr/dev"}},
		{func(cfg config.Config) error { return AskRunCommand(cfg, st, "claude", "/src/gr/dev") }, "90", "24", form, []string{"run", "claude", "/src/gr/dev"}},
		{func(cfg config.Config) error { return AskRunCommand(cfg, st, "claude", "/src/gr/dev", "/src/gr/fix") }, "100", "32", form, []string{"run", "claude", "/src/gr/dev", "/src/gr/fix"}},
		{func(cfg config.Config) error { return AskOpenLayout(cfg, st, "review", "/src/gr/dev") }, "90", "24", form, []string{"layout", "review", "/src/gr/dev"}},
		{func(cfg config.Config) error { return AskSaveLayout(cfg, st, "/src/gr/dev") }, "100", "32", form, []string{"layout", "save", "", "/src/gr/dev"}},
		{func(cfg config.Config) error { return AskCloseSession(cfg, st, "gr@dev") }, "90", "24", form, []string{"close", "gr@dev"}},
		{func(cfg config.Config) error { return AskCloseSession(cfg, st, "gr@dev", "gr@fix") }, "100", "32", form, []string{"close", "gr@dev", "gr@fix"}},
		{func(cfg config.Config) error { return AskForgetSession(cfg, st, "gr@dev") }, "90", "24", form, []string{"forget", "gr@dev"}},
		{func(cfg config.Config) error { return AskForgetSession(cfg, st, "gr@dev", "gr@gone") }, "100", "32", form, []string{"forget", "gr@dev", "gr@gone"}},
		{func(cfg config.Config) error { return AskTrust(cfg, st, "/src/gr") }, "100", "32", form, []string{"trust", "/src/gr"}},
		{func(cfg config.Config) error { return AskCleanup(cfg, st, "/src/gr") }, "100", "32", form, []string{"cleanup", "/src/gr"}},
		{func(cfg config.Config) error { return AskRemoveSessions(cfg, st, []string{"gr@dev", "gr@gone"}) }, "100", "32", form, []string{"cleanup", "--session", "gr@dev", "--session", "gr@gone"}},
		{func(cfg config.Config) error { return AskAddSpace(cfg, st) }, "90", "24", form, []string{"add"}},
		{func(cfg config.Config) error { return LaunchPicker(cfg, 11) }, "120", "32", picker, []string{"pick"}},
	} {
		launched := fakeLaunch(t)
		if err := c.open(config.Config{Sidebar: config.Sidebar{Popup: config.PopupPanel}}); err != nil {
			t.Fatal(err)
		}
		if got, want := launched(), panel(c.columns, c.lines, c.v, c.args); !reflect.DeepEqual(got, want) {
			t.Errorf("panel: launched %q\nwant %q", got, want)
		}
		if err := c.open(config.Config{Sidebar: config.Sidebar{Popup: config.PopupOverlay}}); err != nil {
			t.Fatal(err)
		}
		if got, want := launched(), overlay(c.v, c.args); !reflect.DeepEqual(got, want) {
			t.Errorf("overlay: launched %q\nwant %q", got, want)
		}
	}
}
