package cli

import (
	"reflect"
	"runtime/debug"
	"testing"
)

func TestVersion(t *testing.T) {
	tagged := &debug.BuildInfo{Main: debug.Module{Version: "v0.2.0"}}
	devel := &debug.BuildInfo{Main: debug.Module{Version: "(devel)"}}
	got := []string{
		versionOf("v0.3.0", tagged, true),
		versionOf("", tagged, true),
		versionOf("", devel, true),
		versionOf("", nil, false),
	}
	if want := []string{"v0.3.0", "v0.2.0", "dev", "dev"}; !reflect.DeepEqual(got, want) {
		t.Errorf("got %q, want %q", got, want)
	}
}
