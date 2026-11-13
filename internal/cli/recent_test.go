package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// recent, run from a mapping, hands the focused tab back to its own window and switches to the session
// focused before the focused window's; with no other session it says so.
func TestRecent(t *testing.T) {
	dir, config := t.TempDir(), t.TempDir()
	log, ls := filepath.Join(dir, "calls"), filepath.Join(dir, "ls.json")
	kitten := "#!/bin/sh\n[ \"$2\" = ls ] && cat " + ls + " && exit 0\necho \"$*\" >> " + log + "\n"
	if err := os.WriteFile(filepath.Join(dir, "kitten"), []byte(kitten), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(config, "kspaces"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(config, "kspaces", "config.toml"), []byte("spaces = []\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_CONFIG_HOME", config)
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("KITTY_INSTALLATION_DIR", "")
	t.Setenv("KITTY_WINDOW_ID", "")
	t.Setenv("KSPACES_FROM", "")
	run := func(data string) ([]string, error) {
		if err := os.WriteFile(ls, []byte(data), 0o644); err != nil {
			t.Fatal(err)
		}
		os.Remove(log)
		var out bytes.Buffer
		cmd := (&app{stdout: &out, stderr: &out}).rootCommand()
		cmd.SetArgs([]string{"recent"})
		err := cmd.Execute()
		calls, _ := os.ReadFile(log)
		return strings.Split(strings.TrimSpace(string(calls)), "\n"), err
	}

	// The keyboard is in gr's tab, its sidebar's dock focused last; default was focused before gr.
	got, err := run(`[{"is_focused": true, "tabs": [
		{"id": 1, "is_active": true, "windows": [
			{"id": 10, "session_name": "gr", "last_focused_at": 8},
			{"id": 11, "session_name": "gr", "last_focused_at": 9, "is_active": true, "user_vars": {"kspaces": "1"}}]},
		{"id": 2, "windows": [{"id": 20, "session_name": "gr@dev", "last_focused_at": 3}]},
		{"id": 3, "windows": [{"id": 30, "session_name": "default", "last_focused_at": 5}]}]}]`)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"@ focus-window --match id:10", "@ action goto_session default.kitty-session"}; !reflect.DeepEqual(got, want) {
		t.Errorf("recent ran\n%q\nwant\n%q", got, want)
	}

	got, err = run(`[{"is_focused": true, "tabs": [{"id": 1, "is_active": true, "windows": [
		{"id": 10, "session_name": "gr", "last_focused_at": 8, "is_active": true}]}]}]`)
	if err == nil || err.Error() != "no other session has windows open" {
		t.Errorf("recent alone: got error %v", err)
	}
	if want := []string{""}; !reflect.DeepEqual(got, want) {
		t.Errorf("recent alone ran\n%q\nwant\n%q", got, want)
	}
}
