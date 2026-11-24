package config

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestParse(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	in := `
spaces = ["~/projects/gr", "/src/kitty"]
groups = ["personal", "work"]
launch = ["kitten", "my_launch.py"]
sessions = "~/.local/share/kitty/sessions"

[worktrees]
branch_prefix = "pr/me"
delete_branch = true

[sidebar]
width = 40
popup = "panel"

[kitty]
auto_install = true

[git]
timeout = "1m"
fetch_timeout = 0

[space.gr]
group = "work"
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
		Worktrees:   Worktrees{Dir: ".worktrees", BranchPrefix: "pr/me", DeleteBranch: true},
		Sidebar:     Sidebar{Width: 40, Popup: PopupPanel},
		Kitty:       Kitty{AutoInstall: true},
		Git:         Git{Timeout: time.Minute},
		Space:       map[string]SpaceSettings{"gr": {Group: "work"}},
		Groups:      []string{"personal", "work"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v\nwant %#v", got, want)
	}

	// Nothing set gives the defaults.
	got, err = Parse("")
	if err != nil {
		t.Fatal(err)
	}
	if want := (Config{Launch: defaultLaunch, Worktrees: Worktrees{Dir: ".worktrees"}, Sidebar: Sidebar{Width: 28, Popup: PopupOverlay}, Git: Git{Timeout: 30 * time.Second, FetchTimeout: 2 * time.Minute}}); !reflect.DeepEqual(got, want) {
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
		"[git]\ntimeout = \"-1s\"",
		"[git]\ntimeout = \"soon\"",
		"[git]\ntimeout = 30", // nanoseconds
		"[git]\nfetch_timeout = \"-1m\"",
	} {
		if _, err := Parse(bad); err == nil {
			t.Fatalf("%q was accepted", bad)
		}
	}
}

// The order of groups names each group once, and a space table with a group names a space.
func TestParseBadGroups(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		{`groups = ["work", ""]`, "groups: group 2 has no name"},
		{`groups = ["work", "personal", "work"]`, "groups: work is listed twice"},
		{`groups = "work"`, `toml: line 1 (last key "groups"): incompatible types: TOML value has type string; destination has type slice`},
		{"spaces = [\"/src/gr\"]\n[space.vim]\ngroup = \"work\"", "space.vim: no space is named vim"},
	} {
		_, err := Parse(c.in)
		if got := errString(err); got != c.want {
			t.Errorf("%s:\ngot  %s\nwant %s", c.in, got, c.want)
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

// keeper is provider's earlier name, read as it; both is an error, and a space's own file has neither.
func TestParseProvider(t *testing.T) {
	type result struct {
		Provider []string
		Keeper   []string
		Err      string
	}
	var got []result
	for _, in := range []string{
		`provider = ["kspaces-provider-cm"]`,
		`keeper = ["kspaces-keeper-cm"]`,
		"provider = [\"a\"]\nkeeper = [\"b\"]",
		``,
	} {
		cfg, err := Parse(in)
		got = append(got, result{cfg.Provider, cfg.Keeper, errString(err)})
	}
	want := []result{
		{[]string{"kspaces-provider-cm"}, nil, "<nil>"},
		{[]string{"kspaces-keeper-cm"}, nil, "<nil>"},
		{nil, nil, "keeper is provider's earlier name; set provider alone"},
		{nil, nil, "<nil>"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got  %q\nwant %q", got, want)
	}
}
