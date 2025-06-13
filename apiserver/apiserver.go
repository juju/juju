// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names/v5"
	"github.com/juju/pubsub/v2"
	"github.com/juju/ratelimit"
	"github.com/juju/worker/v3/dependency"
	"github.com/prometheus/client_golang/prometheus"
	"gopkg.in/tomb.v2"

	"github.com/juju/juju/apiserver/apiserverhttp"
	"github.com/juju/juju/apiserver/authentication"
	"github.com/juju/juju/apiserver/authentication/jwt"
	"github.com/juju/juju/apiserver/authentication/macaroon"
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/common/apihttp"
	"github.com/juju/juju/apiserver/common/crossmodel"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/httpcontext"
	"github.com/juju/juju/apiserver/logsink"
	"github.com/juju/juju/apiserver/observer"
	"github.com/juju/juju/apiserver/stateauthenticator"
	"github.com/juju/juju/apiserver/websocket"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/auditlog"
	"github.com/juju/juju/core/cache"
	coredatabase "github.com/juju/juju/core/database"
	"github.com/juju/juju/core/lease"
	"github.com/juju/juju/core/multiwatcher"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/core/presence"
	"github.com/juju/juju/core/resources"
	"github.com/juju/juju/internal/worker/syslogger"
	"github.com/juju/juju/pubsub/apiserver"
	controllermsg "github.com/juju/juju/pubsub/controller"
	"github.com/juju/juju/resource"
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
	tag     names.Tag
	dataDir string
	logDir  string
	facades *facade.Registry

	localMacaroonAuthenticator macaroon.LocalMacaroonAuthenticator
	jwtAuthenticator           jwt.Authenticator

	httpAuthenticators  []authentication.HTTPAuthenticator
	loginAuthenticators []authentication.LoginAuthenticator

	offerAuthCtxt          *crossmodel.AuthContext
	lastConnectionID       uint64
	newObserver            observer.ObserverFactory
	allowModelAccess       bool
	logSinkWriter          io.WriteCloser
	logsinkRateLimitConfig logsink.RateLimitConfig
	apiServerLoggers       apiServerLoggers
	getAuditConfig         func() auditlog.Config
	upgradeComplete        func() bool
	mux                    *apiserverhttp.Mux
	metricsCollector       *Collector
	execEmbeddedCommand    ExecEmbeddedCommandFunc

	// mu guards the fields below it.
	mu sync.Mutex

	// healthStatus is returned from the health endpoint.
	healthStatus string

	// publicDNSName_ holds the value that will be returned in
	// LoginResult.PublicDNSName. Currently this is set once and does
	// not change but in the future it may change when a server
	// certificate is explicitly set, hence it's here guarded by the
	// mutex.
	publicDNSName_ string

	// agentRateLimitMax and agentRateLimitRate are values used to create
	// the token bucket that ratelimits the agent connections. These values
	// come from controller config, and can be updated on the fly to adjust
	// the rate limiting.
	agentRateLimitMax  int
	agentRateLimitRate time.Duration
	agentRateLimit     *ratelimit.Bucket

	// resourceLock is used to limit the number of
	// concurrent resource downloads to units.
	resourceLock resource.ResourceDownloadLock

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
	Presence  presence.Recorder
	Mux       *apiserverhttp.Mux

	// LocalMacaroonAuthenticator is the request authenticator used for verifying
	// local user macaroons.
	LocalMacaroonAuthenticator macaroon.LocalMacaroonAuthenticator

	// JWTAuthenticator is the request authenticator used for validating jwt
	// tokens when the controller has been bootstrapped with a trusted token
	// provider.
	JWTAuthenticator jwt.Authenticator

	// MultiwatcherFactory is used by the API server to create
	// multiwatchers. The real factory is managed by the multiwatcher
	// worker.
	MultiwatcherFactory multiwatcher.Factory

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

	// LogSinkConfig holds parameters to control the API server's
	// logsink endpoint behaviour. If this is nil, the values from
	// DefaultLogSinkConfig() will be used.
	LogSinkConfig *LogSinkConfig

	// SysLogger is a logger that will tee the output from logging
	// to the local syslog.
	SysLogger syslogger.SysLogger

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

	// ExecEmbeddedCommand is a function which creates an embedded Juju CLI instance.
	ExecEmbeddedCommand ExecEmbeddedCommandFunc

	// CharmhubHTTPClient is the HTTP client used for Charmhub API requests.
	CharmhubHTTPClient facade.HTTPClient

	// DBGetter supplies sql.DB references on request, for named databases.
	DBGetter coredatabase.DBGetter
}

