// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"crypto/tls"
	"crypto/x509"
	"net"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/bmizerany/pat"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/utils"
	"golang.org/x/net/websocket"
	"gopkg.in/juju/names.v2"
	"gopkg.in/tomb.v1"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/common/apihttp"
	"github.com/juju/juju/apiserver/observer"
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
	statePool         *state.StatePool
	lis               net.Listener
	tag               names.Tag
	dataDir           string
	logDir            string
	limiter           utils.Limiter
	validator         LoginValidator
	adminAPIFactories map[int]adminAPIFactory
	modelUUID         string
	authCtxt          *authContext
	lastConnectionID  uint64
	newObserver       observer.ObserverFactory
	connCount         int64
}

// LoginValidator functions are used to decide whether login requests
// are to be allowed. The validator is called before credentials are
// checked.
type LoginValidator func(params.LoginRequest) error

// ServerConfig holds parameters required to set up an API server.
type ServerConfig struct {
	Cert        []byte
	Key         []byte
	Tag         names.Tag
	DataDir     string
	LogDir      string
	Validator   LoginValidator
	CertChanged chan params.StateServingInfo

	// NewObserver is a function which will return an observer. This
	// is used per-connection to instantiate a new observer to be
	// notified of key events during API requests.
	NewObserver observer.ObserverFactory

	// StatePool only exists to support testing.
	StatePool *state.StatePool
}

func (c *ServerConfig) Validate() error {
	if c.NewObserver == nil {
		return errors.NotValidf("missing NewObserver")
	}

	return nil
}

// changeCertListener wraps a TLS net.Listener.
// It allows connection handshakes to be
// blocked while the TLS certificate is updated.
type changeCertListener struct {
	net.Listener
	tomb tomb.Tomb

	// A mutex used to block accept operations.
	m sync.Mutex

	// A channel used to pass in new certificate information.
	certChanged <-chan params.StateServingInfo

	// The config to update with any new certificate.
	config *tls.Config
}

func newChangeCertListener(lis net.Listener, certChanged <-chan params.StateServingInfo, config *tls.Config) *changeCertListener {
	cl := &changeCertListener{
		Listener:    lis,
		certChanged: certChanged,
		config:      config,
	}
	go func() {
		defer cl.tomb.Done()
		cl.tomb.Kill(cl.processCertChanges())
	}()
	return cl
}

// Accept waits for and returns the next connection to the listener.
func (cl *changeCertListener) Accept() (net.Conn, error) {
	conn, err := cl.Listener.Accept()
	if err != nil {
		return nil, err
	}
	cl.m.Lock()
	defer cl.m.Unlock()

	// make a copy of cl.config so that update certificate does not mutate
	// the config passed to the tls.Server for this conn.
	config := *cl.config
	return tls.Server(conn, &config), nil
}

// Close closes the listener.
func (cl *changeCertListener) Close() error {
	cl.tomb.Kill(nil)
	return cl.Listener.Close()
}

// processCertChanges receives new certificate information and
// calls a method to update the listener's certificate.
func (cl *changeCertListener) processCertChanges() error {
	for {
		select {
		case info := <-cl.certChanged:
			if info.Cert != "" {
				cl.updateCertificate([]byte(info.Cert), []byte(info.PrivateKey))
			}
		case <-cl.tomb.Dying():
			return tomb.ErrDying
		}
	}
}

// updateCertificate generates a new TLS certificate and assigns it
// to the TLS listener.
func (cl *changeCertListener) updateCertificate(cert, key []byte) {
	cl.m.Lock()
	defer cl.m.Unlock()
	if tlsCert, err := tls.X509KeyPair(cert, key); err != nil {
		logger.Errorf("cannot create new TLS certificate: %v", err)
	} else {
		logger.Infof("updating api server certificate")
		x509Cert, err := x509.ParseCertificate(tlsCert.Certificate[0])
		if err == nil {
			var addr []string
			for _, ip := range x509Cert.IPAddresses {
				addr = append(addr, ip.String())
			}
			logger.Infof("new certificate addresses: %v", strings.Join(addr, ", "))
		}
		cl.config.Certificates = []tls.Certificate{tlsCert}
	}
}

