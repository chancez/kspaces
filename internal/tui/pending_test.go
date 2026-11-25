package tui

import (
	"reflect"
	"testing"
	"time"

	"github.com/chancez/kspaces/internal/spaces"
	"github.com/chancez/kspaces/internal/tui/cells"
	"github.com/chancez/kspaces/internal/tui/look"
)

// A worktree being created or removed has the pending marker, over the status of its windows, and no
// action but opening, which says why it does not.
func TestPendingMarker(t *testing.T) {
	e := newSidebarEnv(t)
	snap := fixture(time.Now())
	snap.Pending = map[string]spaces.PendingOp{"/src/gr/.worktrees/fix": spaces.PendingCreate, "/src/gr/.worktrees/dev": spaces.PendingRemove}
	e.m.Update(snap)
	e.m.render()
	// The icon's cell, after the caret's or the current session's edge.
	head := func(spans ...look.Span) []cells.Cell {
		return cells.Parse(e.m.th.Draw(look.Plain, 5, append([]look.Span{look.NoCaret()}, spans...)...))[0][2:3]
	}
	var got [][]cells.Cell
	for _, s := range []string{"gr@fix", "gr@dev"} {
		got = append(got, cells.Parse(scanned(e.m.renderRow(e.row(s))))[0][2:3])
	}
	if want := [][]cells.Cell{head(look.PendingMarker(false)...), head(look.PendingMarker(true)...)}; !reflect.DeepEqual(got, want) {
		t.Errorf("got\n%v\nwant\n%v", got, want)
	}
	snap.Pending = map[string]spaces.PendingOp{"/src/gr/.worktrees/fix": spaces.PendingCreate}
	e.m.Update(snap)
	applies := map[string]bool{}
	for _, s := range []string{"gr@fix", "gr@dev"} {
		applies[s] = actRemoveWorktree.Applies(e.m, entrySubject{row: e.m.rows[e.row(s)]})
	}
	if want := map[string]bool{"gr@fix": false, "gr@dev": true}; !reflect.DeepEqual(applies, want) {
		t.Errorf("remove applies %v, want %v", applies, want)
	}
}
