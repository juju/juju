// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver_test

import (
	"context"
	"net/http"
	stdtesting "testing"
	"time"

	"github.com/juju/clock/testclock"
	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/pubsub/v2"
	"github.com/juju/tc"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/dependency"
	dt "github.com/juju/worker/v4/dependency/testing"
	"github.com/juju/worker/v4/workertest"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/juju/juju/agent"
	coreapiserver "github.com/juju/juju/apiserver"
	"github.com/juju/juju/apiserver/apiserverhttp"
	"github.com/juju/juju/apiserver/authentication/macaroon"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/auditlog"
	"github.com/juju/juju/core/changestream"
	corehttp "github.com/juju/juju/core/http"
	corelogger "github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/internal/jwtparser"
	"github.com/juju/juju/internal/services"
	"github.com/juju/juju/internal/testhelpers"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/worker/apiserver"
	"github.com/juju/juju/internal/worker/gate"
	"github.com/juju/juju/internal/worker/lease"
	"github.com/juju/juju/internal/worker/trace"
	"github.com/juju/juju/state"
)

type ManifoldSuite struct {
	testhelpers.IsolationSuite

	manifold dependency.Manifold

	agent                   *mockAgent
	auditConfig             stubAuditConfig
	authenticator           *mockAuthenticator
	clock                   *testclock.Clock
	getter                  dependency.Getter
	hub                     pubsub.StructuredHub
	leaseManager            *lease.Manager
	metricsCollector        *coreapiserver.Collector
	mux                     *apiserverhttp.Mux
	prometheusRegisterer    stubPrometheusRegisterer
	state                   stubStateTracker
	upgradeGate             stubGateWaiter
	logSink                 corelogger.ModelLogger
	httpClientGetter        *stubHTTPClientGetter
	charmhubHTTPClient      *http.Client
	dbGetter                stubWatchableDBGetter
	dbDeleter               stubDBDeleter
	domainServicesGetter    *stubDomainServicesGetter
	controllerConfigService *MockControllerConfigService
	modelService            *MockModelService
	tracerGetter            stubTracerGetter
	objectStoreGetter       stubObjectStoreGetter
	jwtParser               *jwtparser.Parser

	stub testhelpers.Stub
}

func TestManifoldSuite(t *stdtesting.T) {
	tc.Run(t, &ManifoldSuite{})
}

func (s *ManifoldSuite) SetUpTest(c *tc.C) {
	s.IsolationSuite.SetUpTest(c)

	s.agent = &mockAgent{}
	s.authenticator = &mockAuthenticator{}
	s.clock = testclock.NewClock(time.Time{})
	s.mux = apiserverhttp.NewMux()
	s.state = stubStateTracker{}
	s.metricsCollector = coreapiserver.NewMetricsCollector()
	s.upgradeGate = stubGateWaiter{}
	s.auditConfig = stubAuditConfig{}
	s.leaseManager = &lease.Manager{}
	s.logSink = &mockModelLogger{}
	s.charmhubHTTPClient = &http.Client{}
	s.httpClientGetter = &stubHTTPClientGetter{
		client: s.charmhubHTTPClient,
	}
	s.jwtParser = &jwtparser.Parser{}
	s.stub.ResetCalls()
	s.domainServicesGetter = &stubDomainServicesGetter{}
	s.dbDeleter = stubDBDeleter{}

	s.getter = s.newGetter(nil)
	s.manifold = apiserver.Manifold(apiserver.ManifoldConfig{
		AgentName:                         "agent",
		AuthenticatorName:                 "authenticator",
		ClockName:                         "clock",
		MuxName:                           "mux",
		StateName:                         "state",
		UpgradeGateName:                   "upgrade",
		AuditConfigUpdaterName:            "auditconfig-updater",
		LeaseManagerName:                  "lease-manager",
		LogSinkName:                       "log-sink",
		HTTPClientName:                    "http-client",
		DomainServicesName:                "domain-services",
		TraceName:                         "trace",
		ObjectStoreName:                   "object-store",
		ChangeStreamName:                  "change-stream",
		DBAccessorName:                    "db-accessor",
		JWTParserName:                     "jwt-parser",
		PrometheusRegisterer:              &s.prometheusRegisterer,
		RegisterIntrospectionHTTPHandlers: func(func(string, http.Handler)) {},
		Hub:                               &s.hub,
		GetControllerConfigService: func(getter dependency.Getter, name string) (apiserver.ControllerConfigService, error) {
			return s.controllerConfigService, nil
		},
		GetModelService: func(getter dependency.Getter, name string) (apiserver.ModelService, error) {
			return s.modelService, nil
		},
		NewWorker:           s.newWorker,
		NewMetricsCollector: s.newMetricsCollector,
	})
}

