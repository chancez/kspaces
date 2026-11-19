package config

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

// SaveSidebarWidth edits the file that sets the width in place, else the one with a [sidebar] table,
// else adds a [sidebar] table to config.toml, and leaves every file alone for the width it already is.
func TestSaveSidebarWidth(t *testing.T) {
	type files struct{ Config, Work string }
	for _, c := range []struct {
		name  string
		files files
		width int
		want  files
	}{{
		name:  "set in config.toml",
		files: files{"# The sidebar.\n[sidebar]\nwidth = 28 # columns\npopup = \"panel\"\n\n[kitty]\nauto_install = true\n", ""},
		width: 40,
		want:  files{"# The sidebar.\n[sidebar]\nwidth = 40 # columns\npopup = \"panel\"\n\n[kitty]\nauto_install = true\n", ""},
	}, {
		name:  "set as a dotted key",
		files: files{"spaces = []\nsidebar.width = 28\n", ""},
		width: 33,
		want:  files{"spaces = []\nsidebar.width = 33\n", ""},
	}, {
		name:  "set in work.toml",
		files: files{"spaces = [\"~/a\"]\n\n[sidebar] # mine\npopup = \"panel\"\n", "# Work.\n[sidebar]\nwidth = 30\n"},
		width: 50,
		want:  files{"spaces = [\"~/a\"]\n\n[sidebar] # mine\npopup = \"panel\"\n", "# Work.\n[sidebar]\nwidth = 50\n"},
	}, {
		name:  "a [sidebar] table in work.toml",
		files: files{"spaces = [\"~/a\"]\n", "[sidebar] # here\npopup = \"panel\"\n"},
		width: 50,
		want:  files{"spaces = [\"~/a\"]\n", "[sidebar] # here\nwidth = 50\npopup = \"panel\"\n"},
	}, {
		name:  "no [sidebar] table",
		files: files{"spaces = [\"~/a\"] # mine\n\n[kitty]\nauto_install = true\n", ""},
		width: 36,
		want:  files{"spaces = [\"~/a\"] # mine\n\n[kitty]\nauto_install = true\n\n[sidebar]\nwidth = 36\n", ""},
	}} {
		t.Run(c.name, func(t *testing.T) {
			dir := t.TempDir()
			t.Setenv("HOME", "/home/me")
			t.Setenv("XDG_CONFIG_HOME", dir)
			main := filepath.Join(dir, "kspaces", "config.toml")
			work := filepath.Join(dir, "kspaces", "work.toml")
			if err := os.MkdirAll(filepath.Dir(main), 0o755); err != nil {
				t.Fatal(err)
			}
			write := func(path, text string) {
				if text == "" {
					return
				}
				if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			write(main, c.files.Config)
			write(work, c.files.Work)
			if err := SaveSidebarWidth(c.width); err != nil {
				t.Fatal(err)
			}
			read := func(path string) string {
				data, _ := os.ReadFile(path)
				return string(data)
			}
			if got := (files{read(main), read(work)}); !reflect.DeepEqual(got, c.want) {
				t.Errorf("got\n%+v\nwant\n%+v", got, c.want)
			}
			cfg, err := Load()
			if err != nil {
				t.Fatal(err)
			}
			if cfg.Sidebar.Width != c.width {
				t.Errorf("loaded width %d, want %d", cfg.Sidebar.Width, c.width)
			}
		})
	}
}

// The width the config already has, by default or as set, writes nothing; one out of range is an error
// that leaves the file as it was.
func TestSaveSidebarWidthUnchanged(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", "/home/me")
	t.Setenv("XDG_CONFIG_HOME", dir)
	main := filepath.Join(dir, "kspaces", "config.toml")
	if err := SaveSidebarWidth(DefaultSidebarWidth); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Dir(main)); !os.IsNotExist(err) {
		t.Errorf("the default width made the config directory: %v", err)
	}

	if err := os.MkdirAll(filepath.Dir(main), 0o755); err != nil {
		t.Fatal(err)
	}
	text := "[sidebar]\nwidth = 40\n"
	if err := os.WriteFile(main, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	old := time.Unix(1_000_000, 0)
	if err := os.Chtimes(main, old, old); err != nil {
		t.Fatal(err)
	}
	if err := SaveSidebarWidth(40); err != nil {
		t.Fatal(err)
	}
	if err := SaveSidebarWidth(MaxSidebarWidth + 1); err == nil || !strings.Contains(err.Error(), "sidebar.width is 121") {
		t.Errorf("an out of range width: %v", err)
	}
	info, err := os.Stat(main)
	if err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(main)
	type file struct {
		Text    string
		ModTime time.Time
	}
	if got, want := (file{string(data), info.ModTime()}), (file{text, old}); !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v\nwant %+v", got, want)
	}
}
