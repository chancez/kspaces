package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/chancez/kspaces/internal/config"
	"github.com/chancez/kspaces/internal/gh"
	"github.com/chancez/kspaces/internal/gitstate"
	"github.com/chancez/kspaces/internal/spaces"
)

// fakeHost is a spaces.Host whose loads are data, calling its subscribers when data changed since the
// last load.
type fakeHost struct {
	mu        sync.Mutex
	data      spaces.HostData
	loads     int
	refreshes int
	last      string
	subs      map[int]func(spaces.HostData)
	next      int
	// asked are the worktrees it was asked to create and remove.
	asked []string
	// store keeps its layouts.
	store *spaces.LocalHost
}

func (h *fakeHost) set(hd spaces.HostData) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.data = hd
}

func (h *fakeHost) Load(fresh bool) (spaces.HostData, error) {
	h.mu.Lock()
	hd := h.data
	if fresh {
		h.loads++
	}
	data, _ := json.Marshal(hd)
	changed := fresh && string(data) != h.last
	if changed {
		h.last = string(data)
	}
	var subs []func(spaces.HostData)
	for _, f := range h.subs {
		subs = append(subs, f)
	}
	h.mu.Unlock()
	if changed {
		for _, f := range subs {
			f(hd)
		}
	}
	return hd, nil
}

func (h *fakeHost) NewWorktree(root, branch, base string) (spaces.Worktree, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.asked = append(h.asked, "new "+root+" "+branch+" "+base)
	if branch == "taken" {
		return spaces.Worktree{}, errors.New("a branch named 'taken' already exists")
	}
	return spaces.Worktree{Path: root + "/.worktrees/" + branch, Branch: branch, Session: "gr@" + branch}, nil
}

func (h *fakeHost) RemoveWorktree(path string, opts spaces.RemoveOptions) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.asked = append(h.asked, fmt.Sprintf("remove %s %+v", path, opts))
	return nil
}

func (h *fakeHost) LoadLayouts(known []spaces.CopyLayout) ([]spaces.CopyLayout, error) {
	return h.store.LoadLayouts(known)
}

func (h *fakeHost) SaveLayout(key string, revision int64, l spaces.SharedLayout) (int64, error) {
	return h.store.SaveLayout(key, revision, l)
}

func (h *fakeHost) Refresh() error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.refreshes++
	return nil
}

func (h *fakeHost) Subscribe(f func(spaces.HostData)) (stop func()) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.subs == nil {
		h.subs = map[int]func(spaces.HostData){}
	}
	id := h.next
	h.next++
	h.subs[id] = f
	return func() {
		h.mu.Lock()
		defer h.mu.Unlock()
		delete(h.subs, id)
	}
}

// pipes is a net.Listener of in-memory connections, which synctest can wait on.
type pipes struct {
	conns  chan net.Conn
	closed chan struct{}
	once   sync.Once
}

func newPipes() *pipes { return &pipes{conns: make(chan net.Conn), closed: make(chan struct{})} }

func (p *pipes) Accept() (net.Conn, error) {
	select {
	case c := <-p.conns:
		return c, nil
	case <-p.closed:
		return nil, net.ErrClosed
	}
}

func (p *pipes) Close() error {
	p.once.Do(func() { close(p.closed) })
	return nil
}

func (p *pipes) Addr() net.Addr { return &net.UnixAddr{Name: "pipes", Net: "unix"} }

// dial connects to the listener, or fails once it is closed.
func (p *pipes) dial() (io.ReadWriteCloser, error) {
	client, server := net.Pipe()
	select {
	case p.conns <- server:
		return client, nil
	case <-p.closed:
		return nil, errors.New("connection refused")
	}
}

var (
	grData   = spaces.HostData{Config: &config.Config{SessionsDir: "/sessions"}, Pending: []spaces.Pending{{Op: spaces.PendingCreate, Root: "/src/gr", Path: "/src/gr/new"}}}
	grStates = map[string]gitstate.State{"/src/gr": {Status: &gitstate.Status{Changes: 1}, PR: &gh.PR{Number: 7}}}
)