// NewServer serves the given state by accepting requests on the given
// listener, using the given certificate and key (in PEM format) for
// authentication.
//
// The Server will close the listener when it exits, even if returns an error.
func NewServer(s *state.State, lis net.Listener, cfg ServerConfig) (*Server, error) {
	if err := cfg.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	// Important note:
	// Do not manipulate the state within NewServer as the API
	// server needs to run before mongo upgrades have happened and
	// any state manipulation may be be relying on features of the
	// database added by upgrades. Here be dragons.
	l, ok := lis.(*net.TCPListener)
	if !ok {
		return nil, errors.Errorf("listener is not of type *net.TCPListener: %T", lis)
	}
	srv, err := newServer(s, l, cfg)
	if err != nil {
		// There is no running server around to close the listener.
		lis.Close()
		return nil, errors.Trace(err)
	}
	return srv, nil
}

func newServer(s *state.State, lis *net.TCPListener, cfg ServerConfig) (_ *Server, err error) {
	tlsCert, err := tls.X509KeyPair(cfg.Cert, cfg.Key)
	if err != nil {
		return nil, err
	}
	// TODO(rog) check that *srvRoot is a valid type for using
	// as an RPC server.
	tlsConfig := utils.SecureTLSConfig()
	tlsConfig.Certificates = []tls.Certificate{tlsCert}

	stPool := cfg.StatePool
	if stPool == nil {
		stPool = state.NewStatePool(s)
	}

	srv := &Server{
		newObserver: cfg.NewObserver,
		state:       s,
		statePool:   stPool,
		lis:         newChangeCertListener(lis, cfg.CertChanged, tlsConfig),
		tag:         cfg.Tag,
		dataDir:     cfg.DataDir,
		logDir:      cfg.LogDir,
		limiter:     utils.NewLimiter(loginRateLimit),
		validator:   cfg.Validator,
		adminAPIFactories: map[int]adminAPIFactory{
			3: newAdminAPIV3,
		},
	}
	srv.authCtxt, err = newAuthContext(s)
	if err != nil {
		return nil, errors.Trace(err)
	}
	go srv.run()
	return srv, nil
}

