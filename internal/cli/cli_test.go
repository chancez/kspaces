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

// --input is NAME=VALUE, the value taking any further =, and anything else is an error.
func TestParseInputs(t *testing.T) {
	type result struct {
		Inputs map[string]string
		Err    string
	}
	var got []result
	for _, flags := range [][]string{
		{"prompt=fix a=b", "model="},
		{"prompt"},
		{"=x"},
	} {
		in, err := parseInputs(flags)
		r := result{Inputs: in}
		if err != nil {
			r.Err = err.Error()
		}
		got = append(got, r)
	}
	want := []result{
		{Inputs: map[string]string{"prompt": "fix a=b", "model": ""}},
		{Err: `--input "prompt" is not NAME=VALUE`},
		{Err: `--input "=x" is not NAME=VALUE`},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %#v\nwant %#v", got, want)
	}
}
