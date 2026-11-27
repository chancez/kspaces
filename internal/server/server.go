package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"sync"
	"time"

	"github.com/chancez/kspaces/internal/logs"
	"github.com/chancez/kspaces/internal/spaces"
)

// Idle is how long a server with no client waits for one before it exits.
const Idle = 5 * time.Minute

// Server serves a host's data to its clients, see docs/server.md.
type Server struct {
	host    spaces.Host
	version string
	name    string
	// idle is how long it waits with no client before Serve returns.
	idle time.Duration
	// changed says whether kspaces' binary was replaced since the server started, as by installing another,
	// so the server exits and a client starts the new one; nil for never.
	changed func() bool
	// every is how often changed is asked.
	every time.Duration

	mu      sync.Mutex
	clients int
	// activity is sent to when a client comes or goes.
	activity chan struct{}
	// exit is closed when a client asks the server to exit.
	exit     chan struct{}
	exitOnce sync.Once
}

// New is a server of host's data, kspaces at version, on the host named name.
func New(host spaces.Host, version, name string) *Server {
	return &Server{host: host, version: version, name: name, idle: Idle, every: 10 * time.Second, activity: make(chan struct{}, 1), exit: make(chan struct{})}
}

// Serve accepts clients from ln until ctx is done, a client asks it to exit, it has had none for its idle
// time, or kspaces' binary changed. It closes ln.
func (s *Server) Serve(ctx context.Context, ln net.Listener) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	go func() {
		<-ctx.Done()
		ln.Close()
	}()
	go s.watch(ctx, cancel)
	for {
		conn, err := ln.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return err
		}
		go s.ServeConn(ctx, conn)
	}
}

// watch cancels once the server has nothing left to do: asked to exit, idle, or its binary replaced.
func (s *Server) watch(ctx context.Context, cancel func()) {
	idle := time.NewTimer(s.idle)
	defer idle.Stop()
	var tick <-chan time.Time
	if s.changed != nil {
		t := time.NewTicker(s.every)
		defer t.Stop()
		tick = t.C
	}
	for {
		select {
		case <-ctx.Done():
			return
		case <-s.exit:
			logs.Debug("server asked to exit")
			cancel()
			return
		case <-idle.C:
			logs.Debug("server idle")
			cancel()
			return
		case <-tick:
			if s.changed() {
				logs.Debug("server's binary replaced")
				cancel()
				return
			}
		case <-s.activity:
			s.mu.Lock()
			clients := s.clients
			s.mu.Unlock()
			idle.Stop()
			if clients == 0 {
				idle.Reset(s.idle)
			}
		}
	}
}

func (s *Server) clientsChanged(by int) {
	s.mu.Lock()
	s.clients += by
	s.mu.Unlock()
	select {
	case s.activity <- struct{}{}:
	default:
	}
}

// ServeConn serves one client on rw until it hangs up or ctx is done.
func (s *Server) ServeConn(ctx context.Context, rw io.ReadWriteCloser) {
	s.clientsChanged(1)
	defer s.clientsChanged(-1)
	c := &serverConn{s: s, rw: rw, enc: json.NewEncoder(rw), push: make(chan spaces.HostData, 1)}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	go func() {
		<-ctx.Done()
		rw.Close()
	}()
	go c.pushes(ctx)
	defer c.unsubscribe()
	dec := json.NewDecoder(rw)
	for {
		var msg message
		if err := dec.Decode(&msg); err != nil {
			if !errors.Is(err, io.EOF) && ctx.Err() == nil {
				logs.Debug("server connection", "err", err)
			}
			return
		}
		go c.handle(msg)
	}
}

// serverConn is one client's connection.
type serverConn struct {
	s     *Server
	rw    io.ReadWriteCloser
	encMu sync.Mutex
	enc   *json.Encoder
	// push holds the latest host data to push, see pushes.
	push  chan spaces.HostData
	subMu sync.Mutex
	stop  func()
}

