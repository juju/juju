package api

import (
	"code.google.com/p/go.net/websocket"
	"crypto/tls"
	"launchpad.net/juju-core/log"
	"launchpad.net/juju-core/state"
	"launchpad.net/tomb"
	"net"
	"net/http"
	"sync"
)

// srvState represents a single client's connection to the state.
type srvState struct {
	srv  *Server
	conn *websocket.Conn
}

type Server struct {
	tomb  tomb.Tomb
	wg    sync.WaitGroup
	state *state.State
	addr  net.Addr
}

// Serve serves the given state by accepting requests
// on the given listener, using the given certificate
// and key (in PEM format) for authentication. 
func NewServer(s *state.State, addr string, cert, key []byte) (*Server, error) {
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, err
	}
	tlsCert, err := tls.X509KeyPair(cert, key)
	if err != nil {
		return nil, err
	}
	srv := &Server{
		state: s,
		addr:  lis.Addr(),
	}
	lis = tls.NewListener(lis, &tls.Config{
		Certificates: []tls.Certificate{tlsCert},
	})
	go func() {
		defer srv.tomb.Done()
		srv.tomb.Kill(srv.run(lis))
	}()
	return srv, nil
}

// Stop stops the server and returns when all requests that
// it is running have completed.
func (srv *Server) Stop() error {
	srv.tomb.Kill(nil)
	return srv.tomb.Wait()
}

func (srv *Server) run(lis net.Listener) error {
	srv.wg.Add(1)
	go func() {
		<-srv.tomb.Dying()
		lis.Close()
		srv.wg.Done()
	}()
	handler := websocket.Handler(func(conn *websocket.Conn) {
		srv.wg.Add(1)
		defer srv.wg.Done()
		// If we've got to this stage and the tomb is still
		// alive, we know that any tomb.Kill must occur after we
		// have called wg.Add, so we avoid the possibility of a
		// handler goroutine running after Stop has returned.
		if srv.tomb.Err() != tomb.ErrStillAlive {
			return
		}
		st := &srvState{
			srv:  srv,
			conn: conn,
		}
		srv.wg.Add(1)
		go func() {
			st.run()
			srv.wg.Done()
		}()
		<-srv.tomb.Dying()
		conn.Close()
	})
	// The error from http.Serve is not interesting.
	http.Serve(lis, handler)
	return nil
}

// Addr returns the address that the server is listening on.
func (srv *Server) Addr() string {
	return srv.addr.String()
}

type rpcRequest struct {
	Request string // placeholder only
}

type rpcResponse struct {
	Response string // placeholder only
	Error    string
}

func (st *srvState) run() {
	for {
		var req rpcRequest
		err := websocket.JSON.Receive(st.conn, &req)
		if err != nil {
			log.Printf("api: error receiving request: %v", err)
			return
		}
		var resp rpcResponse
		// placeholder for executing some arbitrary operation
		// on state.
		m, err := st.srv.state.Machine(req.Request)
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
		err = websocket.JSON.Send(st.conn, &resp)
		if err != nil {
			log.Printf("api: error sending reply: %v", err)
			return
		}
	}
}
