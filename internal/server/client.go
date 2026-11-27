package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/chancez/kspaces/internal/logs"
	"github.com/chancez/kspaces/internal/spaces"
)

// callTimeout is how long a call waits for its answer: a load runs git for every space.
const callTimeout = time.Minute

// ErrNewer is a server of a newer protocol than the client's, as when kspaces was installed again under a
// sidebar still running the old one.
var ErrNewer = errors.New("the kspaces server is newer than this kspaces: restart kitty")

// Client is a spaces.Host that asks a server. It connects at the first call, and again at the next call
// after losing the server, subscribing again if it had. It is safe to call from several goroutines.
type Client struct {
	// dial connects to the server, starting it if need be.
	dial func() (io.ReadWriteCloser, error)
	// replace has the server of an older protocol replaced, given a connection to it: asks it to exit, and
	// waits until it has. Nil for a server that is not the client's to replace, as another host's.
	replace func(c *Client, cn *clientConn) error
	version string

	mu   sync.Mutex
	conn *clientConn
	// The subscribers, by id.
	subs   map[int]func(spaces.HostData)
	nextID int
}

// NewClient is a client of the server dial connects to, kspaces at version.
func NewClient(dial func() (io.ReadWriteCloser, error), version string) *Client {
	return &Client{dial: dial, version: version, subs: map[int]func(spaces.HostData){}}
}

// clientConn is one connection to the server, with the calls waiting on it.
type clientConn struct {
	rw    io.ReadWriteCloser
	encMu sync.Mutex
	enc   *json.Encoder
	mu    sync.Mutex
	calls map[int64]chan message
	next  int64
	// err is why the connection ended, once it has, and done closed then.
	err  error
	done chan struct{}
	// hello is what the server said it is.
	hello Hello
}

// Load is the host data, loaded now when fresh.
func (c *Client) Load(fresh bool) (spaces.HostData, error) {
	var hd spaces.HostData
	err := c.call(methodLoad, loadParams{Fresh: fresh}, &hd)
	return hd, err
}

// Refresh has git and gh asked again at the server's next load.
func (c *Client) Refresh() error {
	return c.call(methodRefresh, struct{}{}, nil)
}

// Exit asks the server to exit.
func (c *Client) Exit() error {
	return c.call(methodExit, struct{}{}, nil)
}

// Close hangs up, if connected. A call after connects again.
func (c *Client) Close() error {
	c.mu.Lock()
	cn := c.conn
	c.conn = nil
	c.mu.Unlock()
	if cn != nil {
		cn.close(errGone)
	}
	return nil
}

// Hello is what the server said it is, connecting if need be.
func (c *Client) Hello() (Hello, error) {
	cn, err := c.connected()
	if err != nil {
		return Hello{}, err
	}
	return cn.hello, nil
}

// Subscribe calls f with the host data whenever the server pushes it, from the next connection, made now
// in the background, or at once on the one there is.
func (c *Client) Subscribe(f func(spaces.HostData)) (stop func()) {
	c.mu.Lock()
	id := c.nextID
	c.nextID++
	c.subs[id] = f
	first := len(c.subs) == 1
	cn := c.conn
	c.mu.Unlock()
	go func() {
		if cn == nil {
			// Connecting subscribes.
			if _, err := c.connected(); err != nil {
				logs.Error("subscribing to the kspaces server", err)
			}
			return
		}
		if first {
			c.subscribe(cn)
		}
	}()
	return func() {
		c.mu.Lock()
		defer c.mu.Unlock()
		delete(c.subs, id)
	}
}

// subscribe asks the server on cn for its pushes, and gives the subscribers the host data it answers with.
func (c *Client) subscribe(cn *clientConn) {
	var hd spaces.HostData
	if err := cn.call(methodSubscribe, struct{}{}, &hd); err != nil {
		logs.Error("subscribing to the kspaces server", err)
		return
	}
	c.publish(hd)
}

func (c *Client) publish(hd spaces.HostData) {
	c.mu.Lock()
	subs := make([]func(spaces.HostData), 0, len(c.subs))
	for _, f := range c.subs {
		subs = append(subs, f)
	}
	c.mu.Unlock()
	for _, f := range subs {
		f(hd)
	}
}

// call asks the server for method, once again on a new connection when the one there was is gone.
func (c *Client) call(method string, params, result any) error {
	for try := 0; ; try++ {
		cn, err := c.connected()
		if err != nil {
			return err
		}
		err = cn.call(method, params, result)
		if !errors.Is(err, errGone) || try > 0 {
			return err
		}
		c.drop(cn)
	}
}

