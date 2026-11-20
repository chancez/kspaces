package tui

import (
	"net"
	"os"
	"path/filepath"

	"github.com/chancez/kspaces/internal/logs"
)

// focusSocketEnv names the socket the sidebar listens on for kitty's focus moving, which the kitty watcher
// that started it sets and sends to, see kspaces_watcher.py. A focus change can be another session, which
// the sidebar should show at once rather than at its next poll.
const focusSocketEnv = "KSPACES_FOCUS_SOCKET"

// kittyFocusMsg is kitty's focus moving, from window to window, unlike tea.FocusMsg, the sidebar's own.
type kittyFocusMsg struct{}

// listenFocus calls changed for each datagram on the socket at path, until the returned stop. A socket it
// cannot listen on is logged, and the sidebar polls as it would without one.
func listenFocus(path string, changed func()) (stop func()) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		logs.Error("listening for focus changes", err)
		return func() {}
	}
	// A sidebar that did not exit cleanly leaves its socket behind.
	os.Remove(path)
	conn, err := net.ListenUnixgram("unixgram", &net.UnixAddr{Name: path, Net: "unixgram"})
	if err != nil {
		logs.Error("listening for focus changes", err)
		return func() {}
	}
	go func() {
		buf := make([]byte, 64)
		for {
			if _, err := conn.Read(buf); err != nil {
				return
			}
			changed()
		}
	}()
	return func() {
		conn.Close()
		os.Remove(path)
	}
}
