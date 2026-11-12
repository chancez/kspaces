package tui

import (
	"errors"
	"os"
	"reflect"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/chancez/kspaces/internal/config"
	"github.com/chancez/kspaces/internal/spaces"
)

// stubForm is a popup's form that works while busy and fails with err.
type stubForm struct {
	busy, err bool
	// next is what the next update leaves busy and err as, and returns.
	next func(f *stubForm) tea.Cmd
}

func (f *stubForm) Init() tea.Cmd { return nil }
func (f *stubForm) Update(tea.Msg) (tea.Model, tea.Cmd) {
	if f.next == nil {
		return f, nil
	}
	next := f.next
	f.next = nil
	return f, next(f)
}
func (f *stubForm) View() tea.View { return tea.NewView("form") }
func (f *stubForm) working() bool  { return f.busy }
func (f *stubForm) failed() bool   { return f.err }

// quits says whether cmd quits the program.
func quits(cmd tea.Cmd) bool {
	if cmd == nil {
		return false
	}
	_, ok := cmd().(tea.QuitMsg)
	return ok
}

// stubPopup is a popup of m opened from window from, its own window 20, recording what it focuses.
func stubPopup(m popupModel, from int, acted bool) (*popup, *[]int) {
	p := newPopup(m, from)
	focused := &[]int{}
	p.self, p.acted = 20, func() bool { return acted }
	p.focus = func(id int) error {
		*focused = append(*focused, id)
		return nil
	}
	return p, focused
}

// A panel quits when it loses focus, and asks kitty for focus reports to know; an overlay does neither.
func TestPopupBlur(t *testing.T) {
	p, _ := stubPopup(&stubForm{}, 11, false)
	if !p.View().ReportFocus {
		t.Error("a panel does not report focus")
	}
	if _, cmd := p.Update(tea.BlurMsg{}); !quits(cmd) {
		t.Error("a panel did not quit on a blur")
	}
	p, _ = stubPopup(&stubForm{}, 0, false)
	if p.View().ReportFocus {
		t.Error("an overlay reports focus")
	}
	if _, cmd := p.Update(tea.BlurMsg{}); quits(cmd) {
		t.Error("an overlay quit on a blur")
	}
}

// A panel that is working stays through a blur, which what it does may cause, and quits when done. Failing
// after one, it takes the keyboard back to show why.
func TestPopupBlurWhileWorking(t *testing.T) {
	f := &stubForm{busy: true}
	p, focused := stubPopup(f, 11, true)
	p.busy = true
	if _, cmd := p.Update(tea.BlurMsg{}); quits(cmd) {
		t.Fatal("a working panel quit on a blur")
	}
	f.next = func(f *stubForm) tea.Cmd { f.busy = false; return tea.Quit }
	if _, cmd := p.Update(struct{}{}); !quits(cmd) {
		t.Fatal("done, the form did not quit")
	}
	if err := p.leave(); err != nil || len(*focused) != 0 {
		t.Errorf("leaving after a blur focused %v, %v", *focused, err)
	}

	f = &stubForm{busy: true}
	p, focused = stubPopup(f, 11, true)
	p.busy = true
	p.Update(tea.BlurMsg{})
	f.next = func(f *stubForm) tea.Cmd { f.busy, f.err = false, true; return nil }
	_, cmd := p.Update(struct{}{})
	if quits(cmd) {
		t.Fatal("failed, the panel quit")
	}
	if want := []int{20}; !reflect.DeepEqual(*focused, want) {
		t.Errorf("failed after a blur, focused %v, want %v", *focused, want)
	}
}

// Closing, a panel gives the keyboard back to the window it was opened from, unless what it did moved it
// or it had lost it; an overlay leaves that to kitty.
func TestPopupLeave(t *testing.T) {
	for _, c := range []struct {
		name  string
		from  int
		acted bool
		// done is whether it finished working, blurred whether it lost focus.
		done, blurred bool
		want          []int
	}{
		{"left with escape", 11, false, false, false, []int{11}},
		{"left after an action failed", 11, true, false, false, []int{11}},
		{"done without moving the keyboard", 11, false, true, false, []int{11}},
		{"done, switched sessions", 11, true, true, false, []int{}},
		{"lost focus", 11, false, false, true, []int{}},
		{"overlay", 0, false, false, false, []int{}},
	} {
		f := &stubForm{}
		p, focused := stubPopup(f, c.from, c.acted)
		if c.done {
			f.busy, p.busy = true, true
			f.next = func(f *stubForm) tea.Cmd { f.busy = false; return tea.Quit }
			p.Update(struct{}{})
		}
		if c.blurred {
			p.Update(tea.BlurMsg{})
		}
		if err := p.leave(); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(*focused, c.want) {
			t.Errorf("%s: focused %v, want %v", c.name, *focused, c.want)
		}
	}
}

// The picker quits working, from the message its choice sends, so it has switched once it has acted.
func TestPopupLeavePicker(t *testing.T) {
	f := &stubForm{busy: true}
	p, focused := stubPopup(f, 11, true)
	p.busy = true
	if err := p.leave(); err != nil || len(*focused) != 0 {
		t.Errorf("focused %v, %v", *focused, err)
	}
}

// The picker is working once a choice is made, so a panel stays through the blur that switching causes;
// a choice that fails leaves it showing why, with the keyboard back.
func TestPickerInPanel(t *testing.T) {
	fakeKittenCalls(t)
	m := newPicker(0)
	p, focused := stubPopup(m, 11, true)
	p.Update(tea.WindowSizeMsg{Width: 40, Height: 26})
	p.Update(fixture(time.Now()))
	if _, cmd := p.Update(tea.KeyPressMsg{Code: tea.KeyEnter}); cmd == nil || !m.working() {
		t.Fatalf("enter chose nothing: %v", m.working())
	}
	if _, cmd := p.Update(tea.BlurMsg{}); quits(cmd) {
		t.Fatal("choosing, the panel quit on a blur")
	}
	_, cmd := p.Update(spaces.Snapshot{Err: errors.New("no such session")})
	if m.working() || !m.failed() || quits(cmd) {
		t.Fatalf("failed: working %v, failed %v", m.working(), m.failed())
	}
	if want := []int{20}; !reflect.DeepEqual(*focused, want) {
		t.Errorf("focused %v, want %v", *focused, want)
	}
}

// The sidebar opens its forms as its config says: with popup = "panel", in a panel for the window beside it.
func TestSidebarFormInPanel(t *testing.T) {
	e := newSidebarEnv(t)
	launched := fakeKitten(t)
	e.m.cfg.Sidebar.Popup = config.PopupPanel
	if msg := e.m.addSpace()(); msg != nil {
		t.Fatalf("adding a space said %v", msg)
	}
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"@", "launch", "--type=os-panel", "--os-panel", "edge=center-sized", "--os-panel", "columns=90",
		"--os-panel", "lines=24", "--os-panel", "focus-policy=exclusive", "--os-panel", "layer=overlay",
		"--var=kspaces_form=1", "--var=kspaces_popup=1", "--env", "KSPACES_FROM=11", exe, "add",
	}
	if got := launched(); !reflect.DeepEqual(got, want) {
		t.Errorf("launched %q\nwant %q", got, want)
	}
}
