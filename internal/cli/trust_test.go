package cli

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/chancez/kspaces/internal/config"
	"github.com/chancez/kspaces/internal/gittest"
	"github.com/chancez/kspaces/internal/spaces"
)

// trust --yes trusts a space's file from any of its worktrees, so list no longer says it is untrusted;
// --revoke forgets it again. A broken file, or a space without one, is an error, and list says what is
// wrong with a broken one trusted.
func TestTrust(t *testing.T) {
	r := gittest.New(t)
	r.Commit("first")
	linked := r.Worktree("dev", ".worktrees/dev")
	other := gittest.New(t)
	other.Commit("first")
	cfgDir, state := t.TempDir(), t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", cfgDir)
	t.Setenv("XDG_STATE_HOME", state)
	if err := os.MkdirAll(filepath.Join(cfgDir, "kspaces"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cfgDir, "kspaces", "config.toml"), []byte(fmt.Sprintf("spaces = [%q, %q]\n", r.Dir, other.Dir)), 0o644); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(r.Dir, config.SpaceFileName)
	write := func(text string) {
		if err := os.WriteFile(file, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("[[commands]]\nname = \"test\"\nrun = [\"make\", \"test\"]\n")
	run := func(args ...string) string {
		var out bytes.Buffer
		a := &app{stdout: &out, stderr: &out}
		cmd := a.rootCommand()
		cmd.SetArgs(args)
		if err := cmd.Execute(); err != nil {
			return "error: " + err.Error()
		}
		return out.String()
	}
	name, otherName := filepath.Base(r.Dir), filepath.Base(other.Dir)
	worktrees := fmt.Sprintf("%s\t%s\tmain\n%s@dev\t%s\tdev\n", name, r.Dir, name, linked.Dir)
	others := fmt.Sprintf("%s\t%s\tmain\n", otherName, other.Dir)
	untrusted := fmt.Sprintf("%s\t%s\tuntrusted config: kspaces trust %s\n", name, file, r.Dir)
	got := []string{
		run("list"),
		run("trust", "--yes", linked.Dir),
		run("list"),
		run("trust", "--revoke", file),
		run("list"),
		run("trust", "--yes", other.Dir),
		run("trust", "--yes", "--revoke", r.Dir),
	}
	write("nope = 1\n")
	got = append(got, run("trust", "--yes", r.Dir))
	// Trusted while broken, as when trusted and then a layout it names goes from the global config.
	if err := spaces.Trust(file, []byte("nope = 1\n")); err != nil {
		t.Fatal(err)
	}
	got = append(got, run("list"))
	want := []string{
		worktrees + untrusted + others,
		"",
		worktrees + others,
		"",
		worktrees + untrusted + others,
		"error: " + otherName + " has no .kspaces.toml",
		"error: if any flags in the group [yes revoke] are set none of the others can be; [revoke yes] were all set",
		"error: " + config.Tilde(file) + ": unknown keys: nope",
		worktrees + fmt.Sprintf("%s\t%s\terror: %s: unknown keys: nope\n", name, r.Dir, config.Tilde(file)) + others,
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got\n%q\nwant\n%q", got, want)
	}
}
