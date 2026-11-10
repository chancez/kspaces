// Command kspaces shows repositories ("spaces") and their git worktrees in a kitty sidebar, and switches
// kitty to a worktree's session, creating the session the first time.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const usage = `usage: kspaces [command]

Commands:
  sidebar      Show the spaces sidebar (the default)
  pick         Pick a space, worktree or status entry to switch to, meant for a kitty overlay
  list         Print every space, its worktrees and their session names
  open PATH    Switch to the session of the worktree at PATH, creating it if needed
  next, prev   Switch to the next or previous session with windows open, in sidebar order
  new [SPACE]  Create a worktree and switch to it, in the space given by root or name, or the current one
`

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "kspaces:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	cmd := "sidebar"
	if len(args) > 0 {
		cmd, args = args[0], args[1:]
	}
	if cmd == "help" || cmd == "-h" || cmd == "--help" {
		fmt.Print(usage)
		return nil
	}
	// The sidebar and the picker load the config themselves, so an error shows in their UI.
	switch cmd {
	case "sidebar":
		return runSidebar()
	case "pick":
		return runPicker()
	case "next":
		return cycle(1)
	case "prev":
		return cycle(-1)
	case "new":
		if len(args) > 1 {
			return fmt.Errorf("new takes at most one space\n\n%s", usage)
		}
		return runNewWorktree(strings.Join(args, ""))
	}
	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	switch cmd {
	case "list":
		return list(cfg)
	case "open":
		if len(args) != 1 {
			return fmt.Errorf("open takes one path\n\n%s", usage)
		}
		return open(cfg, args[0])
	default:
		return fmt.Errorf("unknown command %q\n\n%s", cmd, usage)
	}
}

func list(cfg Config) error {
	for _, root := range cfg.Spaces {
		s := loadSpace(root)
		if s.Err != nil {
			fmt.Printf("%s\t%s\terror: %v\n", s.Name, s.Root, s.Err)
			continue
		}
		for _, wt := range s.Worktrees {
			fmt.Printf("%s\t%s\t%s\n", wt.Session, wt.Path, wt.Branch)
		}
	}
	return nil
}

func open(cfg Config, path string) error {
	// git reports worktree paths with symlinks resolved, for example /private/tmp for /tmp on macOS.
	path, err := filepath.EvalSymlinks(path)
	if err != nil {
		return err
	}
	if path, err = filepath.Abs(path); err != nil {
		return err
	}
	for _, root := range cfg.Spaces {
		for _, wt := range loadSpace(root).Worktrees {
			if wt.Path != path {
				continue
			}
			st, err := loadKittyState(selfWindowID())
			if err != nil {
				return err
			}
			return openWorktree(cfg, st, wt)
		}
	}
	return fmt.Errorf("%s is not a worktree of any configured space", path)
}
