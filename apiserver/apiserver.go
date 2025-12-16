// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery"
	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery/checkers"
	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/ratelimit"
	"github.com/juju/worker/v4/catacomb"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/juju/juju/apiserver/apiserverhttp"
	"github.com/juju/juju/apiserver/authentication"
	"github.com/juju/juju/apiserver/authentication/jwt"
	"github.com/juju/juju/apiserver/authentication/macaroon"
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/common/apihttp"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/httpcontext"
	"github.com/juju/juju/apiserver/internal/crossmodel"
	crossmodelbakery "github.com/juju/juju/apiserver/internal/crossmodel/bakery"
	handlerscrossmodel "github.com/juju/juju/apiserver/internal/handlers/crossmodel"
	"github.com/juju/juju/apiserver/internal/handlers/objects"
	handlersresources "github.com/juju/juju/apiserver/internal/handlers/resources"
	resourcesdownload "github.com/juju/juju/apiserver/internal/handlers/resources/download"
	"github.com/juju/juju/apiserver/logsink"
	"github.com/juju/juju/apiserver/observer"
	"github.com/juju/juju/apiserver/stateauthenticator"
	"github.com/juju/juju/apiserver/websocket"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/auditlog"
	"github.com/juju/juju/core/changestream"
	"github.com/juju/juju/core/database"
	"github.com/juju/juju/core/flightrecorder"
	"github.com/juju/juju/core/lease"
	corelogger "github.com/juju/juju/core/logger"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/core/permission"
	coreresource "github.com/juju/juju/core/resource"
	"github.com/juju/juju/core/securitylog"
	coretrace "github.com/juju/juju/core/trace"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain/model"
	modelerrors "github.com/juju/juju/domain/model/errors"
	internalerrors "github.com/juju/juju/internal/errors"
	internallogger "github.com/juju/juju/internal/logger"
	internalmacaroon "github.com/juju/juju/internal/macaroon"
	"github.com/juju/juju/internal/resource"
	resourcecharmhub "github.com/juju/juju/internal/resource/charmhub"
	"github.com/juju/juju/internal/services"
	"github.com/juju/juju/internal/worker/trace"
	"github.com/juju/juju/internal/worker/watcherregistry"
	"github.com/juju/juju/rpc"
	"github.com/juju/juju/rpc/jsoncodec"
)

const (
	// ErrAPIServerDying is used to indicate to *third parties* that the
	// api-server worker is dying, instead of catacomb.ErrDying, which is
	// unsuitable for propagating inter-worker.
	// This error indicates to consuming workers that their dependency has
	// become unmet and a restart by the dependency engine is imminent.
	ErrAPIServerDying = errors.ConstError("api-server worker is dying")

	// ErrRPCConnectionClosed is used to indicate that the RPC connection
	// has been closed.
	ErrRPCConnectionClosed = errors.ConstError("rpc connection closed")
)

var logger = internallogger.GetLogger("juju.apiserver")

var defaultHTTPMethods = []string{
	http.MethodGet,
	http.MethodPost,
	http.MethodHead,
	http.MethodPut,
	http.MethodDelete,
	http.MethodOptions,
}

// Server holds the server side of the API.
type Server struct {
	catacomb  catacomb.Catacomb
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

	connectionID     uint64
	newObserver      observer.ObserverFactory
	allowModelAccess bool

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
	Mux       *apiserverhttp.Mux

	// ControllerUUID is the controller unique identifier.
	ControllerUUID string

	// ControllerModelUUID is the ID for the controller model.
	ControllerModelUUID coremodel.UUID

	// LocalMacaroonAuthenticator is the request authenticator used for verifying
	// local user macaroons.
	LocalMacaroonAuthenticator macaroon.LocalMacaroonAuthenticator

	// JWTAuthenticator is the request authenticator used for validating jwt
	// tokens when the controller has been bootstrapped with a trusted token
	// provider.
	JWTAuthenticator jwt.Authenticator

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

	// FlightRecorder is used to capture flight recordings of the API server.
	FlightRecorder flightrecorder.FlightRecorder

	// MetricsCollector defines all the metrics to be collected for the
	// apiserver
	MetricsCollector *Collector

	// ExecEmbeddedCommand is a function which creates an embedded Juju CLI instance.
	ExecEmbeddedCommand ExecEmbeddedCommandFunc

	// CharmhubHTTPClient is the HTTP client used for Charmhub API requests.
	CharmhubHTTPClient facade.HTTPClient

	// MacaroonHTTPClient is the HTTP client used to make requests to
	// third party macaroon services.
	MacaroonHTTPClient facade.HTTPClient

	// DomainServicesGetter provides access to the services.
	DomainServicesGetter services.DomainServicesGetter

	// ControllerConfigService provides access to the controller config.
	ControllerConfigService ControllerConfigService

	// DBGetter returns WatchableDB implementations based on namespace.
	DBGetter changestream.WatchableDBGetter

	// TracerGetter returns a tracer for the given namespace, this is used
	// for opentelmetry tracing.
	TracerGetter trace.TracerGetter

	// ObjectStoreGetter returns an object store for the given namespace.
	// This is used for retrieving blobs for charms and agents.
	ObjectStoreGetter objectstore.ObjectStoreGetter

	// WatcherRegistryGetter is used to register and manage watchers.
	WatcherRegistryGetter watcherregistry.WatcherRegistryGetter
}

