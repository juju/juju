// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver_test

import (
	"context"
	"net/http"
	"time"

	"github.com/juju/clock/testclock"
	"github.com/juju/errors"
	"github.com/juju/names/v5"
	"github.com/juju/pubsub/v2"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/dependency"
	dt "github.com/juju/worker/v4/dependency/testing"
	"github.com/juju/worker/v4/workertest"
	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/agent"
	coreapiserver "github.com/juju/juju/apiserver"
	"github.com/juju/juju/apiserver/apiserverhttp"
	"github.com/juju/juju/apiserver/authentication/macaroon"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/auditlog"
	"github.com/juju/juju/core/changestream"
	corelogger "github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/core/presence"
	"github.com/juju/juju/internal/servicefactory"
	"github.com/juju/juju/internal/worker/apiserver"
	"github.com/juju/juju/internal/worker/gate"
	"github.com/juju/juju/internal/worker/lease"
	"github.com/juju/juju/internal/worker/trace"
	"github.com/juju/juju/state"
	coretesting "github.com/juju/juju/testing"
)

type ManifoldSuite struct {
	testing.IsolationSuite

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
	charmhubHTTPClient      *http.Client
	dbGetter                stubWatchableDBGetter
	dbDeleter               stubDBDeleter
	serviceFactoryGetter    *stubServiceFactoryGetter
	controllerConfigService *MockControllerConfigService
	tracerGetter            stubTracerGetter
	objectStoreGetter       stubObjectStoreGetter

	stub testing.Stub
}

var _ = gc.Suite(&ManifoldSuite{})

func (s *ManifoldSuite) SetUpTest(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
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
	s.stub.ResetCalls()
	s.serviceFactoryGetter = &stubServiceFactoryGetter{}
	s.dbDeleter = stubDBDeleter{}
	s.controllerConfigService = NewMockControllerConfigService(ctrl)

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
		CharmhubHTTPClientName:            "charmhub-http-client",
		ServiceFactoryName:                "service-factory",
		TraceName:                         "trace",
		ObjectStoreName:                   "object-store",
		ChangeStreamName:                  "change-stream",
		DBAccessorName:                    "db-accessor",
		PrometheusRegisterer:              &s.prometheusRegisterer,
		RegisterIntrospectionHTTPHandlers: func(func(string, http.Handler)) {},
		Hub:                               &s.hub,
		Presence:                          presence.New(s.clock),
		GetControllerConfigService: func(getter dependency.Getter, name string) (apiserver.ControllerConfigService, error) {
			return s.controllerConfigService, nil
		},
		NewWorker:           s.newWorker,
		NewMetricsCollector: s.newMetricsCollector,
	})
}

