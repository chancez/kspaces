package spaces

import (
	"fmt"

	"github.com/chancez/kspaces/internal/kitty"
)

// Cycle switches to the session delta places from the focused window's session, in the sidebar's order,
// wrapping around. Only sessions with windows open count, as with kitty's next_tab, so stepping through
// never creates a session.
func Cycle(delta int) error {
	snap := Load(kitty.FocusedWindow)
	if snap.Config == nil || snap.Err != nil {
		return snap.Err
	}
	var open []Worktree
	current := -1
	for _, r := range BuildRows(snap.Others, snap.Spaces, nil, snap.Order) {
		if r.Worktree == nil {
			continue
		}
		if _, ok := snap.Kitty.Sessions[r.Worktree.Session]; !ok {
			continue
		}
		if r.Worktree.Session == snap.Kitty.Current {
			current = len(open)
		}
		open = append(open, *r.Worktree)
	}
	if len(open) == 0 {
		return fmt.Errorf("no session has windows open")
	}
	return OpenWorktree(*snap.Config, snap.Kitty, open[cycleIndex(len(open), current, delta)])
}

// cycleIndex steps delta places from current among n, wrapping around. From outside the list, current -1,
// a step forward lands on the first and a step back on the last.
func cycleIndex(n, current, delta int) int {
	if current < 0 && delta < 0 {
		current = 0
	}
	return ((current+delta)%n + n) % n
}
