package api

import (
	"code.google.com/p/go.net/websocket"
	"crypto/tls"
	"launchpad.net/juju-core/log"
	"launchpad.net/juju-core/state"
	"net"
	"net/http"
)

// srvState represents a single client's connection to the state.
type srvState struct {
	state *state.State
	conn  *websocket.Conn
}

type Server struct {
	state *state.State
}

// Serve serves the given state by accepting requests
// on the given listener, using the given certificate
// and key (in PEM format) for authentication. 
func Serve(s *state.State, lis net.Listener, cert, key []byte) error {
	tlsCert, err := tls.X509KeyPair(cert, key)
	if err != nil {
		return err
	}
	lis = tls.NewListener(lis, &tls.Config{
		Certificates: []tls.Certificate{tlsCert},
	})
	return http.Serve(lis, newHandler(s))
}

// newHandler returns an http handler that serves the API
// interface to the given state as a websocket.
func newHandler(s *state.State) http.Handler {
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