// A client loads from the server, and has it refresh; a subscriber hears of the loads that change the
// host data, whoever asked for them, until it stops.
func TestClientLoadsAndSubscribes(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := &fakeHost{data: grData}
		s := New(h, "v1", "laptop")
		ln := newPipes()
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		go s.Serve(ctx, ln)
		a, b := NewClient(ln.dial, "v1"), NewClient(ln.dial, "v1")

		var heard []spaces.HostData
		var mu sync.Mutex
		stop := a.Subscribe(func(hd spaces.HostData) {
			mu.Lock()
			defer mu.Unlock()
			heard = append(heard, hd)
		})
		synctest.Wait()
		got, err := b.Load(true)
		if err != nil || !reflect.DeepEqual(got, grData) {
			t.Fatalf("Load(true) = %+v, %v; want %+v", got, err, grData)
		}
		changed := grData
		changed.Git = grStates
		h.set(changed)
		if err := b.Refresh(); err != nil {
			t.Fatal(err)
		}
		if _, err := b.Load(true); err != nil {
			t.Fatal(err)
		}
		synctest.Wait()
		stop()
		h.set(grData)
		if _, err := b.Load(true); err != nil {
			t.Fatal(err)
		}
		synctest.Wait()
		hello, err := a.Hello()
		if err != nil {
			t.Fatal(err)
		}
		mu.Lock()
		defer mu.Unlock()
		// What subscribing answered with, then the two loads that changed it, the last after stop unheard.
		want := []spaces.HostData{grData, grData, changed}
		if !reflect.DeepEqual(heard, want) {
			t.Errorf("heard\n%+v\nwant\n%+v", heard, want)
		}
		if got, want := hello, (Hello{Protocol: Protocol, Version: "v1", Host: "laptop"}); got != want {
			t.Errorf("hello %+v, want %+v", got, want)
		}
		if h.refreshes != 1 {
			t.Errorf("%d refreshes, want 1", h.refreshes)
		}
	})
}

// A client that loses the server connects again at its next call, to the one started in its place, and
// subscribes again there.
func TestClientReconnects(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := &fakeHost{data: grData}
		// Loaded once already, so the load below changes nothing to push.
		h.Load(true)
		var ln *pipes
		var lnMu sync.Mutex
		serve := func(ctx context.Context) {
			lnMu.Lock()
			ln = newPipes()
			l := ln
			lnMu.Unlock()
			New(h, "v1", "laptop").Serve(ctx, l)
		}
		dial := func() (io.ReadWriteCloser, error) {
			lnMu.Lock()
			l := ln
			lnMu.Unlock()
			return l.dial()
		}
		ctx1, cancel1 := context.WithCancel(t.Context())
		go serve(ctx1)
		synctest.Wait()
		c := NewClient(dial, "v1")
		var heard int
		var mu sync.Mutex
		defer c.Subscribe(func(spaces.HostData) { mu.Lock(); heard++; mu.Unlock() })()
		synctest.Wait()
		cancel1()
		synctest.Wait()
		ctx2, cancel2 := context.WithCancel(t.Context())
		defer cancel2()
		go serve(ctx2)
		synctest.Wait()
		if _, err := c.Load(true); err != nil {
			t.Fatalf("Load after the server restarted: %v", err)
		}
		synctest.Wait()
		mu.Lock()
		defer mu.Unlock()
		// Subscribed on each server, each answering with the host data.
		if heard != 2 {
			t.Errorf("heard %d, want 2", heard)
		}
	})
}

