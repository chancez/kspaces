package spaces

import (
	"errors"
	"path/filepath"
	"reflect"
	"testing"
)

// A host keeps its copies' layouts in a file, so a server started again has them, each at the revision
// its last save left it, and refuses a save behind it.
func TestLayoutStore(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state")
	l := SharedLayout{Version: 1, Sessions: []string{"@a"}, Tabs: []SharedTab{{Panes: []SharedPane{{Session: 0}}}}}
	first := NewLocalHost("", path)
	for _, rev := range []int64{0, 1} {
		if _, err := first.SaveLayout("gr@dev", rev, l); err != nil {
			t.Fatal(err)
		}
	}
	again := NewLocalHost("", path)
	if _, err := again.SaveLayout("gr@dev", 1, l); !errors.Is(err, ErrLayoutConflict) {
		t.Errorf("a save behind = %v", err)
	}
	got, _ := again.LoadLayouts([]CopyLayout{{Key: "gr@dev", Revision: 1}, {Key: "gr"}})
	if want := []CopyLayout{{Key: "gr@dev", Revision: 2, Layout: &l}}; !reflect.DeepEqual(got, want) {
		t.Errorf("loaded %+v, want %+v", got, want)
	}
}
