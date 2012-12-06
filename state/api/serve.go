package api

import (
	"code.google.com/p/go.net/websocket"
	"launchpad.net/juju-core/log"
	"launchpad.net/juju-core/state"
	"net/http"
)

// srvState represents a single client's connection to the state.
type srvState struct {
	state *state.State
	conn  *websocket.Conn
}

// NewHandler returns an http handler that serves the API
// interface to the given state as a websocket.
func NewHandler(s *state.State) http.Handler {
	return websocket.Handler(func(conn *websocket.Conn) {
		srv := &srvState{
			state: s,
			conn:  conn,
		}
		srv.run()
	})
}

type rpcRequest struct {
	Request string // placeholder only
}

type rpcResponse struct {
	Response string // placeholder only
	Error    string
}

func (srv *srvState) run() {
	for {
		var req rpcRequest
		err := websocket.JSON.Receive(srv.conn, &req)
		if err != nil {
			log.Printf("api: error receiving request: %v", err)
			return
		}
		var resp rpcResponse
		// placeholder for executing some arbitrary operation
		// on state.
		m, err := srv.state.Machine(req.Request)
		if err != nil {
			resp.Error = err.Error()
		} else {
			instId, err := m.InstanceId()
			if err != nil {
				resp.Error = err.Error()
			} else {
				resp.Response = string(instId)
			}
		}
		err = websocket.JSON.Send(srv.conn, &resp)
		if err != nil {
			log.Printf("api: error sending reply: %v", err)
			return
		}
	}
}
