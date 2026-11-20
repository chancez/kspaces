package tui

import (
	"github.com/chancez/kspaces/internal/spaces"
)

// Dragging the rule above the STATUS title moves it, giving the rows it passes to the other list. Letting
// go keeps the STATUS list's height, in every sidebar and across restarts, see spaces.SaveSplit.

// splitZone marks the rule between the two lists.
const splitZone = "split"

// startSplit holds the rule from a left press on it until the release.
func (m *sidebar) startSplit() {
	m.unhover()
	m.splitting = true
}

// splitTo moves the rule to screen row y, as far as each list keeps a row.
func (m *sidebar) splitTo(y int) {
	avail := max(m.height-headerRows-1, 1)
	m.statusRows = clampStatus(avail-(y-headerRows)-2, avail)
	m.scrollToCursor()
}

// endSplit lets go of the rule, keeping where it is.
func (m *sidebar) endSplit() {
	m.splitting = false
	if err := spaces.SaveSplit(m.statusRows); err != nil {
		m.failed = err
	}
}

// clampStatus is n rows for the STATUS list, of avail below the SPACES title, leaving the spaces list a
// row and the rule and the STATUS title theirs.
func clampStatus(n, avail int) int {
	return max(min(n, avail-3), 1)
}
