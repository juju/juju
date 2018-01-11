// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"context"
	"crypto/tls"
	"io"
	"log"
	"net"
	"net/http"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/bmizerany/pat"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/pubsub"
	"github.com/juju/utils"
	"github.com/juju/utils/clock"
	"github.com/prometheus/client_golang/prometheus"
	"golang.org/x/crypto/acme"
	"golang.org/x/crypto/acme/autocert"
	"gopkg.in/juju/names.v2"
	"gopkg.in/macaroon-bakery.v1/bakery"
	"gopkg.in/macaroon-bakery.v1/httpbakery"
	"gopkg.in/tomb.v1"

	"github.com/juju/juju/apiserver/authentication"
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/common/apihttp"
	"github.com/juju/juju/apiserver/common/crossmodel"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/logsink"
	"github.com/juju/juju/apiserver/observer"
	"github.com/juju/juju/apiserver/websocket"
	"github.com/juju/juju/core/auditlog"
	"github.com/juju/juju/resource"
	"github.com/juju/juju/resource/resourceadapters"
	"github.com/juju/juju/rpc"
	"github.com/juju/juju/rpc/jsoncodec"
	"github.com/juju/juju/state"
)

var logger = loggo.GetLogger("juju.apiserver")

var defaultHTTPMethods = []string{"GET", "POST", "HEAD", "PUT", "DELETE", "OPTIONS"}

// Server holds the server side of the API.
type Server struct {
	tomb                   tomb.Tomb
	clock                  clock.Clock
	pingClock              clock.Clock
	wg                     sync.WaitGroup
	statePool              *state.StatePool
	lis                    net.Listener
	tag                    names.Tag
	dataDir                string
	logDir                 string
	limiter                utils.Limiter
	loginRetryPause        time.Duration
	facades                *facade.Registry
	modelUUID              string
	loginAuthCtxt          *authContext
	offerAuthCtxt          *crossmodel.AuthContext
	lastConnectionID       uint64
	centralHub             *pubsub.StructuredHub
	newObserver            observer.ObserverFactory
	connCount              int64
	totalConn              int64
	loginAttempts          int64
	getCertificate         func() *tls.Certificate
	tlsConfig              *tls.Config
	allowModelAccess       bool
	logSinkWriter          io.WriteCloser
	logsinkRateLimitConfig logsink.RateLimitConfig
	dbloggers              dbloggers
	auditLogConfig         AuditLogConfig
	auditLogger            auditlog.AuditLog
	upgradeComplete        func() bool
	restoreStatus          func() state.RestoreStatus

	// mu guards the fields below it.
	mu sync.Mutex

	// publicDNSName_ holds the value that will be returned in
	// LoginResult.PublicDNSName. Currently this is set once from
	// AutocertDNSName and does not change but in the future it
	// may change when a server certificate is explicitly set,
	// hence it's here guarded by the mutex.
	publicDNSName_ string

	// registerIntrospectionHandlers is a function that will
	// call a function with (path, http.Handler) tuples. This
	// is to support registering the handlers underneath the
	// "/introspection" prefix.
	registerIntrospectionHandlers func(func(string, http.Handler))
}

// ServerConfig holds parameters required to set up an API server.
type ServerConfig struct {
	Clock     clock.Clock
	PingClock clock.Clock
	Tag       names.Tag
	DataDir   string
	LogDir    string
	Hub       *pubsub.StructuredHub

	// GetCertificate holds a function that returns the current
	// local TLS certificate for the server. The function may
	// return updated values, so should be called whenever a
	// new connection is accepted.
	GetCertificate func() *tls.Certificate

	// UpgradeComplete is a function that reports whether or not
	// the if the agent running the API server has completed
	// running upgrade steps. This is used by the API server to
	// limit logins during upgrades.
	UpgradeComplete func() bool

	// RestoreStatus is a function that reports the restore
	// status most recently observed by the agent running the
	// API server. This is used by the API server to limit logins
	// during a restore.
	RestoreStatus func() state.RestoreStatus

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

	// RegisterIntrospectionHandlers is a function that will
	// call a function with (path, http.Handler) tuples. This
	// is to support registering the handlers underneath the
	// "/introspection" prefix.
	RegisterIntrospectionHandlers func(func(string, http.Handler))

	// RateLimitConfig holds paramaters to control
	// aspects of rate limiting connections and logins.
	RateLimitConfig RateLimitConfig

	// LogSinkConfig holds parameters to control the API server's
	// logsink endpoint behaviour. If this is nil, the values from
	// DefaultLogSinkConfig() will be used.
	LogSinkConfig *LogSinkConfig

	// AuditLogConfig holds parameters to configure audit logging.
	AuditLogConfig AuditLogConfig
	AuditLog       auditlog.AuditLog

	// PrometheusRegisterer registers Prometheus collectors.
	PrometheusRegisterer prometheus.Registerer
}

