package tui

import (
	"fmt"
	"image/color"
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textarea"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/chancez/kspaces/internal/tui/look"
)

// field is a form's text box, which typing goes to while it has focus.
type field interface {
	Focus() tea.Cmd
	Blur()
	Focused() bool
	View() string
	Value() string
	SetWidth(int)
	// fill sets the value, the cursor at its end.
	fill(string)
	// update hands msg, typing, to the box.
	update(tea.Msg) tea.Cmd
	// place puts the cursor where a click on cell x of row y of the box, as View draws it, lands.
	place(x, y int)
	// keeps says whether the box takes msg rather than the form, as up and down between its lines.
	keeps(msg tea.KeyPressMsg) bool
	// mode is the box's vim mode, as NORMAL, or "" without vim, see vimKeys.
	mode() string
	// cursor is the terminal's cursor in the box, from its top left, nil while it draws its own, see
	// vimKeys.cursor.
	cursor() *tea.Cursor
	// formKey is msg as the form takes it, see vimKeys.formKey.
	formKey(msg tea.KeyPressMsg) tea.KeyPressMsg
}

// lineField is one line of text, for a branch, a path or a name. It is a text area of one row, which shows
// the part of a long value the cursor is in, so it edits as a textField does.
type lineField struct {
	textarea.Model
	vim vimKeys
}

// newLineField is a line field in th's styles showing placeholder while empty.
func newLineField(th *look.Theme, placeholder string) lineField {
	in := textarea.New()
	in.Prompt = ""
	in.ShowLineNumbers = false
	in.Placeholder = placeholder
	in.SetHeight(1)
	// Enter is the form's, to submit: the text is one line.
	in.KeyMap.InsertNewline.SetEnabled(false)
	in.SetStyles(th.AreaStyles())
	return lineField{Model: in, vim: vimKeys{th: th}}
}

// Focus gives the box focus, in the vim mode the box before had.
func (f *lineField) Focus() tea.Cmd {
	f.vim.focus(&f.Model)
	return f.Model.Focus()
}

// SetWidth makes the box width wide, its mode included.
func (f *lineField) SetWidth(width int) { f.Model.SetWidth(max(width-f.vim.room(), 1)) }

func (f *lineField) fill(s string) {
	f.SetValue(joinLines(s))
	f.MoveToEnd()
}

func (f *lineField) update(msg tea.Msg) tea.Cmd {
	if p, ok := msg.(tea.PasteMsg); ok {
		msg = tea.PasteMsg{Content: joinLines(p.Content)}
	}
	if cmd, ok := f.vim.update(&f.Model, msg); ok {
		return cmd
	}
	var cmd tea.Cmd
	f.Model, cmd = f.Model.Update(msg)
	return cmd
}

// place puts the cursor where the click lands, which textarea works out through wrapping.
func (f *lineField) place(x, y int) {
	f.vim.leaveVisual(&f.Model)
	f.BeginSelection(x, y)
	f.EndSelection()
}

// joinLines is s with its line breaks as spaces, as a text input takes a paste, so a line field's text
// stays one line.
func joinLines(s string) string {
	return strings.NewReplacer("\r\n", " ", "\n", " ", "\r", " ").Replace(s)
}

func (f *lineField) keeps(msg tea.KeyPressMsg) bool { return f.vim.keeps(&f.Model, msg) }

func (f *lineField) mode() string { return f.vim.mode(&f.Model) }

func (f *lineField) cursor() *tea.Cursor { return f.vim.cursor(&f.Model) }

func (f *lineField) formKey(msg tea.KeyPressMsg) tea.KeyPressMsg { return f.vim.formKey(&f.Model, msg) }

// textField is text that can take several lines, for the inputs of commands and layouts, which go to a
// program as they are. shift+enter starts a line, or alt+enter or ctrl+j, for a terminal that sends
// shift+enter as enter, which the form keeps for submitting. The box grows to show the lines, wrapped, up
// to textLines, then scrolls with the cursor.
type textField struct {
	textarea.Model
	vim vimKeys
}

// textLines is the most lines a text field shows.
const textLines = 6

// newTextField is a text field in th's styles showing placeholder while empty.
func newTextField(th *look.Theme, placeholder string) textField {
	in := textarea.New()
	in.Prompt = ""
	in.ShowLineNumbers = false
	in.Placeholder = placeholder
	in.DynamicHeight = true
	in.MaxHeight = textLines
	// Unset, MaxHeight would also stop the lines at textLines; this is textarea's own limit.
	in.MaxContentHeight = 10000
	in.KeyMap.InsertNewline = key.NewBinding(key.WithKeys("shift+enter", "alt+enter", "ctrl+j"))
	in.SetStyles(th.AreaStyles())
	return textField{Model: in, vim: vimKeys{th: th}}
}

