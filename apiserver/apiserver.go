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
	"github.com/bmizerany/pat"
	"github.com/juju/loggo"
	"github.com/juju/utils"
	"launchpad.net/tomb"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/rpc"
	"github.com/juju/juju/rpc/jsoncodec"
	"github.com/juju/juju/state"
)

var logger = loggo.GetLogger("juju.apiserver")

// loginRateLimit defines how many concurrent Login requests we will
// accept
const loginRateLimit = 10

// Server holds the server side of the API.
type Server struct {
	tomb              tomb.Tomb
	wg                sync.WaitGroup
	state             *state.State
	addr              string
	dataDir           string
	logDir            string
	limiter           utils.Limiter
	validator         LoginValidator
	adminApiFactories map[int]adminApiFactory

	mu          sync.Mutex // protects the fields that follow
	environUUID string
}

// LoginValidator functions are used to decide whether login requests
// are to be allowed. The validator is called before credentials are
// checked.
type LoginValidator func(params.LoginRequest) error
type RestoreContext interface {
	PrepareRestore() error
	BeginRestore() error
	FinishRestore() error
}

// ServerConfig holds parameters required to set up an API server.
type ServerConfig struct {
	Cert           []byte
	Key            []byte
	DataDir        string
	LogDir         string
	Validator      LoginValidator
	RestoreContext RestoreContext
}

// NewServer serves the given state by accepting requests on the given
// listener, using the given certificate and key (in PEM format) for
// authentication.
func NewServer(s *state.State, lis net.Listener, cfg ServerConfig) (*Server, error) {
	logger.Infof("listening on %q", lis.Addr())
	tlsCert, err := tls.X509KeyPair(cfg.Cert, cfg.Key)
	if err != nil {
		return nil, err
	}
	_, listeningPort, err := net.SplitHostPort(lis.Addr().String())
	if err != nil {
		return nil, err
	}
	srv := &Server{
		state:     s,
		addr:      net.JoinHostPort("localhost", listeningPort),
		dataDir:   cfg.DataDir,
		logDir:    cfg.LogDir,
		limiter:   utils.NewLimiter(loginRateLimit),
		validator: cfg.Validator,
		adminApiFactories: map[int]adminApiFactory{
			0: newAdminApiV0,
			1: newAdminApiV1,
		},
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

func (n *requestNotifier) ClientRequest(hdr *rpc.Header, body interface{}) {
}

func (n *requestNotifier) ClientReply(req rpc.Request, hdr *rpc.Header, body interface{}) {
}

func handleAll(mux *pat.PatternServeMux, pattern string, handler http.Handler) {
	mux.Get(pattern, handler)
	mux.Post(pattern, handler)
	mux.Head(pattern, handler)
	mux.Put(pattern, handler)
	mux.Del(pattern, handler)
	mux.Options(pattern, handler)
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
	// for pat based handlers, they are matched in-order of being
	// registered, first match wins. So more specific ones have to be
	// registered first.
	mux := pat.New()
	// For backwards compatibility we register all the old paths
	handleAll(mux, "/environment/:envuuid/log",
		&debugLogHandler{
			httpHandler: httpHandler{state: srv.state},
			logDir:      srv.logDir},
	)
	handleAll(mux, "/environment/:envuuid/charms",
		&charmsHandler{
			httpHandler: httpHandler{state: srv.state},
			dataDir:     srv.dataDir},
	)
	// TODO: We can switch from handleAll to mux.Post/Get/etc for entries
	// where we only want to support specific request methods. However, our
	// tests currently assert that errors come back as application/json and
	// pat only does "text/plain" responses.
	handleAll(mux, "/environment/:envuuid/tools",
		&toolsUploadHandler{toolsHandler{
			httpHandler{state: srv.state},
		}},
	)
	handleAll(mux, "/environment/:envuuid/tools/:version",
		&toolsDownloadHandler{toolsHandler{
			httpHandler{state: srv.state},
		}},
	)
	handleAll(mux, "/environment/:envuuid/backups",
		&backupHandler{httpHandler{state: srv.state}},
	)
	handleAll(mux, "/environment/:envuuid/api", http.HandlerFunc(srv.apiHandler))
	// For backwards compatibility we register all the old paths
	handleAll(mux, "/log",
		&debugLogHandler{
			httpHandler: httpHandler{state: srv.state},
			logDir:      srv.logDir},
	)
	handleAll(mux, "/charms",
		&charmsHandler{
			httpHandler: httpHandler{state: srv.state},
			dataDir:     srv.dataDir},
	)
	handleAll(mux, "/tools",
		&toolsUploadHandler{toolsHandler{
			httpHandler{state: srv.state},
		}},
	)
	handleAll(mux, "/tools/:version",
		&toolsDownloadHandler{toolsHandler{
			httpHandler{state: srv.state},
		}},
	)
	handleAll(mux, "/", http.HandlerFunc(srv.apiHandler))
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
			envUUID := req.URL.Query().Get(":envuuid")
			logger.Tracef("got a request for env %q", envUUID)
			if err := srv.serveConn(conn, reqNotifier, envUUID); err != nil {
				logger.Errorf("error serving RPCs: %v", err)
			}
		},
	}
	wsServer.ServeHTTP(w, req)
}

// Addr returns the address that the server is listening on.
func (srv *Server) Addr() string {
	return srv.addr
}

func (srv *Server) validateEnvironUUID(envUUID string) error {
	if envUUID == "" {
		// We allow the environUUID to be empty for 2 cases
		// 1) Compatibility with older clients
		// 2) On first connect. The environment UUID is currently
		//    generated by 'jujud bootstrap-state', and we haven't
		//    threaded that information all the way back to the 'juju
		//    bootstrap' process to be able to cache the value until
		//    after we've connected one time.
		return nil
	}
	if srv.getEnvironUUID() == "" {
		env, err := srv.state.Environment()
		if err != nil {
			return err
		}
		srv.setEnvironUUID(env.UUID())
	}
	return srv.checkEnvironUUID(envUUID)
}

// checkEnvironUUID checks if the expected envionUUID matches the
// current environUUID set on this Server. It returns nil for a match
// and an error on mismatch.
func (srv *Server) checkEnvironUUID(expected string) error {
	actual := srv.getEnvironUUID()
	if actual != expected {
		return common.UnknownEnvironmentError(expected)
	}
	return nil
}

func (srv *Server) getEnvironUUID() string {
	srv.mu.Lock()
	defer srv.mu.Unlock()
	return srv.environUUID
}

func (srv *Server) setEnvironUUID(uuid string) {
	srv.mu.Lock()
	defer srv.mu.Unlock()
	srv.environUUID = uuid
}

func (srv *Server) serveConn(wsConn *websocket.Conn, reqNotifier *requestNotifier, envUUID string) error {
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

	var err error
	var h *apiHandler
	if err = srv.validateEnvironUUID(envUUID); err == nil {
		h, err = newApiHandler(srv, conn, reqNotifier)
	}
	if err != nil {
		conn.Serve(&errRoot{err}, serverError)
	} else {
		adminApis := make(map[int]interface{})
		for apiVersion, factory := range srv.adminApiFactories {
			adminApis[apiVersion] = factory(srv, h, reqNotifier)
		}
		conn.ServeFinder(newAnonRoot(h, adminApis), serverError)
	}
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