// A server with no client exits after its idle time, and at once when a client asks it to.
func TestServerExits(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		s := New(&fakeHost{}, "v1", "laptop")
		ln := newPipes()
		done := make(chan error)
		start := time.Now()
		go func() { done <- s.Serve(t.Context(), ln) }()
		c := NewClient(ln.dial, "v1")
		if _, err := c.Load(false); err != nil {
			t.Fatal(err)
		}
		// Connected, it stays.
		time.Sleep(2 * Idle)
		select {
		case <-done:
			t.Fatal("exited with a client")
		default:
		}
		c.mu.Lock()
		cn := c.conn
		c.mu.Unlock()
		cn.close(errGone)
		if err := <-done; err != nil || time.Since(start) < 2*Idle {
			t.Errorf("exited %v after %s", err, time.Since(start))
		}

		s, ln = New(&fakeHost{}, "v1", "laptop"), newPipes()
		go func() { done <- s.Serve(t.Context(), ln) }()
		start = time.Now()
		if err := NewClient(ln.dial, "v1").Exit(); err != nil {
			t.Fatal(err)
		}
		if err := <-done; err != nil || time.Since(start) != 0 {
			t.Errorf("exited %v after %s", err, time.Since(start))
		}
	})
}

// fakeServer answers hello with protocol, and exit, closing every connection after it.
func fakeServer(t *testing.T, ln *pipes, protocol int, exited *int) {
	for {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		go func() {
			defer conn.Close()
			dec, enc := json.NewDecoder(conn), json.NewEncoder(conn)
			for {
				var msg message
				if err := dec.Decode(&msg); err != nil {
					return
				}
				switch msg.Method {
				case methodHello:
					res, _ := json.Marshal(Hello{Protocol: protocol, Version: "old"})
					enc.Encode(message{ID: msg.ID, Result: res})
				case methodExit:
					*exited++
					enc.Encode(message{ID: msg.ID, Result: json.RawMessage("{}")})
					ln.Close()
					return
				}
			}
		}()
	}
}

// A client replaces a server of an older protocol, when it may, and refuses one of a newer.
func TestClientProtocols(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		old := newPipes()
		exited := 0
		go fakeServer(t, old, Protocol-1, &exited)
		current := newPipes()
		defer current.Close()
		go New(&fakeHost{data: grData}, "v2", "laptop").Serve(t.Context(), current)
		// The old server until it exits, then the one started in its place.
		dial := func() (io.ReadWriteCloser, error) {
			if conn, err := old.dial(); err == nil {
				return conn, nil
			}
			return current.dial()
		}
		c := NewClient(dial, "v2")
		c.replace = func(_ *Client, cn *clientConn) error {
			if err := cn.call(methodExit, struct{}{}, nil); err != nil {
				return err
			}
			cn.close(errGone)
			// As waitUnlocked waits for the old server to exit.
			<-old.closed
			return nil
		}
		if got, err := c.Load(false); err != nil || !reflect.DeepEqual(got, grData) || exited != 1 {
			t.Errorf("Load from a replaced server = %+v, %v, exited %d", got, err, exited)
		}

		// Another host's, not the client's to replace.
		other := newPipes()
		defer other.Close()
		go fakeServer(t, other, Protocol-1, &exited)
		if _, err := NewClient(other.dial, "v2").Load(false); err == nil || exited != 1 {
			t.Errorf("Load from an older server not replaced = %v, exited %d", err, exited)
		}
		newer := newPipes()
		defer newer.Close()
		go fakeServer(t, newer, Protocol+1, &exited)
		if _, err := NewClient(newer.dial, "v2").Load(false); !errors.Is(err, ErrNewer) {
			t.Errorf("Load from a newer server = %v, want %v", err, ErrNewer)
		}
	})
}

