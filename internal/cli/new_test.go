package cli

import (
	"bytes"
	"context"
	"reflect"
	"testing"
)

// What creates a worktree without the form needs --yes, and a command line after -- a word.
func TestNewNeedsYes(t *testing.T) {
	var got []string
	for _, args := range [][]string{
		{"new", "--branch", "fix"},
		{"new", "--background"},
		{"new", "--", "ls"},
		{"new", "--yes", "--"},
		{"new", "--yes", "--command", "claude", "--", "ls"},
	} {
		var out bytes.Buffer
		cmd := (&app{stdout: &out, stderr: &out}).rootCommand()
		cmd.SetArgs(args)
		got = append(got, cmd.ExecuteContext(context.Background()).Error())
	}
	want := []string{
		"--branch, --dir, --layout, --command, --input and --background need --yes",
		"--branch, --dir, --layout, --command, --input and --background need --yes",
		"a command after -- needs --yes, and a word",
		"a command after -- needs --yes, and a word",
		"a command after -- takes no --layout or --command",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %q\nwant %q", got, want)
	}
}
