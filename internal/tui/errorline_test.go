package tui

import (
	"errors"
	"reflect"
	"slices"
	"testing"

	"github.com/chancez/kspaces/internal/spaces"
)

// An error of several lines, as gh's often are, is one line on the last line: its line breaks would
// push the frame past the sidebar's height.
func TestErrorOnOneLine(t *testing.T) {
	e := newSidebarEnv(t)
	before := e.screen()
	e.m.Update(spaces.Snapshot{Err: errors.New("no git remotes\n  add one\tfirst")})
	want := append(slices.Clone(before[:len(before)-1]), "no git remotes add one first")
	if got := e.screen(); !reflect.DeepEqual(got, want) {
		t.Errorf("screen:\n%q\nwant\n%q", got, want)
	}
}
