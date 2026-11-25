package cli

import (
	"os"
	"testing"
)

// TestMain keeps the tests out of the user's state directory: a test that does not give one of its own
// shares a temporary one.
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "kspaces-cli-test")
	if err != nil {
		panic(err)
	}
	os.Setenv("XDG_STATE_HOME", dir)
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}
