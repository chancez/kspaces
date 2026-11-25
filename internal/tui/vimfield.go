package tui

import (
	"charm.land/bubbles/v2/textarea"
	tea "charm.land/bubbletea/v2"
	"github.com/guygrigsby/vimbubble/v2"

	"github.com/chancez/kspaces/internal/tui/look"
)

// vimKeys are vim's modes on a field's text area, while its theme has them on, see look.Theme.SetVim. A
// field starts in insert mode. Esc goes to normal mode, where the form has esc again, and enter and tab,
// so the form is left, submitted or moved through as without vim.
type vimKeys struct {
	th *look.Theme
	// vim edits area, made for it once the field is in place, as vim keeps the area's address.
	vim  *vimbubble.Modal
	area *textarea.Model
	// owed is set in normal mode while vim waits for more of a command: r for the character it puts, g, d
	// or c for a motion, or o for the object after ci or di, see formKey.
	owed rune
}

// modeRoom is how many cells a field gives up for its mode, drawn after it, see vimKeys.mode.
const modeRoom = len(" NORMAL")

// on is vim on area, nil while the theme has it off. A field copied to another place is a new area, which
// vim is made again for, in the mode it was in.
func (k *vimKeys) on(area *textarea.Model) *vimbubble.Modal {
	if k.th == nil || !k.th.Vim() {
		return nil
	}
	if k.area != area {
		mode := vimbubble.Insert
		if k.vim != nil {
			mode = k.vim.Mode()
		}
		k.vim, k.area = vimbubble.New(area), area
		k.vim.SetMode(mode)
		// The terminal's cursor, which takes the mode's shape, see cursor; the area's own is always a block.
		area.SetVirtualCursor(false)
	}
	return k.vim
}

// keeps says whether vim takes msg before the form does: esc out of insert or visual mode, and every key
// that types a character in normal or visual mode, which are vim's commands there.
func (k *vimKeys) keeps(area *textarea.Model, msg tea.KeyPressMsg) bool {
	v := k.on(area)
	if v == nil {
		return false
	}
	esc := msg.Code == tea.KeyEscape && msg.Mod == 0
	switch v.Mode() {
	case vimbubble.Insert:
		return esc
	case vimbubble.Visual:
		return esc || typesChar(msg)
	case vimbubble.Normal:
		return typesChar(msg)
	}
	return false
}

// update hands msg to vim, saying whether it took it; what it does not goes on to the area. In normal mode
// the arrows, home and end move the cursor, as they do while typing, rather than vim dropping them. Leaving
// insert mode steps the cursor back onto the character it was after, as vim's does.
func (k *vimKeys) update(area *textarea.Model, msg tea.Msg) (tea.Cmd, bool) {
	press, ok := msg.(tea.KeyPressMsg)
	v := k.on(area)
	if !ok || v == nil {
		return nil, false
	}
	if v.Mode() == vimbubble.Normal && movesCursor(press) {
		return nil, false
	}
	was := v.Mode()
	consumed, cmd := v.Update(press)
	if was == vimbubble.Insert && v.Mode() == vimbubble.Normal {
		area.SetCursorColumn(max(area.Column()-1, 0))
	}
	owed := k.owed
	k.owed = 0
	if was == vimbubble.Normal && v.Mode() == vimbubble.Normal {
		k.owed = owes(owed, press)
	}
	k.th.SetVimNormal(v.Mode() != vimbubble.Insert)
	return cmd, consumed
}

// focus puts vim in the mode the field that had focus before left, see look.Theme.SetVimNormal: insert, or
// normal for normal and visual, as a selection is the field's.
func (k *vimKeys) focus(area *textarea.Model) {
	v := k.on(area)
	if v == nil {
		return
	}
	mode := vimbubble.Insert
	if k.th.VimNormal() {
		mode = vimbubble.Normal
	}
	v.SetMode(mode)
	k.owed = 0
}

// owes is what vim waits for after press in normal mode, owed what it waited for before: see vimKeys.owed.
func owes(owed rune, press tea.KeyPressMsg) rune {
	if !typesChar(press) {
		return 0
	}
	r := []rune(press.Text)[0]
	switch {
	case owed == 0 && (r == 'r' || r == 'g' || r == 'd' || r == 'c'):
		return r
	case (owed == 'c' || owed == 'd') && (r == 'i' || r == 'a'):
		return 'o'
	}
	return 0
}

