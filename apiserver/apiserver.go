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
	"github.com/juju/names/v5"
	"github.com/juju/pubsub/v2"
	"github.com/juju/ratelimit"
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
	"github.com/juju/juju/core/changestream"
	"github.com/juju/juju/core/database"
	"github.com/juju/juju/core/lease"
	corelogger "github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/core/presence"
	"github.com/juju/juju/core/resources"
	coretrace "github.com/juju/juju/core/trace"
	internallogger "github.com/juju/juju/internal/logger"
	controllermsg "github.com/juju/juju/internal/pubsub/controller"
	"github.com/juju/juju/internal/resource"
	"github.com/juju/juju/internal/servicefactory"
	"github.com/juju/juju/internal/worker/trace"
	"github.com/juju/juju/rpc"
	"github.com/juju/juju/rpc/jsoncodec"
	"github.com/juju/juju/state"
)

var logger = internallogger.GetLogger("juju.apiserver")

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

	offerAuthCtxt    *crossmodel.AuthContext
	lastConnectionID uint64
	newObserver      observer.ObserverFactory
	allowModelAccess bool
	// TODO(debug-log) - move into logSink
	logSinkWriter          io.WriteCloser
	logsinkRateLimitConfig logsink.RateLimitConfig
	logSink                corelogger.ModelLogger
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

	// ControllerUUID is the controller unique identifier.
	ControllerUUID string

	// ControllerModelUUID is the ID for the controller model.
	ControllerModelUUID model.UUID

	// LocalMacaroonAuthenticator is the request authenticator used for verifying
	// local user macaroons.
	LocalMacaroonAuthenticator macaroon.LocalMacaroonAuthenticator

	// JWTAuthenticator is the request authenticator used for validating jwt
	// tokens when the controller has been bootstrapped with a trusted token
	// provider.
	JWTAuthenticator jwt.Authenticator

	// StatePool is the StatePool used for looking up State
	// to pass to facades. StatePool will not be closed by the
	// server; it is the callers responsibility to close it
	// after the apiserver has exited.
	StatePool *state.StatePool

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

	// LogSink is used to store log records received from connected agents.
	LogSink corelogger.ModelLogger

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

	// SSHImporterHTTPClient is the HTTP client used for ssh key import
	// operations.
	SSHImporterHTTPClient facade.HTTPClient

	// ServiceFactoryGetter provides access to the services.
	ServiceFactoryGetter servicefactory.ServiceFactoryGetter

	// DBGetter returns WatchableDB implementations based on namespace.
	DBGetter changestream.WatchableDBGetter

	// DBDeleter is used to delete databases by namespace.
	DBDeleter database.DBDeleter

	// TracerGetter returns a tracer for the given namespace, this is used
	// for opentelmetry tracing.
	TracerGetter trace.TracerGetter

	// ObjectStoreGetter returns an object store for the given namespace.
	// This is used for retrieving blobs for charms and agents.
	ObjectStoreGetter objectstore.ObjectStoreGetter
}

