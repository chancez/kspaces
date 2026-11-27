// Package server is kspaces' host server, see docs/server.md: it serves a host's data, spaces.HostData, to
// kspaces' processes on the host, over a Unix socket, and to other machines, over ssh.
package server

import "encoding/json"

// Protocol is the version of what is said over the socket. A client of a newer one replaces an older
// server; a client of an older one refuses a newer server, see Client.
const Protocol = 1

// message is one line of the protocol: a request, with an id and a method, its response, with the id and
// a result or an error, or a push, with a method and no id.
type message struct {
	ID     int64           `json:"id,omitempty"`
	Method string          `json:"method,omitempty"`
	Params json.RawMessage `json:"params,omitempty"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  string          `json:"error,omitempty"`
}

// The methods.
const (
	methodHello     = "hello"
	methodLoad      = "load"
	methodRefresh   = "refresh"
	methodSubscribe = "subscribe"
	methodExit      = "exit"
	// methodUpdate is the push of the host data to a client that subscribed.
	methodUpdate = "update"
)

// Hello is what each side says first: its protocol and kspaces' version, and the server its host's name.
type Hello struct {
	Protocol int    `json:"protocol"`
	Version  string `json:"version"`
	Host     string `json:"host,omitempty"`
}

// loadParams are a load's: whether to load now rather than give the last.
type loadParams struct {
	Fresh bool `json:"fresh,omitempty"`
}