// Validate validates the API server configuration.
func (c ServerConfig) Validate() error {
	if c.Hub == nil {
		return errors.NotValidf("missing Hub")
	}
	if c.Clock == nil {
		return errors.NotValidf("missing Clock")
	}
	if c.GetCertificate == nil {
		return errors.NotValidf("missing GetCertificate")
	}
	if c.NewObserver == nil {
		return errors.NotValidf("missing NewObserver")
	}
	if c.UpgradeComplete == nil {
		return errors.NotValidf("nil UpgradeComplete")
	}
	if c.RestoreStatus == nil {
		return errors.NotValidf("nil RestoreStatus")
	}
	if err := c.RateLimitConfig.Validate(); err != nil {
		return errors.Annotate(err, "validating rate limit configuration")
	}
	if c.LogSinkConfig != nil {
		if err := c.LogSinkConfig.Validate(); err != nil {
			return errors.Annotate(err, "validating logsink configuration")
		}
	}
	if c.AuditLogConfig.Enabled && c.AuditLog == nil {
		return errors.NotValidf("audit logging enabled but no logger provided")
	}
	return nil
}

func (c ServerConfig) pingClock() clock.Clock {
	if c.PingClock == nil {
		return c.Clock
	}
	return c.PingClock
}

// NewServer serves the given state by accepting requests on the given
// listener, using the given certificate and key (in PEM format) for
// authentication.
//
// The Server will not close the StatePool; the caller is responsible
// for closing it after the Server has been stopped.
//
// The Server will close the listener when it exits, even if returns
// an error.
func NewServer(stPool *state.StatePool, lis net.Listener, cfg ServerConfig) (*Server, error) {
	if cfg.LogSinkConfig == nil {
		logSinkConfig := DefaultLogSinkConfig()
		cfg.LogSinkConfig = &logSinkConfig
	}
	if err := cfg.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	// Important note:
	// Do not manipulate the state within NewServer as the API
	// server needs to run before mongo upgrades have happened and
	// any state manipulation may be be relying on features of the
	// database added by upgrades. Here be dragons.
	srv, err := newServer(stPool, lis, cfg)
	if err != nil {
		// There is no running server around to close the listener.
		lis.Close()
		return nil, errors.Trace(err)
	}
	return srv, nil
}

