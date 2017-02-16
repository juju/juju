// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"crypto/tls"
	"crypto/x509"
	"io"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/bmizerany/pat"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/pubsub"
	"github.com/juju/utils"
	"github.com/juju/utils/clock"
	"golang.org/x/crypto/acme"
	"golang.org/x/crypto/acme/autocert"
	"golang.org/x/net/websocket"
	"gopkg.in/juju/names.v2"
	"gopkg.in/macaroon-bakery.v1/httpbakery"
	"gopkg.in/tomb.v1"

	"github.com/juju/juju/apiserver/authentication"
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
	clock             clock.Clock
	pingClock         clock.Clock
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
	centralHub        *pubsub.StructuredHub
	newObserver       observer.ObserverFactory
	connCount         int64
	certChanged       <-chan params.StateServingInfo
	tlsConfig         *tls.Config
	allowModelAccess  bool
	logSinkWriter     io.WriteCloser

	// mu guards the fields below it.
	mu sync.Mutex

	// cert holds the current certificate used for tls.Config.
	cert *tls.Certificate

	// certDNSNames holds the DNS names associated with cert.
	certDNSNames []string

	// registerIntrospectionHandlers is a function that will
	// call a function with (path, http.Handler) tuples. This
	// is to support registering the handlers underneath the
	// "/introspection" prefix.
	registerIntrospectionHandlers func(func(string, http.Handler))
}

// LoginValidator functions are used to decide whether login requests
// are to be allowed. The validator is called before credentials are
// checked.
type LoginValidator func(params.LoginRequest) error

// ServerConfig holds parameters required to set up an API server.
type ServerConfig struct {
	Clock       clock.Clock
	PingClock   clock.Clock
	Cert        string
	Key         string
	Tag         names.Tag
	DataDir     string
	LogDir      string
	Validator   LoginValidator
	Hub         *pubsub.StructuredHub
	CertChanged <-chan params.StateServingInfo

	// AutocertDNSName holds the DNS name for which
	// official TLS certificates will be obtained. If this is
	// empty, no certificates will be requested.
	AutocertDNSName string

	// AutocertURL holds the URL from which official
	// TLS certificates will be obtained. By default,
	// acme.LetsEncryptURL will be used.
	AutocertURL string

	// AllowModelAccess holds whether users will be allowed to
	// access models that they have access rights to even when
	// they don't have access to the controller.
	AllowModelAccess bool

	// NewObserver is a function which will return an observer. This
	// is used per-connection to instantiate a new observer to be
	// notified of key events during API requests.
	NewObserver observer.ObserverFactory

	// StatePool is created by the machine agent and passed in.
	StatePool *state.StatePool

	// RegisterIntrospectionHandlers is a function that will
	// call a function with (path, http.Handler) tuples. This
	// is to support registering the handlers underneath the
	// "/introspection" prefix.
	RegisterIntrospectionHandlers func(func(string, http.Handler))
}

func (c *ServerConfig) Validate() error {
	if c.Hub == nil {
		return errors.NotValidf("missing Hub")
	}
	if c.Clock == nil {
		return errors.NotValidf("missing Clock")
	}
	if c.NewObserver == nil {
		return errors.NotValidf("missing NewObserver")
	}
	if c.StatePool == nil {
		return errors.NotValidf("missing StatePool")
	}

	return nil
}

func (c *ServerConfig) pingClock() clock.Clock {
	if c.PingClock == nil {
		return c.Clock
	}
	return c.PingClock
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
	srv, err := newServer(s, lis, cfg)
	if err != nil {
		// There is no running server around to close the listener.
		lis.Close()
		return nil, errors.Trace(err)
	}
	return srv, nil
}

