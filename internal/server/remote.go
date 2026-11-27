package server

import (
	"bytes"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"
)

// NewRemote is a client of the server on another machine, reached by `ssh DEST KSPACES server proxy`,
// kspaces at version. Not its to replace: a server there of another protocol is an error.
func NewRemote(dest, kspaces, version string) *Client {
	return NewClient(func() (io.ReadWriteCloser, error) { return dialSSH(dest, kspaces) }, version)
}

// sshArgs are ssh's arguments to run kspaces server proxy on dest: without a terminal, and failing rather
// than asking for a password no one is there to type.
func sshArgs(dest, kspaces string) []string {
	return []string{"-T", "-o", "BatchMode=yes", "-o", "ConnectTimeout=10", dest, kspaces, "server", "proxy"}
}

// dialSSH runs ssh to the server on dest, its stdin and stdout the connection.
func dialSSH(dest, kspaces string) (io.ReadWriteCloser, error) {
	cmd := exec.Command("ssh", sshArgs(dest, kspaces)...)
	in, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	p := &pipeConn{cmd: cmd, in: in, out: out}
	cmd.Stderr = &p.stderr
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("running ssh to %s: %w", dest, err)
	}
	return p, nil
}

// pipeConn is a connection over a command's stdin and stdout. What it says on stderr, as ssh does when it
// cannot connect, is in the error of a read that finds it gone.
type pipeConn struct {
	cmd    *exec.Cmd
	in     io.WriteCloser
	out    io.ReadCloser
	stderr lockedBuffer
	once   sync.Once
}

func (p *pipeConn) Read(b []byte) (int, error) {
	n, err := p.out.Read(b)
	if err == io.EOF {
		// ssh is done: waiting for it has all it said on stderr.
		p.end(false)
		if said := strings.TrimSpace(p.stderr.String()); said != "" {
			return n, fmt.Errorf("%w: %s", io.ErrUnexpectedEOF, said)
		}
	}
	return n, err
}

func (p *pipeConn) Write(b []byte) (int, error) { return p.in.Write(b) }

// Close ends the command.
func (p *pipeConn) Close() error {
	p.end(true)
	return nil
}

// end closes the command's stdin, which ends the proxy there and ssh with it, killing it too when kill, and
// waits for it.
func (p *pipeConn) end(kill bool) {
	p.once.Do(func() {
		p.in.Close()
		if kill {
			p.cmd.Process.Kill()
		}
		p.cmd.Wait()
	})
}

// lockedBuffer is a bytes.Buffer safe to write from the command's goroutine while read from another.
type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	// Its last lines are what is worth saying.
	if b.buf.Len() > 4096 {
		b.buf.Reset()
	}
	return b.buf.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}
