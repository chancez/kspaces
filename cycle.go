package main

import "fmt"

// cycle switches to the session delta places from the focused window's session, in the sidebar's order,
// wrapping around. Only sessions with windows open count, as with kitty's next_tab, so stepping through
// never creates a session.
func cycle(delta int) error {
	snap := load(focusedWindow)
	if snap.cfg == nil || snap.err != nil {
		return snap.err
	}
	var open []Worktree
	current := -1
	for _, r := range buildRows(snap.others, snap.spaces) {
		if r.worktree == nil {
			continue
		}
		if _, ok := snap.kitty.Sessions[r.worktree.Session]; !ok {
			continue
		}
		if r.worktree.Session == snap.kitty.Current {
			current = len(open)
		}
		open = append(open, *r.worktree)
	}
	if len(open) == 0 {
		return fmt.Errorf("no session has windows open")
	}
	return openWorktree(*snap.cfg, snap.kitty, open[cycleIndex(len(open), current, delta)])
}

// cycleIndex steps delta places from current among n, wrapping around. From outside the list, current -1,
// a step forward lands on the first and a step back on the last.
func cycleIndex(n, current, delta int) int {
	if current < 0 && delta < 0 {
		current = 0
	}
	return ((current+delta)%n + n) % n
}