func (srv *Server) ConnectionCount() int64 {
	return atomic.LoadInt64(&srv.connCount)
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

func (srv *Server) run() {
	logger.Infof("listening on %q", srv.lis.Addr())

	defer func() {
		addr := srv.lis.Addr().String() // Addr not valid after close
		err := srv.lis.Close()
		logger.Infof("closed listening socket %q with final error: %v", addr, err)

		srv.state.HackLeadership() // Break deadlocks caused by BlockUntil... calls.
		srv.wg.Wait()              // wait for any outstanding requests to complete.
		srv.tomb.Done()
		srv.statePool.Close()
		srv.state.Close()
	}()

	srv.wg.Add(1)
	go func() {
		defer srv.wg.Done()
		srv.tomb.Kill(srv.mongoPinger())
	}()

	// for pat based handlers, they are matched in-order of being
	// registered, first match wins. So more specific ones have to be
	// registered first.
	mux := pat.New()
	for _, endpoint := range srv.endpoints() {
		registerEndpoint(endpoint, mux)
	}

	go func() {
		addr := srv.lis.Addr() // not valid after addr closed
		logger.Debugf("Starting API http server on address %q", addr)
		err := http.Serve(srv.lis, mux)
		// normally logging an error at debug level would be grounds for a beating,
		// however in this case the error is *expected* to be non nil, and does not
		// affect the operation of the apiserver, but for completeness log it anyway.
		logger.Debugf("API http server exited, final error was: %v", err)
	}()

	<-srv.tomb.Dying()
}

func (srv *Server) endpoints() []apihttp.Endpoint {
	httpCtxt := httpContext{
		srv: srv,
	}

	endpoints := common.ResolveAPIEndpoints(srv.newHandlerArgs)

	// TODO(ericsnow) Add the following to the registry instead.

	add := func(pattern string, handler http.Handler) {
		// TODO: We can switch from all methods to specific ones for entries
		// where we only want to support specific request methods. However, our
		// tests currently assert that errors come back as application/json and
		// pat only does "text/plain" responses.
		for _, method := range common.DefaultHTTPMethods {
			endpoints = append(endpoints, apihttp.Endpoint{
				Pattern: pattern,
				Method:  method,
				Handler: handler,
			})
		}
	}

	strictCtxt := httpCtxt
	strictCtxt.strictValidation = true
	strictCtxt.controllerModelOnly = true

	mainAPIHandler := srv.trackRequests(http.HandlerFunc(srv.apiHandler))
	logSinkHandler := srv.trackRequests(newLogSinkHandler(httpCtxt, srv.logDir))
	logStreamHandler := srv.trackRequests(newLogStreamEndpointHandler(strictCtxt))
	debugLogHandler := srv.trackRequests(newDebugLogDBHandler(httpCtxt))

	add("/model/:modeluuid/logsink", logSinkHandler)
	add("/model/:modeluuid/logstream", logStreamHandler)
	add("/model/:modeluuid/log", debugLogHandler)
	add("/model/:modeluuid/charms",
		&charmsHandler{
			ctxt:    httpCtxt,
			dataDir: srv.dataDir},
	)
	add("/model/:modeluuid/tools",
		&toolsUploadHandler{
			ctxt: httpCtxt,
		},
	)
	add("/model/:modeluuid/tools/:version",
		&toolsDownloadHandler{
			ctxt: httpCtxt,
		},
	)
	add("/model/:modeluuid/backups",
		&backupHandler{
			ctxt: strictCtxt,
		},
	)
	add("/model/:modeluuid/api", mainAPIHandler)

	endpoints = append(endpoints, guiEndpoints("/gui/:modeluuid/", srv.dataDir, httpCtxt)...)
	add("/gui-archive", &guiArchiveHandler{
		ctxt: httpCtxt,
	})
	add("/gui-version", &guiVersionHandler{
		ctxt: httpCtxt,
	})

	// For backwards compatibility we register all the old paths
	add("/log", debugLogHandler)

	add("/charms",
		&charmsHandler{
			ctxt:    httpCtxt,
			dataDir: srv.dataDir,
		},
	)
	add("/tools",
		&toolsUploadHandler{
			ctxt: httpCtxt,
		},
	)
	add("/tools/:version",
		&toolsDownloadHandler{
			ctxt: httpCtxt,
		},
	)
	add("/register",
		&registerUserHandler{
			httpCtxt,
			srv.authCtxt.userAuth.CreateLocalLoginMacaroon,
		},
	)
	add("/api", mainAPIHandler)
	// Serve the API at / (only) for backward compatiblity. Note that the
	// pat muxer special-cases / so that it does not serve all
	// possible endpoints, but only / itself.
	add("/", mainAPIHandler)

	return endpoints
}

func (srv *Server) newHandlerArgs(spec apihttp.HandlerConstraints) apihttp.NewHandlerArgs {
	ctxt := httpContext{
		srv:                 srv,
		strictValidation:    spec.StrictValidation,
		controllerModelOnly: spec.ControllerModelOnly,
	}

	var args apihttp.NewHandlerArgs
	switch spec.AuthKind {
	case names.UserTagKind:
		args.Connect = ctxt.stateForRequestAuthenticatedUser
	case names.UnitTagKind:
		args.Connect = ctxt.stateForRequestAuthenticatedAgent
	case "":
		logger.Tracef(`no access level specified; proceeding with "unauthenticated"`)
		args.Connect = func(req *http.Request) (*state.State, state.Entity, error) {
			st, err := ctxt.stateForRequestUnauthenticated(req)
			return st, nil, err
		}
	default:
		logger.Infof(`unrecognized access level %q; proceeding with "unauthenticated"`, spec.AuthKind)
		args.Connect = func(req *http.Request) (*state.State, state.Entity, error) {
			st, err := ctxt.stateForRequestUnauthenticated(req)
			return st, nil, err
		}
	}
	return args
}

// trackRequests wraps a http.Handler, incrementing and decrementing
// the apiserver's WaitGroup and blocking request when the apiserver
// is shutting down.
//
// Note: It is only safe to use trackRequests with API handlers which
// are interruptible (i.e. they pay attention to the apiserver tomb)
// or are guaranteed to be short-lived. If it's used with long running
// API handlers which don't watch the apiserver's tomb, apiserver
// shutdown will be blocked until the API handler returns.
func (srv *Server) trackRequests(handler http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Care must be taken to not increment the waitgroup count
		// after the listener has closed.
		//
		// First we check to see if the tomb has not yet been killed
		// because the closure of the listener depends on the tomb being
		// killed to trigger the defer block in srv.run.
		select {
		case <-srv.tomb.Dying():
			// This request was accepted before the listener was closed
			// but after the tomb was killed. As we're in the process of
			// shutting down, do not consider this request as in progress,
			// just send a 503 and return.
			http.Error(w, "apiserver shutdown in progress", 503)
		default:
			// If we get here then the tomb was not killed therefore the
			// listener is still open. It is safe to increment the
			// wg counter as wg.Wait in srv.run has not yet been called.
			srv.wg.Add(1)
			defer srv.wg.Done()
			handler.ServeHTTP(w, r)
		}
	})
}

