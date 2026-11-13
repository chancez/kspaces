package tui

import (
	"strings"

	"github.com/chancez/kspaces/internal/tui/look"
)

// The sidebar's last line, when it shows an error.

// errorZone is the last line while it shows an error.
const errorZone = "error"

// lastError is the error the last line shows, nil for none: an action's over a load's, see failed.
func (m *sidebar) lastError() error {
	if m.failed != nil {
		return m.failed
	}
	return m.err
}

// oneLine is text with its runs of spaces, tabs and line breaks each made one space, for a line of its
// own: an error of several lines, as gh's often are, would push the frame past the sidebar's height.
func oneLine(text string) string {
	return strings.Join(strings.Fields(text), " ")
}

// errorLine is the last line showing err, cut to the sidebar's width, marked for the mouse.
func (m *sidebar) errorLine(err error) string {
	th, width := m.th, m.contentWidth()
	return th.DrawZone(errorZone, look.Plain, width, th.Cut(width, look.Error(oneLine(err.Error())))...)
}

// errorCut is whether the last line cuts the error it shows, so there is more of it to see.
func (m *sidebar) errorCut() bool {
	err := m.lastError()
	return err != nil && m.th.Width(look.Error(oneLine(err.Error()))) > m.contentWidth()
}

// errorPopup is the last line's error in full, wrapped, for the mouse resting on the line while it cuts
// the error, and the screen row it starts at: right above the line, over what is there, under a rule
// closing it off from what it covers. It keeps to the rows below the title, its first rows when it is
// longer. All of it is marked, so the mouse on it is on the popup rather than on what it covers.
func (m *sidebar) errorPopup() ([]string, int) {
	if !m.hoverError || !m.errorCut() {
		return nil, 0
	}
	th, width := m.th, m.contentWidth()
	popup := []string{th.DrawZone(popupZone, look.Plain, width, look.Rule(width, true))}
	for _, row := range look.Error(m.lastError().Error()).Wrapped(width) {
		popup = append(popup, th.DrawZone(popupZone, look.Plain, width, row))
	}
	bottom := m.height - 1
	top := max(bottom-len(popup), headerRows)
	return popup[:max(bottom-top, 0)], top
}