// Validate validates the API server configuration.
func (c ServerConfig) Validate() error {
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
	if c.DomainServicesGetter == nil {
		return errors.NotValidf("missing DomainServicesGetter")
	}
	if c.ControllerConfigService == nil {
		return errors.NotValidf("missing ControllerConfigService")
	}
	if c.TracerGetter == nil {
		return errors.NotValidf("missing TracerGetter")
	}
	if c.ObjectStoreGetter == nil {
		return errors.NotValidf("missing ObjectStoreGetter")
	}
	if c.WatcherRegistryGetter == nil {
		return errors.NotValidf("missing WatcherRegistryGetter")
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
	controllerDomainServices, err := cfg.DomainServicesGetter.ServicesForModel(ctx, cfg.ControllerModelUUID)
	if err != nil {
		return nil, errors.Trace(err)
	}
	controllerConfigService := controllerDomainServices.ControllerConfig()
	controllerConfig, err := controllerConfigService.ControllerConfig(ctx)
	if err != nil {
		return nil, errors.Annotate(err, "getting controller config")
	}

	macaroonService := controllerDomainServices.Macaroon()
	offersThirdPartyKeyPair, err := macaroonService.GetOffersThirdPartyKey(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}

	httpAuthenticators := []authentication.HTTPAuthenticator{cfg.LocalMacaroonAuthenticator, cfg.JWTAuthenticator}
	loginAuthenticators := []authentication.LoginAuthenticator{cfg.LocalMacaroonAuthenticator, cfg.JWTAuthenticator}

	shared, err := newSharedServerContext(sharedServerConfig{
		flightRecorder:           cfg.FlightRecorder,
		leaseManager:             cfg.LeaseManager,
		controllerUUID:           cfg.ControllerUUID,
		controllerModelUUID:      cfg.ControllerModelUUID,
		controllerConfig:         controllerConfig,
		loginTokenRefreshURL:     controllerConfig.LoginTokenRefreshURL(),
		offersThirdPartyKeyPair:  offersThirdPartyKeyPair,
		logger:                   internallogger.GetLogger("juju.apiserver"),
		clock:                    cfg.Clock,
		charmhubHTTPClient:       cfg.CharmhubHTTPClient,
		macaroonHTTPClient:       cfg.MacaroonHTTPClient,
		dbGetter:                 cfg.DBGetter,
		domainServicesGetter:     cfg.DomainServicesGetter,
		controllerDomainServices: controllerDomainServices,
		tracerGetter:             cfg.TracerGetter,
		objectStoreGetter:        cfg.ObjectStoreGetter,
		machineTag:               cfg.Tag,
		dataDir:                  cfg.DataDir,
		logDir:                   cfg.LogDir,
		watcherRegistryGetter:    cfg.WatcherRegistryGetter,
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
	if err := srv.updateResourceDownloadLimiters(controllerConfig); err != nil {
		return nil, errors.Trace(err)
	}

	ready := make(chan struct{})
	if err := catacomb.Invoke(catacomb.Plan{
		Name: "apiserver",
		Site: &srv.catacomb,
		Work: func() error {
			return srv.loop(ready)
		},
	}); err != nil {
		return nil, errors.Trace(err)
	}

	// Don't return until all handlers have been registered.
	select {
	case <-ready:
	case <-srv.clock.After(readyTimeout):
		return nil, errors.New("loop never signalled ready")
	}

	return srv, nil
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
	return srv.catacomb.Dead()
}

// Stop stops the server and returns when all running requests
// have completed.
func (srv *Server) Stop() error {
	srv.Kill()
	return srv.Wait()
}

// Kill implements worker.Worker.Kill.
func (srv *Server) Kill() {
	srv.catacomb.Kill(nil)
}

// Wait implements worker.Worker.Wait.
func (srv *Server) Wait() error {
	// If the server was killed, and in turn any watchers associated with the
	// catacomb are cancelled whilst a request is being processed, it will
	// return context.Canceled. This is expected and should not be treated as an
	// error.
	if err := srv.catacomb.Wait(); errors.Is(err, context.Canceled) {
		return nil
	} else if err != nil {
		return errors.Trace(err)
	}
	return nil
}

func (srv *Server) updateAgentRateLimiter(cfg controller.Config) {
	srv.mu.Lock()
	defer srv.mu.Unlock()
	srv.agentRateLimitMax = cfg.AgentRateLimitMax()
	srv.agentRateLimitRate = cfg.AgentRateLimitRate()
	if srv.agentRateLimitMax > 0 {
		srv.agentRateLimit = ratelimit.NewBucketWithClock(
			srv.agentRateLimitRate, int64(srv.agentRateLimitMax), rateClock{Clock: srv.clock})
	} else {
		srv.agentRateLimit = nil
	}
}

func (srv *Server) updateResourceDownloadLimiters(cfg controller.Config) error {
	srv.mu.Lock()
	defer srv.mu.Unlock()
	globalLimit := cfg.ControllerResourceDownloadLimit()
	appLimit := cfg.ApplicationResourceDownloadLimit()

	var err error
	srv.resourceLock, err = resource.NewResourceDownloadLimiter(globalLimit, appLimit)
	if err != nil {
		return errors.Trace(err)
	}

	return nil
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
	modelUUID coremodel.UUID
}

// Record an outgoing request which produced an http.Response.
func (w httpRequestRecorderWrapper) Record(method string, url *url.URL, res *http.Response, rtt time.Duration) {
	// Note: Do not log url.Path as REST queries _can_ include the name of the
	// entities (charms, architectures, etc).
	w.collector.TotalRequests.WithLabelValues(w.modelUUID.String(), url.Host, strconv.FormatInt(int64(res.StatusCode), 10)).Inc()
	if res.StatusCode >= 400 {
		w.collector.TotalRequestErrors.WithLabelValues(w.modelUUID.String(), url.Host).Inc()
	}
	w.collector.TotalRequestsDuration.WithLabelValues(w.modelUUID.String(), url.Host).Observe(rtt.Seconds())
}

// RecordError records an outgoing request that returned back an error.
func (w httpRequestRecorderWrapper) RecordError(method string, url *url.URL, err error) {
	// Note: Do not log url.Path as REST queries _can_ include the name of the
	// entities (charms, architectures, etc).
	w.collector.TotalRequests.WithLabelValues(w.modelUUID.String(), url.Host, "unknown").Inc()
	w.collector.TotalRequestErrors.WithLabelValues(w.modelUUID.String(), url.Host).Inc()
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

	ctx := srv.catacomb.Context(context.Background())

	controllerConfigService := srv.shared.controllerDomainServices.ControllerConfig()
	controllerConfigWatcher, err := controllerConfigService.WatchControllerConfig(ctx)
	if err != nil {
		return errors.Trace(err)
	}

	if err := srv.catacomb.Add(controllerConfigWatcher); err != nil {
		return errors.Trace(err)
	}

	close(ready)

	srv.mu.Lock()
	srv.healthStatus = "running"
	// Security Event Logging: This log statement is required to comply with Canonical's SSDLC Security Event Logging policy.
	securitylog.LogSystem(securitylog.SystemLifecycleSecurityEvent{
		Event: securitylog.SystemLifecycleEventStartup,
		Actor: securitylog.DefaultAdminName,
	})
	srv.mu.Unlock()

	for {
		select {
		case <-srv.catacomb.Dying():
			srv.mu.Lock()
			srv.healthStatus = "stopping"
			// Security Event Logging: This log statement is required to comply with Canonical's SSDLC Security Event Logging policy.
			securitylog.LogSystem(securitylog.SystemLifecycleSecurityEvent{
				Event: securitylog.SystemLifecycleEventShutdown,
				Actor: securitylog.DefaultAdminName,
			})
			srv.mu.Unlock()

			srv.wg.Wait() // wait for any outstanding requests to complete.
			return ErrAPIServerDying

		case <-controllerConfigWatcher.Changes():
			controllerConfig, err := controllerConfigService.ControllerConfig(ctx)
			if err != nil {
				logger.Errorf(ctx, "failed to get controller config: %v", err)
				continue
			}

			srv.updateAgentRateLimiter(controllerConfig)
			srv.shared.updateControllerConfig(ctx, controllerConfig)

			// If the update fails, there is nothing else we can do but log the
			// error. The server will continue to run with the old limits.
			if err := srv.updateResourceDownloadLimiters(controllerConfig); err != nil {
				logger.Errorf(ctx, "failed to update resource download limiters: %v", err)
				continue
			}
		}
	}
}

const (
	modelRoutePrefix         = "/model/:modeluuid"
	charmsObjectsRoutePrefix = "/model-:modeluuid/charms/:object"
	objectsRoutePrefix       = "/model-:modeluuid/objects/:object"
	migrateResourcesPrefix   = "/migrate/resources"
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
	controllerModelUUID := srv.shared.controllerModelUUID

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

		// Register the [httpcontext.ControllerModelSignalHandler] for every
		// handler.
		h = httpcontext.ControllerModelSignalHandler{
			ControllerModelUUID: controllerModelUUID,
			Handler:             h,
		}

		if !handler.noModelUUID {
			if strings.HasPrefix(handler.pattern, modelRoutePrefix) {
				h = &httpcontext.QueryModelHandler{
					Handler: h,
					Query:   ":modeluuid",
				}
			} else if strings.HasPrefix(handler.pattern, charmsObjectsRoutePrefix) ||
				// The charm upload path differs from [modelRoutePrefix] hence
				// the existence of this special case.
				strings.HasPrefix(handler.pattern, objectsRoutePrefix) {
				h = &httpcontext.QueryModelHandler{
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
	mainAPIHandler := srv.monitoredHandler(http.HandlerFunc(srv.apiHandler), "api")
	healthHandler := srv.monitoredHandler(http.HandlerFunc(srv.healthHandler), "health")
	embeddedCLIHandler := srv.monitoredHandler(newEmbeddedCLIHandler(httpCtxt), "commands")
	controllerAdminAuthorizer := controllerAdminAuthorizer{
		controllerTag: names.NewControllerTag(srv.shared.controllerUUID),
	}
	var debuglogAuth httpcontext.CompositeAuthorizer = []authentication.Authorizer{
		tagKindAuthorizer{names.MachineTagKind, names.ControllerAgentTagKind},
		controllerAdminAuthorizer,
		modelPermissionAuthorizer{
			perm: permission.ReadAccess,
		},
	}
	debugLogHandler := srv.monitoredHandler(newDebugLogTailerHandler(
		httpCtxt,
		httpAuthenticator,
		debuglogAuth,
		srv.logDir,
	), "log")
	logSinkHandler := logsink.NewHTTPHandler(
		newAgentLogWriteFunc(httpCtxt, srv.logSink),
		httpCtxt.stop(),
		&srv.logsinkRateLimitConfig,
		logsinkMetricsCollectorWrapper{collector: srv.metricsCollector},
		controllerModelUUID.String(),
	)
	logSinkAuthorizer := tagKindAuthorizer(stateauthenticator.AgentTags)
	logTransferHandler := logsink.NewHTTPHandler(
		// We don't need to save the migrated logs
		// to a logfile as well as to the DB.
		newMigrationLogWriteFunc(httpCtxt, srv.logSink),
		httpCtxt.stop(),
		nil, // no rate-limiting
		logsinkMetricsCollectorWrapper{collector: srv.metricsCollector},
		controllerModelUUID.String(),
	)
	var charmsObjectsAuthorizer httpcontext.CompositeAuthorizer = []authentication.Authorizer{
		controllerAdminAuthorizer,
		modelPermissionAuthorizer{
			perm: permission.WriteAccess,
		},
	}

	modelObjectsCharmsHTTPHandler := srv.monitoredHandler(objects.NewObjectsCharmHTTPHandler(
		&applicationServiceGetter{ctxt: httpCtxt},
		objects.CharmURLFromLocator,
	), "charms")
	modelObjectsHTTPHandler := srv.monitoredHandler(objects.NewObjectsHTTPHandler(
		&objectStoreServiceGetter{ctxt: httpCtxt},
	), "objects")

	modelToolsUploadHandler := srv.monitoredHandler(newToolsUploadHandler(
		BlockCheckerGetterForServices(httpCtxt.domainServicesForRequestContext),
		modelAgentBinaryStoreForHTTPContext(httpCtxt),
	), "tools")

	// toolsUploadAuthorizer defines the authorizer that MUST be used for tools
	// uploading in the controller. If the user is a controller admin then we
	// can allow the request through, this must also be the case the if the
	// model being uploaded to is the controller model. All other models it is
	// acceptable for the user to be a model admin.
	var toolsUploadAuthorizer httpcontext.CompositeAuthorizer = []authentication.Authorizer{
		controllerAdminAuthorizer,
		controllerModelPermissionAuthorizer{
			controllerAdminAuthorizer: controllerAdminAuthorizer,
			fallThroughAuthorizer: modelPermissionAuthorizer{
				perm: permission.AdminAccess,
			},
			ModelAuthorizationInfo: modelAuthorizationInfoForRequest(),
		},
	}

	modelToolsDownloadHandler := srv.monitoredHandler(newToolsDownloadHandler(httpCtxt), "tools")

	resourceAuthFunc := func(req *http.Request, tagKinds ...string) (names.Tag, error) {
		return httpCtxt.authenticatedTagFromRequest(req, tagKinds...)
	}
	resourceChangeAllowedFunc := func(ctx context.Context) error {
		serviceFactory, err := httpCtxt.domainServicesForRequestContext(ctx)
		if err != nil {
			return errors.Trace(err)
		}

		blockChecker := common.NewBlockChecker(serviceFactory.BlockCommand())
		if err := blockChecker.ChangeAllowed(ctx); err != nil {
			return errors.Trace(err)
		}
		return nil
	}
	resourcesHandler := srv.monitoredHandler(handlersresources.NewResourceHandler(
		resourceAuthFunc,
		resourceChangeAllowedFunc,
		&resourcesResourceServiceGetter{domainServiceForRequest: httpCtxt.domainServicesForRequest},
		&resourcesApplicationServiceGetter{domainServiceForRequest: httpCtxt.domainServicesForRequest},
		resourcesdownload.NewDownloader(logger.Child("resourcedownloader"), resourcesdownload.DefaultFileSystem()),
		logger,
	), "applications")
	unitResourceNewOpenerFunc := resourceOpenerGetter(func(req *http.Request, tagKinds ...string) (coreresource.Opener, error) {
		tagStr := req.URL.Query().Get(":unit")
		tag, err := names.ParseUnitTag(tagStr)
		if err != nil {
			return nil, errors.Trace(err)
		}
		unitName, err := coreunit.NewName(tag.Id())
		if err != nil {
			return nil, errors.Trace(err)
		}

		domainServices, err := httpCtxt.domainServicesForRequest(req)
		if err != nil {
			return nil, errors.Trace(errors.Annotate(err, "cannot get domain services for unit resource request"))
		}
		args := resource.ResourceOpenerArgs{
			ApplicationService: domainServices.Application(),
			ResourceService:    domainServices.Resource(),
			CharmhubClientGetter: resourcecharmhub.NewCharmHubOpener(
				domainServices.Config(),
			),
		}

		opener, err := resource.NewResourceOpenerForUnit(
			req.Context(),
			args,
			srv.getResourceDownloadLimiter,
			unitName,
		)
		if err != nil {
			return nil, errors.Trace(err)
		}
		return opener, nil
	})
	unitResourcesHandler := srv.monitoredHandler(handlersresources.NewUnitResourcesHandler(
		unitResourceNewOpenerFunc,
		logger,
	), "units")

	migrateObjectsCharmsHTTPHandler := srv.monitoredHandler(objects.NewObjectsCharmHTTPHandler(
		&migratingObjectsApplicationServiceGetter{ctxt: httpCtxt},
		objects.CharmURLFromLocatorDuringMigration,
	), "charms")
	migrateToolsUploadHandler := srv.monitoredHandler(newToolsUploadHandler(
		BlockCheckerGetterForServices(httpCtxt.domainServicesForRequestContext),
		migratingAgentBinaryStoreForHTTPContext(httpCtxt),
	), "tools")
	resourcesMigrationUploadHandler := srv.monitoredHandler(handlersresources.NewResourceMigrationUploadHandler(
		&resourcesModelServiceGetter{domainServiceForRequest: httpCtxt.domainServicesDuringMigrationForRequest},
		&resourcesResourceServiceGetter{domainServiceForRequest: httpCtxt.domainServicesDuringMigrationForRequest},
		logger,
	), "applications")
	registerHandler := srv.monitoredHandler(&registerUserHandler{
		ctxt: httpCtxt,
	}, "register")

	// HTTP handler for application offer macaroon authentication.
	if err := handlerscrossmodel.AddOfferAuthHandlers(srv.shared, srv.shared.offersThirdPartyKeyPair, srv.mux, srv.shared.logger); err != nil {
		return nil, errors.Trace(err)
	}

	handlers := []handler{{
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
		pattern:    modelRoutePrefix + "/tools",
		handler:    modelToolsUploadHandler,
		authorizer: toolsUploadAuthorizer,
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
		pattern:    "/migrate/charms/:object",
		handler:    migrateObjectsCharmsHTTPHandler,
		authorizer: controllerAdminAuthorizer,
	}, {
		pattern:    "/migrate/tools",
		handler:    migrateToolsUploadHandler,
		authorizer: controllerAdminAuthorizer,
	}, {
		pattern:    migrateResourcesPrefix,
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
	}, {
		pattern: objectsRoutePrefix,
		methods: []string{"GET"},
		handler: modelObjectsHTTPHandler,
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
// are interruptible (i.e. they pay attention to the apiserver catacomb)
// or are guaranteed to be short-lived. If it's used with long running
// API handlers which don't watch the apiserver's catacomb, apiserver
// shutdown will be blocked until the API handler returns.
func (srv *Server) trackRequests(handler http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Care must be taken to not increment the waitgroup count
		// after the listener has closed.
		//
		// First we check to see if the catacomb has not yet been killed
		// because the closure of the listener depends on the catacomb being
		// killed to trigger the defer block in srv.run.
		select {
		case <-srv.catacomb.Dying():
			// This request was accepted before the listener was closed
			// but after the catacomb was killed. As we're in the process of
			// shutting down, do not consider this request as in progress,
			// just send a 503 and return.
			http.Error(w, "apiserver shutdown in progress", http.StatusServiceUnavailable)
		default:
			// If we get here then the catacomb was not killed therefore the
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

	_, _ = fmt.Fprintf(w, "%s\n", status)
}

func (srv *Server) apiHandler(w http.ResponseWriter, req *http.Request) {
	ctx := req.Context()

	connectionID := atomic.AddUint64(&srv.connectionID, 1)
	fd := -1
	if v, ok := ctx.Value("raw-http-fd").(int); ok {
		fd = v
	}

	apiObserver := srv.newObserver()
	apiObserver.Join(ctx, req, connectionID, fd)
	defer func() {
		// Don't use the request context as it will cause the Leave to be
		// cancelled and not report the leave correctly. Giving it a timeout
		// should ensure that the request doesn't hang indefinitely.
		ctx, cancel := context.WithTimeout(srv.catacomb.Context(context.Background()), time.Second*5)
		defer cancel()

		apiObserver.Leave(ctx)
	}()

	// Create a new offer auth context. This will be used to bake new
	// macaroons for offers, and to validate incoming macaroons.
	crossModelAuthContext, err := srv.shared.NewCrossModelAuthContext(req.Host)
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to create offer auth context: %v", err), http.StatusInternalServerError)
		return
	}

	websocket.Serve(w, req, func(wsConn *websocket.Conn) {
		modelUUID, modelOnlyLogin := httpcontext.RequestModelUUID(req.Context())

		// If the modelUUID wasn't present in the request, then this is
		// considered a controller-only login.
		controllerOnlyLogin := !modelOnlyLogin

		// If the request is for the controller model, then we need to
		// resolve the modelUUID to the controller model.
		resolvedModelUUID := coremodel.UUID(modelUUID)
		if controllerOnlyLogin {
			resolvedModelUUID = srv.shared.controllerModelUUID
		}

		// Put the modelUUID into the context for the request. This will
		// allow the peeling of the modelUUID from the request to be
		// deferred to the facade methods.
		ctx := coremodel.WithContextModelUUID(req.Context(), resolvedModelUUID)
		ctx, cancel := context.WithCancelCause(ctx)
		defer cancel(nil)

		logger.Tracef(ctx, "got a request for model %q fd:%v", modelUUID, fd)

		codec := jsoncodec.NewWebsocket(wsConn.Conn)
		recorderFactory := observer.NewRecorderFactory(apiObserver, nil, observer.NoCaptureArgs)
		rpcConn := rpc.NewConn(codec, recorderFactory)

		if root, err := srv.serveConn(
			srv.catacomb.Context(ctx),
			rpcConn,
			resolvedModelUUID,
			controllerOnlyLogin,
			connectionID,
			apiObserver,
			req.Host,
			crossModelAuthContext,
		); errors.Is(err, modelerrors.NotFound) {
			// If the model is not found then we need to close the connection
			// with the appropriate error so that the client can handle it.
			err := fmt.Errorf("%w: %q", apiservererrors.UnknownModelError, modelUUID)
			rpcConn.ServeRoot(&errRoot{err: errors.Trace(err)}, recorderFactory, serverError)
		} else if err != nil {
			err := fmt.Errorf("serving model %q: %w", modelUUID, err)
			rpcConn.ServeRoot(&errRoot{err: errors.Trace(err)}, recorderFactory, serverError)
		} else {
			rpcConn.ServeRoot(root, recorderFactory, serverError)
		}

		rpcConn.Start(ctx)
		select {
		case <-rpcConn.Dead():
			cancel(ErrRPCConnectionClosed)
		case <-srv.catacomb.Dying():
		}
		if err := rpcConn.Close(); err != nil {
			logger.Errorf(ctx, "error closing RPC connection: %v", err)
		}
	})
}

func (srv *Server) serveConn(
	ctx context.Context,
	conn *rpc.Conn,
	modelUUID coremodel.UUID,
	controllerOnlyLogin bool,
	connectionID uint64,
	apiObserver observer.Observer,
	host string,
	crossModelAuthContext facade.CrossModelAuthContext,
) (rpc.Root, error) {
	domainServices, err := srv.shared.domainServicesGetter.ServicesForModel(ctx, modelUUID)
	if err != nil {
		return nil, errors.Annotatef(err, "getting domain services for model %q", modelUUID)
	}

	if err := srv.isModelAvailable(ctx, domainServices.Model(), modelUUID); err != nil {
		return nil, errors.Annotatef(err, "checking model %q availability", modelUUID)
	}

	tracer, err := srv.shared.tracerGetter.GetTracer(
		ctx,
		coretrace.Namespace("apiserver", modelUUID.String()),
	)
	if err != nil {
		logger.Tracef(ctx, "failed to get tracer for model %q: %v", modelUUID, err)
		tracer = coretrace.NoopTracer{}
	}

	// Grab the object store for the model.
	objectStore, err := srv.shared.objectStoreGetter.GetObjectStore(ctx, modelUUID.String())
	if err != nil {
		return nil, errors.Annotatef(err, "getting object store for model %q", modelUUID)
	}

	// Grab the object store for the controller, this is primarily used for
	// the agent tools.
	controllerObjectStore, err := srv.shared.objectStoreGetter.GetObjectStore(ctx, database.ControllerNS)
	if err != nil {
		return nil, errors.Annotatef(err, "getting controller object store")
	}

	watcherRegistry, err := srv.shared.watcherRegistryGetter.GetWatcherRegistry(ctx, connectionID)
	if err != nil {
		return nil, errors.Annotatef(err, "getting watcher registry for connection %d", connectionID)
	}

	handler := newAPIHandler(
		ctx,
		srv,
		conn,
		domainServices,
		srv.shared.domainServicesGetter,
		tracer,
		objectStore,
		srv.shared.objectStoreGetter,
		controllerObjectStore,
		watcherRegistry,
		modelUUID,
		controllerOnlyLogin,
		connectionID,
		host,
		crossModelAuthContext,
	)

	// Set up the admin apis used to accept logins and direct
	// requests to the relevant business facade.
	// There may be more than one since we need a new API each
	// time login changes in a non-backwards compatible way.
	adminAPIs := make(map[int]interface{})
	for apiVersion, factory := range adminAPIFactories {
		adminAPIs[apiVersion] = factory(srv, handler, apiObserver)
	}

	return newAdminRoot(handler, adminAPIs), nil
}

// ModelService defines the subset of model.Service used to check
// model existence and redirection.
type ModelService interface {
	// CheckModelExists returns whether the model with the given
	// UUID exists on this controller.
	CheckModelExists(ctx context.Context, modelUUID coremodel.UUID) (bool, error)
	// ModelRedirection returns the model redirection information
	// for the given model UUID.
	ModelRedirection(ctx context.Context, modelUUID coremodel.UUID) (model.ModelRedirection, error)
}

func (srv *Server) isModelAvailable(
	ctx context.Context,
	modelService ModelService,
	modelUUID coremodel.UUID,
) error {
	// Check that model exists before proceeding any further. There is no need
	// in setting up any additional operations if the model is not present.
	exists, err := modelService.CheckModelExists(ctx, modelUUID)
	if err != nil {
		return errors.Trace(err)
	} else if exists {
		return nil
	}

	// If this model used to be hosted on this controller but got
	// migrated allow clients to connect and wait for a login
	// request to decide whether the users should be redirected to
	// the new controller for this model or not.
	if _, migErr := modelService.ModelRedirection(ctx, modelUUID); migErr != nil {
		// Return not found on any error.
		// TODO (stickupkid): This is very brute force. What if there
		// is an error with the database? The caller will assume that it
		// is no longer on this controller. If we return a different error
		// then it can at least retry the request.
		return modelerrors.NotFound
	}

	return nil
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

func newOfferAuthContext(
	accessService AccessService,
	macaroonService MacaroonService,
	keyPair *bakery.KeyPair,
	host,
	controllerUUID string,
	controllerModelUUID coremodel.UUID,
	loginTokenRefreshURL string,
	httpClient crossmodelbakery.HTTPClient,
	clock clock.Clock,
	logger corelogger.Logger,
) (*crossmodel.AuthContext, error) {
	bakery, err := getMacaroonBakeryByURL(
		loginTokenRefreshURL,
		macaroonService,
		keyPair,
		host,
		controllerModelUUID,
		httpClient,
		clock,
		logger,
	)
	if err != nil {
		return nil, errors.Annotate(err, "getting macaroon bakery")
	}

	// Create a auth context for offer authentication.
	return crossmodel.NewAuthContext(
		accessService,
		bakery,
		keyPair,
		controllerUUID,
		controllerModelUUID,
		clock,
		logger,
	), nil
}

func authContextLocation(modelUUID coremodel.UUID) string {
	return "juju model " + modelUUID.String()
}

const (
	localOfferAccessLocationPath = "/offeraccess"
)

func getMacaroonBakeryByURL(
	endpoint string,
	macaroonService MacaroonService,
	key *bakery.KeyPair,
	host string,
	controllerModelUUID coremodel.UUID,
	httpClient crossmodelbakery.HTTPClient,
	clock clock.Clock,
	logger corelogger.Logger,
) (crossmodel.OfferBakery, error) {
	location := authContextLocation(controllerModelUUID)
	checker := checkers.New(internalmacaroon.MacaroonNamespace)
	authorizer := crossmodel.NewCMRAuthorizer(logger)

	// Create a local bakery for validating macaroons.
	if endpoint == "" {
		// Create an endpoint that will be used for third-party caveats
		// that require discharge from the local controller.
		endpoint := localEndpointURL(host)
		return crossmodelbakery.NewLocalOfferBakery(
			key,
			location, endpoint,
			macaroonService,
			checker, authorizer,
			clock, logger,
		)
	}

	// We have a URL, it's intended to be used by JAAS, but it's possible
	// that another service could be used here. It's a shame that we don't use
	// a login token service kind here, that way we could ensure that we're
	// creating the correct kind of bakery.
	return crossmodelbakery.NewJAASOfferBakery(
		key,
		location, endpoint,
		macaroonService,
		checker, authorizer,
		httpClient,
		clock, logger,
	)
}

func localEndpointURL(serverHost string) string {
	return (&url.URL{
		Scheme: "https",
		Host:   serverHost,
		Path:   localOfferAccessLocationPath,
	}).String()
}

func serverError(err error) error {
	return apiservererrors.ServerError(err)
}

type applicationServiceGetter struct {
	ctxt httpContext
}

func (a *applicationServiceGetter) Application(r *http.Request) (objects.ApplicationService, error) {
	domainServices, err := a.ctxt.domainServicesForRequest(r)
	if err != nil {
		return nil, internalerrors.Capture(err)
	}

	return domainServices.Application(), nil
}

type migratingObjectsApplicationServiceGetter struct {
	ctxt httpContext
}

func (a *migratingObjectsApplicationServiceGetter) Application(r *http.Request) (objects.ApplicationService, error) {
	domainServices, err := a.ctxt.domainServicesDuringMigrationForRequest(r)
	if err != nil {
		return nil, internalerrors.Capture(err)
	}
	return domainServices.Application(), nil
}

type objectStoreServiceGetter struct {
	ctxt httpContext
}

func (a *objectStoreServiceGetter) ObjectStore(r *http.Request) (objects.ObjectStoreService, error) {
	objectStore, err := a.ctxt.objectStoreForRequest(r.Context())
	if err != nil {
		return nil, internalerrors.Capture(err)
	}

	return objectStore, nil
}

type domainServiceGetter func(r *http.Request) (services.DomainServices, error)

type resourcesModelServiceGetter struct {
	domainServiceForRequest domainServiceGetter
}

func (m *resourcesModelServiceGetter) Model(r *http.Request) (handlersresources.ModelService, error) {
	domainServices, err := m.domainServiceForRequest(r)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return domainServices.ModelInfo(), nil
}

type resourcesApplicationServiceGetter struct {
	domainServiceForRequest domainServiceGetter
}

func (a *resourcesApplicationServiceGetter) Application(r *http.Request) (handlersresources.ApplicationService, error) {
	domainServices, err := a.domainServiceForRequest(r)
	if err != nil {
		return nil, errors.Trace(err)
	}

	return domainServices.Application(), nil
}

type resourcesResourceServiceGetter struct {
	domainServiceForRequest domainServiceGetter
}

func (a *resourcesResourceServiceGetter) Resource(r *http.Request) (handlersresources.ResourceService, error) {
	domainServices, err := a.domainServiceForRequest(r)
	if err != nil {
		return nil, internalerrors.Capture(err)
	}
	return domainServices.Resource(), nil
}

type resourceOpenerGetter func(r *http.Request, tagKinds ...string) (coreresource.Opener, error)

func (rog resourceOpenerGetter) Opener(r *http.Request, tagKinds ...string) (coreresource.Opener, error) {
	return rog(r, tagKinds...)
}
