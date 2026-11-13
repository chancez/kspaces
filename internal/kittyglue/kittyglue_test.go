package kittyglue

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/chancez/kspaces/internal/config"
)

func names(files []File) []string {
	var out []string
	for _, f := range files {
		out = append(out, f.Name)
	}
	return out
}

// The session saver is installed only when the config asks, and kspaces.conf loads what is installed, by
// absolute path.
func TestFiles(t *testing.T) {
	dir := "/cfg/kitty/kspaces"
	var cfg config.Config
	files, err := Files(cfg, dir)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := names(files), []string{"kspaces.conf", "kspaces_watcher.py", "kspaces_toggle.py", "kspaces_focus.py"}; !reflect.DeepEqual(got, want) {
		t.Errorf("files = %q, want %q", got, want)
	}
	watchers := func(files []File) []string {
		var out []string
		for _, line := range strings.Split(string(files[0].Data), "\n") {
			if strings.HasPrefix(line, "watcher ") {
				out = append(out, line)
			}
		}
		return out
	}
	if got, want := watchers(files), []string{"watcher /cfg/kitty/kspaces/kspaces_watcher.py"}; !reflect.DeepEqual(got, want) {
		t.Errorf("watchers = %q, want %q", got, want)
	}

	cfg.Kitty.SaveSessions = true
	files, err = Files(cfg, dir)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := names(files), []string{"kspaces.conf", "kspaces_watcher.py", "kspaces_toggle.py", "kspaces_focus.py", "save_sessions.py"}; !reflect.DeepEqual(got, want) {
		t.Errorf("files with save_sessions = %q, want %q", got, want)
	}
	if got, want := watchers(files), []string{"watcher /cfg/kitty/kspaces/save_sessions.py", "watcher /cfg/kitty/kspaces/kspaces_watcher.py"}; !reflect.DeepEqual(got, want) {
		t.Errorf("watchers with save_sessions = %q, want %q", got, want)
	}
}

// kspaces.conf as a whole, with the picker in an overlay, the default, and with the panel kspaces opens.
func TestConf(t *testing.T) {
	const head = `# Written by kspaces kitty install, which rewrites it; do not edit. Include it from kitty.conf with
#
#   include kspaces/kspaces.conf
#
# before your own mappings, and map keys to the actions below, such as: map f4 kspaces_toggle

# Dock a sidebar in every tab that belongs to a session.
watcher /cfg/kitty/kspaces/kspaces_watcher.py

# Hide and show every sidebar, and move the keyboard into this tab's sidebar and back.
action_alias kspaces_toggle kitten /cfg/kitty/kspaces/kspaces_toggle.py
action_alias kspaces_focus kitten /cfg/kitty/kspaces/kspaces_focus.py

# The picker; the next and previous session with windows open, and the one used before this one.
`
	const tail = `action_alias kspaces_next launch --type=background --allow-remote-control kspaces next
action_alias kspaces_prev launch --type=background --allow-remote-control kspaces prev
action_alias kspaces_recent launch --type=background --allow-remote-control kspaces recent

# The next and previous window whose program needs you: blocked, failed or done.
action_alias kspaces_attention_next launch --type=background --allow-remote-control kspaces next --attention
action_alias kspaces_attention_prev launch --type=background --allow-remote-control kspaces prev --attention

# Send a key to this tab's sidebar without focusing it, as in: map --mode spaces j kspaces_key j
action_alias kspaces_key remote_control send-key --match "var:^kspaces$ and state:parent_focused"
`
	for _, c := range []struct {
		popup, pick string
	}{
		{config.PopupOverlay, "action_alias kspaces_pick launch --type=overlay --var=kspaces_picker=1 kspaces pick\n"},
		{config.PopupPanel, "action_alias kspaces_pick launch --type=background --allow-remote-control kspaces pick --from @active-kitty-window-id\n"},
	} {
		got := conf(config.Config{Sidebar: config.Sidebar{Popup: c.popup}}, "/cfg/kitty/kspaces")
		if want := head + c.pick + tail; got != want {
			t.Errorf("popup %s:\n%s\nwant\n%s", c.popup, got, want)
		}
	}
}

// Stale names what install would change: missing, edited, and owned files the config no longer wants,
// which install removes. Files it does not own are left alone.
func TestInstallAndStale(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "kspaces")
	cfg := config.Config{Kitty: config.Kitty{SaveSessions: true}}
	stale := func(cfg config.Config) []string {
		t.Helper()
		got, err := Stale(cfg, dir)
		if err != nil {
			t.Fatal(err)
		}
		return got
	}
	if got, want := stale(cfg), []string{"kspaces.conf", "kspaces_watcher.py", "kspaces_toggle.py", "kspaces_focus.py", "save_sessions.py"}; !reflect.DeepEqual(got, want) {
		t.Errorf("before install: %q, want %q", got, want)
	}
	if err := Install(cfg, dir); err != nil {
		t.Fatal(err)
	}
	if got := stale(cfg); got != nil {
		t.Errorf("after install: %q", got)
	}
	if err := os.WriteFile(filepath.Join(dir, "kspaces_focus.py"), []byte("edited"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "mine.conf"), []byte("mine"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg.Kitty.SaveSessions = false
	if got, want := stale(cfg), []string{"kspaces.conf", "kspaces_focus.py", "save_sessions.py"}; !reflect.DeepEqual(got, want) {
		t.Errorf("edited, and the saver turned off: %q, want %q", got, want)
	}
	if err := Install(cfg, dir); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var left []string
	for _, e := range entries {
		left = append(left, e.Name())
	}
	if want := []string{"kspaces.conf", "kspaces_focus.py", "kspaces_toggle.py", "kspaces_watcher.py", "mine.conf"}; !reflect.DeepEqual(left, want) {
		t.Errorf("after the second install: %q, want %q", left, want)
	}
}
