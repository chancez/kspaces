package tui

// paint is a press on a list of boxes, the sidebar's in the select mode or a form's: the row pressed on is
// ticked or unticked, and every row the mouse then passes, those it skips between two reports too, gets
// the same, what the first one changed to. A row with nothing to tick, or one that cannot be ticked, is
// passed over as it is.
type paint struct {
	on bool
	// last is the row painted last.
	last int
}

// startPaint ticks or unticks row i with toggle, which says what the row is now and whether it has a box
// that can be ticked, and starts painting from it. Nil when it cannot be.
func startPaint(i int, toggle func(i int) (on, ok bool)) *paint {
	on, ok := toggle(i)
	if !ok {
		return nil
	}
	return &paint{on: on, last: i}
}

// to paints the rows from the one after the last painted up to row i with set, which leaves alone a row
// that cannot be ticked.
func (p *paint) to(i int, set func(i int, on bool)) {
	for i != p.last {
		if i > p.last {
			p.last++
		} else {
			p.last--
		}
		set(p.last, p.on)
	}
}