func (s *ManifoldSuite) newGetter(overlay map[string]interface{}) dependency.Getter {
	resources := map[string]interface{}{
		"agent":               s.agent,
		"authenticator":       s.authenticator,
		"clock":               s.clock,
		"mux":                 s.mux,
		"state":               &s.state,
		"upgrade":             &s.upgradeGate,
		"auditconfig-updater": s.auditConfig.get,
		"lease-manager":       s.leaseManager,
		"log-sink":            s.logSink,
		"http-client":         s.httpClientGetter,
		"change-stream":       s.dbGetter,
		"db-accessor":         s.dbDeleter,
		"domain-services":     s.domainServicesGetter,
		"trace":               s.tracerGetter,
		"object-store":        s.objectStoreGetter,
		"jwt-parser":          s.jwtParser,
	}
	for k, v := range overlay {
		resources[k] = v
	}
	return dt.StubGetter(resources)
}

type mockModelLogger struct {
	corelogger.ModelLogger
}

func (*mockModelLogger) Log([]corelogger.LogRecord) error {
	return nil
}

func (s *ManifoldSuite) newWorker(ctx context.Context, config apiserver.Config) (worker.Worker, error) {
	s.stub.MethodCall(s, "NewWorker", config)
	if err := s.stub.NextErr(); err != nil {
		return nil, err
	}
	return worker.NewRunner(worker.RunnerParams{
		Name: "apiserver",
	})
}

func (s *ManifoldSuite) newMetricsCollector() *coreapiserver.Collector {
	return s.metricsCollector
}

var expectedInputs = []string{
	"agent", "authenticator", "clock", "mux",
	"state", "upgrade", "auditconfig-updater", "lease-manager",
	"http-client", "change-stream",
	"domain-services", "trace", "object-store", "log-sink", "db-accessor",
	"jwt-parser",
}

func (s *ManifoldSuite) TestInputs(c *tc.C) {
	c.Assert(s.manifold.Inputs, tc.SameContents, expectedInputs)
}

func (s *ManifoldSuite) TestMissingInputs(c *tc.C) {
	for _, input := range expectedInputs {
		getter := s.newGetter(map[string]interface{}{
			input: dependency.ErrMissing,
		})
		_, err := s.manifold.Start(c.Context(), getter)
		c.Assert(err, tc.ErrorIs, dependency.ErrMissing)

		// The state tracker must have either no calls, or a Use and a Done.
		if len(s.state.Calls()) > 0 {
			s.state.CheckCallNames(c, "Use", "Done")
		}
		s.state.ResetCalls()
	}
}

func (s *ManifoldSuite) TestStart(c *tc.C) {
	w := s.startWorkerClean(c)
	workertest.CleanKill(c, w)

	s.stub.CheckCallNames(c, "NewWorker")
	args := s.stub.Calls()[0].Args
	c.Assert(args, tc.HasLen, 1)
	c.Assert(args[0], tc.FitsTypeOf, apiserver.Config{})
	config := args[0].(apiserver.Config)

	c.Assert(config.GetAuditConfig, tc.NotNil)
	c.Assert(config.GetAuditConfig(), tc.DeepEquals, s.auditConfig.config)
	config.GetAuditConfig = nil

	c.Assert(config.UpgradeComplete, tc.NotNil)
	config.UpgradeComplete()
	config.UpgradeComplete = nil
	s.upgradeGate.CheckCallNames(c, "IsUnlocked")

	c.Assert(config.RegisterIntrospectionHTTPHandlers, tc.NotNil)
	config.RegisterIntrospectionHTTPHandlers = nil

	// NewServer is hard-coded by the manifold to an internal shim.
	c.Assert(config.NewServer, tc.NotNil)
	config.NewServer = nil

	// EmbeddedCommand is hard-coded by the manifold to an internal shim.
	c.Assert(config.EmbeddedCommand, tc.NotNil)
	config.EmbeddedCommand = nil

	c.Assert(config, tc.DeepEquals, apiserver.Config{
		AgentConfig:                &s.agent.conf,
		LocalMacaroonAuthenticator: s.authenticator,
		Clock:                      s.clock,
		Mux:                        s.mux,
		StatePool:                  &s.state.pool,
		LeaseManager:               s.leaseManager,
		MetricsCollector:           s.metricsCollector,
		Hub:                        &s.hub,
		LogSink:                    s.logSink,
		CharmhubHTTPClient:         s.charmhubHTTPClient,
		DBGetter:                   s.dbGetter,
		DBDeleter:                  s.dbDeleter,
		DomainServicesGetter:       s.domainServicesGetter,
		ControllerConfigService:    s.controllerConfigService,
		TracerGetter:               s.tracerGetter,
		ObjectStoreGetter:          s.objectStoreGetter,
		ModelService:               s.modelService,
		JWTParser:                  s.jwtParser,
	})
}

