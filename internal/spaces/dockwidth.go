package spaces

// A sidebar's dock can be dragged to another width in kitty 0.51. The leader saves the width it is
// dragged to as the config's, so the next sidebars open at it, telling a drag from kitty.conf's watcher
// resizing the docks to a width just set in the config by which of the two changed first.

// dockDrag follows the sidebar docks' width and the config's from one poll to the next, see observe.
type dockDrag struct {
	have bool
	// The dock and config widths at the last poll.
	dock, cfg int
	// The width a drag under way has reached, zero for none.
	pending int
}

// observe takes a poll's dock width and config width, and says the width to save, zero for none. A
// config that changed is the watcher's to apply to the docks, so the docks moving next is no drag. A dock
// width that changed with the config as it was is a drag, saved once it holds for a poll, so a long drag
// saves its final width alone. A dock width of zero, no dock seen, starts over.
func (d *dockDrag) observe(dock, cfg int) int {
	if dock == 0 {
		*d = dockDrag{}
		return 0
	}
	prev := *d
	d.have, d.dock, d.cfg = true, dock, cfg
	switch {
	case !prev.have:
	case cfg != prev.cfg:
		d.pending = 0
	case dock != prev.dock:
		d.pending = dock
	case d.pending != 0:
		save := d.pending
		d.pending = 0
		if save != cfg {
			return save
		}
	}
	return 0
}

// dockWidth is the width of the sidebar docks, widths, for the config's width cfg: the first that is not
// cfg, as a drag moves one OS window's dock alone, else cfg, and zero without docks.
func dockWidth(widths []int, cfg int) int {
	for _, w := range widths {
		if w != cfg {
			return w
		}
	}
	if len(widths) == 0 {
		return 0
	}
	return cfg
}
