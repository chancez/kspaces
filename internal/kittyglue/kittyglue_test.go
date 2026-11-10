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