// Validate validates the API server configuration.
func (c ServerConfig) Validate() error {
	if c.StatePool == nil {
		return errors.NotValidf("missing StatePool")
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
	if c.ControllerUUID == "" {
		return errors.NotValidf("missing ControllerUUID")
	}
	if c.ControllerModelUUID == "" {
		return errors.NotValidf("missing ControllerModelUUID")
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
	if c.LogSink == nil {
		return errors.NotValidf("nil LogSink")
	}
	if c.MetricsCollector == nil {
		return errors.NotValidf("missing MetricsCollector")
	}
	if c.DBGetter == nil {
		return errors.NotValidf("missing DBGetter")
	}
	if c.DBDeleter == nil {
		return errors.NotValidf("missing DBDeleter")
	}
	if c.ServiceFactoryGetter == nil {
		return errors.NotValidf("missing ServiceFactoryGetter")
	}
	if c.TracerGetter == nil {
		return errors.NotValidf("missing TracerGetter")
	}
	if c.ObjectStoreGetter == nil {
		return errors.NotValidf("missing ObjectStoreGetter")
	}
	if c.SSHImporterHTTPClient == nil {
		return errors.NotValidf("missing SSHImporterHTTPClient")
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
func NewServer(ctx context.Context, cfg ServerConfig) (*Server, error) {
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
	return newServer(ctx, cfg)
}

const readyTimeout = time.Second * 30

func newServer(ctx context.Context, cfg ServerConfig) (_ *Server, err error) {
	controllerServiceFactory := cfg.ServiceFactoryGetter.FactoryForModel(cfg.ControllerModelUUID)
	controllerConfigService := controllerServiceFactory.ControllerConfig()
	controllerConfig, err := controllerConfigService.ControllerConfig(ctx)
	if err != nil {
		return nil, errors.Annotate(err, "unable to get controller config")
	}

	httpAuthenticators := []authentication.HTTPAuthenticator{cfg.LocalMacaroonAuthenticator}
	loginAuthenticators := []authentication.LoginAuthenticator{cfg.LocalMacaroonAuthenticator}
	// We only want to add the jwt authenticator if it's not nil.
	if cfg.JWTAuthenticator != nil {
		httpAuthenticators = append([]authentication.HTTPAuthenticator{cfg.JWTAuthenticator}, httpAuthenticators...)
		loginAuthenticators = append([]authentication.LoginAuthenticator{cfg.JWTAuthenticator}, loginAuthenticators...)
	}

	shared, err := newSharedServerContext(sharedServerConfig{
		statePool:             cfg.StatePool,
		centralHub:            cfg.Hub,
		presence:              cfg.Presence,
		leaseManager:          cfg.LeaseManager,
		controllerUUID:        cfg.ControllerUUID,
		controllerModelUUID:   cfg.ControllerModelUUID,
		controllerConfig:      controllerConfig,
		logger:                internallogger.GetLogger("juju.apiserver"),
		charmhubHTTPClient:    cfg.CharmhubHTTPClient,
		sshImporterHTTPClient: cfg.SSHImporterHTTPClient,
		dbGetter:              cfg.DBGetter,
		dbDeleter:             cfg.DBDeleter,
		serviceFactoryGetter:  cfg.ServiceFactoryGetter,
		tracerGetter:          cfg.TracerGetter,
		objectStoreGetter:     cfg.ObjectStoreGetter,
		machineTag:            cfg.Tag,
		dataDir:               cfg.DataDir,
		logDir:                cfg.LogDir,
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
		getAuditConfig:      cfg.GetAuditConfig,
		logSink:             cfg.LogSink,
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

	macaroonService := controllerServiceFactory.Macaroon()

	// The auth context for authenticating access to application offers.
	srv.offerAuthCtxt, err = newOfferAuthContext(
		ctx, cfg.StatePool,
		controllerServiceFactory.Access(),
		controllerServiceFactory.ModelInfo(),
		controllerConfigService, macaroonService,
	)
	if err != nil {
		unsubscribeControllerConfig()
		return nil, fmt.Errorf("creating offer auth context: %w", err)
	}

	systemState, err := cfg.StatePool.SystemState()
	if err != nil {
		return nil, errors.Trace(err)
	}
	model, err := systemState.Model()
	if err != nil {
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

	ready := make(chan struct{})
	srv.tomb.Go(func() error {
		defer srv.logSink.Close()
		defer srv.logSinkWriter.Close()
		defer srv.shared.Close()
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

const (
	modelRoutePrefix         = "/model/:modeluuid"
	charmsObjectsRoutePrefix = "/model-:modeluuid/charms/:object"
)

func (srv *Server) endpoints() ([]apihttp.Endpoint, error) {
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
				h = &httpcontext.ControllerModelHandler{
					Handler:             h,
					ControllerModelUUID: controllerModelUUID,
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
	healthHandler := http.HandlerFunc(srv.healthHandler)
	embeddedCLIHandler := newEmbeddedCLIHandler(httpCtxt)
	debugLogHandler := newDebugLogTailerHandler(
		httpCtxt,
		httpAuthenticator,
		tagKindAuthorizer{
			names.MachineTagKind,
			names.ControllerAgentTagKind,
			names.UserTagKind,
			names.ApplicationTagKind,
		},
		srv.logDir,
	)
	pubsubHandler := newPubSubHandler(httpCtxt, srv.shared.centralHub)
	logSinkHandler := logsink.NewHTTPHandler(
		newAgentLogWriteCloserFunc(httpCtxt, srv.logSinkWriter, srv.logSink),
		httpCtxt.stop(),
		&srv.logsinkRateLimitConfig,
		logsinkMetricsCollectorWrapper{collector: srv.metricsCollector},
		controllerModelUUID,
	)
	logSinkAuthorizer := tagKindAuthorizer(stateauthenticator.AgentTags)
	logTransferHandler := logsink.NewHTTPHandler(
		// We don't need to save the migrated logs
		// to a logfile as well as to the DB.
		newMigrationLogWriteCloserFunc(httpCtxt, srv.logSink),
		httpCtxt.stop(),
		nil, // no rate-limiting
		logsinkMetricsCollectorWrapper{collector: srv.metricsCollector},
		controllerModelUUID,
	)
	modelCharmsHandler := &charmsHandler{
		ctxt:              httpCtxt,
		dataDir:           srv.dataDir,
		stateAuthFunc:     httpCtxt.stateForRequestAuthenticatedUser,
		objectStoreGetter: srv.shared.objectStoreGetter,
		logger:            logger.Child("charms-handler"),
	}
	modelCharmsHTTPHandler := &charmsHTTPHandler{
		getHandler: modelCharmsHandler.ServeGet,
	}
	charmsObjectsAuthorizer := tagKindAuthorizer{names.UserTagKind}

	modelObjectsCharmsHTTPHandler := &objectsCharmHTTPHandler{
		ctxt:              httpCtxt,
		stateAuthFunc:     httpCtxt.stateForRequestAuthenticatedUser,
		objectStoreGetter: srv.shared.objectStoreGetter,
	}

	modelToolsUploadHandler := &toolsUploadHandler{
		ctxt:          httpCtxt,
		stateAuthFunc: httpCtxt.stateForRequestAuthenticatedUser,
	}
	modelToolsUploadAuthorizer := tagKindAuthorizer{names.UserTagKind}
	modelToolsDownloadHandler := newToolsDownloadHandler(httpCtxt)
	resourcesHandler := &ResourcesHandler{
		StateAuthFunc: func(req *http.Request, tagKinds ...string) (ResourcesBackend, state.PoolHelper, names.Tag,
			error) {
			st, entity, err := httpCtxt.stateForRequestAuthenticatedTag(req, tagKinds...)
			if err != nil {
				return nil, nil, nil, errors.Trace(err)
			}

			store, err := httpCtxt.objectStoreForRequest(req.Context())
			if err != nil {
				return nil, nil, nil, errors.Trace(err)
			}
			rst := st.Resources(store)
			return rst, st, entity.Tag(), nil
		},
		ChangeAllowedFunc: func(ctx context.Context) error {
			st, err := httpCtxt.stateForRequestUnauthenticated(ctx)
			if err != nil {
				return errors.Trace(err)
			}
			defer st.Release()

			blockChecker := common.NewBlockChecker(st)
			if err := blockChecker.ChangeAllowed(ctx); err != nil {
				return errors.Trace(err)
			}
			return nil
		},
	}
	unitResourcesHandler := &UnitResourcesHandler{
		NewOpener: func(req *http.Request, tagKinds ...string) (resources.Opener, state.PoolHelper, error) {
			st, _, err := httpCtxt.stateForRequestAuthenticatedTag(req, tagKinds...)
			if err != nil {
				return nil, nil, errors.Trace(err)
			}
			store, err := httpCtxt.objectStoreForRequest(req.Context())
			if err != nil {
				return nil, nil, errors.Trace(err)
			}

			tagStr := req.URL.Query().Get(":unit")
			tag, err := names.ParseUnitTag(tagStr)
			if err != nil {
				return nil, nil, errors.Trace(err)
			}
			serviceFactory, err := httpCtxt.serviceFactoryForRequest(req.Context())
			if err != nil {
				return nil, nil, errors.Trace(errors.Annotate(err, "cannot get service factory for unit resource request"))
			}

			args := resource.ResourceOpenerArgs{
				State:              st.State,
				ModelConfigService: serviceFactory.Config(),
				Store:              store,
			}
			opener, err := resource.NewResourceOpener(args, srv.getResourceDownloadLimiter, tag.Id())
			if err != nil {
				return nil, nil, errors.Trace(err)
			}
			return opener, st, nil
		},
	}

	controllerAdminAuthorizer := controllerAdminAuthorizer{
		controllerTag: systemState.ControllerTag(),
	}
	migrateCharmsHandler := &charmsHandler{
		ctxt:              httpCtxt,
		dataDir:           srv.dataDir,
		stateAuthFunc:     httpCtxt.stateForMigrationImporting,
		objectStoreGetter: srv.shared.objectStoreGetter,
		logger:            logger.Child("charms-handler"),
	}
	migrateCharmsHTTPHandler := &charmsHTTPHandler{
		getHandler: migrateCharmsHandler.ServeUnsupported,
	}
	migrateObjectsCharmsHTTPHandler := &objectsCharmHTTPHandler{
		ctxt:              httpCtxt,
		stateAuthFunc:     httpCtxt.stateForMigrationImporting,
		objectStoreGetter: srv.shared.objectStoreGetter,
	}
	migrateToolsUploadHandler := &toolsUploadHandler{
		ctxt:          httpCtxt,
		stateAuthFunc: httpCtxt.stateForMigrationImporting,
	}
	resourcesMigrationUploadHandler := &resourcesMigrationUploadHandler{
		ctxt:          httpCtxt,
		stateAuthFunc: httpCtxt.stateForMigrationImporting,
		objectStore:   httpCtxt.objectStoreForRequest,
	}
	registerHandler := &registerUserHandler{
		ctxt: httpCtxt,
	}

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
		// GET /charms has no authorizer
		pattern: modelRoutePrefix + "/charms",
		methods: []string{"GET"},
		handler: modelCharmsHTTPHandler,
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
		// Legacy migration endpoint. Used by Juju 3.3 and prior
		pattern:    "/migrate/charms",
		handler:    migrateCharmsHTTPHandler,
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
		pattern: charmsObjectsRoutePrefix,
		methods: []string{"GET"},
		handler: modelObjectsCharmsHTTPHandler,
	}, {
		pattern:    charmsObjectsRoutePrefix,
		methods:    []string{"PUT"},
		handler:    modelObjectsCharmsHTTPHandler,
		authorizer: charmsObjectsAuthorizer,
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
	srv.metricsCollector.TotalConnections.Inc()

	gauge := srv.metricsCollector.APIConnections.WithLabelValues("api")
	gauge.Inc()
	defer gauge.Dec()

	connectionID := atomic.AddUint64(&srv.lastConnectionID, 1)

	apiObserver := srv.newObserver()
	apiObserver.Join(req, connectionID)
	defer apiObserver.Leave()

	websocket.Serve(w, req, func(conn *websocket.Conn) {
		modelUUID, modelOnlyLogin := httpcontext.RequestModelUUID(req.Context())

		// If the modelUUID wasn't present in the request, then this is
		// considered a controller-only login.
		controllerOnlyLogin := !modelOnlyLogin

		// If the request is for the controller model, then we need to
		// resolve the modelUUID to the controller model.
		resolvedModelUUID := model.UUID(modelUUID)
		if controllerOnlyLogin {
			resolvedModelUUID = srv.shared.controllerModelUUID
		}

		// Put the modelUUID into the context for the request. This will
		// allow the peeling of the modelUUID from the request to be
		// deferred to the facade methods.
		ctx := model.WithContextModelUUID(req.Context(), resolvedModelUUID)

		logger.Tracef("got a request for model %q", modelUUID)
		if err := srv.serveConn(
			srv.tomb.Context(ctx),
			conn,
			resolvedModelUUID,
			controllerOnlyLogin,
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
	modelUUID model.UUID,
	controllerOnlyLogin bool,
	connectionID uint64,
	apiObserver observer.Observer,
	host string,
) error {
	codec := jsoncodec.NewWebsocket(wsConn.Conn)
	recorderFactory := observer.NewRecorderFactory(apiObserver, nil, observer.NoCaptureArgs)
	conn := rpc.NewConn(codec, recorderFactory)

	tracer, err := srv.shared.tracerGetter.GetTracer(
		ctx,
		coretrace.Namespace("apiserver", modelUUID.String()),
	)
	if err != nil {
		logger.Infof("failed to get tracer for model %q: %v", modelUUID, err)
		tracer = coretrace.NoopTracer{}
	}

	// Grab the object store for the model.
	objectStore, err := srv.shared.objectStoreGetter.GetObjectStore(ctx, modelUUID.String())
	if err != nil {
		return errors.Annotatef(err, "getting object store for model %q", modelUUID)
	}

	// Grab the object store for the controller, this is primarily used for
	// the agent tools.
	controllerObjectStore, err := srv.shared.objectStoreGetter.GetObjectStore(ctx, database.ControllerNS)
	if err != nil {
		return errors.Annotatef(err, "getting controller object store")
	}

	serviceFactory := srv.shared.serviceFactoryGetter.FactoryForModel(modelUUID)

	var handler *apiHandler
	st, err := srv.shared.statePool.Get(modelUUID.String())
	if err == nil {
		defer st.Release()
		handler, err = newAPIHandler(
			ctx,
			srv,
			st.State,
			conn,
			serviceFactory,
			srv.shared.serviceFactoryGetter,
			tracer,
			objectStore,
			srv.shared.objectStoreGetter,
			controllerObjectStore,
			modelUUID,
			controllerOnlyLogin,
			connectionID,
			host,
		)
	}
	if errors.Is(err, errors.NotFound) {
		err = fmt.Errorf("%w: %q", apiservererrors.UnknownModelError, modelUUID)
	}

	if err != nil {
		conn.ServeRoot(&errRoot{err: errors.Trace(err)}, recorderFactory, serverError)
	} else {
		// Set up the admin apis used to accept logins and direct
		// requests to the relevant business facade.
		// There may be more than one since we need a new API each
		// time login changes in a non-backwards compatible way.
		adminAPIs := make(map[int]interface{})
		for apiVersion, factory := range adminAPIFactories {
			adminAPIs[apiVersion] = factory(srv, handler, apiObserver)
		}
		conn.ServeRoot(newAdminRoot(handler, adminAPIs), recorderFactory, serverError)
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

// GetAuditConfig returns a copy of the current audit logging
// configuration.
func (srv *Server) GetAuditConfig() auditlog.Config {
	// Delegates to the getter passed in.
	return srv.getAuditConfig()
}

func serverError(err error) error {
	return apiservererrors.ServerError(err)
}
