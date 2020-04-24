// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver_test

import (
	"net/http"
	"time"

	"github.com/juju/clock/testclock"
	"github.com/juju/errors"
	"github.com/juju/names/v4"
	"github.com/juju/pubsub"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v2"
	"github.com/juju/worker/v2/dependency"
	dt "github.com/juju/worker/v2/dependency/testing"
	"github.com/juju/worker/v2/workertest"
	"github.com/prometheus/client_golang/prometheus"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/agent"
	coreapiserver "github.com/juju/juju/apiserver"
	"github.com/juju/juju/apiserver/apiserverhttp"
	"github.com/juju/juju/apiserver/httpcontext"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/auditlog"
	"github.com/juju/juju/core/cache"
	"github.com/juju/juju/core/multiwatcher"
	"github.com/juju/juju/core/presence"
	"github.com/juju/juju/state"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker/apiserver"
	"github.com/juju/juju/worker/gate"
	"github.com/juju/juju/worker/lease"
)

type ManifoldSuite struct {
	testing.IsolationSuite

	manifold dependency.Manifold

	agent                *mockAgent
	auditConfig          stubAuditConfig
	authenticator        *mockAuthenticator
	clock                *testclock.Clock
	context              dependency.Context
	controller           *cache.Controller
	hub                  pubsub.StructuredHub
	leaseManager         *lease.Manager
	metricsCollector     *coreapiserver.Collector
	multiwatcherFactory  multiwatcher.Factory
	mux                  *apiserverhttp.Mux
	prometheusRegisterer stubPrometheusRegisterer
	state                stubStateTracker
	upgradeGate          stubGateWaiter

	stub testing.Stub
}

var _ = gc.Suite(&ManifoldSuite{})

func (s *ManifoldSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)

	s.agent = &mockAgent{}
	s.authenticator = &mockAuthenticator{}
	s.clock = testclock.NewClock(time.Time{})
	controller, err := cache.NewController(cache.ControllerConfig{
		Changes: make(chan interface{}),
	})
	c.Assert(err, jc.ErrorIsNil)
	s.controller = controller
	s.mux = apiserverhttp.NewMux()
	s.state = stubStateTracker{}
	s.metricsCollector = coreapiserver.NewMetricsCollector()
	s.upgradeGate = stubGateWaiter{}
	s.auditConfig = stubAuditConfig{}
	s.multiwatcherFactory = &fakeMultiwatcherFactory{}
	s.leaseManager = &lease.Manager{}
	s.stub.ResetCalls()

	s.context = s.newContext(nil)
	s.manifold = apiserver.Manifold(apiserver.ManifoldConfig{
		AgentName:                         "agent",
		AuthenticatorName:                 "authenticator",
		ClockName:                         "clock",
		MuxName:                           "mux",
		ModelCacheName:                    "modelcache",
		MultiwatcherName:                  "multiwatcher",
		RestoreStatusName:                 "restore-status",
		StateName:                         "state",
		UpgradeGateName:                   "upgrade",
		AuditConfigUpdaterName:            "auditconfig-updater",
		LeaseManagerName:                  "lease-manager",
		RaftTransportName:                 "raft-transport",
		PrometheusRegisterer:              &s.prometheusRegisterer,
		RegisterIntrospectionHTTPHandlers: func(func(string, http.Handler)) {},
		Hub:                               &s.hub,
		Presence:                          presence.New(s.clock),
		NewWorker:                         s.newWorker,
		NewMetricsCollector:               s.newMetricsCollector,
	})
}

func (s *ManifoldSuite) newContext(overlay map[string]interface{}) dependency.Context {
	resources := map[string]interface{}{
		"agent":               s.agent,
		"authenticator":       s.authenticator,
		"clock":               s.clock,
		"mux":                 s.mux,
		"modelcache":          s.controller,
		"multiwatcher":        s.multiwatcherFactory,
		"restore-status":      s.RestoreStatus,
		"state":               &s.state,
		"upgrade":             &s.upgradeGate,
		"auditconfig-updater": s.auditConfig.get,
		"lease-manager":       s.leaseManager,
		"raft-transport":      nil,
	}
	for k, v := range overlay {
		resources[k] = v
	}
	return dt.StubContext(nil, resources)
}

func (s *ManifoldSuite) RestoreStatus() state.RestoreStatus {
	s.stub.MethodCall(s, "RestoreStatus")
	return ""
}

func (s *ManifoldSuite) newWorker(config apiserver.Config) (worker.Worker, error) {
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
	"agent", "authenticator", "clock", "modelcache", "multiwatcher", "mux",
	"restore-status", "state", "upgrade", "auditconfig-updater", "lease-manager",
	"raft-transport",
}

func (s *ManifoldSuite) TestInputs(c *gc.C) {
	c.Assert(s.manifold.Inputs, jc.SameContents, expectedInputs)
}

func (s *ManifoldSuite) TestMissingInputs(c *gc.C) {
	for _, input := range expectedInputs {
		context := s.newContext(map[string]interface{}{
			input: dependency.ErrMissing,
		})
		_, err := s.manifold.Start(context)
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

	c.Assert(config.RestoreStatus, gc.NotNil)
	config.RestoreStatus()
	config.RestoreStatus = nil
	s.stub.CheckCallNames(c, "NewWorker", "RestoreStatus")

	c.Assert(config.RegisterIntrospectionHTTPHandlers, gc.NotNil)
	config.RegisterIntrospectionHTTPHandlers = nil

	c.Assert(config.Presence, gc.NotNil)
	config.Presence = nil

	// NewServer is hard-coded by the manifold to an internal shim.
	c.Assert(config.NewServer, gc.NotNil)
	config.NewServer = nil

	c.Assert(config, jc.DeepEquals, apiserver.Config{
		AgentConfig:         &s.agent.conf,
		Authenticator:       s.authenticator,
		Clock:               s.clock,
		Controller:          s.controller,
		Mux:                 s.mux,
		MultiwatcherFactory: s.multiwatcherFactory,
		StatePool:           &s.state.pool,
		LeaseManager:        s.leaseManager,
		MetricsCollector:    s.metricsCollector,
		Hub:                 &s.hub,
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
	w, err := s.manifold.Start(s.context)
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
	pool state.StatePool
}

func (s *stubStateTracker) Use() (*state.StatePool, error) {
	s.MethodCall(s, "Use")
	return &s.pool, s.NextErr()
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
	httpcontext.LocalMacaroonAuthenticator
}

type fakeMultiwatcherFactory struct {
	multiwatcher.Factory
}