func newServer(s *state.State, lis net.Listener, cfg ServerConfig) (_ *Server, err error) {
	stPool := cfg.StatePool
	if stPool == nil {
		stPool = state.NewStatePool(s)
	}

	srv := &Server{
		clock:       cfg.Clock,
		pingClock:   cfg.pingClock(),
		lis:         lis,
		newObserver: cfg.NewObserver,
		state:       s,
		statePool:   stPool,
		tag:         cfg.Tag,
		dataDir:     cfg.DataDir,
		logDir:      cfg.LogDir,
		limiter:     utils.NewLimiter(loginRateLimit),
		validator:   cfg.Validator,
		adminAPIFactories: map[int]adminAPIFactory{
			3: newAdminAPIV3,
		},
		centralHub:                    cfg.Hub,
		certChanged:                   cfg.CertChanged,
		allowModelAccess:              cfg.AllowModelAccess,
		registerIntrospectionHandlers: cfg.RegisterIntrospectionHandlers,
	}

	srv.tlsConfig = srv.newTLSConfig(cfg)
	srv.lis = tls.NewListener(lis, srv.tlsConfig)

	srv.authCtxt, err = newAuthContext(s)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if err := srv.updateCertificate(cfg.Cert, cfg.Key); err != nil {
		return nil, errors.Annotatef(err, "cannot set initial certificate")
	}

	logSinkWriter, err := newLogSinkWriter(filepath.Join(srv.logDir, "logsink.log"))
	if err != nil {
		return nil, errors.Annotate(err, "creating logsink writer")
	}
	srv.logSinkWriter = logSinkWriter

	go srv.run()
	return srv, nil
}

