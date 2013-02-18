package api

import (
	"code.google.com/p/go.net/websocket"
	"crypto/tls"
	"encoding/json"
	"io"
	"launchpad.net/juju-core/log"
	"launchpad.net/juju-core/rpc"
	"launchpad.net/juju-core/state"
	"launchpad.net/tomb"
	"net"
	"net/http"
	"sync"
)

type Server struct {
	tomb   tomb.Tomb
	wg     sync.WaitGroup
	state  *state.State
	addr   net.Addr
	rpcSrv *rpc.Server
}

// Serve serves the given state by accepting requests
// on the given listener, using the given certificate
// and key (in PEM format) for authentication.
func NewServer(s *state.State, addr string, cert, key []byte) (*Server, error) {
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, err
	}
	log.Printf("state/api: listening on %q", addr)
	tlsCert, err := tls.X509KeyPair(cert, key)
	if err != nil {
		return nil, err
	}
	srv := &Server{
		state: s,
		addr:  lis.Addr(),
	}
	srv.rpcSrv, err = rpc.NewServer(&srvRoot{}, serverError)
	lis = tls.NewListener(lis, &tls.Config{
		Certificates: []tls.Certificate{tlsCert},
	})
	go srv.run(lis)
	return srv, nil
}

// Dead returns a channel that signals when the server has exited.
func (srv *Server) Dead() <-chan struct{} {
	return srv.tomb.Dead()
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
		if err := srv.serveConn(conn); err != nil {
			log.Printf("state/api: error serving RPCs: %v", err)
		}
	})
	// The error from http.Serve is not interesting.
	http.Serve(lis, handler)
}

// Addr returns the address that the server is listening on.
func (srv *Server) Addr() string {
	return srv.addr.String()
}

func (srv *Server) serveConn(conn *websocket.Conn) error {
	msgs := make(chan serverReq)
	go readRequests(conn, msgs)
	defer func() {
		conn.Close()
		// Wait for readRequests to see the closed connection and quit.
		for _ = range msgs {
		}
	}()
	return srv.rpcSrv.ServeCodec(&serverCodec{
		srv:  srv,
		conn: conn,
		msgs: msgs,
	}, newStateServer(srv, conn))
}

func readRequests(conn *websocket.Conn, c chan<- serverReq) {
	defer close(c)
	var req serverReq
	for {
		req = serverReq{} // avoid any potential cross-message contamination.
		err := websocket.JSON.Receive(conn, &req)
		if err == io.EOF {
			break
		}
		if err != nil {
			log.Printf("api: error receiving request: %v", err)
			break
		}
		c <- req
	}
}

type serverReq struct {
	RequestId uint64
	Type      string
	Id        string
	Request   string
	Params    json.RawMessage
}

type serverResp struct {
	RequestId uint64
	Error     string      `json:",omitempty"`
	ErrorCode string 	`json:",omitempty"`
	Response  interface{} `json:",omitempty"`
}

type serverCodec struct {
	srv  *Server
	conn *websocket.Conn
	msgs <-chan serverReq
	req  serverReq
}

func (c *serverCodec) ReadRequestHeader(req *rpc.Request) error {
	var ok bool
	select {
	case c.req, ok = <-c.msgs:
		if !ok {
			return io.EOF
		}
	case <-c.srv.tomb.Dying():
		return io.EOF
	}
	req.RequestId = c.req.RequestId
	req.Type = c.req.Type
	req.Id = c.req.Id
	req.Request = c.req.Request
	return nil
}

func (c *serverCodec) ReadRequestBody(body interface{}) error {
	if body == nil {
		return nil
	}
	return json.Unmarshal(c.req.Params, body)
}

func (c *serverCodec) WriteResponse(resp *rpc.Response, body interface{}) error {
	return websocket.JSON.Send(c.conn, &serverResp{
		RequestId: resp.RequestId,
		Error:     resp.Error,
		ErrorCode: resp.ErrorCode,
		Response:  body,
	})
}