// The socket's lock lets one server listen at a time, and the one listening goes when it is closed.
func TestListenOnce(t *testing.T) {
	path := filepath.Join(t.TempDir(), "s.sock")
	ln, unlock, err := Listen(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := Listen(path); !errors.Is(err, ErrRunning) {
		t.Errorf("second Listen = %v, want %v", err, ErrRunning)
	}
	ln.Close()
	unlock()
	ln, unlock, err = Listen(path)
	if err != nil {
		t.Fatalf("Listen after the first closed = %v", err)
	}
	ln.Close()
	unlock()
}

// The host data the server sends is the host data the client gets back, config and all.
func TestHostDataRoundTrip(t *testing.T) {
	cfg, err := config.Parse(`
spaces = [{ path = "/src/gr", group = "work", worktree_layout = "agent" }, "/src/kitty"]
groups = ["work"]
provider = ["kspaces-provider-cm"]
launch = ["launch", "--type=tab"]

[worktrees]
dir = ".worktrees"
branch_prefix = "pr/me"

[sidebar]
width = 30
popup = "panel"

[git]
timeout = "5s"

[[commands]]
name = "test"
run = ["go", "test", "{package}"]
inputs = [{ name = "package", default = "./..." }]

[[layouts]]
name = "agent"
[[layouts.tabs]]
title = "agent"
panes = [{ run = ["claude"] }, { split = "right", cwd = "{worktree}/docs" }]
`)
	if err != nil {
		t.Fatal(err)
	}
	hd := spaces.HostData{Config: &cfg, Pending: grData.Pending, Git: grStates}
	data, err := json.Marshal(hd)
	if err != nil {
		t.Fatal(err)
	}
	var got spaces.HostData
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, hd) {
		t.Errorf("got\n%#v\nwant\n%#v", *got.Config, cfg)
	}
}

// A client has the server's host create and remove worktrees, and hears why it could not.
func TestClientWorktrees(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := &fakeHost{data: grData}
		ln := newPipes()
		defer ln.Close()
		go New(h, "v1", "white").Serve(t.Context(), ln)
		c := NewClient(ln.dial, "v1")
		wt, err := c.NewWorktree("/src/gr", "dev", "main")
		if want := (spaces.Worktree{Path: "/src/gr/.worktrees/dev", Branch: "dev", Session: "gr@dev"}); err != nil || wt != want {
			t.Errorf("NewWorktree = %+v, %v; want %+v", wt, err, want)
		}
		if _, err := c.NewWorktree("/src/gr", "taken", ""); err == nil || err.Error() != "a branch named 'taken' already exists" {
			t.Errorf("NewWorktree of a taken branch = %v", err)
		}
		if err := c.RemoveWorktree("/src/gr/.worktrees/dev", spaces.RemoveOptions{Force: true, DeleteBranch: true}); err != nil {
			t.Fatal(err)
		}
		want := []string{"new /src/gr dev main", "new /src/gr taken ", "remove /src/gr/.worktrees/dev {Force:true DeleteBranch:true ForceBranch:false}"}
		if !reflect.DeepEqual(h.asked, want) {
			t.Errorf("asked %q, want %q", h.asked, want)
		}
	})
}

// A client saves a copy's layout on the server's host at the revision it last had, and loads those saved
// since; a save behind another is a conflict.
func TestClientLayouts(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := &fakeHost{data: grData, store: spaces.NewLocalHost("", "")}
		ln := newPipes()
		defer ln.Close()
		go New(h, "v1", "white").Serve(t.Context(), ln)
		a, b := NewClient(ln.dial, "v1"), NewClient(ln.dial, "v1")
		l := spaces.SharedLayout{Version: 1, Sessions: []string{"@a"}, Tabs: []spaces.SharedTab{{Panes: []spaces.SharedPane{{Session: 0}}}}}
		rev, err := a.SaveLayout("gr@dev", 0, l)
		if err != nil || rev != 1 {
			t.Fatalf("SaveLayout = %d, %v", rev, err)
		}
		if _, err := b.SaveLayout("gr@dev", 0, l); !errors.Is(err, spaces.ErrLayoutConflict) {
			t.Errorf("a save behind = %v, want %v", err, spaces.ErrLayoutConflict)
		}
		got, err := b.LoadLayouts([]spaces.CopyLayout{{Key: "gr@dev"}, {Key: "gr", Revision: 0}, {Key: "gr@old", Revision: 3}})
		want := []spaces.CopyLayout{{Key: "gr@dev", Revision: 1, Layout: &l}, {Key: "gr@old"}}
		if err != nil || !reflect.DeepEqual(got, want) {
			t.Errorf("LoadLayouts = %+v, %v; want %+v", got, err, want)
		}
	})
}
