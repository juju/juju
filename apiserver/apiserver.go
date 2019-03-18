// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"context"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/pubsub"
	"github.com/juju/utils"
	"github.com/prometheus/client_golang/prometheus"
	"gopkg.in/juju/names.v2"
	"gopkg.in/juju/worker.v1/dependency"
	"gopkg.in/macaroon-bakery.v2-unstable/bakery"
	"gopkg.in/macaroon-bakery.v2-unstable/httpbakery"
	"gopkg.in/tomb.v2"

	"github.com/juju/juju/apiserver/apiserverhttp"
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/common/apihttp"
	"github.com/juju/juju/apiserver/common/crossmodel"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/httpcontext"
	"github.com/juju/juju/apiserver/logsink"
	"github.com/juju/juju/apiserver/observer"
	"github.com/juju/juju/apiserver/websocket"
	"github.com/juju/juju/core/auditlog"
	"github.com/juju/juju/core/cache"
	"github.com/juju/juju/core/lease"
	"github.com/juju/juju/core/presence"
	"github.com/juju/juju/pubsub/apiserver"
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
	tomb      tomb.Tomb
	clock     clock.Clock
	pingClock clock.Clock
	wg        sync.WaitGroup

	shared *sharedServerContext

	// tag of the machine where the API server is running.
	tag                    names.Tag
	dataDir                string
	logDir                 string
	limiter                utils.Limiter
	loginRetryPause        time.Duration
	facades                *facade.Registry
	authenticator          httpcontext.LocalMacaroonAuthenticator
	offerAuthCtxt          *crossmodel.AuthContext
	lastConnectionID       uint64
	newObserver            observer.ObserverFactory
	allowModelAccess       bool
	logSinkWriter          io.WriteCloser
	logsinkRateLimitConfig logsink.RateLimitConfig
	dbloggers              dbloggers
	getAuditConfig         func() auditlog.Config
	upgradeComplete        func() bool
	restoreStatus          func() state.RestoreStatus
	mux                    *apiserverhttp.Mux
	metricsCollector       *Collector

	// mu guards the fields below it.
	mu sync.Mutex

	// publicDNSName_ holds the value that will be returned in
	// LoginResult.PublicDNSName. Currently this is set once and does
	// not change but in the future it may change when a server
	// certificate is explicitly set, hence it's here guarded by the
	// mutex.
	publicDNSName_ string

	// registerIntrospectionHandlers is a function that will
	// call a function with (path, http.Handler) tuples. This
	// is to support registering the handlers underneath the
	// "/introspection" prefix.
	registerIntrospectionHandlers func(func(string, http.Handler))
}

// ServerConfig holds parameters required to set up an API server.
type ServerConfig struct {
	Clock         clock.Clock
	PingClock     clock.Clock
	Tag           names.Tag
	DataDir       string
	LogDir        string
	Hub           *pubsub.StructuredHub
	Presence      presence.Recorder
	Mux           *apiserverhttp.Mux
	Authenticator httpcontext.LocalMacaroonAuthenticator

	// StatePool is the StatePool used for looking up State
	// to pass to facades. StatePool will not be closed by the
	// server; it is the callers responsibility to close it
	// after the apiserver has exited.
	StatePool *state.StatePool

	// Controller is the in-memory representation of the models
	// in the controller. It is kept up to date with an all model
	// watcher and the modelcache worker.
	Controller *cache.Controller

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

	// PublicDNSName is reported to the API clients who connect.
	PublicDNSName string

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

	// RateLimitConfig holds parameters to control
	// aspects of rate limiting connections and logins.
	RateLimitConfig RateLimitConfig

	// LogSinkConfig holds parameters to control the API server's
	// logsink endpoint behaviour. If this is nil, the values from
	// DefaultLogSinkConfig() will be used.
	LogSinkConfig *LogSinkConfig

	// GetAuditConfig holds a function that returns the current audit
	// logging config. The function may return updated values, so
	// should be called every time a new login is handled.
	GetAuditConfig func() auditlog.Config

	// LeaseManager gives access to leadership and singular claimers
	// and checkers for use in API facades.
	LeaseManager lease.Manager

	// MetricsCollector defines all the metrics to be collected for the
	// apiserver
	MetricsCollector *Collector
}

