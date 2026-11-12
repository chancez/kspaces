package config

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestParse(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	in := `
spaces = ["~/projects/gr", "/src/kitty"]
launch = ["kitten", "my_launch.py"]
sessions = "~/.local/share/kitty/sessions"

[worktrees]
branch_prefix = "pr/me"

[sidebar]
width = 40
popup = "panel"

[kitty]
auto_install = true
`
	got, err := Parse(in)
	if err != nil {
		t.Fatal(err)
	}
	// worktrees.dir is left out, so it has its default.
	want := Config{
		Spaces:      []string{filepath.Join(home, "projects/gr"), "/src/kitty"},
		Launch:      []string{"kitten", "my_launch.py"},
		SessionsDir: filepath.Join(home, ".local/share/kitty/sessions"),
		Worktrees:   Worktrees{Dir: ".worktrees", BranchPrefix: "pr/me"},
		Sidebar:     Sidebar{Width: 40, Popup: PopupPanel},
		Kitty:       Kitty{AutoInstall: true},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v\nwant %#v", got, want)
	}

	// Nothing set gives the defaults.
	got, err = Parse("")
	if err != nil {
		t.Fatal(err)
	}
	if want := (Config{Launch: defaultLaunch, Worktrees: Worktrees{Dir: ".worktrees"}, Sidebar: Sidebar{Width: 28, Popup: PopupOverlay}}); !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v\nwant %#v", got, want)
	}

	for _, bad := range []string{
		`space = ["~/x"]`,                 // a typo of spaces
		"[worktrees]\nprefix = \"pr/me\"", // a typo inside a table
		`launch = []`,
		`new_tab = ["launch"]`, // renamed launch
		`spaces = "~/x"`,       // a string, not a list
		"[sidebar]\nwidth = 11",
		"[sidebar]\nwidth = 121",
		"[sidebar]\npopup = \"window\"",
		"[sidebar]\npopup = \"\"",
	} {
		if _, err := Parse(bad); err == nil {
			t.Fatalf("%q was accepted", bad)
		}
	}
}

// Tilde writes the home directory as ~/, and leaves the home directory itself and paths outside it, or
// beside it with the same prefix, as they are.
func TestTilde(t *testing.T) {
	t.Setenv("HOME", "/home/me")
	var got []string
	for _, p := range []string{"/home/me/src/gr", "/home/me", "/home/me/", "/home/mee/x", "/src/gr"} {
		got = append(got, Tilde(p))
	}
	want := []string{"~/src/gr", "/home/me", "/home/me/", "/home/mee/x", "/src/gr"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %q, want %q", got, want)
	}
}
