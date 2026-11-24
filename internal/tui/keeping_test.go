package tui

import (
	"reflect"
	"testing"
	"time"

	"github.com/chancez/kspaces/internal/spaces"
	"github.com/chancez/kspaces/internal/tui/cells"
	"github.com/chancez/kspaces/internal/tui/look"
)

// A session the provider is keeping says it is being suspended or closed, rather than its windows open or
// closed, until the list says it is kept: suspended over the suspended marker, which the session listed as
// suspended before keeps until then.
func TestKeepingMarker(t *testing.T) {
	e := newSidebarEnv(t)
	snap := fixture(time.Now())
	snap.Keeping = map[string]spaces.KeepMode{"gr@fix": spaces.KeepSuspending, "kitty": spaces.KeepClosing}
	snap.Suspended = map[string]bool{"gr@fix": true}
	e.m.Update(snap)
	e.m.render()
	head := func(spans ...look.Span) []cells.Cell {
		return cells.Parse(e.m.th.Draw(look.Plain, 5, append([]look.Span{look.NoCaret()}, spans...)...))[0]
	}
	var got [][]cells.Cell
	for _, s := range []string{"gr@fix", "kitty"} {
		got = append(got, cells.Parse(scanned(e.m.renderRow(e.row(s))))[0][:5])
	}
	if want := [][]cells.Cell{head(look.KeepingMarker(true)...), head(look.KeepingMarker(false)...)}; !reflect.DeepEqual(got, want) {
		t.Errorf("got\n%v\nwant\n%v", got, want)
	}
}

// scanned is s without its zones' marks.
func scanned(s string) string {
	frame, _ := look.Scan(s)
	return frame
}
