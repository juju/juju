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
}

// Serve serves the given state by accepting requests
// on the given listener, using the given certificate
// and key (in PEM format) for authentication. 
func NewServer(s *state.State, addr string, cert, key []byte) *Server {
	srv := &Server{
		state: s,
	}
	go func() {
		defer srv.tomb.Done()
		srv.tomb.Kill(srv.run(addr, cert, key))
	}()
	return srv
}

func (srv *Server) Stop() error {
	srv.tomb.Kill(nil)
	return srv.tomb.Wait()
}

func (srv *Server) run(addr string, cert, key []byte) error {
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	srv.wg.Add(1)
	go func() {
		<-srv.tomb.Dying()
		lis.Close()
		srv.wg.Done()
	}()
	tlsCert, err := tls.X509KeyPair(cert, key)
	if err != nil {
		return err
	}
	lis = tls.NewListener(lis, &tls.Config{
		Certificates: []tls.Certificate{tlsCert},
	})
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
			log.Printf("run finishing")
		}()
		<-srv.tomb.Dying()
		log.Printf("tomb dying, closing conn")
		conn.Close()
	})
	http.Serve(lis, handler)
	// The error from http.Serve is not interesting.
	return nil
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
