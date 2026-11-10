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
new_tab = ["kitten", "my_launch.py", "--type=tab"]
sessions = "~/.local/share/kitty/sessions"

[worktrees]
branch_prefix = "pr/me"

[sidebar]
width = 40
`
	got, err := Parse(in)
	if err != nil {
		t.Fatal(err)
	}
	// worktrees.dir is left out, so it has its default.
	want := Config{
		Spaces:      []string{filepath.Join(home, "projects/gr"), "/src/kitty"},
		NewTab:      []string{"kitten", "my_launch.py", "--type=tab"},
		SessionsDir: filepath.Join(home, ".local/share/kitty/sessions"),
		Worktrees:   Worktrees{Dir: ".worktrees", BranchPrefix: "pr/me"},
		Sidebar:     Sidebar{Width: 40},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v\nwant %#v", got, want)
	}

	// Nothing set gives the defaults.
	got, err = Parse("")
	if err != nil {
		t.Fatal(err)
	}
	if want := (Config{NewTab: defaultNewTab, Worktrees: Worktrees{Dir: ".worktrees"}, Sidebar: Sidebar{Width: 28}}); !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v\nwant %#v", got, want)
	}

	for _, bad := range []string{
		`space = ["~/x"]`,                 // a typo of spaces
		"[worktrees]\nprefix = \"pr/me\"", // a typo inside a table
		`new_tab = []`,
		`spaces = "~/x"`, // a string, not a list
		"[sidebar]\nwidth = 11",
		"[sidebar]\nwidth = 121",
	} {
		if _, err := Parse(bad); err == nil {
			t.Fatalf("%q was accepted", bad)
		}
	}
}
