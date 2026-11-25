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
// the arrows, home and end move the cursor, as they do while typing, rather than vim dropping them.
func (k *vimKeys) update(area *textarea.Model, msg tea.Msg) (tea.Cmd, bool) {
	press, ok := msg.(tea.KeyPressMsg)
	v := k.on(area)
	if !ok || v == nil {
		return nil, false
	}
	if v.Mode() == vimbubble.Normal && movesCursor(press) {
		return nil, false
	}
	consumed, cmd := v.Update(press)
	return cmd, consumed
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