func (s *ManifoldSuite) TestStopWorkerClosesState(c *tc.C) {
	w := s.startWorkerClean(c)
	defer workertest.CleanKill(c, w)

	s.state.CheckCallNames(c, "Use")

	workertest.CleanKill(c, w)
	s.state.CheckCallNames(c, "Use", "Done")
}

func (s *ManifoldSuite) startWorkerClean(c *tc.C) worker.Worker {
	w, err := s.manifold.Start(c.Context(), s.getter)
	c.Assert(err, tc.ErrorIsNil)
	workertest.CheckAlive(c, w)
	return w
}

func (s *ManifoldSuite) TestAddsAndRemovesMuxClients(c *tc.C) {
	waitFinished := make(chan struct{})
	w := s.startWorkerClean(c)
	go func() {
		defer close(waitFinished)
		s.mux.Wait()
	}()

	select {
	case <-waitFinished:
		c.Fatalf("didn't add clients to the mux")
	case <-time.After(coretesting.ShortWait):
	}

	workertest.CleanKill(c, w)

	select {
	case <-waitFinished:
	case <-time.After(coretesting.LongWait):
		c.Fatalf("didn't tell the mux we were finished")
	}
}

type mockAgent struct {
	agent.Agent
	conf mockAgentConfig
}

func (ma *mockAgent) CurrentConfig() agent.Config {
	return &ma.conf
}

type mockAgentConfig struct {
	agent.Config
	dataDir string
	logDir  string
	info    *controller.StateServingInfo
	values  map[string]string
}

func (c *mockAgentConfig) Tag() names.Tag {
	return names.NewMachineTag("123")
}

func (c *mockAgentConfig) LogDir() string {
	return c.logDir
}

func (c *mockAgentConfig) DataDir() string {
	return c.dataDir
}

func (c *mockAgentConfig) StateServingInfo() (controller.StateServingInfo, bool) {
	if c.info != nil {
		return *c.info, true
	}
	return controller.StateServingInfo{}, false
}

func (c *mockAgentConfig) Value(key string) string {
	return c.values[key]
}

type stubStateTracker struct {
	testhelpers.Stub
	pool  state.StatePool
	state state.State
}

func (s *stubStateTracker) Use() (*state.StatePool, *state.State, error) {
	s.MethodCall(s, "Use")
	return &s.pool, &s.state, s.NextErr()
}

func (s *stubStateTracker) Done() error {
	s.MethodCall(s, "Done")
	return s.NextErr()
}

func (s *stubStateTracker) Report() map[string]interface{} {
	s.MethodCall(s, "Report")
	return nil
}

type stubPrometheusRegisterer struct {
	testhelpers.Stub
}

func (s *stubPrometheusRegisterer) MustRegister(...prometheus.Collector) {
	panic("should not be called")
}

func (s *stubPrometheusRegisterer) Register(c prometheus.Collector) error {
	s.MethodCall(s, "Register", c)
	return s.NextErr()
}

func (s *stubPrometheusRegisterer) Unregister(c prometheus.Collector) bool {
	s.MethodCall(s, "Unregister", c)
	return false
}

type stubGateWaiter struct {
	testhelpers.Stub
	gate.Waiter
}

func (w *stubGateWaiter) IsUnlocked() bool {
	w.MethodCall(w, "IsUnlocked")
	return true
}

type stubAuditConfig struct {
	testhelpers.Stub
	config auditlog.Config
}

func (c *stubAuditConfig) get() auditlog.Config {
	c.MethodCall(c, "get")
	return c.config
}

type mockAuthenticator struct {
	macaroon.LocalMacaroonAuthenticator
}

type stubWatchableDBGetter struct{}

func (s stubWatchableDBGetter) GetWatchableDB(namespace string) (changestream.WatchableDB, error) {
	if namespace != "controller" {
		return nil, errors.Errorf(`expected a request for "controller" DB; got %q`, namespace)
	}
	return nil, nil
}

type stubDBDeleter struct{}

func (s stubDBDeleter) DeleteDB(namespace string) error {
	return nil
}

type stubDomainServicesGetter struct {
	services.DomainServicesGetter
}

func (s *stubDomainServicesGetter) ServicesForModel(context.Context, model.UUID) (services.DomainServices, error) {
	return nil, nil
}

type stubTracerGetter struct {
	trace.TracerGetter
}

type stubObjectStoreGetter struct {
	objectstore.ObjectStoreGetter
}

type stubHTTPClientGetter struct {
	client *http.Client
}

func (s *stubHTTPClientGetter) GetHTTPClient(ctx context.Context, namespace corehttp.Purpose) (corehttp.HTTPClient, error) {
	return s.client, nil
}