func (c *serverConn) send(msg message) {
	c.encMu.Lock()
	defer c.encMu.Unlock()
	if err := c.enc.Encode(msg); err != nil {
		logs.Debug("server write", "err", err)
		c.rw.Close()
	}
}

// pushes sends the host data subscribed to as it comes, the latest only, so a slow client never holds up
// a load.
func (c *serverConn) pushes(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case hd := <-c.push:
			params, err := json.Marshal(hd)
			if err != nil {
				continue
			}
			c.send(message{Method: methodUpdate, Params: params})
		}
	}
}

// pushLatest queues hd to push, in place of any not pushed yet.
func (c *serverConn) pushLatest(hd spaces.HostData) {
	for {
		select {
		case c.push <- hd:
			return
		default:
		}
		select {
		case <-c.push:
		default:
		}
	}
}

func (c *serverConn) unsubscribe() {
	c.subMu.Lock()
	defer c.subMu.Unlock()
	if c.stop != nil {
		c.stop()
		c.stop = nil
	}
}

// handle answers msg.
func (c *serverConn) handle(msg message) {
	if msg.ID == 0 {
		return
	}
	result, err := c.answer(msg)
	resp := message{ID: msg.ID}
	if err != nil {
		resp.Error = err.Error()
	} else if resp.Result, err = json.Marshal(result); err != nil {
		resp.Error = err.Error()
	}
	c.send(resp)
	if msg.Method == methodExit {
		c.s.exitOnce.Do(func() { close(c.s.exit) })
	}
}

func (c *serverConn) answer(msg message) (any, error) {
	h := c.s.host
	switch msg.Method {
	case methodHello:
		return Hello{Protocol: Protocol, Version: c.s.version, Host: c.s.name}, nil
	case methodLoad:
		var p loadParams
		if len(msg.Params) > 0 {
			if err := json.Unmarshal(msg.Params, &p); err != nil {
				return nil, err
			}
		}
		return h.Load(p.Fresh)
	case methodRefresh:
		return struct{}{}, h.Refresh()
	case methodSubscribe:
		c.subMu.Lock()
		if c.stop == nil {
			c.stop = h.Subscribe(c.pushLatest)
		}
		c.subMu.Unlock()
		return h.Load(false)
	case methodExit:
		return struct{}{}, nil
	case methodNewWorktree:
		var p newWorktreeParams
		if err := json.Unmarshal(msg.Params, &p); err != nil {
			return nil, err
		}
		return h.NewWorktree(p.Root, p.Branch, p.Base)
	case methodRemoveWorktree:
		var p removeWorktreeParams
		if err := json.Unmarshal(msg.Params, &p); err != nil {
			return nil, err
		}
		return struct{}{}, h.RemoveWorktree(p.Path, spaces.RemoveOptions{Force: p.Force, DeleteBranch: p.DeleteBranch, ForceBranch: p.ForceBranch})
	case methodLoadLayouts:
		var p loadLayoutsParams
		if err := json.Unmarshal(msg.Params, &p); err != nil {
			return nil, err
		}
		got, err := h.LoadLayouts(p.Layouts)
		return loadLayoutsParams{Layouts: got}, err
	case methodSaveLayout:
		var p saveLayoutParams
		if err := json.Unmarshal(msg.Params, &p); err != nil {
			return nil, err
		}
		rev, err := h.SaveLayout(p.Key, p.Revision, p.Layout)
		return saved{Revision: rev}, err
	}
	return nil, fmt.Errorf("no method %q", msg.Method)
}

// ExeChanged is a Server's check that kspaces' binary at path was replaced since now: another file there,
// or none.
func ExeChanged(path string) func() bool {
	before, err := os.Stat(path)
	if err != nil {
		return nil
	}
	return func() bool {
		now, err := os.Stat(path)
		return err != nil || !os.SameFile(before, now) || !now.ModTime().Equal(before.ModTime())
	}
}

// WatchExe has the server exit once changed says kspaces' binary was replaced.
func (s *Server) WatchExe(changed func() bool) {
	s.changed = changed
}
