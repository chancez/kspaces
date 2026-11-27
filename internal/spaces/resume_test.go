package spaces

import (
	"reflect"
	"testing"
)

// Opening a copy afresh opens the layout its host keeps, its sessions as this machine's provider names
// them, but those the provider no longer has running or shelved: a split of one left out opens as a tab.
func TestKeptOpens(t *testing.T) {
	h := NewLocalHost("", "")
	l := SharedLayout{Version: 1, Sessions: []string{"@1", "@2", "@3", "@4"}, Tabs: []SharedTab{
		{Title: "edit", Panes: []SharedPane{{Session: 0}, {Session: 1, Split: "right", Of: 1, Size: 40}}},
		{Panes: []SharedPane{{Session: 2}, {Session: 3, Split: "below", Of: 1}}},
	}}
	if _, err := h.SaveLayout("gr@dev", 0, l); err != nil {
		t.Fatal(err)
	}
	list := &ProviderState{Sessions: []ProviderSession{
		{ID: "white:@1", State: "running"}, {ID: "white:@2", State: "shelved"}, {ID: "white:@3", State: "exited"}, {ID: "white:@4", State: "running"},
	}}
	change, sessions, err := keptOpens("white:gr@dev", h, list)
	if err != nil {
		t.Fatal(err)
	}
	want := sharedChange{Open: []sharedAdd{
		{Session: "@1", Title: "edit"},
		{Session: "@2", SplitOf: "@1", Split: "right", Size: 40},
		{Session: "@4"},
	}}
	if !reflect.DeepEqual(change, want) {
		t.Errorf("opens %+v\nwant  %+v", change, want)
	}
	if want := []ProviderSession{list.Sessions[0], list.Sessions[1], list.Sessions[3]}; !reflect.DeepEqual(sessions, want) {
		t.Errorf("sessions %+v, want %+v", sessions, want)
	}
	// Nothing kept, nothing to open.
	if change, _, err := keptOpens("white:gr@other", h, list); err != nil || !change.Empty() {
		t.Errorf("with nothing kept: %+v, %v", change, err)
	}
}
