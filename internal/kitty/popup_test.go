package kitty

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

// fakeKitten puts a kitten on PATH that records each call, its arguments one a line, prints ls for
// `@ ls`, and fails a launch of a panel when refuse is not empty, as kitty does on X11. It returns the
// calls so far.
func fakeKitten(t *testing.T, ls, refuse string) func() [][]string {
	t.Helper()
	dir := t.TempDir()
	log := filepath.Join(dir, "calls")
	// Only shell builtins: PATH is this directory alone.
	script := `#!/bin/sh
for a; do printf '%s\n' "$a"; done >> ` + log + `
echo -- >> ` + log + `
case "$2 $3" in
"ls ") printf '%s\n' '` + ls + `' ;;
"launch --type=os-panel") if [ -n "` + refuse + `" ]; then echo "` + refuse + `" >&2; exit 1; fi ;;
esac
`
	if err := os.WriteFile(filepath.Join(dir, "kitten"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	t.Setenv("KITTY_INSTALLATION_DIR", "")
	return func() [][]string {
		data, err := os.ReadFile(log)
		if err != nil {
			return nil
		}
		var out [][]string
		for _, call := range strings.Split(strings.TrimSuffix(string(data), "--\n"), "--\n") {
			out = append(out, strings.Split(strings.TrimSuffix(call, "\n"), "\n"))
		}
		return out
	}
}

// A panel is centered, of the popup's size, takes the keyboard, sits above other windows and is told
// the window it is for; an overlay goes over that window, as it always has.
func TestLaunchPopup(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	panel := Popup{Panel: true, Columns: 90, Lines: 24}
	panelLaunch := []string{
		"@", "launch", "--type=os-panel", "--os-panel", "edge=center-sized", "--os-panel", "columns=90",
		"--os-panel", "lines=24", "--os-panel", "focus-policy=exclusive", "--os-panel", "layer=overlay",
		"--var=kspaces_form=1", "--env", "KSPACES_FROM=11", exe, "new", "/src/gr",
	}
	overlayLaunch := []string{"@", "launch", "--type=overlay", "--var=kspaces_form=1", "--next-to", "id:11", exe, "new", "/src/gr"}
	for _, c := range []struct {
		name   string
		p      Popup
		from   int
		refuse string
		want   [][]string
	}{
		{"overlay", Popup{}, 11, "", [][]string{overlayLaunch}},
		{"overlay over the active window", Popup{}, 0, "", [][]string{{"@", "launch", "--type=overlay", "--var=kspaces_form=1", exe, "new", "/src/gr"}}},
		{"panel", panel, 11, "", [][]string{panelLaunch}},
		// With no window to be for, it would be for no session.
		{"panel for no window", panel, 0, "", [][]string{{"@", "launch", "--type=overlay", "--var=kspaces_form=1", exe, "new", "/src/gr"}}},
		{"panel refused", panel, 11, "Creating desktop panels is not supported on this platform", [][]string{panelLaunch, overlayLaunch}},
	} {
		calls := fakeKitten(t, "", c.refuse)
		err := LaunchPopup(c.p, c.from, "kspaces_form", "new", "/src/gr")
		if got := calls(); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: launched %q, want %q", c.name, got, c.want)
		}
		var refused *PanelRefused
		switch {
		case c.refuse == "" && err != nil:
			t.Errorf("%s: %v", c.name, err)
		case c.refuse != "" && (!errors.As(err, &refused) || err.Error() != "opened an overlay, as kitty could not open a panel: kitten @ launch: "+c.refuse):
			t.Errorf("%s: error %v", c.name, err)
		}
	}
}

// In a panel an action runs on the window the panel was opened from, unless it names its own.
func TestActionInPanel(t *testing.T) {
	calls := fakeKitten(t, "", "")
	if err := Action("launch", "--type=tab"); err != nil {
		t.Fatal(err)
	}
	t.Setenv(FromEnv, "11")
	if err := Action("launch", "--type=tab"); err != nil {
		t.Fatal(err)
	}
	if err := Action("--match=id:5", "launch", "--type=window"); err != nil {
		t.Fatal(err)
	}
	want := [][]string{
		{"@", "action", "launch", "--type=tab"},
		{"@", "action", "--match=id:11", "launch", "--type=tab"},
		{"@", "action", "--match=id:5", "launch", "--type=window"},
	}
	if got := calls(); !reflect.DeepEqual(got, want) {
		t.Errorf("got %q, want %q", got, want)
	}
	if !Acted() {
		t.Error("actions ran, but not Acted")
	}
}

// Leaving a panel focuses the window it was opened from and returns once kitty has focused that
// window's OS window; it gives up, saying so, when kitty keeps the focus in the panel.
func TestLeavePanel(t *testing.T) {
	calls := fakeKitten(t, `[{"is_focused": true, "tabs": [{"windows": [{"id": 11}]}]}, {"tabs": [{"windows": [{"id": 20}]}]}]`, "")
	if err := LeavePanel(); err != nil || calls() != nil {
		t.Fatalf("outside a panel: %v, %q", err, calls())
	}
	t.Setenv(FromEnv, "11")
	if err := LeavePanel(); err != nil {
		t.Fatal(err)
	}
	if got, want := calls(), [][]string{{"@", "focus-window", "--match", "id:11"}, {"@", "ls"}}; !reflect.DeepEqual(got, want) {
		t.Errorf("got %q, want %q", got, want)
	}

	defer func(d time.Duration) { leaveWait = d }(leaveWait)
	leaveWait = 0
	fakeKitten(t, `[{"tabs": [{"windows": [{"id": 11}]}]}, {"is_focused": true, "tabs": [{"windows": [{"id": 20}]}]}]`, "")
	if err := LeavePanel(); err == nil || err.Error() != "kitty did not move its focus to window 11, so this panel would have taken the session" {
		t.Errorf("kept in the panel: %v", err)
	}
}