// drop forgets cn, so the next call connects again.
func (c *Client) drop(cn *clientConn) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn == cn {
		c.conn = nil
	}
}

// connected is the connection to the server, made now if there is none: said hello on, replacing a server
// of an older protocol, and subscribed on when there are subscribers.
func (c *Client) connected() (*clientConn, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn != nil {
		select {
		case <-c.conn.done:
			c.conn = nil
		default:
			return c.conn, nil
		}
	}
	cn, err := c.connect()
	if err != nil {
		return nil, err
	}
	if cn.hello.Protocol < Protocol && c.replace != nil {
		if err := c.replace(c, cn); err != nil {
			return nil, fmt.Errorf("replacing the kspaces server of protocol %d: %w", cn.hello.Protocol, err)
		}
		if cn, err = c.connect(); err != nil {
			return nil, err
		}
	}
	switch {
	case cn.hello.Protocol > Protocol:
		cn.close(ErrNewer)
		return nil, ErrNewer
	case cn.hello.Protocol < Protocol:
		cn.close(errGone)
		return nil, fmt.Errorf("the kspaces server speaks protocol %d, this kspaces %d", cn.hello.Protocol, Protocol)
	}
	c.conn = cn
	if len(c.subs) > 0 {
		go c.subscribe(cn)
	}
	return cn, nil
}

// connect dials the server and says hello. c.mu is held.
func (c *Client) connect() (*clientConn, error) {
	rw, err := c.dial()
	if err != nil {
		return nil, err
	}
	cn := &clientConn{rw: rw, enc: json.NewEncoder(rw), calls: map[int64]chan message{}, done: make(chan struct{})}
	go cn.read(c)
	if err := cn.call(methodHello, Hello{Protocol: Protocol, Version: c.version}, &cn.hello); err != nil {
		cn.close(err)
		return nil, err
	}
	return cn, nil
}

// errGone is a connection that ended.
var errGone = errors.New("the kspaces server hung up")

// read takes the server's messages off cn until it ends: the answers to the calls waiting, and the pushes,
// for c's subscribers.
func (cn *clientConn) read(c *Client) {
	dec := json.NewDecoder(cn.rw)
	for {
		var msg message
		if err := dec.Decode(&msg); err != nil {
			cn.close(fmt.Errorf("%w: %w", errGone, err))
			return
		}
		if msg.ID == 0 {
			if msg.Method == methodUpdate {
				var hd spaces.HostData
				if err := json.Unmarshal(msg.Params, &hd); err == nil {
					c.publish(hd)
				}
			}
			continue
		}
		cn.mu.Lock()
		ch := cn.calls[msg.ID]
		delete(cn.calls, msg.ID)
		cn.mu.Unlock()
		if ch != nil {
			ch <- msg
		}
	}
}

// close ends cn with err, failing the calls waiting on it.
func (cn *clientConn) close(err error) {
	cn.mu.Lock()
	defer cn.mu.Unlock()
	if cn.err != nil {
		return
	}
	cn.err = err
	close(cn.done)
	cn.rw.Close()
}

// call sends method and waits for its answer, decoding it into result unless that is nil.
func (cn *clientConn) call(method string, params, result any) error {
	raw, err := json.Marshal(params)
	if err != nil {
		return err
	}
	ch := make(chan message, 1)
	cn.mu.Lock()
	if cn.err != nil {
		cn.mu.Unlock()
		return cn.err
	}
	cn.next++
	id := cn.next
	cn.calls[id] = ch
	cn.mu.Unlock()
	cn.encMu.Lock()
	err = cn.enc.Encode(message{ID: id, Method: method, Params: raw})
	cn.encMu.Unlock()
	if err != nil {
		cn.close(fmt.Errorf("%w: %w", errGone, err))
		return cn.err
	}
	timer := time.NewTimer(callTimeout)
	defer timer.Stop()
	answer := func(msg message) error {
		if msg.Error != "" {
			return errors.New(msg.Error)
		}
		if result == nil {
			return nil
		}
		return json.Unmarshal(msg.Result, result)
	}
	select {
	case msg := <-ch:
		return answer(msg)
	case <-cn.done:
		// The answer came before the server hung up, as it does after answering exit.
		select {
		case msg := <-ch:
			return answer(msg)
		default:
			return cn.err
		}
	case <-timer.C:
		cn.mu.Lock()
		delete(cn.calls, id)
		cn.mu.Unlock()
		return fmt.Errorf("the kspaces server did not answer %s in %s", method, callTimeout)
	}
}
