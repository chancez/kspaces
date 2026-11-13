package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// kspaces reply WINDOW_ID TEXT sends TEXT and Enter to the window, an empty TEXT just Enter, and with
// --no-enter TEXT alone, a backslash as one; a window id that is no number is refused before kitten runs,
// and without TEXT a window kitty does not have is refused before the form opens.
func TestReply(t *testing.T) {
	dir := t.TempDir()
	log := filepath.Join(dir, "calls")
	// kitten @ ls has no windows. Each other call is recorded, each argument ended by a NUL, so one with a
	// newline stays one, and -- after each.
	script := "#!/bin/sh\n[ \"$2\" = ls ] && { echo '[]'; exit; }\n{ for a; do printf '%s\\0' \"$a\"; done; printf -- '--\\0'; } >> " + log + "\n"
	if err := os.WriteFile(filepath.Join(dir, "kitten"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("KITTY_INSTALLATION_DIR", "")
	var errs []string
	for _, args := range [][]string{
		{"reply", "7", "fix it\nplease"},
		{"reply", "7", ""},
		{"reply", "--no-enter", "7", `1\n`},
		{"reply", "x", "hi"},
		{"reply", "0", "hi"},
		{"reply", "99"},
	} {
		var out bytes.Buffer
		cmd := (&app{stdout: &out, stderr: &out}).rootCommand()
		cmd.SetArgs(args)
		msg := "<nil>"
		if err := cmd.ExecuteContext(context.Background()); err != nil {
			msg = err.Error()
		}
		errs = append(errs, msg)
	}
	if want := []string{"<nil>", "<nil>", "<nil>", `window id "x" is not a number above 0`, `window id "0" is not a number above 0`, "no window 99"}; !reflect.DeepEqual(errs, want) {
		t.Errorf("errors %q, want %q", errs, want)
	}
	data, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	got := strings.Split(strings.TrimSuffix(string(data), "\x00"), "\x00")
	want := []string{
		"@", "send-text", "--match", "id:7", "--", "fix it\nplease\\r", "--",
		"@", "send-text", "--match", "id:7", "--", `\r`, "--",
		"@", "send-text", "--match", "id:7", "--", `1\\n`, "--",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("kitten ran as\n%q\nwant\n%q", got, want)
	}
}
