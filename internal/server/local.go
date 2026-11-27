package server

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"

	"github.com/chancez/kspaces/internal/config"
	"github.com/chancez/kspaces/internal/state"
)

// startWait is how long a client waits for the server it started to listen.
const startWait = 5 * time.Second

// SocketPath is where this host's server listens: in the runtime directory, named after what decides the
// host data, the config's file, the environments it reads and the state directory, so a sandbox with its
// own config or state has a server of its own.
func SocketPath() (string, error) {
	cfg, err := config.FilePath()
	if err != nil {
		return "", err
	}
	st, err := state.Dir()
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256([]byte(cfg + "\x00" + os.Getenv("KSPACES_ENV") + "\x00" + st))
	return filepath.Join(state.RuntimeDir(), "server-"+hex.EncodeToString(sum[:8])+".sock"), nil
}

// Listen listens on the socket at path, once it holds the lock beside it, so one server listens there at a
// time. ErrRunning when another holds it. unlock releases the lock, after the listener is closed.
func Listen(path string) (ln net.Listener, unlock func(), err error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, nil, err
	}
	lock, err := os.OpenFile(path+".lock", os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, nil, err
	}
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		lock.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) {
			return nil, nil, ErrRunning
		}
		return nil, nil, err
	}
	// A socket left by a server that did not get to remove it.
	os.Remove(path)
	ln, err = net.Listen("unix", path)
	if err != nil {
		lock.Close()
		return nil, nil, err
	}
	return ln, func() { lock.Close() }, nil
}

// ErrRunning is another server listening.
var ErrRunning = errors.New("a kspaces server is already running")

// NewLocal is a client of this host's server, at the socket path, which it starts with kspaces' binary
// exe when it is not running, and replaces when it speaks an older protocol.
func NewLocal(path, exe, version string) *Client {
	c := NewClient(func() (io.ReadWriteCloser, error) { return DialLocal(path, exe) }, version)
	c.replace = func(_ *Client, cn *clientConn) error {
		if err := cn.call(methodExit, struct{}{}, nil); err != nil {
			return err
		}
		cn.close(errGone)
		return waitUnlocked(path)
	}
	return c
}

// DialLocal connects to the socket at path, starting the server with exe first when nothing listens.
func DialLocal(path, exe string) (io.ReadWriteCloser, error) {
	if conn, err := net.Dial("unix", path); err == nil {
		return conn, nil
	}
	if err := start(exe); err != nil {
		return nil, fmt.Errorf("starting the kspaces server: %w", err)
	}
	deadline := time.Now().Add(startWait)
	for {
		conn, err := net.Dial("unix", path)
		if err == nil {
			return conn, nil
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("the kspaces server did not start: %w", err)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// start runs `exe server` in a session of its own, so it outlives the process that started it and the
// terminal it runs in. Its output goes to the log, through logs.
func start(exe string) error {
	devnull, err := os.OpenFile(os.DevNull, os.O_RDWR, 0)
	if err != nil {
		return err
	}
	defer devnull.Close()
	cmd := exec.Command(exe, "server")
	cmd.Stdin, cmd.Stdout, cmd.Stderr = devnull, devnull, devnull
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return err
	}
	// Reaped by whoever adopts it once this process exits; until then, in the background.
	go cmd.Wait()
	return nil
}

// waitUnlocked waits until no server holds the lock beside the socket at path.
func waitUnlocked(path string) error {
	deadline := time.Now().Add(startWait)
	for {
		lock, err := os.OpenFile(path+".lock", os.O_CREATE|os.O_RDWR, 0o600)
		if err != nil {
			return err
		}
		err = syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		lock.Close()
		if err == nil {
			return nil
		}
		if time.Now().After(deadline) {
			return errors.New("the old kspaces server did not exit")
		}
		time.Sleep(20 * time.Millisecond)
	}
}