func (srv *Server) newTLSConfig(cfg ServerConfig) *tls.Config {
	tlsConfig := utils.SecureTLSConfig()
	if cfg.AutocertDNSName == "" {
		// No official DNS name, no certificate.
		tlsConfig.GetCertificate = func(clientHello *tls.ClientHelloInfo) (*tls.Certificate, error) {
			cert, _ := srv.localCertificate(clientHello.ServerName)
			return cert, nil
		}
		return tlsConfig
	}
	m := autocert.Manager{
		Prompt:     autocert.AcceptTOS,
		Cache:      srv.state.AutocertCache(),
		HostPolicy: autocert.HostWhitelist(cfg.AutocertDNSName),
	}
	if cfg.AutocertURL != "" {
		m.Client = &acme.Client{
			DirectoryURL: cfg.AutocertURL,
		}
	}
	tlsConfig.GetCertificate = func(clientHello *tls.ClientHelloInfo) (*tls.Certificate, error) {
		logger.Infof("getting certificate for server name %q", clientHello.ServerName)
		// Get the locally created certificate and whether it's appropriate
		// for the SNI name. If not, we'll try to get an acme cert and
		// fall back to the local certificate if that fails.
		cert, shouldUse := srv.localCertificate(clientHello.ServerName)
		if shouldUse {
			return cert, nil
		}
		acmeCert, err := m.GetCertificate(clientHello)
		if err == nil {
			return acmeCert, nil
		}
		logger.Errorf("cannot get autocert certificate for %q: %v", clientHello.ServerName, err)
		return cert, nil
	}
	return tlsConfig
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

// loggoWrapper is an io.Writer() that forwards the messages to a loggo.Logger.
// Unfortunately http takes a concrete stdlib log.Logger struct, and not an
// interface, so we can't just proxy all of the log levels without inspecting
// the string content. For now, we just want to get the messages into the log
// file.
type loggoWrapper struct {
	logger loggo.Logger
	level  loggo.Level
}

func (w *loggoWrapper) Write(content []byte) (int, error) {
	w.logger.Logf(w.level, "%s", string(content))
	return len(content), nil
}

func (srv *Server) run() {
	logger.Infof("listening on %q", srv.lis.Addr())

	defer func() {
		addr := srv.lis.Addr().String() // Addr not valid after close
		err := srv.lis.Close()
		logger.Infof("closed listening socket %q with final error: %v", addr, err)

		// Break deadlocks caused by leadership BlockUntil... calls.
		srv.statePool.KillWorkers()
		srv.state.KillWorkers()

		srv.wg.Wait() // wait for any outstanding requests to complete.
		srv.tomb.Done()
		srv.statePool.Close()
		srv.state.Close()
		srv.logSinkWriter.Close()
	}()

	srv.wg.Add(1)
	go func() {
		defer srv.wg.Done()
		srv.tomb.Kill(srv.mongoPinger())
	}()

	srv.wg.Add(1)
	go func() {
		defer srv.wg.Done()
		srv.tomb.Kill(srv.expireLocalLoginInteractions())
	}()

	srv.wg.Add(1)
	go func() {
		defer srv.wg.Done()
		srv.tomb.Kill(srv.processCertChanges())
	}()

	srv.wg.Add(1)
	go func() {
		defer srv.wg.Done()
		srv.tomb.Kill(srv.processModelRemovals())
	}()

	// for pat based handlers, they are matched in-order of being
	// registered, first match wins. So more specific ones have to be
	// registered first.
	mux := pat.New()
	for _, endpoint := range srv.endpoints() {
		registerEndpoint(endpoint, mux)
	}

	go func() {
		logger.Debugf("Starting API http server on address %q", srv.lis.Addr())
		httpSrv := &http.Server{
			Handler:   mux,
			TLSConfig: srv.tlsConfig,
			ErrorLog: log.New(&loggoWrapper{
				level:  loggo.WARNING,
				logger: logger,
			}, "", 0), // no prefix and no flags so log.Logger doesn't add extra prefixes
		}
		err := httpSrv.Serve(srv.lis)
		// Normally logging an error at debug level would be grounds for a beating,
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
	logStreamHandler := srv.trackRequests(newLogStreamEndpointHandler(strictCtxt))
	debugLogHandler := srv.trackRequests(newDebugLogDBHandler(httpCtxt))
	pubsubHandler := srv.trackRequests(newPubSubHandler(httpCtxt, srv.centralHub))

	// This handler is model specific even though it only ever makes sense
	// for a controller because the API caller that is handed to the worker
	// that is forwarding the messages between controllers is bound to the
	// /model/:modeluuid namespace.
	add("/model/:modeluuid/pubsub", pubsubHandler)
	add("/model/:modeluuid/logstream", logStreamHandler)
	add("/model/:modeluuid/log", debugLogHandler)

	logSinkHandler := newLogSinkHandler(httpCtxt, srv.logSinkWriter, newAgentLoggingStrategy)
	add("/model/:modeluuid/logsink", srv.trackRequests(logSinkHandler))

	// We don't need to save the migrated logs to a logfile as well as to the DB.
	logTransferHandler := newLogSinkHandler(httpCtxt, ioutil.Discard, newMigrationLoggingStrategy)
	add("/migrate/logtransfer", srv.trackRequests(logTransferHandler))

	modelCharmsHandler := &charmsHandler{
		ctxt:          httpCtxt,
		dataDir:       srv.dataDir,
		stateAuthFunc: httpCtxt.stateForRequestAuthenticatedUser,
	}
	charmsServer := &CharmsHTTPHandler{
		PostHandler: modelCharmsHandler.ServePost,
		GetHandler:  modelCharmsHandler.ServeGet,
	}
	add("/model/:modeluuid/charms", charmsServer)
	add("/model/:modeluuid/tools",
		&toolsUploadHandler{
			ctxt:          httpCtxt,
			stateAuthFunc: httpCtxt.stateForRequestAuthenticatedUser,
		},
	)

	migrateCharmsHandler := &charmsHandler{
		ctxt:          httpCtxt,
		dataDir:       srv.dataDir,
		stateAuthFunc: httpCtxt.stateForMigrationImporting,
	}
	add("/migrate/charms",
		&CharmsHTTPHandler{
			PostHandler: migrateCharmsHandler.ServePost,
			GetHandler:  migrateCharmsHandler.ServeUnsupported,
		},
	)
	add("/migrate/tools",
		&toolsUploadHandler{
			ctxt:          httpCtxt,
			stateAuthFunc: httpCtxt.stateForMigrationImporting,
		},
	)
	add("/migrate/resources",
		&resourceUploadHandler{
			ctxt:          httpCtxt,
			stateAuthFunc: httpCtxt.stateForMigrationImporting,
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

	// GUI now supports URLs without the model uuid, just the user/model.
	endpoints = append(endpoints, guiEndpoints(guiURLPathPrefix+"u/:user/:modelname/", srv.dataDir, httpCtxt)...)
	endpoints = append(endpoints, guiEndpoints(guiURLPathPrefix+":modeluuid/", srv.dataDir, httpCtxt)...)
	add("/gui-archive", &guiArchiveHandler{
		ctxt: httpCtxt,
	})
	add("/gui-version", &guiVersionHandler{
		ctxt: httpCtxt,
	})

	// For backwards compatibility we register all the old paths
	add("/log", debugLogHandler)

	add("/charms", charmsServer)
	add("/tools",
		&toolsUploadHandler{
			ctxt:          httpCtxt,
			stateAuthFunc: httpCtxt.stateForRequestAuthenticatedUser,
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
	add("/api", mainAPIHandler)
	// Serve the API at / (only) for backward compatiblity. Note that the
	// pat muxer special-cases / so that it does not serve all
	// possible endpoints, but only / itself.
	add("/", mainAPIHandler)

	// Register the introspection endpoints.
	if srv.registerIntrospectionHandlers != nil {
		handle := func(subpath string, handler http.Handler) {
			add(path.Join("/introspection/", subpath),
				introspectionHandler{
					httpCtxt,
					handler,
				},
			)
		}
		srv.registerIntrospectionHandlers(handle)
	}

	// Add HTTP handlers for local-user macaroon authentication.
	localLoginHandlers := &localLoginHandlers{srv.authCtxt, srv.state}
	dischargeMux := http.NewServeMux()
	httpbakery.AddDischargeHandler(
		dischargeMux,
		localUserIdentityLocationPath,
		localLoginHandlers.authCtxt.localUserThirdPartyBakeryService,
		localLoginHandlers.checkThirdPartyCaveat,
	)
	dischargeMux.Handle(
		localUserIdentityLocationPath+"/login",
		makeHandler(handleJSON(localLoginHandlers.serveLogin)),
	)
	dischargeMux.Handle(
		localUserIdentityLocationPath+"/wait",
		makeHandler(handleJSON(localLoginHandlers.serveWait)),
	)
	add(localUserIdentityLocationPath+"/discharge", dischargeMux)
	add(localUserIdentityLocationPath+"/publickey", dischargeMux)
	add(localUserIdentityLocationPath+"/login", dischargeMux)
	add(localUserIdentityLocationPath+"/wait", dischargeMux)

	return endpoints
}

func (srv *Server) expireLocalLoginInteractions() error {
	for {
		select {
		case <-srv.tomb.Dying():
			return tomb.ErrDying
		case <-srv.clock.After(authentication.LocalLoginInteractionTimeout):
			now := srv.authCtxt.clock.Now()
			srv.authCtxt.localUserInteractions.Expire(now)
		}
	}
}

func (srv *Server) newHandlerArgs(spec apihttp.HandlerConstraints) apihttp.NewHandlerArgs {
	ctxt := httpContext{
		srv:                 srv,
		strictValidation:    spec.StrictValidation,
		controllerModelOnly: spec.ControllerModelOnly,
	}
	return apihttp.NewHandlerArgs{
		Connect: func(req *http.Request) (*state.State, func(), state.Entity, error) {
			return ctxt.stateForRequestAuthenticatedTag(req, spec.AuthKinds...)
		},
	}
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
			if err := srv.serveConn(conn, modelUUID, apiObserver, req.Host); err != nil {
				logger.Errorf("error serving RPCs: %v", err)
			}
		},
	}
	wsServer.ServeHTTP(w, req)
}

func (srv *Server) serveConn(wsConn *websocket.Conn, modelUUID string, apiObserver observer.Observer, host string) error {
	codec := jsoncodec.NewWebsocket(wsConn)

	conn := rpc.NewConn(codec, apiObserver)

	// Note that we don't overwrite modelUUID here because
	// newAPIHandler treats an empty modelUUID as signifying
	// the API version used.
	resolvedModelUUID, err := validateModelUUID(validateArgs{
		statePool: srv.statePool,
		modelUUID: modelUUID,
	})
	var (
		st       *state.State
		h        *apiHandler
		releaser func()
	)
	if err == nil {
		st, releaser, err = srv.statePool.Get(resolvedModelUUID)
	}

	if err == nil {
		defer releaser()
		h, err = newAPIHandler(srv, st, conn, modelUUID, host)
	}

	if err != nil {
		conn.ServeRoot(&errRoot{errors.Trace(err)}, serverError)
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

func (srv *Server) mongoPinger() error {
	session := srv.state.MongoSession().Copy()
	defer session.Close()
	for {
		if err := session.Ping(); err != nil {
			logger.Infof("got error pinging mongo: %v", err)
			return errors.Annotate(err, "error pinging mongo")
		}
		select {
		case <-srv.clock.After(mongoPingInterval):
		case <-srv.tomb.Dying():
			return tomb.ErrDying
		}
	}
}

// localCertificate returns the local server certificate and reports
// whether it should be used to serve a connection addressed to the
// given server name.
func (srv *Server) localCertificate(serverName string) (*tls.Certificate, bool) {
	srv.mu.Lock()
	defer srv.mu.Unlock()
	if net.ParseIP(serverName) != nil {
		// IP address connections always use the local certificate.
		return srv.cert, true
	}
	if !strings.Contains(serverName, ".") {
		// If the server name doesn't contain a period there's no
		// way we can obtain a certificate for it.
		// This applies to the common case where "juju-apiserver" is
		// used as the server name.
		return srv.cert, true
	}
	// Perhaps the server name is explicitly mentioned by the server certificate.
	for _, name := range srv.certDNSNames {
		if name == serverName {
			return srv.cert, true
		}
	}
	return srv.cert, false
}

// processCertChanges receives new certificate information and
// calls a method to update the listener's certificate.
func (srv *Server) processCertChanges() error {
	for {
		select {
		case info := <-srv.certChanged:
			if info.Cert == "" {
				break
			}
			logger.Infof("received API server certificate")
			if err := srv.updateCertificate(info.Cert, info.PrivateKey); err != nil {
				logger.Errorf("cannot update certificate: %v", err)
			}
		case <-srv.tomb.Dying():
			return tomb.ErrDying
		}
	}
}

// updateCertificate updates the current CA certificate and key
// from the given cert and key.
func (srv *Server) updateCertificate(cert, key string) error {
	srv.mu.Lock()
	defer srv.mu.Unlock()
	tlsCert, err := tls.X509KeyPair([]byte(cert), []byte(key))
	if err != nil {
		return errors.Annotatef(err, "cannot create new TLS certificate")
	}
	x509Cert, err := x509.ParseCertificate(tlsCert.Certificate[0])
	if err != nil {
		return errors.Annotatef(err, "parsing x509 cert")
	}
	var addr []string
	for _, ip := range x509Cert.IPAddresses {
		addr = append(addr, ip.String())
	}
	logger.Infof("new certificate addresses: %v", strings.Join(addr, ", "))
	srv.cert = &tlsCert
	srv.certDNSNames = x509Cert.DNSNames
	return nil
}

func serverError(err error) error {
	if err := common.ServerError(err); err != nil {
		return err
	}
	return nil
}

func (srv *Server) processModelRemovals() error {
	w := srv.state.WatchModelLives()
	defer w.Stop()
	for {
		select {
		case <-srv.tomb.Dying():
			return tomb.ErrDying
		case modelUUIDs := <-w.Changes():
			for _, modelUUID := range modelUUIDs {
				model, err := srv.state.GetModel(names.NewModelTag(modelUUID))
				gone := errors.IsNotFound(err)
				dead := err == nil && model.Life() == state.Dead
				if err != nil && !gone {
					return errors.Trace(err)
				}
				if !dead && !gone {
					continue
				}

				logger.Debugf("removing model %v from the state pool", modelUUID)
				// Model's gone away - ensure that it gets removed
				// from from the state pool once people are finished
				// with it.
				err = srv.statePool.Remove(modelUUID)
				if err != nil {
					return errors.Trace(err)
				}
			}
		}
	}
}
