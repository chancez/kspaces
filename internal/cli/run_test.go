package cli

import (
	"bytes"
	"context"
	"reflect"
	"testing"
)

// A command line after -- needs a word, and takes no form or inputs; --background needs --yes or one.
func TestRunArgsFlags(t *testing.T) {
	var got []string
	for _, args := range [][]string{
		{"run", ".", "--"},
		{"run", "--yes", ".", "--", "ls"},
		{"run", "--input", "prompt=x", "--", "ls"},
		{"run", "--background", "claude"},
	} {
		var out bytes.Buffer
		cmd := (&app{stdout: &out, stderr: &out}).rootCommand()
		cmd.SetArgs(args)
		got = append(got, cmd.ExecuteContext(context.Background()).Error())
	}
	want := []string{
		"no command after --",
		"a command after -- takes no --from, --yes or --input",
		"a command after -- takes no --from, --yes or --input",
		"--background needs --yes",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %q\nwant %q", got, want)
	}
}
