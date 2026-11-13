package spaces

import (
	"cmp"
	"encoding/json"
	"path/filepath"
	"slices"
	"strings"

	"github.com/chancez/kspaces/internal/status"
)

// SortMode is how the sidebar orders the spaces, the linked worktrees within each, and the sessions in no
// space. In every mode a space's main worktree stays its first row and its missing worktrees its last.
type SortMode string

const (
	// SortManual is config.toml's order of spaces and the order linked worktrees were moved into, see
	// Order.Worktrees; the sessions in no space by name.
	SortManual SortMode = "manual"
	// SortActivity puts the session focused most recently first, a space by its most recent session.
	// Sessions without windows go last, in manual order.
	SortActivity SortMode = "activity"
	// SortStatus puts the most urgent status first, as the status list ranks them, a space by its most
	// urgent session; ties by activity.
	SortStatus SortMode = "status"
)

// SortModes are the modes in the order the sort button cycles through them.
var SortModes = []SortMode{SortManual, SortActivity, SortStatus}

// Next is the mode after s, round to the first. The zero mode is manual.
func (s SortMode) Next() SortMode {
	return SortModes[(max(slices.Index(SortModes, s), 0)+1)%len(SortModes)]
}

// The sort mode, and each space's manual order of its linked worktrees, are shared by the sidebars as
// the folded spaces are.
const (
	sortFile  = "sort"
	orderFile = "order"
)

// LoadSort reads the sort mode, manual when none is saved.
func LoadSort() SortMode {
	if s := SortMode(strings.TrimSpace(string(readState(sortFile)))); slices.Contains(SortModes, s) {
		return s
	}
	return SortManual
}

// SaveSort saves the sort mode.
func SaveSort(s SortMode) error {
	return writeState(sortFile, []byte(string(s)+"\n"))
}

// LoadOrder reads the manual order of each space's linked worktrees: by root, their directory names. A
// missing or broken file orders none.
func LoadOrder() map[string][]string {
	ans := map[string][]string{}
	if data := readState(orderFile); len(data) > 0 {
		if err := json.Unmarshal(data, &ans); err != nil {
			return map[string][]string{}
		}
	}
	return ans
}

// SaveOrder saves the manual order of each space's linked worktrees, see LoadOrder.
func SaveOrder(order map[string][]string) error {
	data, err := json.MarshalIndent(order, "", "  ")
	if err != nil {
		return err
	}
	return writeState(orderFile, append(data, '\n'))
}

// Order is how BuildRows orders its rows.
type Order struct {
	// The zero mode is manual.
	Mode SortMode
	// Worktrees is the manual order of linked worktrees, by space root: their directory names. Those it
	// does not name follow in git's order.
	Worktrees map[string][]string
	// Groups is the order of the groups, config.toml's groups: those it does not list follow, by name.
	// Spaces in no group come before them all, and each group's spaces are in the mode's order.
	Groups []string
	// Focused is when each session was last focused, see kitty.State.Focused.
	Focused map[string]float64
	// Statuses are kitty's, for SortStatus.
	Statuses []status.Status
	// Held, when set, ranks rows to keep where they are, ahead of the mode: spaces by root, worktrees and
	// sessions in no space by session. What it does not rank follows, in the mode's order. The sidebar
	// holds the order it shows while the mouse is pressed or a menu is open, so rows do not move under it.
	Held map[string]int
}

// arrange is others and spaces in o's order, as copies, so the snapshot they came from keeps its own.
func (o Order) arrange(others, spaces []Space) ([]Space, []Space) {
	others, spaces = slices.Clone(others), slices.Clone(spaces)
	sortBy(o, others, func(s Space) string { return s.Worktrees[0].Session }, spaceSessions)
	for i := range spaces {
		s := &spaces[i]
		if len(s.Worktrees) < 3 {
			continue
		}
		wts := slices.Clone(s.Worktrees)
		linked, manual := wts[1:], o.Worktrees[s.Root]
		rank := func(wt Worktree) int {
			if i := slices.Index(manual, filepath.Base(wt.Path)); i >= 0 {
				return i
			}
			return len(manual)
		}
		slices.SortStableFunc(linked, func(a, b Worktree) int { return cmp.Compare(rank(a), rank(b)) })
		sortBy(o, linked, func(wt Worktree) string { return wt.Session }, func(wt Worktree) []string { return []string{wt.Session} })
		s.Worktrees = wts
	}
	sortBy(o, spaces, func(s Space) string { return s.Root }, spaceSessions)
	return others, spaces
}

// grouped is spaces, in their order, by group, see Order.Groups: those in no group first, under the
// empty name, then each group a space is in.
func (o Order) grouped(spaces []Space) []*Group {
	by := map[string]*Group{}
	var names []string
	for i := range spaces {
		s := &spaces[i]
		g, ok := by[s.Group]
		if !ok {
			g = &Group{Name: s.Group}
			by[s.Group] = g
			names = append(names, s.Group)
		}
		g.Spaces = append(g.Spaces, s)
	}
	rank := func(name string) int {
		if name == "" {
			return -1
		}
		if i := slices.Index(o.Groups, name); i >= 0 {
			return i
		}
		return len(o.Groups)
	}
	slices.SortFunc(names, func(a, b string) int {
		return cmp.Or(cmp.Compare(rank(a), rank(b)), cmp.Compare(a, b))
	})
	out := make([]*Group, len(names))
	for i, name := range names {
		out[i] = by[name]
	}
	return out
}

// GroupNames are the groups spaces are in, in their order, see Order.Groups.
func (o Order) GroupNames(spaces []Space) []string {
	var ans []string
	for _, g := range o.grouped(spaces) {
		if g.Name != "" {
			ans = append(ans, g.Name)
		}
	}
	return ans
}

// spaceSessions are the sessions of a space's worktrees, missing ones too.
func spaceSessions(s Space) []string {
	var ans []string
	for _, wt := range slices.Concat(s.Worktrees, s.Missing) {
		ans = append(ans, wt.Session)
	}
	return ans
}

// sortBy sorts items, which are in manual order, stably by o: held first, then by the mode. key names an
// item for Held, sessions are the sessions it stands for.
func sortBy[T any](o Order, items []T, key func(T) string, sessions func(T) []string) {
	activity := func(t T) float64 {
		at := 0.0
		for _, s := range sessions(t) {
			at = max(at, o.Focused[s])
		}
		return at
	}
	urgency := func(t T) int {
		u := -1
		for _, s := range sessions(t) {
			u = max(u, sessionUrgency(o.Statuses, s))
		}
		return u
	}
	slices.SortStableFunc(items, func(a, b T) int {
		if o.Held != nil {
			ra, oka := o.Held[key(a)]
			rb, okb := o.Held[key(b)]
			switch {
			case oka && okb:
				return cmp.Compare(ra, rb)
			case oka != okb:
				if oka {
					return -1
				}
				return 1
			}
		}
		switch o.Mode {
		case SortStatus:
			if c := cmp.Compare(urgency(b), urgency(a)); c != 0 {
				return c
			}
			return cmp.Compare(activity(b), activity(a))
		case SortActivity:
			return cmp.Compare(activity(b), activity(a))
		}
		return 0
	})
}

// sessionUrgency ranks the most urgent status of a session's windows as the status list does, from
// blocked down to idle, and below idle a session with none.
func sessionUrgency(statuses []status.Status, session string) int {
	u := -1
	for _, s := range statuses {
		if s.Session == session {
			u = max(u, status.Urgency(s.State))
		}
	}
	return u
}
