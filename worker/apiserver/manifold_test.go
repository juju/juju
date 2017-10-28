// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver_test

import (
	"crypto/tls"
	"net/http"
	"time"

	"github.com/juju/errors"
	"github.com/juju/pubsub"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/prometheus/client_golang/prometheus"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"
	worker "gopkg.in/juju/worker.v1"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker/apiserver"
	"github.com/juju/juju/worker/dependency"
	dt "github.com/juju/juju/worker/dependency/testing"
	"github.com/juju/juju/worker/workertest"
)

type ManifoldSuite struct {
	testing.IsolationSuite

	manifold             dependency.Manifold
	context              dependency.Context
	agent                *mockAgent
	clock                *testing.Clock
	state                stubStateTracker
	prometheusRegisterer stubPrometheusRegisterer
	certWatcher          stubCertWatcher
	hub                  pubsub.StructuredHub

	stub testing.Stub
}

var _ = gc.Suite(&ManifoldSuite{})

func (s *ManifoldSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)

	s.agent = &mockAgent{}
	s.clock = testing.NewClock(time.Time{})
	s.state = stubStateTracker{
		done: make(chan struct{}),
	}
	s.prometheusRegisterer = stubPrometheusRegisterer{}
	s.certWatcher = stubCertWatcher{}
	s.stub.ResetCalls()

	s.context = s.newContext(nil)
	s.manifold = apiserver.Manifold(apiserver.ManifoldConfig{
		AgentName:                         "agent",
		CertWatcherName:                   "cert-watcher",
		ClockName:                         "clock",
		StateName:                         "state",
		PrometheusRegisterer:              &s.prometheusRegisterer,
		RegisterIntrospectionHTTPHandlers: func(func(string, http.Handler)) {},
		LoginValidator:                    func(names.Tag) error { return nil },
		Hub:                               &s.hub,
		SetStatePool:                      func(*state.StatePool) {},
		NewStoreAuditEntryFunc:            s.newStoreAuditEntryFunc,
		NewWorker:                         s.newWorker,
	})
}

func (s *ManifoldSuite) newContext(overlay map[string]interface{}) dependency.Context {
	resources := map[string]interface{}{
		"agent":        s.agent,
		"cert-watcher": s.certWatcher.get,
		"clock":        s.clock,
		"state":        &s.state,
	}
	for k, v := range overlay {
		resources[k] = v
	}
	return dt.StubContext(nil, resources)
}

func (s *ManifoldSuite) newStoreAuditEntryFunc(st *state.State) apiserver.StoreAuditEntryFunc {
	s.stub.MethodCall(s, "NewStoreAuditEntryFunc", st)
	return nil
}

func (s *ManifoldSuite) newWorker(config apiserver.Config) (worker.Worker, error) {
	s.stub.MethodCall(s, "NewWorker", config)
	if err := s.stub.NextErr(); err != nil {
		return nil, err
	}
	return worker.NewRunner(worker.RunnerParams{}), nil
}

var expectedInputs = []string{"agent", "cert-watcher", "clock", "state"}

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
	}
}

func (s *ManifoldSuite) TestStart(c *gc.C) {
	w := s.startWorkerClean(c)
	workertest.CleanKill(c, w)

	s.stub.CheckCallNames(c, "NewStoreAuditEntryFunc", "NewWorker")
	args := s.stub.Calls()[1].Args
	c.Assert(args, gc.HasLen, 1)
	c.Assert(args[0], gc.FitsTypeOf, apiserver.Config{})
	config := args[0].(apiserver.Config)

	c.Assert(config.GetCertificate, gc.NotNil)
	c.Assert(config.GetCertificate(), gc.Equals, &s.certWatcher.cert)
	config.GetCertificate = nil

	c.Assert(config.LoginValidator, gc.NotNil)
	config.LoginValidator = nil

	c.Assert(config.RegisterIntrospectionHTTPHandlers, gc.NotNil)
	config.RegisterIntrospectionHTTPHandlers = nil

	c.Assert(config.SetStatePool, gc.NotNil)
	config.SetStatePool = nil

	// NewServer is hard-coded by the manifold to an internal shim.
	c.Assert(config.NewServer, gc.NotNil)
	config.NewServer = nil

	c.Assert(config, jc.DeepEquals, apiserver.Config{
		AgentConfig:          &s.agent.conf,
		Clock:                s.clock,
		State:                &s.state.st,
		PrometheusRegisterer: &s.prometheusRegisterer,
		Hub:                  &s.hub,
	})
}

func (s *ManifoldSuite) TestStopWorkerClosesState(c *gc.C) {
	w := s.startWorkerClean(c)
	defer workertest.CleanKill(c, w)

	select {
	case <-s.state.done:
		c.Fatal("unexpected state release")
	case <-time.After(coretesting.ShortWait):
	}

	workertest.CleanKill(c, w)
	select {
	case <-s.state.done:
	case <-time.After(coretesting.LongWait):
		c.Fatal("timed out waiting for state to be released")
	}
}

func (s *ManifoldSuite) startWorkerClean(c *gc.C) worker.Worker {
	w, err := s.manifold.Start(s.context)
	c.Assert(err, jc.ErrorIsNil)
	workertest.CheckAlive(c, w)
	return w
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
	info    *params.StateServingInfo
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

func (c *mockAgentConfig) StateServingInfo() (params.StateServingInfo, bool) {
	if c.info != nil {
		return *c.info, true
	}
	return params.StateServingInfo{}, false
}

func (c *mockAgentConfig) Value(key string) string {
	return c.values[key]
}

type stubStateTracker struct {
	testing.Stub
	st   state.State
	done chan struct{}
}

func (s *stubStateTracker) Use() (*state.State, error) {
	s.MethodCall(s, "Use")
	return &s.st, s.NextErr()
}

func (s *stubStateTracker) Done() error {
	s.MethodCall(s, "Done")
	err := s.NextErr()
	// close must be the last read or write on stubStateTracker in Done
	close(s.done)
	return err
}

func (s *stubStateTracker) waitDone(c *gc.C) {
	select {
	case <-s.done:
	case <-time.After(coretesting.LongWait):
		c.Fatal("timed out waiting for state to be released")
	}
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

type stubCertWatcher struct {
	testing.Stub
	cert tls.Certificate
}

func (w *stubCertWatcher) get() *tls.Certificate {
	w.MethodCall(w, "get")
	return &w.cert
}
