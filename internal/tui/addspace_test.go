package tui

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// addSpaceEnv is a home directory with a repository at ~/repo, a directory in no repository at ~/plain and
// a config listing ~/other, with a comment, in a config directory of the test's own.
func addSpaceEnv(t *testing.T) (home, config string) {
	t.Helper()
	home, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	for _, dir := range []string{filepath.Join(home, "repo", "sub"), filepath.Join(home, "plain"), filepath.Join(home, ".config", "kspaces")} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	git := exec.Command("git", "-C", filepath.Join(home, "repo"), "init", "-q")
	git.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null")
	if out, err := git.CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
	config = filepath.Join(home, ".config", "kspaces", "config.toml")
	if err := os.WriteFile(config, []byte("# Mine.\nspaces = [\n  \"~/other\", # old\n]\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return home, config
}

func formScreen(f interface{ render() string }) []string {
	var out []string
	for _, l := range strings.Split(ansi.Strip(f.render()), "\n") {
		out = append(out, strings.TrimRight(l, " "))
	}
	return out
}

// The form shows the repository a path adds, ~ for the home directory, refuses one that cannot be added
// before anything is written, and adds to the config keeping its comments.
func TestAddSpaceForm(t *testing.T) {
	home, config := addSpaceEnv(t)
	if err := os.Symlink(filepath.Join(home, "repo"), filepath.Join(home, "link")); err != nil {
		t.Fatal(err)
	}
	roots, err := configuredSpaces()
	if err != nil {
		t.Fatal(err)
	}
	f := newAddSpaceForm(roots, filepath.Join(home, "repo", "sub"))
	f.Update(tea.WindowSizeMsg{Width: 70, Height: 20})
	screen := func(path, says, buttons string) []string {
		return []string{"", "  Add a space", "", "  repository " + path, "", "  " + says, "", "   " + buttons, ""}
	}
	if got, want := formScreen(f), screen("~/repo/sub", "Adds ~/repo to the spaces", "Add enter    Cancel esc"); !reflect.DeepEqual(got, want) {
		t.Errorf("a directory in a repository:\n%q\nwant\n%q", got, want)
	}

	// Each path the form cannot add greys Add, and enter adds nothing.
	type step struct {
		typed string
		want  []string
	}
	for _, s := range []step{
		{"", screen("a directory in a git repository", "Name a directory in the repository to add", "Add    Cancel esc")},
		{"~/plain", screen("~/plain", "~/plain is not in a git repository", "Add    Cancel esc")},
		{"~/nothing", screen("~/nothing", "~/nothing is not a directory", "Add    Cancel esc")},
	} {
		f.Update(keyMsg("ctrl+u"))
		typeInto(f, s.typed)
		f.Update(keyMsg("enter"))
		if got := formScreen(f); !reflect.DeepEqual(got, s.want) || f.busy {
			t.Errorf("%q: busy %v\n%q\nwant\n%q", s.typed, f.busy, got, s.want)
		}
	}
	// A space already, through a symlink in the config.
	f = newAddSpaceForm([]string{filepath.Join(home, "link")}, "~/repo")
	f.Update(tea.WindowSizeMsg{Width: 70, Height: 20})
	if got, want := formScreen(f), screen("~/repo", "~/repo is already a space", "Add    Cancel esc"); !reflect.DeepEqual(got, want) {
		t.Errorf("a space already:\n%q\nwant\n%q", got, want)
	}

	f = newAddSpaceForm(roots, "~/repo")
	f.Update(tea.WindowSizeMsg{Width: 70, Height: 20})
	_, cmd := f.Update(keyMsg("enter"))
	if want := append(screen("~/repo", "Adds ~/repo to the spaces", "Cancel esc")[:7], "  Adding...", "", "   Cancel esc", ""); cmd == nil || !reflect.DeepEqual(formScreen(f), want) {
		t.Fatalf("enter: cmd %v\n%q\nwant\n%q", cmd != nil, formScreen(f), want)
	}
	if _, quit := f.Update(cmd()); quit == nil || quit() != tea.Quit() {
		t.Errorf("the form stayed after adding: %v", f.err)
	}
	data, err := os.ReadFile(config)
	if want := "# Mine.\nspaces = [\n  \"~/other\", # old\n  \"~/repo\",\n]\n"; err != nil || string(data) != want {
		t.Errorf("config %q, %v\nwant %q", data, err, want)
	}
	// A second form, as from another sidebar, refuses it now.
	roots, _ = configuredSpaces()
	if got := newAddSpaceForm(roots, "~/repo/sub").problem; got == nil || got.Error() != "~/repo is already a space" {
		t.Errorf("adding it again: %v", got)
	}
	if _, cmd := f.Update(keyMsg("esc")); cmd == nil || cmd() != tea.Quit() {
		t.Error("esc does not quit")
	}
}

// AddSpace adds without asking, and says why it cannot.
func TestAddSpaceWithoutAsking(t *testing.T) {
	home, config := addSpaceEnv(t)
	got := []string{}
	for _, p := range []string{filepath.Join(home, "plain"), filepath.Join(home, "repo", "sub"), "~/repo"} {
		msg := "added"
		if err := AddSpace(p); err != nil {
			msg = err.Error()
		}
		got = append(got, msg)
	}
	data, _ := os.ReadFile(config)
	got = append(got, string(data))
	want := []string{
		filepath.Join(home, "plain") + " is not in a git repository",
		"added",
		"~/repo is already a space",
		"# Mine.\nspaces = [\n  \"~/other\", # old\n  \"~/repo\",\n]\n",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %q\nwant %q", got, want)
	}
}