func (s *ManifoldSuite) newGetter(overlay map[string]interface{}) dependency.Getter {
	resources := map[string]interface{}{
		"agent":                s.agent,
		"authenticator":        s.authenticator,
		"clock":                s.clock,
		"mux":                  s.mux,
		"state":                &s.state,
		"upgrade":              &s.upgradeGate,
		"auditconfig-updater":  s.auditConfig.get,
		"lease-manager":        s.leaseManager,
		"log-sink":             s.logSink,
		"charmhub-http-client": s.charmhubHTTPClient,
		"change-stream":        s.dbGetter,
		"db-accessor":          s.dbDeleter,
		"service-factory":      s.serviceFactoryGetter,
		"trace":                s.tracerGetter,
		"object-store":         s.objectStoreGetter,
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
	return worker.NewRunner(worker.RunnerParams{}), nil
}

func (s *ManifoldSuite) newMetricsCollector() *coreapiserver.Collector {
	return s.metricsCollector
}

var expectedInputs = []string{
	"agent", "authenticator", "clock", "mux",
	"state", "upgrade", "auditconfig-updater", "lease-manager",
	"charmhub-http-client", "change-stream", "service-factory",
	"trace", "object-store", "log-sink", "db-accessor",
}

func (s *ManifoldSuite) TestInputs(c *gc.C) {
	c.Assert(s.manifold.Inputs, jc.SameContents, expectedInputs)
}

func (s *ManifoldSuite) TestMissingInputs(c *gc.C) {
	for _, input := range expectedInputs {
		getter := s.newGetter(map[string]interface{}{
			input: dependency.ErrMissing,
		})
		_, err := s.manifold.Start(context.Background(), getter)
		c.Assert(errors.Cause(err), gc.Equals, dependency.ErrMissing)

		// The state tracker must have either no calls, or a Use and a Done.
		if len(s.state.Calls()) > 0 {
			s.state.CheckCallNames(c, "Use", "Done")
		}
		s.state.ResetCalls()
	}
}

func (s *ManifoldSuite) TestStart(c *gc.C) {
	w := s.startWorkerClean(c)
	workertest.CleanKill(c, w)

	s.stub.CheckCallNames(c, "NewWorker")
	args := s.stub.Calls()[0].Args
	c.Assert(args, gc.HasLen, 1)
	c.Assert(args[0], gc.FitsTypeOf, apiserver.Config{})
	config := args[0].(apiserver.Config)

	c.Assert(config.GetAuditConfig, gc.NotNil)
	c.Assert(config.GetAuditConfig(), gc.DeepEquals, s.auditConfig.config)
	config.GetAuditConfig = nil

	c.Assert(config.UpgradeComplete, gc.NotNil)
	config.UpgradeComplete()
	config.UpgradeComplete = nil
	s.upgradeGate.CheckCallNames(c, "IsUnlocked")

	c.Assert(config.RegisterIntrospectionHTTPHandlers, gc.NotNil)
	config.RegisterIntrospectionHTTPHandlers = nil

	c.Assert(config.Presence, gc.NotNil)
	config.Presence = nil

	// NewServer is hard-coded by the manifold to an internal shim.
	c.Assert(config.NewServer, gc.NotNil)
	config.NewServer = nil

	// EmbeddedCommand is hard-coded by the manifold to an internal shim.
	c.Assert(config.EmbeddedCommand, gc.NotNil)
	config.EmbeddedCommand = nil

	c.Assert(config, jc.DeepEquals, apiserver.Config{
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
		ServiceFactoryGetter:       s.serviceFactoryGetter,
		TracerGetter:               s.tracerGetter,
		ObjectStoreGetter:          s.objectStoreGetter,
		ControllerConfigService:    s.controllerConfigService,
	})
}

func (s *ManifoldSuite) TestStopWorkerClosesState(c *gc.C) {
	w := s.startWorkerClean(c)
	defer workertest.CleanKill(c, w)

	s.state.CheckCallNames(c, "Use")

	workertest.CleanKill(c, w)
	s.state.CheckCallNames(c, "Use", "Done")
}

func (s *ManifoldSuite) startWorkerClean(c *gc.C) worker.Worker {
	w, err := s.manifold.Start(context.Background(), s.getter)
	c.Assert(err, jc.ErrorIsNil)
	workertest.CheckAlive(c, w)
	return w
}

func (s *ManifoldSuite) TestAddsAndRemovesMuxClients(c *gc.C) {
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
	testing.Stub
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
	testing.Stub
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
	testing.Stub
	gate.Waiter
}

func (w *stubGateWaiter) IsUnlocked() bool {
	w.MethodCall(w, "IsUnlocked")
	return true
}

type stubAuditConfig struct {
	testing.Stub
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

type stubServiceFactoryGetter struct {
	servicefactory.ServiceFactoryGetter
}

func (s *stubServiceFactoryGetter) FactoryForModel(modelUUID string) servicefactory.ServiceFactory {
	return nil
}

type stubTracerGetter struct {
	trace.TracerGetter
}

type stubObjectStoreGetter struct {
	objectstore.ObjectStoreGetter
}
