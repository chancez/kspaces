// Package kittyglue is what kitty needs to run kspaces: the watcher that docks a sidebar in every tab, the
// kittens that hide and focus it, the optional session saver, and a kitty.conf to include that loads them.
// The scripts are built into kspaces, so a kspaces from mise, go install, a release or a local build
// installs the files that go with it.
package kittyglue

import (
	"bytes"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/chancez/kspaces/internal/config"
)

//go:embed files/*.py
var scripts embed.FS

// ConfName is the kitty.conf to include, which loads the rest.
const ConfName = "kspaces.conf"

// sessionsScript is the session saver, installed only when the config asks for it.
const sessionsScript = "save_sessions.py"

// scriptNames are the scripts in files/, in the order they are installed.
var scriptNames = []string{"kspaces_watcher.py", "kspaces_toggle.py", "kspaces_focus.py", sessionsScript}

// owned are the names install writes, and so may remove when cfg no longer wants them.
var owned = append([]string{ConfName}, scriptNames...)

// File is a file install writes, by name in the install directory.
type File struct {
	Name string
	Data []byte
}

// DefaultDir is kspaces' directory in kitty's config directory, as kitty finds that: KITTY_CONFIG_DIRECTORY,
// else kitty under XDG_CONFIG_HOME or ~/.config.
func DefaultDir() (string, error) {
	if d := os.Getenv("KITTY_CONFIG_DIRECTORY"); d != "" {
		return filepath.Join(d, "kspaces"), nil
	}
	base := os.Getenv("XDG_CONFIG_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		base = filepath.Join(home, ".config")
	}
	return filepath.Join(base, "kitty", "kspaces"), nil
}

// Files are what install writes to dir for cfg. kspaces.conf names the scripts by absolute path, as kitty
// resolves a relative one against its own config directory, not the included file's.
func Files(cfg config.Config, dir string) ([]File, error) {
	var out []File
	for _, name := range scriptNames {
		if name == sessionsScript && !cfg.Kitty.SaveSessions {
			continue
		}
		data, err := scripts.ReadFile("files/" + name)
		if err != nil {
			return nil, err
		}
		out = append(out, File{Name: name, Data: data})
	}
	return append([]File{{Name: ConfName, Data: []byte(conf(cfg, dir))}}, out...), nil
}

// conf is kspaces.conf: the watchers, and actions to map keys to, so kitty.conf names no paths.
func conf(cfg config.Config, dir string) string {
	path := func(name string) string { return filepath.Join(dir, name) }
	var b strings.Builder
	b.WriteString(`# Written by kspaces kitty install, which rewrites it; do not edit. Include it from kitty.conf with
#
#   include kspaces/kspaces.conf
#
# before your own mappings, and map keys to the actions below, such as: map f4 kspaces_toggle

`)
	if cfg.Kitty.SaveSessions {
		fmt.Fprintf(&b, "# Save each session to its own file, at quit and as its windows change, and restore them at startup.\nwatcher %s\n", path(sessionsScript))
		fmt.Fprintf(&b, "action_alias kspaces_save_sessions kitten %s\n\n", path(sessionsScript))
	}
	fmt.Fprintf(&b, `# Dock a sidebar in every tab that belongs to a session.
watcher %s

# Hide and show every sidebar, and move the keyboard into this tab's sidebar and back.
action_alias kspaces_toggle kitten %s
action_alias kspaces_focus kitten %s

# The picker, and the next and previous session with windows open.
action_alias kspaces_pick launch --type=overlay --var=kspaces_picker=1 kspaces pick
action_alias kspaces_next launch --type=background --allow-remote-control kspaces next
action_alias kspaces_prev launch --type=background --allow-remote-control kspaces prev

# The next and previous window whose program needs you: blocked, failed or done.
action_alias kspaces_attention_next launch --type=background --allow-remote-control kspaces next --attention
action_alias kspaces_attention_prev launch --type=background --allow-remote-control kspaces prev --attention

# Send a key to this tab's sidebar without focusing it, as in: map --mode spaces j kspaces_key j
action_alias kspaces_key remote_control send-key --match "var:^kspaces$ and state:parent_focused"
`, path("kspaces_watcher.py"), path("kspaces_toggle.py"), path("kspaces_focus.py"))
	return b.String()
}

// Stale are the files in dir that are missing or differ from what install would write for cfg, and the
// owned files install would remove.
func Stale(cfg config.Config, dir string) ([]string, error) {
	files, err := Files(cfg, dir)
	if err != nil {
		return nil, err
	}
	want := map[string]bool{}
	var stale []string
	for _, f := range files {
		want[f.Name] = true
		data, err := os.ReadFile(filepath.Join(dir, f.Name))
		switch {
		case errors.Is(err, fs.ErrNotExist):
			stale = append(stale, f.Name)
		case err != nil:
			return nil, err
		case !bytes.Equal(data, f.Data):
			stale = append(stale, f.Name)
		}
	}
	for _, name := range owned {
		if _, err := os.Stat(filepath.Join(dir, name)); !want[name] && err == nil {
			stale = append(stale, name)
		}
	}
	return stale, nil
}

// Install writes the files for cfg to dir, each by renaming a temporary file over it, so kitty never reads
// one half written, and removes the owned files cfg no longer wants.
func Install(cfg config.Config, dir string) error {
	files, err := Files(cfg, dir)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	want := map[string]bool{}
	for _, f := range files {
		want[f.Name] = true
		if err := writeFile(filepath.Join(dir, f.Name), f.Data); err != nil {
			return err
		}
	}
	for _, name := range owned {
		if want[name] {
			continue
		}
		if err := os.Remove(filepath.Join(dir, name)); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
	}
	return nil
}

func writeFile(path string, data []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(0o644); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}
