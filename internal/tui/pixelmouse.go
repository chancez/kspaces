package tui

import (
	"os"

	tea "charm.land/bubbletea/v2"
	"golang.org/x/sys/unix"
)

// kitty tells a program the mouse left its window only in SGR pixel mouse mode, DEC 1016, where every mouse
// event says where it is in pixels. The sidebar turns that on, so the hover popup can close when the mouse
// leaves for another window, and turns positions back into cells before the sidebar sees them.

// setPixelMouse turns on SGR pixel mouse mode. Resetting bubbletea's SGR mode on exit turns it off too: kitty
// keeps one mouse protocol, and resetting any of them resets it.
const setPixelMouse = "\x1b[?1016h"

// mouseLeftMsg is the mouse leaving the sidebar's window.
type mouseLeftMsg struct{}

// pixelMouse is the sidebar's pixel mouse mode: whether it is on, and the size of a cell to turn pixels into
// cells with.
type pixelMouse struct {
	on    bool
	cellW float64
	cellH float64
}

// resize takes the size of a cell from the terminal, which kitty gives in pixels along with the rows and
// columns. Without it pixels cannot be turned into cells, so the mode stays off.
func (p *pixelMouse) resize() {
	ws, err := unix.IoctlGetWinsize(int(os.Stdout.Fd()), unix.TIOCGWINSZ)
	if err != nil || ws.Col == 0 || ws.Row == 0 || ws.Xpixel == 0 || ws.Ypixel == 0 {
		p.cellW, p.cellH = 0, 0
		return
	}
	p.cellW, p.cellH = float64(ws.Xpixel)/float64(ws.Col), float64(ws.Ypixel)/float64(ws.Row)
}

// start turns the mode on, once: on the first mouse event, which comes only after bubbletea has turned its own
// mouse mode on, as turning that on afterwards would turn this off.
func (p *pixelMouse) start() tea.Cmd {
	if p.on || p.cellW == 0 {
		return nil
	}
	p.on = true
	return tea.Raw(setPixelMouse)
}

// filter turns the pixels of mouse events into cells while the mode is on, and the event kitty sends when the
// mouse leaves into a mouseLeftMsg. That event is motion with bit 8 set, which bubbletea drops, leaving motion
// with the left button down: what a drag would be, which the sidebar has no use for.
func (p *pixelMouse) filter(_ tea.Model, msg tea.Msg) tea.Msg {
	if !p.on {
		return msg
	}
	cell := func(m tea.Mouse) tea.Mouse {
		m.X, m.Y = int(float64(m.X)/p.cellW), int(float64(m.Y)/p.cellH)
		return m
	}
	switch msg := msg.(type) {
	case tea.MouseMotionMsg:
		if msg.Button == tea.MouseLeft {
			return mouseLeftMsg{}
		}
		return tea.MouseMotionMsg(cell(tea.Mouse(msg)))
	case tea.MouseClickMsg:
		return tea.MouseClickMsg(cell(tea.Mouse(msg)))
	case tea.MouseReleaseMsg:
		return tea.MouseReleaseMsg(cell(tea.Mouse(msg)))
	case tea.MouseWheelMsg:
		return tea.MouseWheelMsg(cell(tea.Mouse(msg)))
	}
	return msg
}
