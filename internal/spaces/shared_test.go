package spaces

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/chancez/kspaces/internal/kitty"
)

// sharedTabs are gr@dev's two tabs and one of another session's: a splits tab of windows 11, 12 and 13,
// 13 below the pair 11|12, 12 showing nothing of the provider's; and a stack tab of window 21. Window 31 is
// in gr@fix.
func sharedTabs(t *testing.T) []kitty.FullTab {
	t.Helper()
	const data = `[
	 {"id": 1, "layout": "splits", "windows": [
	   {"id": 11, "session_name": "gr@dev"}, {"id": 12, "session_name": "gr@dev"}, {"id": 13, "session_name": "gr@dev"}],
	  "layout_state": {
	   "pairs": {"horizontal": false, "bias": 0.7, "one": {"bias": 0.6, "one": 1, "two": 2}, "two": 3},
	   "all_windows": {"active_group_idx": 0, "window_groups": [
	     {"id": 1, "window_ids": [11]}, {"id": 2, "window_ids": [12]}, {"id": 3, "window_ids": [13]}]}}},
	 {"id": 2, "layout": "stack", "title": "logs", "title_overridden": true,
	  "windows": [{"id": 21, "session_name": "gr@dev"}],
	  "layout_state": {"all_windows": {"active_group_idx": 0, "window_groups": [{"id": 4, "window_ids": [21]}]}}},
	 {"id": 3, "layout": "splits", "windows": [{"id": 31, "session_name": "gr@fix"}],
	  "layout_state": {"pairs": {"one": 5}, "all_windows": {"active_group_idx": 0, "window_groups": [{"id": 5, "window_ids": [31]}]}}}
	]`
	var tabs []kitty.FullTab
	if err := json.Unmarshal([]byte(data), &tabs); err != nil {
		t.Fatal(err)
	}
	return tabs
}

// A worktree's shared layout is its panes that show the host's sessions, the splits tree as those panes
// alone would have it; a pane showing nothing of the provider's, or another host's session, is left out.
func TestCaptureShared(t *testing.T) {
	tabs := sharedTabs(t)
	shows := map[int]ProviderSession{
		11: {ID: "server:@a", Host: "server"},
		13: {ID: "server:@c", Host: "server"},
		21: {ID: "server:@d", Host: "server"},
		31: {ID: "server:@e", Host: "server"},
	}
	got, at := captureShared(tabs, "gr@dev", "server", shows)
	want := SharedLayout{Version: 1, Sessions: []string{"server:@a", "server:@c", "server:@d"}, Tabs: []SharedTab{
		{Panes: []SharedPane{{Session: 0}, {Session: 1, Split: "below", Of: 1, Size: 30}}},
		{Title: "logs", Layout: "stack", Panes: []SharedPane{{Session: 2}}},
	}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("layout\n%+v\nwant\n%+v", got, want)
	}
	wantAt := sharedLocal{
		Window: map[string]int{"server:@a": 11, "server:@c": 13, "server:@d": 21},
		Tab:    map[string]int{"server:@a": 1, "server:@c": 1, "server:@d": 2},
	}
	if !reflect.DeepEqual(at, wantAt) {
		t.Errorf("at %+v, want %+v", at, wantAt)
	}

	// Another host's session is not this host's to share.
	shows[13] = ProviderSession{ID: "laptop:@c", Host: "laptop"}
	got, _ = captureShared(tabs, "gr@dev", "server", shows)
	if want := `[{"panes":[{"session":"server:@a"}]},{"title":"logs","layout":"stack","panes":[{"session":"server:@d"}]}]`; got.Shape() != want {
		t.Errorf("shape %s, want %s", got.Shape(), want)
	}
}

// The shape of a layout names sessions in place, so the order they are listed in and who saved it do not
// make two layouts of the same panes differ.
func TestSharedShape(t *testing.T) {
	a := SharedLayout{Writer: "laptop", Sessions: []string{"@a", "@b"}, Tabs: []SharedTab{{Panes: []SharedPane{{Session: 0}, {Session: 1, Split: "right", Of: 1}}}}}
	b := SharedLayout{Writer: "desktop", Sessions: []string{"@b", "@a"}, Tabs: []SharedTab{{Panes: []SharedPane{{Session: 1}, {Session: 0, Split: "right", Of: 1}}}}}
	if a.Shape() != b.Shape() {
		t.Errorf("%s != %s", a.Shape(), b.Shape())
	}
}

// layoutOf is a layout of tabs of sessions by id, each tab's panes split right of the one before.
func layoutOf(tabs ...[]string) SharedLayout {
	l := SharedLayout{Version: 1, Sessions: []string{}, Tabs: []SharedTab{}}
	for _, ids := range tabs {
		var t SharedTab
		for i, id := range ids {
			p := SharedPane{Session: len(l.Sessions)}
			if i > 0 {
				p.Split, p.Of = "right", i
			}
			l.Sessions = append(l.Sessions, id)
			t.Panes = append(t.Panes, p)
		}
		l.Tabs = append(l.Tabs, t)
	}
	return l
}

// Taking another machine's layout opens what it added, as a split of the pane it split when that is open,
// else as a tab, and closes what it removed; what this machine added or removed since stays.
func TestMergeShared(t *testing.T) {
	base := layoutOf([]string{"a", "b"}, []string{"c"})
	tests := []struct {
		name          string
		base          *SharedLayout
		local, remote SharedLayout
		want          sharedChange
	}{
		{
			name:   "nothing changed",
			base:   &base,
			local:  base,
			remote: base,
		},
		{
			name:   "remote closed a split and a tab",
			base:   &base,
			local:  base,
			remote: layoutOf([]string{"a"}),
			want:   sharedChange{Close: []string{"b", "c"}},
		},
		{
			name:   "remote added a split and a tab",
			base:   &base,
			local:  base,
			remote: layoutOf([]string{"a", "b", "d"}, []string{"c"}, []string{"e", "f"}),
			want: sharedChange{Open: []sharedAdd{
				{Session: "d", SplitOf: "b", Split: "right"},
				{Session: "e"},
				{Session: "f", SplitOf: "e", Split: "right"},
			}},
		},
		{
			name:   "local changes stay",
			base:   &base,
			local:  layoutOf([]string{"a"}, []string{"c"}, []string{"g"}),
			remote: layoutOf([]string{"a", "b"}),
			// b was closed here, c there, and g opened here.
			want: sharedChange{Close: []string{"c"}},
		},
		{
			name:   "a split of a pane closed opens by the pane before it",
			base:   &base,
			local:  base,
			remote: layoutOf([]string{"b", "d"}),
			want:   sharedChange{Close: []string{"a", "c"}, Open: []sharedAdd{{Session: "d", SplitOf: "b", Split: "right"}}},
		},
		{
			name:   "no base opens what is missing and closes nothing",
			local:  layoutOf([]string{"a"}),
			remote: layoutOf([]string{"b"}),
			want:   sharedChange{Open: []sharedAdd{{Session: "b"}}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := mergeShared(tt.base, tt.local, tt.remote); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got  %+v\nwant %+v", got, tt.want)
			}
		})
	}
}
