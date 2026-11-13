package tui

import "strings"

// The sidebar's last line, when it shows an error.

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