// formKey is msg as the form takes it: in normal mode, j and k are down and up, which the field keeps to
// move between its lines and the form takes on its first or last to move to the field above or below. Vim
// has them while it waits for more of a command, as rj, and in visual mode, where they select.
func (k *vimKeys) formKey(area *textarea.Model, msg tea.KeyPressMsg) tea.KeyPressMsg {
	if v := k.on(area); v == nil || v.Mode() != vimbubble.Normal || k.owed != 0 {
		return msg
	}
	return vimArrow(msg)
}

// formKey is msg as a form takes it with fl, the field with focus, nil for a choice: see vimKeys.formKey.
// On a choice, while th has vim on, j and k are down and up too, so they go on through the form.
func formKey(th *look.Theme, fl field, msg tea.KeyPressMsg) tea.KeyPressMsg {
	if fl != nil {
		return fl.formKey(msg)
	}
	if th.Vim() {
		return vimArrow(msg)
	}
	return msg
}

// vimArrow is j as down and k as up, as vim moves, and any other key as it is.
func vimArrow(msg tea.KeyPressMsg) tea.KeyPressMsg {
	if msg.Mod != 0 {
		return msg
	}
	switch msg.Text {
	case "j":
		return tea.KeyPressMsg{Code: tea.KeyDown}
	case "k":
		return tea.KeyPressMsg{Code: tea.KeyUp}
	}
	return msg
}

// leaveVisual goes back to normal mode from visual, as a click places the cursor rather than extending
// the selection.
func (k *vimKeys) leaveVisual(area *textarea.Model) {
	if v := k.on(area); v != nil && v.Mode() == vimbubble.Visual {
		v.SetMode(vimbubble.Normal)
	}
}

// mode is the mode's name, as NORMAL, or "" without vim.
func (k *vimKeys) mode(area *textarea.Model) string {
	v := k.on(area)
	if v == nil {
		return ""
	}
	switch v.Mode() {
	case vimbubble.Normal:
		return "NORMAL"
	case vimbubble.Insert:
		return "INSERT"
	case vimbubble.Visual:
		return "VISUAL"
	}
	return ""
}

// cursor is the terminal's cursor in area, from the area's top left, nil without vim, as the area draws its
// own then: a bar in insert mode and a block otherwise, steady, as vim's.
func (k *vimKeys) cursor(area *textarea.Model) *tea.Cursor {
	v := k.on(area)
	if v == nil {
		return nil
	}
	c := area.Cursor()
	if c == nil {
		return nil
	}
	// No color keeps the terminal's own, as its theme has it: the area's NoColor would be set as black.
	c.Color = nil
	c.Blink = false
	c.Shape = tea.CursorBlock
	if v.Mode() == vimbubble.Insert {
		c.Shape = tea.CursorBar
	}
	return c
}

// fieldCursor is the terminal's cursor for fl, the field with focus, nil for a choice, where fieldLines drew
// it in the frame zones were scanned from. It is nil without vim, and while fl is not drawn.
func fieldCursor(fl field, zones map[string][]look.Zone) *tea.Cursor {
	if fl == nil {
		return nil
	}
	c := fl.cursor()
	rows := zones[fieldZone(fl)]
	if c == nil || c.Y < 0 || c.Y >= len(rows) {
		return nil
	}
	c.X += rows[c.Y].X + labelWidth
	c.Y = rows[c.Y].Y
	return c
}

// room is how many cells of a field's width its mode takes, none without vim.
func (k *vimKeys) room() int {
	if k.th == nil || !k.th.Vim() {
		return 0
	}
	return modeRoom
}

// typesChar is whether msg types one character, shift aside, as vim reads its commands.
func typesChar(msg tea.KeyPressMsg) bool {
	return msg.Mod&^tea.ModShift == 0 && len([]rune(msg.Text)) == 1
}

// movesCursor is whether msg is a key that moves the cursor without changing the text.
func movesCursor(msg tea.KeyPressMsg) bool {
	switch msg.String() {
	case "left", "right", "up", "down", "home", "end":
		return true
	}
	return false
}