// Validate validates the API server configuration.
func (c ServerConfig) Validate() error {
	if c.StatePool == nil {
		return errors.NotValidf("missing StatePool")
	}
	if c.Controller == nil {
		return errors.NotValidf("missing Controller")
	}
	if c.Hub == nil {
		return errors.NotValidf("missing Hub")
	}
	if c.Presence == nil {
		return errors.NotValidf("missing Presence")
	}
	if c.Mux == nil {
		return errors.NotValidf("missing Mux")
	}
	if c.Authenticator == nil {
		return errors.NotValidf("missing Authenticator")
	}
	if c.Clock == nil {
		return errors.NotValidf("missing Clock")
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
	if c.GetAuditConfig == nil {
		return errors.NotValidf("missing GetAuditConfig")
	}
	if err := c.RateLimitConfig.Validate(); err != nil {
		return errors.Annotate(err, "validating rate limit configuration")
	}
	if c.LogSinkConfig != nil {
		if err := c.LogSinkConfig.Validate(); err != nil {
			return errors.Annotate(err, "validating logsink configuration")
		}
	}
	if c.MetricsCollector == nil {
		return errors.NotValidf("missing MetricsCollector")
	}
	return nil
}

func (c ServerConfig) pingClock() clock.Clock {
	if c.PingClock == nil {
		return c.Clock
	}
	return c.PingClock
}

// NewServer serves API requests using the given configuration.
func NewServer(cfg ServerConfig) (*Server, error) {
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
	return newServer(cfg)
}

const readyTimeout = time.Second * 30

func newServer(cfg ServerConfig) (_ *Server, err error) {
	limiter := utils.NewLimiterWithPause(
		cfg.RateLimitConfig.LoginRateLimit, cfg.RateLimitConfig.LoginMinPause,
		cfg.RateLimitConfig.LoginMaxPause, clock.WallClock)

	shared, err := newSharedServerContex(sharedServerConfig{
		statePool:    cfg.StatePool,
		controller:   cfg.Controller,
		centralHub:   cfg.Hub,
		presence:     cfg.Presence,
		leaseManager: cfg.LeaseManager,
		logger:       loggo.GetLogger("juju.apiserver"),
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	srv := &Server{
		clock:                         cfg.Clock,
		pingClock:                     cfg.pingClock(),
		newObserver:                   cfg.NewObserver,
		shared:                        shared,
		tag:                           cfg.Tag,
		dataDir:                       cfg.DataDir,
		logDir:                        cfg.LogDir,
		limiter:                       limiter,
		loginRetryPause:               cfg.RateLimitConfig.LoginRetryPause,
		upgradeComplete:               cfg.UpgradeComplete,
		restoreStatus:                 cfg.RestoreStatus,
		facades:                       AllFacades(),
		mux:                           cfg.Mux,
		authenticator:                 cfg.Authenticator,
		allowModelAccess:              cfg.AllowModelAccess,
		publicDNSName_:                cfg.PublicDNSName,
		registerIntrospectionHandlers: cfg.RegisterIntrospectionHandlers,
		logsinkRateLimitConfig: logsink.RateLimitConfig{
			Refill: cfg.LogSinkConfig.RateLimitRefill,
			Burst:  cfg.LogSinkConfig.RateLimitBurst,
			Clock:  cfg.Clock,
		},
		getAuditConfig: cfg.GetAuditConfig,
		dbloggers: dbloggers{
			clock:                 cfg.Clock,
			dbLoggerBufferSize:    cfg.LogSinkConfig.DBLoggerBufferSize,
			dbLoggerFlushInterval: cfg.LogSinkConfig.DBLoggerFlushInterval,
		},
		metricsCollector: cfg.MetricsCollector,
	}

	// The auth context for authenticating access to application offers.
	srv.offerAuthCtxt, err = newOfferAuthcontext(cfg.StatePool)
	if err != nil {
		return nil, errors.Trace(err)
	}

	model, err := srv.shared.statePool.SystemState().Model()
	if err != nil {
		return nil, errors.Trace(err)
	}
	if model.Type() == state.ModelTypeCAAS {
		// CAAS controller writes log to stdOut.
		srv.logSinkWriter = os.Stdout
	} else {
		srv.logSinkWriter, err = logsink.NewFileWriter(filepath.Join(srv.logDir, "logsink.log"))
		if err != nil {
			return nil, errors.Annotate(err, "creating logsink writer")
		}
	}

	unsubscribe, err := cfg.Hub.Subscribe(apiserver.RestartTopic, func(string, map[string]interface{}) {
		srv.tomb.Kill(dependency.ErrBounce)
	})
	if err != nil {
		return nil, errors.Annotate(err, "unable to subscribe to restart message")
	}

	ready := make(chan struct{})
	srv.tomb.Go(func() error {
		defer srv.dbloggers.dispose()
		defer srv.logSinkWriter.Close()
		defer srv.shared.Close()
		defer unsubscribe()
		return srv.loop(ready)
	})

	// Don't return until all handlers have been registered.
	select {
	case <-ready:
	case <-srv.clock.After(readyTimeout):
		return nil, errors.New("loop never signalled ready")
	}

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

// logsinkMetricsCollectorWrapper defines a wrapper for exposing the essentials
// for the logsink api handler to interact with the metrics collector.
type logsinkMetricsCollectorWrapper struct {
	collector *Collector
}

func (w logsinkMetricsCollectorWrapper) TotalConnections() prometheus.Counter {
	return w.collector.TotalConnections
}

func (w logsinkMetricsCollectorWrapper) Connections() prometheus.Gauge {
	return w.collector.APIConnections.WithLabelValues("logsink")
}

func (w logsinkMetricsCollectorWrapper) PingFailureCount(modelUUID string) prometheus.Counter {
	return w.collector.PingFailureCount.WithLabelValues(modelUUID, "logsink")
}

func (w logsinkMetricsCollectorWrapper) LogWriteCount(modelUUID, state string) prometheus.Counter {
	return w.collector.LogWriteCount.WithLabelValues(modelUUID, state)
}

func (w logsinkMetricsCollectorWrapper) LogReadCount(modelUUID, state string) prometheus.Counter {
	return w.collector.LogReadCount.WithLabelValues(modelUUID, state)
}

// loop is the main loop for the server.
func (srv *Server) loop(ready chan struct{}) error {
	// for pat based handlers, they are matched in-order of being
	// registered, first match wins. So more specific ones have to be
	// registered first.
	for _, ep := range srv.endpoints() {
		srv.mux.AddHandler(ep.Method, ep.Pattern, ep.Handler)
		defer srv.mux.RemoveHandler(ep.Method, ep.Pattern)
		if ep.Method == "GET" {
			srv.mux.AddHandler("HEAD", ep.Pattern, ep.Handler)
			defer srv.mux.RemoveHandler("HEAD", ep.Pattern)
		}
	}
	close(ready)
	<-srv.tomb.Dying()
	srv.wg.Wait() // wait for any outstanding requests to complete.
	return tomb.ErrDying
}

func (srv *Server) endpoints() []apihttp.Endpoint {
	const modelRoutePrefix = "/model/:modeluuid"

	type handler struct {
		pattern         string
		methods         []string
		handler         http.Handler
		unauthenticated bool
		authorizer      httpcontext.Authorizer
		tracked         bool
		noModelUUID     bool
	}
	var endpoints []apihttp.Endpoint
	controllerModelUUID := srv.shared.statePool.SystemState().ModelUUID()
	addHandler := func(handler handler) {
		methods := handler.methods
		if methods == nil {
			methods = defaultHTTPMethods
		}
		h := handler.handler
		if handler.tracked {
			h = srv.trackRequests(h)
		}
		if !handler.unauthenticated {
			h = &httpcontext.BasicAuthHandler{
				Handler:       h,
				Authenticator: srv.authenticator,
				Authorizer:    handler.authorizer,
			}
		}
		if !handler.noModelUUID {
			if strings.HasPrefix(handler.pattern, modelRoutePrefix) {
				h = &httpcontext.QueryModelHandler{
					Handler: h,
					Query:   ":modeluuid",
				}
			} else {
				h = &httpcontext.ImpliedModelHandler{
					Handler:   h,
					ModelUUID: controllerModelUUID,
				}
			}
		}
		for _, method := range methods {
			endpoints = append(endpoints, apihttp.Endpoint{
				Pattern: handler.pattern,
				Method:  method,
				Handler: h,
			})
		}
	}

	httpCtxt := httpContext{srv: srv}
	mainAPIHandler := http.HandlerFunc(srv.apiHandler)
	logStreamHandler := newLogStreamEndpointHandler(httpCtxt)
	debugLogHandler := newDebugLogDBHandler(
		httpCtxt, srv.authenticator,
		tagKindAuthorizer{names.MachineTagKind, names.UserTagKind, names.ApplicationTagKind})
	pubsubHandler := newPubSubHandler(httpCtxt, srv.shared.centralHub)
	logSinkHandler := logsink.NewHTTPHandler(
		newAgentLogWriteCloserFunc(httpCtxt, srv.logSinkWriter, &srv.dbloggers),
		httpCtxt.stop(),
		&srv.logsinkRateLimitConfig,
		logsinkMetricsCollectorWrapper{collector: srv.metricsCollector},
		controllerModelUUID,
	)
	logSinkAuthorizer := tagKindAuthorizer{names.MachineTagKind, names.UnitTagKind, names.ApplicationTagKind}
	logTransferHandler := logsink.NewHTTPHandler(
		// We don't need to save the migrated logs
		// to a logfile as well as to the DB.
		newMigrationLogWriteCloserFunc(httpCtxt, &srv.dbloggers),
		httpCtxt.stop(),
		nil, // no rate-limiting
		logsinkMetricsCollectorWrapper{collector: srv.metricsCollector},
		controllerModelUUID,
	)
	modelRestHandler := &modelRestHandler{
		ctxt:          httpCtxt,
		dataDir:       srv.dataDir,
		stateAuthFunc: httpCtxt.stateForRequestAuthenticatedUser,
	}
	modelRestServer := &RestHTTPHandler{
		GetHandler: modelRestHandler.ServeGet,
	}
	modelCharmsHandler := &charmsHandler{
		ctxt:          httpCtxt,
		dataDir:       srv.dataDir,
		stateAuthFunc: httpCtxt.stateForRequestAuthenticatedUser,
	}
	modelCharmsHTTPHandler := &CharmsHTTPHandler{
		PostHandler: modelCharmsHandler.ServePost,
		GetHandler:  modelCharmsHandler.ServeGet,
	}
	modelCharmsUploadAuthorizer := tagKindAuthorizer{names.UserTagKind}
	modelToolsUploadHandler := &toolsUploadHandler{
		ctxt:          httpCtxt,
		stateAuthFunc: httpCtxt.stateForRequestAuthenticatedUser,
	}
	modelToolsUploadAuthorizer := tagKindAuthorizer{names.UserTagKind}
	modelToolsDownloadHandler := &toolsDownloadHandler{
		ctxt: httpCtxt,
	}
	resourcesHandler := &ResourcesHandler{
		StateAuthFunc: func(req *http.Request, tagKinds ...string) (ResourcesBackend, state.PoolHelper, names.Tag, error) {
			st, entity, err := httpCtxt.stateForRequestAuthenticatedTag(req, tagKinds...)
			if err != nil {
				return nil, nil, nil, errors.Trace(err)
			}
			rst, err := st.Resources()
			if err != nil {
				return nil, nil, nil, errors.Trace(err)
			}
			return rst, st, entity.Tag(), nil
		},
	}
	unitResourcesHandler := &UnitResourcesHandler{
		NewOpener: func(req *http.Request, tagKinds ...string) (resource.Opener, state.PoolHelper, error) {
			st, _, err := httpCtxt.stateForRequestAuthenticatedTag(req, tagKinds...)
			if err != nil {
				return nil, nil, errors.Trace(err)
			}
			tagStr := req.URL.Query().Get(":unit")
			tag, err := names.ParseUnitTag(tagStr)
			if err != nil {
				return nil, nil, errors.Trace(err)
			}
			opener, err := resourceadapters.NewResourceOpener(st.State, tag.Id())
			if err != nil {
				return nil, nil, errors.Trace(err)
			}
			return opener, st, nil
		},
	}
	controllerAdminAuthorizer := controllerAdminAuthorizer{srv.shared.statePool.SystemState()}
	migrateCharmsHandler := &charmsHandler{
		ctxt:          httpCtxt,
		dataDir:       srv.dataDir,
		stateAuthFunc: httpCtxt.stateForMigrationImporting,
	}
	migrateCharmsHTTPHandler := &CharmsHTTPHandler{
		PostHandler: migrateCharmsHandler.ServePost,
		GetHandler:  migrateCharmsHandler.ServeUnsupported,
	}
	migrateToolsUploadHandler := &toolsUploadHandler{
		ctxt:          httpCtxt,
		stateAuthFunc: httpCtxt.stateForMigrationImporting,
	}
	resourcesMigrationUploadHandler := &resourcesMigrationUploadHandler{
		ctxt:          httpCtxt,
		stateAuthFunc: httpCtxt.stateForMigrationImporting,
	}
	backupHandler := &backupHandler{ctxt: httpCtxt}
	registerHandler := &registerUserHandler{ctxt: httpCtxt}
	guiArchiveHandler := &guiArchiveHandler{ctxt: httpCtxt}
	guiVersionHandler := &guiVersionHandler{ctxt: httpCtxt}

	// HTTP handler for application offer macaroon authentication.
	appOfferHandler := &localOfferAuthHandler{authCtx: srv.offerAuthCtxt}
	appOfferDischargeMux := http.NewServeMux()
	httpbakery.AddDischargeHandler(
		appOfferDischargeMux,
		localOfferAccessLocationPath,
		// Sadly we need a type assertion since the method doesn't accept an interface.
		srv.offerAuthCtxt.ThirdPartyBakeryService().(*bakery.Service),
		appOfferHandler.checkThirdPartyCaveat,
	)

	handlers := []handler{{
		// This handler is model specific even though it only
		// ever makes sense for a controller because the API
		// caller that is handed to the worker that is forwarding
		// the messages between controllers is bound to the
		// /model/:modeluuid namespace.
		pattern:    modelRoutePrefix + "/pubsub",
		handler:    pubsubHandler,
		tracked:    true,
		authorizer: controllerAuthorizer{},
	}, {
		pattern: modelRoutePrefix + "/logstream",
		handler: logStreamHandler,
		tracked: true,
	}, {
		pattern: modelRoutePrefix + "/log",
		handler: debugLogHandler,
		tracked: true,
		// The authentication is handled within the debugLogHandler in order
		// for discharge required errors to be handled correctly.
		unauthenticated: true,
	}, {
		pattern:    modelRoutePrefix + "/logsink",
		handler:    logSinkHandler,
		tracked:    true,
		authorizer: logSinkAuthorizer,
	}, {
		pattern:         modelRoutePrefix + "/api",
		handler:         mainAPIHandler,
		tracked:         true,
		unauthenticated: true,
	}, {
		pattern: modelRoutePrefix + "/rest/1.0/:entity/:name/:attribute",
		handler: modelRestServer,
	}, {
		// GET /charms has no authorizer
		pattern: modelRoutePrefix + "/charms",
		methods: []string{"GET"},
		handler: modelCharmsHTTPHandler,
	}, {
		pattern:    modelRoutePrefix + "/charms",
		methods:    []string{"POST"},
		handler:    modelCharmsHTTPHandler,
		authorizer: modelCharmsUploadAuthorizer,
	}, {
		pattern:    modelRoutePrefix + "/tools",
		handler:    modelToolsUploadHandler,
		authorizer: modelToolsUploadAuthorizer,
	}, {
		pattern:         modelRoutePrefix + "/tools/:version",
		handler:         modelToolsDownloadHandler,
		unauthenticated: true,
	}, {
		pattern: modelRoutePrefix + "/applications/:application/resources/:resource",
		handler: resourcesHandler,
	}, {
		pattern: modelRoutePrefix + "/units/:unit/resources/:resource",
		handler: unitResourcesHandler,
	}, {
		pattern: modelRoutePrefix + "/backups",
		handler: backupHandler,
	}, {
		pattern:    "/migrate/charms",
		handler:    migrateCharmsHTTPHandler,
		authorizer: controllerAdminAuthorizer,
	}, {
		pattern:    "/migrate/tools",
		handler:    migrateToolsUploadHandler,
		authorizer: controllerAdminAuthorizer,
	}, {
		pattern:    "/migrate/resources",
		handler:    resourcesMigrationUploadHandler,
		authorizer: controllerAdminAuthorizer,
	}, {
		pattern:    "/migrate/logtransfer",
		handler:    logTransferHandler,
		tracked:    true,
		authorizer: controllerAdminAuthorizer,
	}, {
		pattern:         "/api",
		handler:         mainAPIHandler,
		tracked:         true,
		unauthenticated: true,
		noModelUUID:     true,
	}, {
		// Serve the API at / for backward compatibility. Note that the
		// pat muxer special-cases / so that it does not serve all
		// possible endpoints, but only / itself.
		pattern:         "/",
		handler:         mainAPIHandler,
		tracked:         true,
		unauthenticated: true,
		noModelUUID:     true,
	}, {
		pattern:         "/register",
		handler:         registerHandler,
		unauthenticated: true,
	}, {
		pattern:    "/tools",
		handler:    modelToolsUploadHandler,
		authorizer: modelToolsUploadAuthorizer,
	}, {
		pattern:         "/tools/:version",
		handler:         modelToolsDownloadHandler,
		unauthenticated: true,
	}, {
		pattern: "/log",
		handler: debugLogHandler,
		tracked: true,
		// The authentication is handled within the debugLogHandler in order
		// for discharge required errors to be handled correctly.
		unauthenticated: true,
	}, {
		// GET /charms has no authorizer
		pattern: "/charms",
		methods: []string{"GET"},
		handler: modelCharmsHTTPHandler,
	}, {
		pattern:    "/charms",
		methods:    []string{"POST"},
		handler:    modelCharmsHTTPHandler,
		authorizer: modelCharmsUploadAuthorizer,
	}, {
		pattern: "/gui-archive",
		methods: []string{"POST"},
		handler: guiArchiveHandler,
	}, {
		pattern:         "/gui-archive",
		methods:         []string{"GET"},
		handler:         guiArchiveHandler,
		unauthenticated: true,
	}, {
		pattern: "/gui-version",
		handler: guiVersionHandler,
	}, {
		pattern:         localOfferAccessLocationPath + "/discharge",
		handler:         appOfferDischargeMux,
		unauthenticated: true,
	}, {
		pattern:         localOfferAccessLocationPath + "/publickey",
		handler:         appOfferDischargeMux,
		unauthenticated: true,
	}}
	if srv.registerIntrospectionHandlers != nil {
		add := func(subpath string, h http.Handler) {
			handlers = append(handlers, handler{
				pattern: path.Join("/introspection/", subpath),
				handler: introspectionHandler{httpCtxt, h},
			})
		}
		srv.registerIntrospectionHandlers(add)
	}

	// Construct endpoints from handler structs.
	for _, handler := range handlers {
		addHandler(handler)
	}

	// Finally, register GUI content endpoints.
	guiEndpoints := guiEndpoints(guiURLPathPrefix, srv.dataDir, httpCtxt)
	endpoints = append(endpoints, guiEndpoints...)
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

func (srv *Server) apiHandler(w http.ResponseWriter, req *http.Request) {
	srv.metricsCollector.TotalConnections.Inc()

	gauge := srv.metricsCollector.APIConnections.WithLabelValues("api")
	gauge.Inc()
	defer gauge.Dec()

	// This is the deprecated api connections gauge, note it doesn't have the
	// labels to pivot on.
	// Remove this post 2.6 release
	deprecatedGauge := srv.metricsCollector.DeprecatedAPIConnections
	deprecatedGauge.Inc()
	defer deprecatedGauge.Dec()

	connectionID := atomic.AddUint64(&srv.lastConnectionID, 1)

	apiObserver := srv.newObserver()
	apiObserver.Join(req, connectionID)
	defer apiObserver.Leave()

	websocket.Serve(w, req, func(conn *websocket.Conn) {
		modelUUID := httpcontext.RequestModelUUID(req)
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
	recorderFactory := observer.NewRecorderFactory(
		apiObserver, nil, observer.NoCaptureArgs)
	conn := rpc.NewConn(codec, recorderFactory)

	// Note that we don't overwrite modelUUID here because
	// newAPIHandler treats an empty modelUUID as signifying
	// the API version used.
	resolvedModelUUID := modelUUID
	statePool := srv.shared.statePool
	if modelUUID == "" {
		resolvedModelUUID = statePool.SystemState().ModelUUID()
	}
	var (
		st *state.PooledState
		h  *apiHandler
	)

	st, err := statePool.Get(resolvedModelUUID)
	if err == nil {
		defer st.Release()
		h, err = newAPIHandler(srv, st.State, conn, modelUUID, connectionID, host)
	}
	if errors.IsNotFound(err) {
		err = errors.Wrap(err, common.UnknownModelError(resolvedModelUUID))
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

func serverError(err error) error {
	return common.ServerError(err)
}

// GetAuditConfig returns a copy of the current audit logging
// configuration.
func (srv *Server) GetAuditConfig() auditlog.Config {
	// Delegates to the getter passed in.
	return srv.getAuditConfig()
}
