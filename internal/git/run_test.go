package git

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// A git that runs past the timeout is asked to stop, and one that will not is killed killDelay later. The
// timeout leaves sh the time to start and set its trap.
func TestAskTimeout(t *testing.T) {
	for _, tc := range []struct {
		name   string
		script string
		want   string
	}{
		{name: "stops", script: "exec /bin/sleep 60", want: "signal: terminated"},
		{name: "ignores TERM", script: "trap '' TERM; while :; do /bin/sleep 1; done", want: "signal: killed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, "git"), []byte("#!/bin/sh\n"+tc.script+"\n"), 0o755); err != nil {
				t.Fatal(err)
			}
			t.Setenv("PATH", dir)
			SetTimeout(time.Second)
			t.Cleanup(func() { SetTimeout(DefaultTimeout) })
			delay := killDelay
			killDelay = 200 * time.Millisecond
			t.Cleanup(func() { killDelay = delay })
			start := time.Now()
			_, err := Ask((*exec.Cmd).Output, "status")
			if took := time.Since(start); took > 5*time.Second {
				t.Errorf("took %s", took)
			}
			if err == nil || err.Error() != tc.want {
				t.Errorf("got %v, want %s", err, tc.want)
			}
		})
	}
}
