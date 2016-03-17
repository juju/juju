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
	"github.com/juju/names"
	"github.com/juju/utils"
	"golang.org/x/net/websocket"
	"launchpad.net/tomb"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/common/apihttp"
	"github.com/juju/juju/apiserver/params"
	resourceapi "github.com/juju/juju/resource/api"
	resourceinternal "github.com/juju/juju/resource/api/private"
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
	adminApiFactories map[int]adminApiFactory
	mongoUnavailable  uint32 // non zero if mongoUnavailable
	modelUUID         string
	authCtxt          *authContext
	connections       int32 // count of active websocket connections
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
	config := cl.config
	return tls.Server(conn, config), nil
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
	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{tlsCert},
		MinVersion:   tls.VersionTLS10,
	}
	srv := &Server{
		state:     s,
		statePool: state.NewStatePool(s),
		lis:       newChangeCertListener(lis, cfg.CertChanged, tlsConfig),
		tag:       cfg.Tag,
		dataDir:   cfg.DataDir,
		logDir:    cfg.LogDir,
		limiter:   utils.NewLimiter(loginRateLimit),
		validator: cfg.Validator,
		adminApiFactories: map[int]adminApiFactory{
			3: newAdminApiV3,
		},
	}
	srv.authCtxt = newAuthContext(srv)
	go srv.run()
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

	// count is incremented by calls to join, and deincremented
	// by calls to leave.
	count *int32
}

var globalCounter int64

func newRequestNotifier(count *int32) *requestNotifier {
	return &requestNotifier{
		id:    atomic.AddInt64(&globalCounter, 1),
		tag_:  "<unknown>",
		start: time.Now(),
		count: count,
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
	// Until secrets are removed, we only log the body of the requests at trace level
	// which is below the default level of debug.
	if logger.IsTraceEnabled() {
		logger.Tracef("<- [%X] %s %s", n.id, n.tag(), jsoncodec.DumpRequest(hdr, body))
	} else {
		logger.Debugf("<- [%X] %s %s", n.id, n.tag(), jsoncodec.DumpRequest(hdr, "'params redacted'"))
	}
}

func (n *requestNotifier) ServerReply(req rpc.Request, hdr *rpc.Header, body interface{}, timeSpent time.Duration) {
	if req.Type == "Pinger" && req.Action == "Ping" {
		return
	}
	// TODO(rog) 2013-10-11 remove secrets from some responses.
	// Until secrets are removed, we only log the body of the requests at trace level
	// which is below the default level of debug.
	if logger.IsTraceEnabled() {
		logger.Tracef("-> [%X] %s %s", n.id, n.tag(), jsoncodec.DumpRequest(hdr, body))
	} else {
		logger.Debugf("-> [%X] %s %s %s %s[%q].%s", n.id, n.tag(), timeSpent, jsoncodec.DumpRequest(hdr, "'body redacted'"), req.Type, req.Id, req.Action)
	}
}

func (n *requestNotifier) join(req *http.Request) {
	active := atomic.AddInt32(n.count, 1)
	logger.Infof("[%X] API connection from %s, active connections: %d", n.id, req.RemoteAddr, active)
}

func (n *requestNotifier) leave() {
	active := atomic.AddInt32(n.count, -1)
	logger.Infof("[%X] %s API connection terminated after %v, active connections: %d", n.id, n.tag(), time.Since(n.start), active)
}

func (n *requestNotifier) ClientRequest(hdr *rpc.Header, body interface{}) {
}

func (n *requestNotifier) ClientReply(req rpc.Request, hdr *rpc.Header, body interface{}) {
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
		err := srv.mongoPinger()
		// Before killing the tomb, inform the API handlers that
		// Mongo is unavailable. API handlers can use this to decide
		// not to perform non-critical Mongo-related operations when
		// tearing down.
		atomic.AddUint32(&srv.mongoUnavailable, 1)
		srv.tomb.Kill(err)
		srv.wg.Done()
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

	mainAPIHandler := srv.trackRequests(http.HandlerFunc(srv.apiHandler))
	logSinkHandler := srv.trackRequests(newLogSinkHandler(httpCtxt, srv.logDir))
	debugLogHandler := srv.trackRequests(newDebugLogDBHandler(httpCtxt))

	add("/model/:modeluuid"+resourceapi.HTTPEndpointPattern,
		newResourceHandler(httpCtxt),
	)
	add("/model/:modeluuid"+resourceinternal.HTTPEndpointPattern,
		newUnitResourceHandler(httpCtxt),
	)
	add("/model/:modeluuid/logsink", logSinkHandler)
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
	strictCtxt := httpCtxt
	strictCtxt.strictValidation = true
	strictCtxt.controllerModelOnly = true
	add("/model/:modeluuid/backups",
		&backupHandler{
			ctxt: strictCtxt,
		},
	)
	add("/model/:modeluuid/api", mainAPIHandler)

	add("/model/:modeluuid/images/:kind/:series/:arch/:filename",
		&imagesDownloadHandler{
			ctxt:    httpCtxt,
			dataDir: srv.dataDir,
			state:   srv.state,
		},
	)
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
			ctxt: httpCtxt,
		},
	)
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
	case "":
		logger.Tracef(`no access level specified; proceeding with "unauthenticated"`)
		args.Connect = func(req *http.Request) (*state.State, state.Entity, error) {
			st, err := ctxt.stateForRequestUnauthenticated(req)
			return st, nil, err
		}
	default:
		logger.Warningf(`unrecognized access level %q; proceeding with "unauthenticated"`, spec.AuthKind)
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
		srv.wg.Add(1)
		defer srv.wg.Done()
		// If we've got to this stage and the tomb is still
		// alive, we know that any tomb.Kill must occur after we
		// have called wg.Add, so we avoid the possibility of a
		// handler goroutine running after Stop has returned.
		if srv.tomb.Err() != tomb.ErrStillAlive {
			return
		}

		handler.ServeHTTP(w, r)
	})
}

