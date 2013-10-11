// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"crypto/tls"
	"net"
	"net/http"
	"sync"

	"code.google.com/p/go.net/websocket"
	"launchpad.net/loggo"
	"launchpad.net/tomb"

	"launchpad.net/juju-core/rpc"
	"launchpad.net/juju-core/rpc/jsoncodec"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/apiserver/common"
)

var logger = loggo.GetLogger("juju.state.apiserver")

// Server holds the server side of the API.
type Server struct {
	tomb  tomb.Tomb
	wg    sync.WaitGroup
	state *state.State
	addr  net.Addr
}

// Serve serves the given state by accepting requests on the given
// listener, using the given certificate and key (in PEM format) for
// authentication.
func NewServer(s *state.State, addr string, cert, key []byte) (*Server, error) {
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, err
	}
	logger.Infof("listening on %q", lis.Addr())
	tlsCert, err := tls.X509KeyPair(cert, key)
	if err != nil {
		return nil, err
	}
	srv := &Server{
		state: s,
		addr:  lis.Addr(),
	}
	// TODO(rog) check that *srvRoot is a valid type for using
	// as an RPC server.
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

// Stop stops the server and returns when all running requests
// have completed.
func (srv *Server) Stop() error {
	srv.tomb.Kill(nil)
	return srv.tomb.Wait()
}

// Kill implements worker.Worker.Kill.
func (srv *Server) Kill() {
	srv.tomb.Kill(nil)
}

// Wait implements worker.Worker.Wait.
func (srv *Server) Wait() error {
	return srv.tomb.Wait()
}

type requestNotifier struct {
	remoteAddr net.Addr
}

func (n requestNotifier) ServerRequest(hdr *rpc.Header, body interface{}) {
	if hdr.Request.Type == "Pinger" && hdr.Request.Action == "Ping" {
		return
	}
	// TODO(rog) 2013-10-11 remove secrets from some requests.
	logger.Debugf("<- %s", jsoncodec.DumpRequest(hdr, body))
}

func (n requestNotifier) ServerReply(req rpc.Request, hdr *rpc.Header, body interface{}) {
	if req.Type == "Pinger" && req.Action == "Ping" {
		return
	}
	logger.Debugf("<- %s %s[%q].%s", jsoncodec.DumpRequest(hdr, body), req.Type, req.Id, req.Action)
}

func (n requestNotifier) ClientRequest(hdr *rpc.Header, body interface{}) {
}

func (n requestNotifier) ClientReply(req rpc.Request, hdr *rpc.Header, body interface{}) {
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
			logger.Errorf("error serving RPCs: %v", err)
		}
	})
	// The error from http.Serve is not interesting.
	http.Serve(lis, handler)
}

// Addr returns the address that the server is listening on.
func (srv *Server) Addr() string {
	return srv.addr.String()
}

func (srv *Server) serveConn(wsConn *websocket.Conn) error {
	codec := jsoncodec.NewWebsocket(wsConn)
	if loggo.GetLogger("juju.rpc.jsoncodec").EffectiveLogLevel() <= loggo.TRACE {
		codec.SetLogging(true)
	}
	var notifier rpc.RequestNotifier
	if logger.EffectiveLogLevel() <= loggo.DEBUG {
		notifier = requestNotifier{wsConn.RemoteAddr()}
	}
	conn := rpc.NewConn(codec, notifier)
	conn.Serve(newStateServer(srv, conn), serverError)
	conn.Start()
	select {
	case <-conn.Dead():
	case <-srv.tomb.Dying():
	}
	return conn.Close()
}

func serverError(err error) error {
	if err := common.ServerError(err); err != nil {
		return err
	}
	return nil
}

var logRequests = true
