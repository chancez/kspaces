package tui

import (
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// Each datagram on the socket is a change, a socket left behind is replaced, and stop removes it.
func TestListenFocus(t *testing.T) {
	// Short, as a socket's path has a limit, which the test's temporary directory can pass on macOS.
	dir, err := os.MkdirTemp("", "kf")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	path := filepath.Join(dir, "run", "focus.sock")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	changed := make(chan struct{}, 2)
	stop := listenFocus(path, func() { changed <- struct{}{} })
	for range 2 {
		conn, err := net.Dial("unixgram", path)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := conn.Write([]byte("focus")); err != nil {
			t.Fatal(err)
		}
		conn.Close()
		select {
		case <-changed:
		case <-time.After(5 * time.Second):
			t.Fatal("no change for a datagram")
		}
	}
	stop()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("socket after stop: %v", err)
	}
}
