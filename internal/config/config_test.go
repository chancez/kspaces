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
trace = "~/tea.log"

[sidebar.popup_size]
picker = "150x40"

[kitty]
auto_install = true

[git]
timeout = "1m"
fetch_timeout = 0

[editor]
vim = true

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
		Sidebar:     Sidebar{Width: 40, Popup: PopupPanel, PopupSize: PopupSizes{Picker: PopupSize{Columns: 150, Lines: 40}}, Trace: filepath.Join(home, "tea.log")},
		Kitty:       Kitty{AutoInstall: true},
		Git:         Git{Timeout: time.Minute},
		Editor:      Editor{Vim: true},
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
		"[sidebar.popup_size]\nform = \"90\"",
		"[sidebar.popup_size]\nform = \"90x\"",
		"[sidebar.popup_size]\nform = \"39x24\"",
		"[sidebar.popup_size]\nlist = \"90x9\"",
		"[sidebar.popup_size]\npicker = 120",
		"[sidebar.popup_size]\nwindow = \"90x24\"",
		"[sidebar.popup_size]\nform = \"0%x50%\"",
		"[sidebar.popup_size]\nform = \"101%x50%\"",
		"[sidebar.popup_size]\nform = \"%x50%\"",
		"[sidebar.popup_size]\nform = \"50%x9\"",
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

// Each popup size is one setting: two files may set different ones, not the same one.
func TestParsePopupSizesAcrossFiles(t *testing.T) {
	got, err := parse([]string{"", ""}, []string{"[sidebar.popup_size]\nform = \"80x20\"", "[sidebar.popup_size]\npicker = \"150x40\""})
	if err != nil {
		t.Fatal(err)
	}
	want := PopupSizes{Form: PopupSize{Columns: 80, Lines: 20}, Picker: PopupSize{Columns: 150, Lines: 40}}
	if got.Sidebar.PopupSize != want {
		t.Errorf("got %+v, want %+v", got.Sidebar.PopupSize, want)
	}
	if _, err := parse([]string{"", ""}, []string{"[sidebar.popup_size]\nform = \"80x20\"", "[sidebar.popup_size]\nform = \"90x24\""}); err == nil {
		t.Error("form set in two files was accepted")
	}
}

// A size is in cells or a percentage of the screen, each way round; a percentage is no smaller than the
// smallest panel, and either no larger than the screen.
func TestPopupSizeCells(t *testing.T) {
	type cells struct{ columns, lines int }
	for _, c := range []struct {
		text string
		size PopupSize
		// On a screen of 430x88 cells, and of 50x12.
		big, small cells
	}{
		{"90x24", PopupSize{Columns: 90, Lines: 24}, cells{90, 24}, cells{50, 12}},
		{"60%x70%", PopupSize{Columns: 60, ColumnsPercent: true, Lines: 70, LinesPercent: true}, cells{258, 61}, cells{40, 10}},
		{"120x50%", PopupSize{Columns: 120, Lines: 50, LinesPercent: true}, cells{120, 44}, cells{50, 10}},
		{"100%x40", PopupSize{Columns: 100, ColumnsPercent: true, Lines: 40}, cells{430, 40}, cells{50, 12}},
	} {
		var got PopupSize
		if err := got.UnmarshalText([]byte(c.text)); err != nil {
			t.Fatal(err)
		}
		if got != c.size || got.String() != c.text {
			t.Errorf("%q read as %+v, written %q, want %+v", c.text, got, got.String(), c.size)
		}
		var big, small cells
		big.columns, big.lines = got.Cells(430, 88)
		small.columns, small.lines = got.Cells(50, 12)
		if big != c.big || small != c.small {
			t.Errorf("%q: %v on the big screen, %v on the small one, want %v and %v", c.text, big, small, c.big, c.small)
		}
	}
}