// Validate validates the API server configuration.
func (c ServerConfig) Validate() error {
	if c.StatePool == nil {
		return errors.NotValidf("missing StatePool")
	}
	if c.Controller == nil {
		return errors.NotValidf("missing Controller")
	}
	if c.MultiwatcherFactory == nil {
		return errors.NotValidf("missing MultiwatcherFactory")
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
	if c.LocalMacaroonAuthenticator == nil {
		return errors.NotValidf("missing local macaroon authenticator")
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
	if c.GetAuditConfig == nil {
		return errors.NotValidf("missing GetAuditConfig")
	}
	if c.LogSinkConfig != nil {
		if err := c.LogSinkConfig.Validate(); err != nil {
			return errors.Annotate(err, "validating logsink configuration")
		}
	}
	if c.SysLogger == nil {
		return errors.NotValidf("nil SysLogger")
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
	// any state manipulation may be relying on features of the
	// database added by upgrades. Here be dragons.
	return newServer(cfg)
}

const readyTimeout = time.Second * 30

func newServer(cfg ServerConfig) (_ *Server, err error) {
	systemState, err := cfg.StatePool.SystemState()
	if err != nil {
		return nil, errors.Trace(err)
	}
	controllerConfig, err := systemState.ControllerConfig()
	if err != nil {
		return nil, errors.Annotate(err, "unable to get controller config")
	}

	shared, err := newSharedServerContext(sharedServerConfig{
		statePool:           cfg.StatePool,
		controller:          cfg.Controller,
		multiwatcherFactory: cfg.MultiwatcherFactory,
		centralHub:          cfg.Hub,
		presence:            cfg.Presence,
		leaseManager:        cfg.LeaseManager,
		controllerConfig:    controllerConfig,
		logger:              loggo.GetLogger("juju.apiserver"),
		charmhubHTTPClient:  cfg.CharmhubHTTPClient,
		dbGetter:            cfg.DBGetter,
	})
	if err != nil {
		return nil, errors.Trace(err)
	}

	systemState, err = cfg.StatePool.SystemState()
	if err != nil {
		return nil, errors.Trace(err)
	}
	model, err := systemState.Model()
	if err != nil {
		return nil, errors.Trace(err)
	}

	modelConfig, err := model.Config()
	if err != nil {
		return nil, errors.Trace(err)
	}
	loggingOutputs, _ := modelConfig.LoggingOutput()

	httpAuthenticators := []authentication.HTTPAuthenticator{cfg.LocalMacaroonAuthenticator, cfg.JWTAuthenticator}
	loginAuthenticators := []authentication.LoginAuthenticator{cfg.LocalMacaroonAuthenticator, cfg.JWTAuthenticator}

	srv := &Server{
		clock:                         cfg.Clock,
		pingClock:                     cfg.pingClock(),
		newObserver:                   cfg.NewObserver,
		shared:                        shared,
		tag:                           cfg.Tag,
		dataDir:                       cfg.DataDir,
		logDir:                        cfg.LogDir,
		upgradeComplete:               cfg.UpgradeComplete,
		facades:                       AllFacades(),
		mux:                           cfg.Mux,
		localMacaroonAuthenticator:    cfg.LocalMacaroonAuthenticator,
		jwtAuthenticator:              cfg.JWTAuthenticator,
		httpAuthenticators:            httpAuthenticators,
		loginAuthenticators:           loginAuthenticators,
		allowModelAccess:              cfg.AllowModelAccess,
		publicDNSName_:                cfg.PublicDNSName,
		registerIntrospectionHandlers: cfg.RegisterIntrospectionHandlers,
		logsinkRateLimitConfig: logsink.RateLimitConfig{
			Refill: cfg.LogSinkConfig.RateLimitRefill,
			Burst:  cfg.LogSinkConfig.RateLimitBurst,
			Clock:  cfg.Clock,
		},
		getAuditConfig: cfg.GetAuditConfig,
		apiServerLoggers: apiServerLoggers{
			syslogger:           cfg.SysLogger,
			loggingOutputs:      loggingOutputs,
			clock:               cfg.Clock,
			loggerBufferSize:    cfg.LogSinkConfig.DBLoggerBufferSize,
			loggerFlushInterval: cfg.LogSinkConfig.DBLoggerFlushInterval,
		},
		metricsCollector:    cfg.MetricsCollector,
		execEmbeddedCommand: cfg.ExecEmbeddedCommand,

		healthStatus: "starting",
	}
	srv.updateAgentRateLimiter(controllerConfig)
	srv.updateResourceDownloadLimiters(controllerConfig)

	// We are able to get the current controller config before subscribing to changes
	// because the changes are only ever published in response to an API call,
	// and we know that we can't make any API calls until the server has started.
	unsubscribeControllerConfig, err := cfg.Hub.Subscribe(
		controllermsg.ConfigChanged,
		func(topic string, data controllermsg.ConfigChangedMessage, err error) {
			if err != nil {
				logger.Criticalf("programming error in %s message data: %v", topic, err)
				return
			}
			srv.updateAgentRateLimiter(data.Config)
			srv.updateResourceDownloadLimiters(data.Config)
		})
	if err != nil {
		logger.Criticalf("programming error in subscribe function: %v", err)
		return nil, errors.Trace(err)
	}

	srv.shared.cancel = srv.tomb.Dying()

	// The auth context for authenticating access to application offers.
	srv.offerAuthCtxt, err = newOfferAuthcontext(cfg.StatePool)
	if err != nil {
		unsubscribeControllerConfig()
		return nil, errors.Trace(err)
	}

	if model.Type() == state.ModelTypeCAAS {
		// CAAS controller writes log to stdout. We should ensure that we don't
		// close the logSinkWriter when we stopping the tomb, otherwise we get
		// no output to stdout anymore.
		srv.logSinkWriter = nonCloseableWriter{
			WriteCloser: os.Stdout,
		}
	} else {
		srv.logSinkWriter, err = logsink.NewFileWriter(
			filepath.Join(srv.logDir, "logsink.log"),
			controllerConfig.AgentLogfileMaxSizeMB(),
			controllerConfig.AgentLogfileMaxBackups(),
		)
		if err != nil {
			return nil, errors.Annotate(err, "creating logsink writer")
		}
	}

	unsubscribe, err := cfg.Hub.Subscribe(apiserver.RestartTopic, func(string, map[string]interface{}) {
		srv.tomb.Kill(dependency.ErrBounce)
	})
	if err != nil {
		unsubscribeControllerConfig()
		return nil, errors.Annotate(err, "unable to subscribe to restart message")
	}

	ready := make(chan struct{})
	srv.tomb.Go(func() error {
		defer srv.apiServerLoggers.dispose()
		defer srv.logSinkWriter.Close()
		defer srv.shared.Close()
		defer unsubscribe()
		defer unsubscribeControllerConfig()
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

// nonCloseableWriter ensures that we never close the underlying writer. If the
// underlying writer is os.stdout and we close that, then nothing will be
// written until a new instance of the program is launched.
type nonCloseableWriter struct {
	io.WriteCloser
}

// Close does not do anything in this instance.
func (nonCloseableWriter) Close() error {
	return nil
}

// Report is shown in the juju_engine_report.
func (srv *Server) Report() map[string]interface{} {
	srv.mu.Lock()
	defer srv.mu.Unlock()
	result := map[string]interface{}{
		"agent-ratelimit-max":  srv.agentRateLimitMax,
		"agent-ratelimit-rate": srv.agentRateLimitRate,
	}

	if srv.publicDNSName_ != "" {
		result["public-dns-name"] = srv.publicDNSName_
	}
	return result
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

func (srv *Server) updateAgentRateLimiter(cfg controller.Config) {
	srv.mu.Lock()
	defer srv.mu.Unlock()
	srv.agentRateLimitMax = cfg.AgentRateLimitMax()
	srv.agentRateLimitRate = cfg.AgentRateLimitRate()
	if srv.agentRateLimitMax > 0 {
		srv.agentRateLimit = ratelimit.NewBucketWithClock(
			srv.agentRateLimitRate, int64(srv.agentRateLimitMax), rateClock{srv.clock})
	} else {
		srv.agentRateLimit = nil
	}
}

func (srv *Server) updateResourceDownloadLimiters(cfg controller.Config) {
	srv.mu.Lock()
	defer srv.mu.Unlock()
	globalLimit := cfg.ControllerResourceDownloadLimit()
	appLimit := cfg.ApplicationResourceDownloadLimit()
	srv.resourceLock = resource.NewResourceDownloadLimiter(globalLimit, appLimit)
}

func (srv *Server) getResourceDownloadLimiter() resource.ResourceDownloadLock {
	srv.mu.Lock()
	defer srv.mu.Unlock()
	return srv.resourceLock
}

type rateClock struct {
	clock.Clock
}

func (rateClock) Sleep(time.Duration) {
	// no-op, we don't sleep.
}

func (srv *Server) getAgentToken() error {
	srv.mu.Lock()
	defer srv.mu.Unlock()
	// agentRateLimit is nil if rate limiting is disabled.
	if srv.agentRateLimit == nil {
		return nil
	}

	// Try to take one token, but don't wait any time for it.
	if _, ok := srv.agentRateLimit.TakeMaxDuration(1, 0); !ok {
		return apiservererrors.ErrTryAgain
	}
	return nil
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

// httpRequestRecorderWrapper defines a wrapper from exposing the
// essentials for the http request recorder.
type httpRequestRecorderWrapper struct {
	collector *Collector
	modelUUID string
}

// Record an outgoing request which produced an http.Response.
func (w httpRequestRecorderWrapper) Record(method string, url *url.URL, res *http.Response, rtt time.Duration) {
	// Note: Do not log url.Path as REST queries _can_ include the name of the
	// entities (charms, architectures, etc).
	w.collector.TotalRequests.WithLabelValues(w.modelUUID, url.Host, strconv.FormatInt(int64(res.StatusCode), 10)).Inc()
	if res.StatusCode >= 400 {
		w.collector.TotalRequestErrors.WithLabelValues(w.modelUUID, url.Host).Inc()
	}
	w.collector.TotalRequestsDuration.WithLabelValues(w.modelUUID, url.Host).Observe(rtt.Seconds())
}

// RecordError records an outgoing request that returned back an error.
func (w httpRequestRecorderWrapper) RecordError(method string, url *url.URL, err error) {
	// Note: Do not log url.Path as REST queries _can_ include the name of the
	// entities (charms, architectures, etc).
	w.collector.TotalRequests.WithLabelValues(w.modelUUID, url.Host, "unknown").Inc()
	w.collector.TotalRequestErrors.WithLabelValues(w.modelUUID, url.Host).Inc()
}

// loop is the main loop for the server.
func (srv *Server) loop(ready chan struct{}) error {
	// for pat based handlers, they are matched in-order of being
	// registered, first match wins. So more specific ones have to be
	// registered first.
	endpoints, err := srv.endpoints()
	if err != nil {
		return errors.Trace(err)
	}
	for _, ep := range endpoints {
		_ = srv.mux.AddHandler(ep.Method, ep.Pattern, ep.Handler)
		defer srv.mux.RemoveHandler(ep.Method, ep.Pattern)
		if ep.Method == "GET" {
			_ = srv.mux.AddHandler("HEAD", ep.Pattern, ep.Handler)
			defer srv.mux.RemoveHandler("HEAD", ep.Pattern)
		}
	}

	close(ready)
	srv.mu.Lock()
	srv.healthStatus = "running"
	srv.mu.Unlock()

	<-srv.tomb.Dying()

	srv.mu.Lock()
	srv.healthStatus = "stopping"
	srv.mu.Unlock()

	srv.wg.Wait() // wait for any outstanding requests to complete.
	return tomb.ErrDying
}

func (srv *Server) endpoints() ([]apihttp.Endpoint, error) {
	const modelRoutePrefix = "/model/:modeluuid"
	const charmsObjectsRoutePrefix = "/model-:modeluuid/charms/:object"

	type handler struct {
		pattern         string
		methods         []string
		handler         http.Handler
		unauthenticated bool
		authorizer      authentication.Authorizer
		tracked         bool
		noModelUUID     bool
	}
	var endpoints []apihttp.Endpoint
	systemState, err := srv.shared.statePool.SystemState()
	if err != nil {
		return nil, errors.Trace(err)
	}
	controllerModelUUID := systemState.ModelUUID()

	httpAuthenticator := authentication.HTTPStrategicAuthenticator(srv.httpAuthenticators)

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
			h = &httpcontext.AuthHandler{
				NextHandler:   h,
				Authenticator: httpAuthenticator,
				Authorizer:    handler.authorizer,
			}
		}
		if !handler.noModelUUID {
			if strings.HasPrefix(handler.pattern, modelRoutePrefix) {
				h = &httpcontext.QueryModelHandler{
					Handler: h,
					Query:   ":modeluuid",
				}
			} else if strings.HasPrefix(handler.pattern, charmsObjectsRoutePrefix) {
				h = &httpcontext.BucketModelHandler{
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
	mainAPIHandler := srv.monitoredHandler(http.HandlerFunc(srv.apiHandler), "api")
	healthHandler := srv.monitoredHandler(http.HandlerFunc(srv.healthHandler), "health")
	logStreamHandler := srv.monitoredHandler(newLogStreamEndpointHandler(httpCtxt), "logstream")
	embeddedCLIHandler := srv.monitoredHandler(newEmbeddedCLIHandler(httpCtxt), "commands")
	var debuglogAuth httpcontext.CompositeAuthorizer = []authentication.Authorizer{
		tagKindAuthorizer{names.MachineTagKind, names.ControllerAgentTagKind},
		controllerAdminAuthorizer{
			controllerTag: systemState.ControllerTag(),
		},
		modelPermissionAuthorizer{
			perm: permission.ReadAccess,
		},
	}
	debugLogHandler := srv.monitoredHandler(newDebugLogDBHandler(
		httpCtxt,
		httpAuthenticator,
		debuglogAuth,
	), "log")
	pubsubHandler := srv.monitoredHandler(newPubSubHandler(httpCtxt, srv.shared.centralHub), "pubsub")
	logSinkHandler := logsink.NewHTTPHandler(
		newAgentLogWriteCloserFunc(httpCtxt, srv.logSinkWriter, &srv.apiServerLoggers),
		httpCtxt.stop(),
		&srv.logsinkRateLimitConfig,
		logsinkMetricsCollectorWrapper{collector: srv.metricsCollector},
		controllerModelUUID,
	)
	logSinkAuthorizer := tagKindAuthorizer(stateauthenticator.AgentTags)
	logTransferHandler := logsink.NewHTTPHandler(
		// We don't need to save the migrated logs
		// to a logfile as well as to the DB.
		newMigrationLogWriteCloserFunc(httpCtxt, &srv.apiServerLoggers),
		httpCtxt.stop(),
		nil, // no rate-limiting
		logsinkMetricsCollectorWrapper{collector: srv.metricsCollector},
		controllerModelUUID,
	)
	modelRestHandler := &modelRestHandler{
		ctxt:    httpCtxt,
		dataDir: srv.dataDir,
	}
	modelRestServer := srv.monitoredHandler(&RestHTTPHandler{
		GetHandler: modelRestHandler.ServeGet,
	}, "rest")
	modelCharmsHandler := &charmsHandler{
		ctxt:          httpCtxt,
		dataDir:       srv.dataDir,
		stateAuthFunc: httpCtxt.stateForRequestAuthenticatedUser,
	}
	modelCharmsHTTPHandler := srv.monitoredHandler(&CharmsHTTPHandler{
		PostHandler: modelCharmsHandler.ServePost,
		GetHandler:  modelCharmsHandler.ServeGet,
	}, "charms")
	modelCharmsUploadAuthorizer := tagKindAuthorizer{names.UserTagKind}

	modelObjectsCharmsHandler := &objectsCharmHandler{
		ctxt:          httpCtxt,
		stateAuthFunc: httpCtxt.stateForRequestAuthenticatedUser,
	}
	modelObjectsCharmsHTTPHandler := srv.monitoredHandler(&objectsCharmHTTPHandler{
		GetHandler:          modelObjectsCharmsHandler.ServeGet,
		PutHandler:          modelObjectsCharmsHandler.ServePut,
		LegacyCharmsHandler: modelCharmsHTTPHandler,
	}, "charms")

	modelToolsUploadHandler := srv.monitoredHandler(&toolsUploadHandler{
		ctxt:          httpCtxt,
		stateAuthFunc: httpCtxt.stateForRequestAuthenticatedUser,
	}, "tools")
	modelToolsUploadAuthorizer := tagKindAuthorizer{names.UserTagKind}
	modelToolsDownloadHandler := srv.monitoredHandler(newToolsDownloadHandler(httpCtxt), "tools")
	resourcesHandler := srv.monitoredHandler(&ResourcesHandler{
		StateAuthFunc: func(req *http.Request, tagKinds ...string) (ResourcesBackend, state.PoolHelper, names.Tag,
			error) {
			st, entity, err := httpCtxt.stateForRequestAuthenticatedTag(req, tagKinds...)
			if err != nil {
				return nil, nil, nil, errors.Trace(err)
			}

			rst := st.Resources()
			return rst, st, entity.Tag(), nil
		},
		ChangeAllowedFunc: func(req *http.Request) error {
			st, err := httpCtxt.stateForRequestUnauthenticated(req)
			if err != nil {
				return errors.Trace(err)
			}
			defer st.Release()

			blockChecker := common.NewBlockChecker(st)
			if err := blockChecker.ChangeAllowed(); err != nil {
				return errors.Trace(err)
			}
			return nil
		},
	}, "applications")
	unitResourcesHandler := srv.monitoredHandler(&UnitResourcesHandler{
		NewOpener: func(req *http.Request, tagKinds ...string) (resources.Opener, state.PoolHelper, error) {
			st, _, err := httpCtxt.stateForRequestAuthenticatedTag(req, tagKinds...)
			if err != nil {
				return nil, nil, errors.Trace(err)
			}

			tagStr := req.URL.Query().Get(":unit")
			tag, err := names.ParseUnitTag(tagStr)
			if err != nil {
				return nil, nil, errors.Trace(err)
			}
			opener, err := resource.NewResourceOpener(st.State, srv.getResourceDownloadLimiter, tag.Id())
			if err != nil {
				return nil, nil, errors.Trace(err)
			}
			return opener, st, nil
		},
	}, "units")

	controllerAdminAuthorizer := controllerAdminAuthorizer{
		controllerTag: systemState.ControllerTag(),
	}
	migrateCharmsHandler := &charmsHandler{
		ctxt:          httpCtxt,
		dataDir:       srv.dataDir,
		stateAuthFunc: httpCtxt.stateForMigrationImporting,
	}
	migrateCharmsHTTPHandler := &CharmsHTTPHandler{
		PostHandler: migrateCharmsHandler.ServePost,
		GetHandler:  migrateCharmsHandler.ServeUnsupported,
	}
	migrateObjectsCharmsHandler := &objectsCharmHandler{
		ctxt:          httpCtxt,
		stateAuthFunc: httpCtxt.stateForMigrationImporting,
	}
	migrateObjectsCharmsHTTPHandler := srv.monitoredHandler(&objectsCharmHTTPHandler{
		PutHandler:          migrateObjectsCharmsHandler.ServePut,
		GetHandler:          migrateObjectsCharmsHandler.ServeUnsupported,
		LegacyCharmsHandler: migrateCharmsHTTPHandler,
	}, "charms")
	migrateToolsUploadHandler := srv.monitoredHandler(&toolsUploadHandler{
		ctxt:          httpCtxt,
		stateAuthFunc: httpCtxt.stateForMigrationImporting,
	}, "tools")
	resourcesMigrationUploadHandler := srv.monitoredHandler(&resourcesMigrationUploadHandler{
		ctxt:          httpCtxt,
		stateAuthFunc: httpCtxt.stateForMigrationImporting,
	}, "resources")
	backupHandler := srv.monitoredHandler(&backupHandler{ctxt: httpCtxt}, "backups")
	registerHandler := srv.monitoredHandler(&registerUserHandler{ctxt: httpCtxt}, "register")

	// HTTP handler for application offer macaroon authentication.
	addOfferAuthHandlers(srv.offerAuthCtxt, srv.mux)

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
		pattern:         modelRoutePrefix + "/commands",
		handler:         embeddedCLIHandler,
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
		pattern:    modelRoutePrefix + "/backups",
		handler:    backupHandler,
		authorizer: controllerAdminAuthorizer,
	}, {
		// Legacy migration endpoint. Used by Juju 3.3 and prior
		pattern:    "/migrate/charms",
		handler:    srv.monitoredHandler(migrateCharmsHTTPHandler, "charms"),
		authorizer: controllerAdminAuthorizer,
	}, {
		pattern:    "/migrate/charms/:object",
		handler:    migrateObjectsCharmsHTTPHandler,
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
		pattern:         "/commands",
		handler:         embeddedCLIHandler,
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
		pattern:         "/health",
		methods:         []string{"GET"},
		handler:         healthHandler,
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
		// GET has no authorizer
		pattern: charmsObjectsRoutePrefix,
		methods: []string{"GET"},
		handler: modelObjectsCharmsHTTPHandler,
	}, {
		pattern:    charmsObjectsRoutePrefix,
		methods:    []string{"PUT"},
		handler:    modelObjectsCharmsHTTPHandler,
		authorizer: modelCharmsUploadAuthorizer,
	}}
	if srv.registerIntrospectionHandlers != nil {
		add := func(subpath string, h http.Handler) {
			handlers = append(handlers, handler{
				pattern: path.Join("/introspection/", subpath),
				handler: srv.monitoredHandler(introspectionHandler{httpCtxt, h}, "introspection"),
			})
		}
		srv.registerIntrospectionHandlers(add)
	}

	// Construct endpoints from handler structs.
	for _, handler := range handlers {
		addHandler(handler)
	}

	return endpoints, nil
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
			http.Error(w, "apiserver shutdown in progress", http.StatusServiceUnavailable)
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

func (srv *Server) healthHandler(w http.ResponseWriter, req *http.Request) {
	srv.mu.Lock()
	status := srv.healthStatus
	srv.mu.Unlock()
	if status != "running" {
		w.WriteHeader(http.StatusServiceUnavailable)
	}

	fmt.Fprintf(w, "%s\n", status)
}

func (srv *Server) apiHandler(w http.ResponseWriter, req *http.Request) {
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
		systemState, err := statePool.SystemState()
		if err != nil {
			return errors.Trace(err)
		}
		resolvedModelUUID = systemState.ModelUUID()
	}
	var (
		st *state.PooledState
		h  *apiHandler
	)

	var stateClosing <-chan struct{}
	st, err := statePool.Get(resolvedModelUUID)
	if err == nil {
		defer st.Release()
		stateClosing = st.Removing()
		h, err = newAPIHandler(srv, st.State, conn, modelUUID, connectionID, host)
	}
	if errors.Is(err, errors.NotFound) {
		err = fmt.Errorf("%w: %q", apiservererrors.UnknownModelError, resolvedModelUUID)
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
	case <-stateClosing:
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
	return apiservererrors.ServerError(err)
}

// GetAuditConfig returns a copy of the current audit logging
// configuration.
func (srv *Server) GetAuditConfig() auditlog.Config {
	// Delegates to the getter passed in.
	return srv.getAuditConfig()
}

// monitoredHandler wraps an HTTP handler for tracking metrics with given label.
// It increments and decrements connection counters for monitoring purposes.
func (srv *Server) monitoredHandler(handler http.Handler, label string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		srv.metricsCollector.TotalConnections.Inc()
		gauge := srv.metricsCollector.APIConnections.WithLabelValues(label)
		gauge.Inc()
		defer gauge.Dec()
		handler.ServeHTTP(w, r)
	})
}