func registerEndpoint(ep apihttp.Endpoint, mux *pat.PatternServeMux) {
	mux.Add(ep.Method, ep.Pattern, ep.Handler)
	if ep.Method == "GET" {
		mux.Add("HEAD", ep.Pattern, ep.Handler)
	}
}

func (srv *Server) apiHandler(w http.ResponseWriter, req *http.Request) {
	addCount := func(delta int64) {
		atomic.AddInt64(&srv.connCount, delta)
	}

	addCount(1)
	defer addCount(-1)

	connectionID := atomic.AddUint64(&srv.lastConnectionID, 1)

	apiObserver := srv.newObserver()
	apiObserver.Join(req, connectionID)
	defer apiObserver.Leave()

	wsServer := websocket.Server{
		Handler: func(conn *websocket.Conn) {
			modelUUID := req.URL.Query().Get(":modeluuid")
			logger.Tracef("got a request for model %q", modelUUID)
			if err := srv.serveConn(conn, modelUUID, apiObserver); err != nil {
				logger.Errorf("error serving RPCs: %v", err)
			}
		},
	}
	wsServer.ServeHTTP(w, req)
}

func (srv *Server) serveConn(wsConn *websocket.Conn, modelUUID string, apiObserver observer.Observer) error {
	codec := jsoncodec.NewWebsocket(wsConn)

	conn := rpc.NewConn(codec, apiObserver)

	h, err := srv.newAPIHandler(conn, modelUUID)
	if err != nil {
		conn.ServeRoot(&errRoot{err}, serverError)
	} else {
		adminAPIs := make(map[int]interface{})
		for apiVersion, factory := range srv.adminAPIFactories {
			adminAPIs[apiVersion] = factory(srv, h, apiObserver)
		}
		conn.ServeRoot(newAnonRoot(h, adminAPIs), serverError)
	}
	conn.Start()
	select {
	case <-conn.Dead():
	case <-srv.tomb.Dying():
	}
	return conn.Close()
}

func (srv *Server) newAPIHandler(conn *rpc.Conn, modelUUID string) (*apiHandler, error) {
	// Note that we don't overwrite modelUUID here because
	// newAPIHandler treats an empty modelUUID as signifying
	// the API version used.
	resolvedModelUUID, err := validateModelUUID(validateArgs{
		statePool: srv.statePool,
		modelUUID: modelUUID,
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	st, err := srv.statePool.Get(resolvedModelUUID)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return newAPIHandler(srv, st, conn, modelUUID)
}

func (srv *Server) mongoPinger() error {
	// TODO(fwereade): 2016-03-17 lp:1558657
	timer := time.NewTimer(0)
	session := srv.state.MongoSession().Copy()
	defer session.Close()
	for {
		select {
		case <-timer.C:
		case <-srv.tomb.Dying():
			return tomb.ErrDying
		}
		if err := session.Ping(); err != nil {
			logger.Infof("got error pinging mongo: %v", err)
			return errors.Annotate(err, "error pinging mongo")
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
