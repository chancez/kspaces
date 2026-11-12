package cli

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/chancez/kspaces/internal/gittest"
)

// open --no-layout opens the worktree's session with one tab though it has a default layout, the
// current directory's worktree without a PATH.
func TestOpenNoLayout(t *testing.T) {
	r := gittest.New(t)
	r.Commit("first")
	dir, config := t.TempDir(), t.TempDir()
	log := filepath.Join(dir, "calls")
	kitten := "#!/bin/sh\n[ \"$2\" = ls ] && echo '[]' && exit 0\necho \"$*\" >> " + log + "\n"
	if err := os.WriteFile(filepath.Join(dir, "kitten"), []byte(kitten), 0o755); err != nil {
		t.Fatal(err)
	}
	toml := fmt.Sprintf("spaces = [%q]\nlayout = \"two\"\n\n[[layouts]]\nname = \"two\"\ntabs = [{ panes = [{}, { split = \"right\" }] }]\n", r.Dir)
	if err := os.MkdirAll(filepath.Join(config, "kspaces"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(config, "kspaces", "config.toml"), []byte(toml), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_CONFIG_HOME", config)
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("KITTY_INSTALLATION_DIR", "")
	t.Setenv("KITTY_WINDOW_ID", "")
	root, err := filepath.EvalSymlinks(r.Dir)
	if err != nil {
		t.Fatal(err)
	}
	run := func(args ...string) []string {
		var out bytes.Buffer
		cmd := (&app{stdout: &out, stderr: &out}).rootCommand()
		cmd.SetArgs(append([]string{"open", "--no-layout"}, args...))
		if err := cmd.Execute(); err != nil {
			t.Fatal(err)
		}
		data, err := os.ReadFile(log)
		if err != nil {
			t.Fatal(err)
		}
		os.Remove(log)
		return strings.Split(strings.TrimSpace(string(data)), "\n")
	}
	session := filepath.Base(root)
	want := []string{
		"@ action launch --type=tab --cwd=" + root + " --add-to-session=" + session,
		"@ action goto_session " + session + ".kitty-session",
	}
	if got := run(r.Dir); !reflect.DeepEqual(got, want) {
		t.Errorf("open --no-layout PATH ran\n%q\nwant\n%q", got, want)
	}
	t.Chdir(r.Dir)
	if got := run(); !reflect.DeepEqual(got, want) {
		t.Errorf("open --no-layout ran\n%q\nwant\n%q", got, want)
	}
}