// Focus gives the box focus, in the vim mode the box before had.
func (f *textField) Focus() tea.Cmd {
	f.vim.focus(&f.Model)
	return f.Model.Focus()
}

// SetWidth makes the box width wide, its mode included.
func (f *textField) SetWidth(width int) { f.Model.SetWidth(max(width-f.vim.room(), 1)) }

func (f *textField) fill(s string) {
	f.SetValue(s)
	f.MoveToEnd()
}

func (f *textField) update(msg tea.Msg) tea.Cmd {
	if cmd, ok := f.vim.update(&f.Model, msg); ok {
		return cmd
	}
	var cmd tea.Cmd
	f.Model, cmd = f.Model.Update(msg)
	return cmd
}

// place puts the cursor where the click lands, which textarea works out through wrapping and scrolling.
func (f *textField) place(x, y int) {
	f.vim.leaveVisual(&f.Model)
	f.BeginSelection(x, y)
	f.EndSelection()
}

// keeps up and down while they move between the box's lines, so only up on its first and down on its
// last go on to the form, to the field above or below.
func (f *textField) keeps(msg tea.KeyPressMsg) bool {
	if f.vim.keeps(&f.Model, msg) {
		return true
	}
	li := f.LineInfo()
	switch msg.String() {
	case "up":
		return f.Line() > 0 || li.RowOffset > 0
	case "down":
		return f.Line() < f.LineCount()-1 || li.RowOffset < li.Height-1
	}
	return false
}

func (f *textField) mode() string { return f.vim.mode(&f.Model) }

func (f *textField) cursor() *tea.Cursor { return f.vim.cursor(&f.Model) }

func (f *textField) formKey(msg tea.KeyPressMsg) tea.KeyPressMsg { return f.vim.formKey(&f.Model, msg) }

// keptBy says whether fl, the focused field, nil for a choice, keeps msg from the form's keys.
func keptBy(fl field, msg tea.KeyPressMsg) bool { return fl != nil && fl.keeps(msg) }

// newlineHints are the keys that start a line in fl, for a form's key hints, when it takes several.
func newlineHints(fl field) []look.Span {
	if _, ok := fl.(*textField); !ok {
		return nil
	}
	return []look.Span{look.Text("  "), look.Key("shift+enter"), look.KeyDesc(" newline")}
}

// fieldZone is the zone fl's row is drawn in: its address, as every field has its own.
func fieldZone(fl field) string { return fmt.Sprintf("field:%p", fl) }

// fieldLines draw fl after its label, labelWidth cells, a line for each of its box's, in width cells, and
// its vim mode at the right of the first while it has focus. Each is marked as fl's zone, so a click on the
// label or in the box finds it, see clickedField.
func fieldLines(th *look.Theme, label string, fl field, width int) []string {
	head := th.DrawCut(look.Plain, labelWidth-1, look.Label(label)) + " "
	mode := fl.mode()
	if !fl.Focused() {
		mode = ""
	}
	var out []string
	for _, row := range strings.Split(fl.View(), "\n") {
		line := head + row
		if mode != "" {
			line += strings.Repeat(" ", max(width-modeRoom-ansi.StringWidth(line), 0)) + th.Render(look.Plain, look.Muted(" "+mode))
			mode = ""
		}
		out = append(out, th.DrawZone(fieldZone(fl), look.Plain, width, look.Styled(func(color.Color) string { return line })))
		head = strings.Repeat(" ", labelWidth)
	}
	return out
}

// clickedField is the index in fields, nil for a choice, of the one whose lines, as fieldLines draws them,
// msg is a left click on, in the frame zones were scanned from. A click in the box puts its cursor there.
func clickedField(fields []field, zones map[string][]look.Zone, msg tea.MouseClickMsg) (int, bool) {
	if msg.Button != tea.MouseLeft {
		return 0, false
	}
	for i, fl := range fields {
		if fl == nil {
			continue
		}
		for row, z := range zones[fieldZone(fl)] {
			if !z.Contains(msg.X, msg.Y) {
				continue
			}
			if x := msg.X - z.X - labelWidth; x >= 0 {
				fl.place(x, row)
			}
			return i, true
		}
	}
	return 0, false
}

// moveFocus moves focus by delta over fields, round from the last to the first, from *focus, which it
// updates. A nil field is a choice, which takes no typing.
func moveFocus(fields []field, focus *int, delta int) tea.Cmd {
	for _, in := range fields {
		if in != nil {
			in.Blur()
		}
	}
	if len(fields) == 0 {
		return nil
	}
	*focus = ((*focus+delta)%len(fields) + len(fields)) % len(fields)
	if in := fields[*focus]; in != nil {
		return in.Focus()
	}
	return nil
}