func registerEndpoint(ep apihttp.Endpoint, mux *pat.PatternServeMux) {
	switch ep.Method {
	case "GET":
		mux.Get(ep.Pattern, ep.Handler)
	case "POST":
		mux.Post(ep.Pattern, ep.Handler)
	case "HEAD":
		mux.Head(ep.Pattern, ep.Handler)
	case "PUT":
		mux.Put(ep.Pattern, ep.Handler)
	case "DEL":
		mux.Del(ep.Pattern, ep.Handler)
	case "OPTIONS":
		mux.Options(ep.Pattern, ep.Handler)
	}
}

func (srv *Server) apiHandler(w http.ResponseWriter, req *http.Request) {
	reqNotifier := newRequestNotifier(&srv.connections)
	reqNotifier.join(req)
	defer reqNotifier.leave()
	wsServer := websocket.Server{
		Handler: func(conn *websocket.Conn) {
			modelUUID := req.URL.Query().Get(":modeluuid")
			logger.Tracef("got a request for model %q", modelUUID)
			if err := srv.serveConn(conn, reqNotifier, modelUUID); err != nil {
				logger.Errorf("error serving RPCs: %v", err)
			}
		},
	}
	wsServer.ServeHTTP(w, req)
}

func (srv *Server) serveConn(wsConn *websocket.Conn, reqNotifier *requestNotifier, modelUUID string) error {
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

	h, err := srv.newAPIHandler(conn, reqNotifier, modelUUID)
	if err != nil {
		conn.ServeFinder(&errRoot{err}, serverError)
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

func (srv *Server) newAPIHandler(conn *rpc.Conn, reqNotifier *requestNotifier, modelUUID string) (*apiHandler, error) {
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
	return newApiHandler(srv, st, conn, reqNotifier, modelUUID)
}

func (srv *Server) mongoPinger() error {
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
