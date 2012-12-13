package api

import (
	"code.google.com/p/go.net/websocket"
	"crypto/tls"
	"io"
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
	go srv.run(lis)
	return srv, nil
}

// Stop stops the server and returns when all requests that
// it is running have completed.
func (srv *Server) Stop() error {
	srv.tomb.Kill(nil)
	return srv.tomb.Wait()
}

func (srv *Server) run(lis net.Listener) {
	defer srv.tomb.Done()
	defer srv.wg.Wait() // wait for any outstanding requests to complete.
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
		st.run()
	})
	// The error from http.Serve is not interesting.
	http.Serve(lis, handler)
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
	msgs := make(chan rpcRequest)
	go st.readRequests(msgs)
	defer func() {
		st.conn.Close()
		// Wait for readRequests to see the closed connection and quit.
		for _ = range msgs {
		}
	}()
	for {
		var req rpcRequest
		var ok bool
		select {
		case req, ok = <-msgs:
			if !ok {
				return
			}
		case <-st.srv.tomb.Dying():
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

func (st *srvState) readRequests(c chan<- rpcRequest) {
	var req rpcRequest
	for {
		req = rpcRequest{} // avoid any potential cross-message contamination.
		err := websocket.JSON.Receive(st.conn, &req)
		if err == io.EOF {
			break
		}
		if err != nil {
			log.Printf("api: error receiving request: %v", err)
			break
		}
		c <- req
	}
	close(c)
}
