package logs

import (
	"bytes"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"
)

// A program that fails is logged as an error, with what it said, and an action with how it ended; a
// program that succeeds, or exits with what Query takes for an answer, only at debug. Every line says
// the process's pid and role.
func TestLog(t *testing.T) {
	failed := []string{
		`level=ERROR msg=run role=test cmd="sh -c echo nope >&2; exit 3" err="exit status 3: nope"`,
	}
	ran := []string{
		`level=DEBUG msg=run role=test cmd="sh -c exit 1" err="exit status 1"`,
		`level=DEBUG msg=run role=test cmd=true`,
	}
	actions := []string{
		`level=INFO msg="create worktree" role=test branch=fix`,
		`level=ERROR msg="remove worktree" role=test dir=/x err="file does not exist"`,
	}
	for _, tc := range []struct {
		env  string
		want []string
	}{
		{env: "", want: slices.Concat(failed, actions)},
		{env: "debug", want: slices.Concat(failed, ran, actions)},
	} {
		dir := t.TempDir()
		t.Setenv("XDG_STATE_HOME", dir)
		t.Setenv("KSPACES_LOG", tc.env)
		Init("test")
		t.Cleanup(func() { logger = slog.New(slog.DiscardHandler) })
		Output(exec.Command("sh", "-c", "echo nope >&2; exit 3"))
		Query(exec.Command("sh", "-c", "exit 1"))
		Output(exec.Command("true"))
		Action("create worktree", time.Now(), nil, "branch", "fix")
		Action("remove worktree", time.Now(), os.ErrNotExist, "dir", "/x")
		data, err := os.ReadFile(filepath.Join(dir, "kspaces", "kspaces.log"))
		if err != nil {
			t.Fatal(err)
		}
		// What varies from run to run.
		vary := regexp.MustCompile(`time=\S+ | dur=\S+|pid=\d+ `)
		var got []string
		for _, l := range strings.Split(strings.TrimSpace(string(data)), "\n") {
			got = append(got, vary.ReplaceAllString(l, ""))
		}
		if !reflect.DeepEqual(got, tc.want) {
			t.Errorf("KSPACES_LOG=%q: got\n%s\nwant\n%s", tc.env, strings.Join(got, "\n"), strings.Join(tc.want, "\n"))
		}
	}
}

// Past maxSize the log is moved to kspaces.log.1, the one before it replaced, and a new one started.
func TestRotate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "kspaces.log")
	w := &file{path: path}
	chunk := bytes.Repeat([]byte("x"), 1<<20)
	for range 6 {
		if _, err := w.Write(chunk); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := w.Write([]byte("last\n")); err != nil {
		t.Fatal(err)
	}
	sizes := func(names ...string) []int64 {
		var out []int64
		for _, n := range names {
			info, err := os.Stat(n)
			if err != nil {
				t.Fatal(err)
			}
			out = append(out, info.Size())
		}
		return out
	}
	if got, want := sizes(path+".1", path), []int64{6 << 20, 5}; !reflect.DeepEqual(got, want) {
		t.Errorf("sizes %v, want %v", got, want)
	}
}
