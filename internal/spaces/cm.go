package spaces

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"slices"
)

// A window kspaces' launcher runs in a cm session is `cm attach NAME`, so what runs in it, and where, is
// what cm knows of session NAME.

// cmSession is a session in `cm list --json`.
type cmSession struct {
	Name  string   `json:"name"`
	Names []string `json:"names"`
	// The command its shell runs, empty at the prompt.
	Command string `json:"command"`
	Cwd     string `json:"cwd"`
}

// cmAttached is the cm session a window whose program is cmdline attaches to, if it is `cm attach NAME`.
func cmAttached(cmdline []string) (string, bool) {
	if len(cmdline) < 3 || filepath.Base(cmdline[0]) != "cm" || cmdline[1] != "attach" {
		return "", false
	}
	// The name is the last word before any command after --, its flags taken as --flag=value.
	args := cmdline[2:]
	if i := slices.Index(args, "--"); i >= 0 {
		args = args[:i]
	}
	for i := len(args) - 1; i >= 0; i-- {
		if args[i] != "" && args[i][0] != '-' {
			return args[i], true
		}
	}
	return "", false
}

// cmSessions are cm's sessions, by each of their names.
func cmSessions() (map[string]cmSession, error) {
	out, err := exec.Command("cm", "list", "--json").Output()
	if err != nil {
		return nil, fmt.Errorf("cm list: %w", err)
	}
	var list []cmSession
	if err := json.Unmarshal(out, &list); err != nil {
		return nil, fmt.Errorf("parsing cm list: %w", err)
	}
	by := map[string]cmSession{}
	for _, s := range list {
		for _, n := range append([]string{s.Name}, s.Names...) {
			by[n] = s
		}
	}
	return by, nil
}
