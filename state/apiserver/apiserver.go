// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"code.google.com/p/go.net/websocket"
	"github.com/juju/loggo"
	"launchpad.net/tomb"

	"launchpad.net/juju-core/rpc"
	"launchpad.net/juju-core/rpc/jsoncodec"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/apiserver/common"
	"launchpad.net/juju-core/utils"
)

var logger = loggo.GetLogger("juju.state.apiserver")

// loginRateLimit defines how many concurrent Login requests we will
// accept
const loginRateLimit = 10

// Server holds the server side of the API.
type Server struct {
	tomb    tomb.Tomb
	wg      sync.WaitGroup
	state   *state.State
	addr    net.Addr
	dataDir string
	logDir  string
	limiter utils.Limiter
}

// NewServer serves the given state by accepting requests on the given
// listener, using the given certificate and key (in PEM format) for
// authentication.
func NewServer(s *state.State, addr string, cert, key []byte, datadir, logDir string) (*Server, error) {
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
		state:   s,
		addr:    lis.Addr(),
		dataDir: datadir,
		logDir:  logDir,
		limiter: utils.NewLimiter(loginRateLimit),
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
	id    int64
	start time.Time

	mu   sync.Mutex
	tag_ string
}

var globalCounter int64

func newRequestNotifier() *requestNotifier {
	return &requestNotifier{
		id:    atomic.AddInt64(&globalCounter, 1),
		tag_:  "<unknown>",
		start: time.Now(),
	}
}

func (n *requestNotifier) login(tag string) {
	n.mu.Lock()
	n.tag_ = tag
	n.mu.Unlock()
}

func (n *requestNotifier) tag() (tag string) {
	n.mu.Lock()
	tag = n.tag_
	n.mu.Unlock()
	return
}

func (n *requestNotifier) ServerRequest(hdr *rpc.Header, body interface{}) {
	if hdr.Request.Type == "Pinger" && hdr.Request.Action == "Ping" {
		return
	}
	// TODO(rog) 2013-10-11 remove secrets from some requests.
	logger.Debugf("<- [%X] %s %s", n.id, n.tag(), jsoncodec.DumpRequest(hdr, body))
}

func (n *requestNotifier) ServerReply(req rpc.Request, hdr *rpc.Header, body interface{}, timeSpent time.Duration) {
	if req.Type == "Pinger" && req.Action == "Ping" {
		return
	}
	logger.Debugf("-> [%X] %s %s %s %s[%q].%s", n.id, n.tag(), timeSpent, jsoncodec.DumpRequest(hdr, body), req.Type, req.Id, req.Action)
}

func (n *requestNotifier) join(req *http.Request) {
	logger.Infof("[%X] API connection from %s", n.id, req.RemoteAddr)
}

func (n *requestNotifier) leave() {
	logger.Infof("[%X] %s API connection terminated after %v", n.id, n.tag(), time.Since(n.start))
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
	srv.wg.Add(1)
	go func() {
		err := srv.mongoPinger()
		srv.tomb.Kill(err)
		srv.wg.Done()
	}()
	mux := http.NewServeMux()
	mux.HandleFunc("/", srv.apiHandler)
	mux.Handle("/log",
		&debugLogHandler{
			httpHandler: httpHandler{state: srv.state},
			logDir:      srv.logDir})
	mux.Handle("/charms",
		&charmsHandler{
			httpHandler: httpHandler{state: srv.state},
			dataDir:     srv.dataDir})
	mux.Handle("/tools",
		&toolsHandler{httpHandler{state: srv.state}})
	// The error from http.Serve is not interesting.
	http.Serve(lis, mux)
}

func (srv *Server) apiHandler(w http.ResponseWriter, req *http.Request) {
	reqNotifier := newRequestNotifier()
	reqNotifier.join(req)
	defer reqNotifier.leave()
	wsServer := websocket.Server{
		Handler: func(conn *websocket.Conn) {
			srv.wg.Add(1)
			defer srv.wg.Done()
			// If we've got to this stage and the tomb is still
			// alive, we know that any tomb.Kill must occur after we
			// have called wg.Add, so we avoid the possibility of a
			// handler goroutine running after Stop has returned.
			if srv.tomb.Err() != tomb.ErrStillAlive {
				return
			}
			if err := srv.serveConn(conn, reqNotifier); err != nil {
				logger.Errorf("error serving RPCs: %v", err)
			}
		},
	}
	wsServer.ServeHTTP(w, req)
}

// Addr returns the address that the server is listening on.
func (srv *Server) Addr() string {
	return srv.addr.String()
}

func (srv *Server) serveConn(wsConn *websocket.Conn, reqNotifier *requestNotifier) error {
	codec := jsoncodec.NewWebsocket(wsConn)
	if loggo.GetLogger("juju.rpc.jsoncodec").EffectiveLogLevel() <= loggo.TRACE {
		codec.SetLogging(true)
	}
	var notifier rpc.RequestNotifier
	if logger.EffectiveLogLevel() <= loggo.DEBUG {
		// Incur request monitoring overhead only if we
		// know we'll need it.
		notifier = reqNotifier
	}
	conn := rpc.NewConn(codec, notifier)
	conn.Serve(newStateServer(srv, conn, reqNotifier, srv.limiter), serverError)
	conn.Start()
	select {
	case <-conn.Dead():
	case <-srv.tomb.Dying():
	}
	return conn.Close()
}

func (srv *Server) mongoPinger() error {
	timer := time.NewTimer(0)
	session := srv.state.MongoSession()
	for {
		select {
		case <-timer.C:
		case <-srv.tomb.Dying():
			return tomb.ErrDying
		}
		if err := session.Ping(); err != nil {
			logger.Infof("got error pinging mongo: %v", err)
			return fmt.Errorf("error pinging mongo: %v", err)
		}
		timer.Reset(mongoPingInterval)
	}
}

func serverError(err error) error {
	if err := common.ServerError(err); err != nil {
		return err
	}
	return nil
}

var logRequests = true