func newServer(stPool *state.StatePool, lis net.Listener, cfg ServerConfig) (_ *Server, err error) {
	limiter := utils.NewLimiterWithPause(
		cfg.RateLimitConfig.LoginRateLimit, cfg.RateLimitConfig.LoginMinPause,
		cfg.RateLimitConfig.LoginMaxPause, clock.WallClock)
	srv := &Server{
		clock:                         cfg.Clock,
		pingClock:                     cfg.pingClock(),
		lis:                           lis,
		newObserver:                   cfg.NewObserver,
		statePool:                     stPool,
		tag:                           cfg.Tag,
		dataDir:                       cfg.DataDir,
		logDir:                        cfg.LogDir,
		limiter:                       limiter,
		loginRetryPause:               cfg.RateLimitConfig.LoginRetryPause,
		upgradeComplete:               cfg.UpgradeComplete,
		restoreStatus:                 cfg.RestoreStatus,
		facades:                       AllFacades(),
		centralHub:                    cfg.Hub,
		getCertificate:                cfg.GetCertificate,
		allowModelAccess:              cfg.AllowModelAccess,
		publicDNSName_:                cfg.AutocertDNSName,
		registerIntrospectionHandlers: cfg.RegisterIntrospectionHandlers,
		logsinkRateLimitConfig: logsink.RateLimitConfig{
			Refill: cfg.LogSinkConfig.RateLimitRefill,
			Burst:  cfg.LogSinkConfig.RateLimitBurst,
			Clock:  cfg.Clock,
		},
		auditLogConfig: cfg.AuditLogConfig,
		auditLogger:    cfg.AuditLog,
		dbloggers: dbloggers{
			clock:                 cfg.Clock,
			dbLoggerBufferSize:    cfg.LogSinkConfig.DBLoggerBufferSize,
			dbLoggerFlushInterval: cfg.LogSinkConfig.DBLoggerFlushInterval,
		},
	}

	srv.tlsConfig = srv.newTLSConfig(cfg)
	srv.lis = newThrottlingListener(
		tls.NewListener(lis, srv.tlsConfig), cfg.RateLimitConfig, clock.WallClock)

	// The auth context for authenticating logins.
	srv.loginAuthCtxt, err = newAuthContext(stPool.SystemState())
	if err != nil {
		return nil, errors.Trace(err)
	}

	// The auth context for authenticating access to application offers.
	srv.offerAuthCtxt, err = newOfferAuthcontext(stPool)
	if err != nil {
		return nil, errors.Trace(err)
	}

	logSinkWriter, err := logsink.NewFileWriter(filepath.Join(srv.logDir, "logsink.log"))
	if err != nil {
		return nil, errors.Annotate(err, "creating logsink writer")
	}
	srv.logSinkWriter = logSinkWriter

	if cfg.PrometheusRegisterer != nil {
		apiserverCollectior := NewMetricsCollector(&metricAdaptor{srv})
		cfg.PrometheusRegisterer.Unregister(apiserverCollectior)
		if err := cfg.PrometheusRegisterer.Register(apiserverCollectior); err != nil {
			return nil, errors.Annotate(err, "registering apiserver metrics collector")
		}
	}

	go func() {
		defer srv.tomb.Done()
		srv.tomb.Kill(srv.loop())
	}()
	return srv, nil
}

type metricAdaptor struct {
	srv *Server
}

func (a *metricAdaptor) TotalConnections() int64 {
	return a.srv.TotalConnections()
}

func (a *metricAdaptor) ConnectionCount() int64 {
	return a.srv.ConnectionCount()
}

func (a *metricAdaptor) ConcurrentLoginAttempts() int64 {
	return a.srv.LoginAttempts()
}

