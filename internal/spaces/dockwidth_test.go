package spaces

import (
	"errors"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/chancez/kspaces/internal/config"
	"github.com/chancez/kspaces/internal/kitty"
)

// observe saves a drag's final width once it holds for a poll, and never a width the docks reach by
// following the config.
func TestDockDrag(t *testing.T) {
	type poll struct{ dock, cfg int }
	for _, c := range []struct {
		name  string
		polls []poll
		// What each poll says to save.
		want []int
	}{{
		name:  "drag",
		polls: []poll{{28, 28}, {40, 28}, {40, 28}, {40, 28}},
		want:  []int{0, 0, 40, 0},
	}, {
		name:  "long drag",
		polls: []poll{{28, 28}, {30, 28}, {33, 28}, {37, 28}, {41, 28}, {41, 28}, {41, 28}},
		want:  []int{0, 0, 0, 0, 0, 41, 0},
	}, {
		// The config is saved as dragged, which the next poll reads, the docks already at it.
		name:  "drag then its save",
		polls: []poll{{28, 28}, {40, 28}, {40, 28}, {40, 40}, {40, 40}},
		want:  []int{0, 0, 40, 0, 0},
	}, {
		// The config is edited to 40, and the watcher resizes the docks to it a poll or two later.
		name:  "config edit then watcher resize",
		polls: []poll{{28, 28}, {28, 40}, {28, 40}, {40, 40}, {40, 40}, {40, 40}},
		want:  []int{0, 0, 0, 0, 0, 0},
	}, {
		name:  "config edit with the docks resized by the next poll",
		polls: []poll{{28, 28}, {40, 40}, {40, 40}},
		want:  []int{0, 0, 0},
	}, {
		name:  "drag back to the config width",
		polls: []poll{{28, 28}, {35, 28}, {28, 28}, {28, 28}},
		want:  []int{0, 0, 0, 0},
	}, {
		name:  "first poll mismatch",
		polls: []poll{{40, 28}, {40, 28}, {40, 28}},
		want:  []int{0, 0, 0},
	}, {
		// The docks gone for a poll, as between kitty's OS windows closing and opening, start over.
		name:  "docks gone",
		polls: []poll{{28, 28}, {0, 28}, {40, 28}, {40, 28}},
		want:  []int{0, 0, 0, 0},
	}} {
		t.Run(c.name, func(t *testing.T) {
			var d dockDrag
			var got []int
			for _, p := range c.polls {
				got = append(got, d.observe(p.dock, p.cfg))
			}
			if !reflect.DeepEqual(got, c.want) {
				t.Errorf("got %v, want %v", got, c.want)
			}
		})
	}
}

func TestDockWidth(t *testing.T) {
	got := []int{dockWidth(nil, 28), dockWidth([]int{28, 28}, 28), dockWidth([]int{28, 40}, 28), dockWidth([]int{40, 28}, 28)}
	if want := []int{0, 28, 40, 40}; !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

// The poller saves a dragged dock's width from what it loads, and a failed save is no failed poll.
func TestPollerSavesDraggedWidth(t *testing.T) {
	dock := func(size string) []kitty.OSWindow {
		return []kitty.OSWindow{{Tabs: []kitty.Tab{{
			ID: 1, IsActive: true,
			Windows: []kitty.Window{{ID: 2}, {ID: 3, UserVars: map[string]string{"kspaces": "1"}}},
			Groups:  []kitty.Group{{ID: 2, Windows: []int{2}}, {ID: 3, Windows: []int{3}, Dock: &kitty.Dock{Edge: "left", Size: size}}},
		}}}}
	}
	sizes := []string{"28", "40", "40", "40"}
	n := 0
	load := func() Shared {
		cfg := config.Config{Sidebar: config.Sidebar{Width: 28}}
		sh := Shared{Config: &cfg, Windows: dock(sizes[n])}
		n++
		return sh
	}
	p := newPoller(filepath.Join(t.TempDir(), "kitty.json"), load)
	var saved []int
	p.saveWidth = func(w int) error {
		saved = append(saved, w)
		return errors.New("read-only")
	}
	var polled []int
	for range sizes {
		polled = append(polled, kitty.SidebarWidths(p.Poll().Windows)...)
	}
	if want := []int{40}; !reflect.DeepEqual(saved, want) {
		t.Errorf("saved %v, want %v", saved, want)
	}
	if want := []int{28, 40, 40, 40}; !reflect.DeepEqual(polled, want) {
		t.Errorf("polled %v, want %v", polled, want)
	}
}
