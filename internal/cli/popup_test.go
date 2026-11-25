package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"testing"
)

// --size opens the panel of a command run from a mapping at that size, rather than the config's.
func TestPopupSizeFlag(t *testing.T) {
	dir, config := t.TempDir(), t.TempDir()
	log := filepath.Join(dir, "calls")
	ls := `[{"is_focused": true, "tabs": [{"id": 1, "is_active": true, "windows": [{"id": 11, "session_name": "gr", "is_active": true}]}]}]`
	kitten := "#!/bin/sh\n[ \"$2\" = ls ] && echo '" + ls + "' && exit 0\necho \"$*\" >> " + log + "\n"
	if err := os.WriteFile(filepath.Join(dir, "kitten"), []byte(kitten), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(config, "kspaces"), 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := "spaces = []\n[sidebar]\npopup = \"panel\"\n[sidebar.popup_size]\nform = \"80x20\"\n"
	if err := os.WriteFile(filepath.Join(config, "kspaces", "kspaces.toml"), []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_CONFIG_HOME", config)
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("KITTY_INSTALLATION_DIR", "")
	t.Setenv("KITTY_WINDOW_ID", "")
	t.Setenv("KSPACES_FROM", "")
	size := regexp.MustCompile(`columns=(\d+) --os-panel lines=(\d+)`)
	run := func(args ...string) []string {
		os.Remove(log)
		var out bytes.Buffer
		cmd := (&app{stdout: &out, stderr: &out}).rootCommand()
		cmd.SetArgs(args)
		if err := cmd.Execute(); err != nil {
			t.Fatalf("%q: %v\n%s", args, err, out.String())
		}
		calls, _ := os.ReadFile(log)
		return size.FindStringSubmatch(string(calls))[1:]
	}
	for _, c := range []struct {
		args []string
		want []string
	}{
		{[]string{"pick", "--from", "11"}, []string{"120", "32"}},
		{[]string{"pick", "--from", "11", "--size", "150x40"}, []string{"150", "40"}},
		{[]string{"add", "--from", "11"}, []string{"80", "20"}},
		{[]string{"add", "--from", "11", "--size", "60x16"}, []string{"60", "16"}},
	} {
		if got := run(c.args...); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%q opened a panel of %q, want %q", c.args, got, c.want)
		}
	}
}