func (a *metricAdaptor) ConnectionPauseTime() time.Duration {
	return a.srv.lis.(*throttlingListener).pauseTime()
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
		Cache:      srv.statePool.SystemState().AutocertCache(),
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

// TotalConnections returns the total number of connections ever made.
func (srv *Server) TotalConnections() int64 {
	return atomic.LoadInt64(&srv.totalConn)
}

// ConnectionCount returns the number of current connections.
func (srv *Server) ConnectionCount() int64 {
	return atomic.LoadInt64(&srv.connCount)
}

// LoginAttempts returns the number of current login attempts.
func (srv *Server) LoginAttempts() int64 {
	return atomic.LoadInt64(&srv.loginAttempts)
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

// loop is the main loop for the server.
func (srv *Server) loop() error {
	logger.Infof("listening on %q", srv.lis.Addr())

	defer func() {
		addr := srv.lis.Addr().String() // Addr not valid after close
		err := srv.lis.Close()
		logger.Infof("closed listening socket %q with final error: %v", addr, err)

		srv.wg.Wait() // wait for any outstanding requests to complete.
		srv.dbloggers.dispose()
		srv.logSinkWriter.Close()
	}()

	// for pat based handlers, they are matched in-order of being
	// registered, first match wins. So more specific ones have to be
	// registered first.
	mux := pat.New()
	for _, endpoint := range srv.endpoints() {
		registerEndpoint(endpoint, mux)
	}

	// TODO(axw) graceful HTTP server shutdown. Then we'll shutdown the
	// server, rather than closing the listener.

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

	for {
		select {
		case <-srv.tomb.Dying():
			return tomb.ErrDying
		case <-srv.clock.After(authentication.LocalLoginInteractionTimeout):
			now := srv.loginAuthCtxt.clock.Now()
			srv.loginAuthCtxt.localUserInteractions.Expire(now)
		}
	}
}

func (srv *Server) endpoints() []apihttp.Endpoint {
	var endpoints []apihttp.Endpoint

	add := func(pattern string, handler http.Handler) {
		// TODO: We can switch from all methods to specific ones for entries
		// where we only want to support specific request methods. However, our
		// tests currently assert that errors come back as application/json and
		// pat only does "text/plain" responses.
		for _, method := range defaultHTTPMethods {
			endpoints = append(endpoints, apihttp.Endpoint{
				Pattern: pattern,
				Method:  method,
				Handler: handler,
			})
		}
	}

	httpCtxt := httpContext{
		srv: srv,
	}

	strictCtxt := httpCtxt
	strictCtxt.strictValidation = true
	strictCtxt.controllerModelOnly = true

	mainAPIHandler := srv.trackRequests(http.HandlerFunc(srv.apiHandler))
	logStreamHandler := srv.trackRequests(newLogStreamEndpointHandler(httpCtxt))
	debugLogHandler := srv.trackRequests(newDebugLogDBHandler(httpCtxt))
	pubsubHandler := srv.trackRequests(newPubSubHandler(httpCtxt, srv.centralHub))

	// This handler is model specific even though it only ever makes sense
	// for a controller because the API caller that is handed to the worker
	// that is forwarding the messages between controllers is bound to the
	// /model/:modeluuid namespace.
	add("/model/:modeluuid/pubsub", pubsubHandler)
	add("/model/:modeluuid/logstream", logStreamHandler)
	add("/model/:modeluuid/log", debugLogHandler)

	logSinkHandler := logsink.NewHTTPHandler(
		newAgentLogWriteCloserFunc(httpCtxt, srv.logSinkWriter, &srv.dbloggers),
		httpCtxt.stop(),
		&srv.logsinkRateLimitConfig,
	)
	add("/model/:modeluuid/logsink", srv.trackRequests(logSinkHandler))

	// We don't need to save the migrated logs to a logfile as well as to the DB.
	logTransferHandler := logsink.NewHTTPHandler(
		newMigrationLogWriteCloserFunc(httpCtxt, &srv.dbloggers),
		httpCtxt.stop(),
		nil, // no rate-limiting
	)
	add("/migrate/logtransfer", srv.trackRequests(logTransferHandler))

	modelRestHandler := &modelRestHandler{
		ctxt:          httpCtxt,
		dataDir:       srv.dataDir,
		stateAuthFunc: httpCtxt.stateForRequestAuthenticatedUser,
	}
	modelRestServer := &RestHTTPHandler{
		GetHandler: modelRestHandler.ServeGet,
	}
	add("/model/:modeluuid/rest/1.0/:entity/:name/:attribute", modelRestServer)

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

	add("/model/:modeluuid/applications/:application/resources/:resource", &ResourcesHandler{
		StateAuthFunc: func(req *http.Request, tagKinds ...string) (ResourcesBackend, state.StatePoolReleaser, names.Tag, error) {
			st, closer, entity, err := httpCtxt.stateForRequestAuthenticatedTag(req, tagKinds...)
			if err != nil {
				return nil, nil, nil, errors.Trace(err)
			}
			rst, err := st.Resources()
			if err != nil {
				return nil, nil, nil, errors.Trace(err)
			}
			return rst, closer, entity.Tag(), nil
		},
	})
	add("/model/:modeluuid/units/:unit/resources/:resource", &UnitResourcesHandler{
		NewOpener: func(req *http.Request, tagKinds ...string) (resource.Opener, state.StatePoolReleaser, error) {
			st, closer, _, err := httpCtxt.stateForRequestAuthenticatedTag(req, tagKinds...)
			if err != nil {
				return nil, nil, errors.Trace(err)
			}
			tagStr := req.URL.Query().Get(":unit")
			tag, err := names.ParseUnitTag(tagStr)
			if err != nil {
				return nil, nil, errors.Trace(err)
			}
			opener, err := resourceadapters.NewResourceOpener(st, tag.Id())
			if err != nil {
				return nil, nil, errors.Trace(err)
			}
			return opener, closer, nil
		},
	})

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
		&resourcesMigrationUploadHandler{
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

	// GUI related paths.
	endpoints = append(endpoints, guiEndpoints(guiURLPathPrefix, srv.dataDir, httpCtxt)...)
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
	localLoginHandlers := &localLoginHandlers{srv.loginAuthCtxt, srv.statePool.SystemState()}
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

	// Add HTTP handlers for application offer macaroon authentication.
	appOfferHandler := &localOfferAuthHandler{authCtx: srv.offerAuthCtxt}
	appOfferDischargeMux := http.NewServeMux()
	httpbakery.AddDischargeHandler(
		appOfferDischargeMux,
		localOfferAccessLocationPath,
		// Sadly we need a type assertion since the method doesn't accept an interface.
		srv.offerAuthCtxt.ThirdPartyBakeryService().(*bakery.Service),
		appOfferHandler.checkThirdPartyCaveat,
	)
	add(localOfferAccessLocationPath+"/discharge", appOfferDischargeMux)
	add(localOfferAccessLocationPath+"/publickey", appOfferDischargeMux)

	return endpoints
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
	atomic.AddInt64(&srv.totalConn, 1)
	addCount := func(delta int64) {
		atomic.AddInt64(&srv.connCount, delta)
	}

	addCount(1)
	defer addCount(-1)

	connectionID := atomic.AddUint64(&srv.lastConnectionID, 1)

	apiObserver := srv.newObserver()
	apiObserver.Join(req, connectionID)
	defer apiObserver.Leave()

	websocket.Serve(w, req, func(conn *websocket.Conn) {
		modelUUID := req.URL.Query().Get(":modeluuid")
		logger.Tracef("got a request for model %q", modelUUID)
		if err := srv.serveConn(
			req.Context(),
			conn,
			modelUUID,
			connectionID,
			apiObserver,
			req.Host,
		); err != nil {
			logger.Errorf("error serving RPCs: %v", err)
		}
	})
}

func (srv *Server) serveConn(
	ctx context.Context,
	wsConn *websocket.Conn,
	modelUUID string,
	connectionID uint64,
	apiObserver observer.Observer,
	host string,
) error {
	codec := jsoncodec.NewWebsocket(wsConn.Conn)
	recorderFactory := observer.NewRecorderFactory(apiObserver, nil)
	conn := rpc.NewConn(codec, recorderFactory)

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
		releaser state.StatePoolReleaser
	)
	if err == nil {
		st, releaser, err = srv.statePool.Get(resolvedModelUUID)
	}

	if err == nil {
		defer releaser()
		h, err = newAPIHandler(srv, st, conn, modelUUID, connectionID, host)
	}

	if err != nil {
		conn.ServeRoot(&errRoot{errors.Trace(err)}, recorderFactory, serverError)
	} else {
		// Set up the admin apis used to accept logins and direct
		// requests to the relevant business facade.
		// There may be more than one since we need a new API each
		// time login changes in a non-backwards compatible way.
		adminAPIs := make(map[int]interface{})
		for apiVersion, factory := range adminAPIFactories {
			adminAPIs[apiVersion] = factory(srv, h, apiObserver)
		}
		conn.ServeRoot(newAdminRoot(h, adminAPIs), recorderFactory, serverError)
	}
	conn.Start(ctx)
	select {
	case <-conn.Dead():
	case <-srv.tomb.Dying():
	}
	return conn.Close()
}

// publicDNSName returns the current public hostname.
func (srv *Server) publicDNSName() string {
	srv.mu.Lock()
	defer srv.mu.Unlock()
	return srv.publicDNSName_
}

// localCertificate returns the local server certificate and reports
// whether it should be used to serve a connection addressed to the
// given server name.
func (srv *Server) localCertificate(serverName string) (*tls.Certificate, bool) {
	cert := srv.getCertificate()
	if net.ParseIP(serverName) != nil {
		// IP address connections always use the local certificate.
		return cert, true
	}
	if !strings.Contains(serverName, ".") {
		// If the server name doesn't contain a period there's no
		// way we can obtain a certificate for it.
		// This applies to the common case where "juju-apiserver" is
		// used as the server name.
		return cert, true
	}
	// Perhaps the server name is explicitly mentioned by the server certificate.
	for _, name := range cert.Leaf.DNSNames {
		if name == serverName {
			return cert, true
		}
	}
	return cert, false
}

func serverError(err error) error {
	if err := common.ServerError(err); err != nil {
		return err
	}
	return nil
}
