// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package raftforwarder_test

import (
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/hashicorp/raft"
	"github.com/juju/errors"
	"github.com/juju/loggo"
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
	"gopkg.in/natefinch/lumberjack.v2"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/core/raftlease"
	"github.com/juju/juju/state"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker/common"
	"github.com/juju/juju/worker/raft/raftforwarder"
)

type manifoldSuite struct {
	testing.IsolationSuite

	context  dependency.Context
	manifold dependency.Manifold
	config   raftforwarder.ManifoldConfig

	agent        *mockAgent
	raft         *raft.Raft
	stateTracker *stubStateTracker
	hub          *pubsub.StructuredHub
	logger       loggo.Logger
	worker       worker.Worker
	target       raftlease.NotifyTarget

	logDir string
	stub   testing.Stub
}

var _ = gc.Suite(&manifoldSuite{})

func (s *manifoldSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)

	s.stub.ResetCalls()

	s.logDir = c.MkDir()
	s.agent = &mockAgent{conf: mockAgentConfig{
		logDir: s.logDir,
		uuid:   "controller-uuid",
	}}
	s.stateTracker = &stubStateTracker{
		done: make(chan struct{}),
	}

	s.raft = &raft.Raft{}
	s.hub = &pubsub.StructuredHub{}

	s.worker = &mockWorker{}
	s.logger = loggo.GetLogger("juju.worker.raftforwarder_test")
	s.target = &struct{ raftlease.NotifyTarget }{}

	s.context = s.newContext(nil)
	s.config = raftforwarder.ManifoldConfig{
		AgentName:            "agent",
		RaftName:             "raft",
		StateName:            "state",
		CentralHubName:       "hub",
		RequestTopic:         "test.request",
		Logger:               &s.logger,
		PrometheusRegisterer: &noopRegisterer{},
		NewWorker:            s.newWorker,
		NewTarget:            s.newTarget,
	}
	s.manifold = raftforwarder.Manifold(s.config)
}

func (s *manifoldSuite) newContext(overlay map[string]interface{}) dependency.Context {
	resources := map[string]interface{}{
		"agent": s.agent,
		"raft":  s.raft,
		"state": s.stateTracker,
		"hub":   s.hub,
	}
	for k, v := range overlay {
		resources[k] = v
	}
	return dt.StubContext(nil, resources)
}

func (s *manifoldSuite) newWorker(config raftforwarder.Config) (worker.Worker, error) {
	s.stub.MethodCall(s, "NewWorker", config)
	if err := s.stub.NextErr(); err != nil {
		return nil, err
	}
	return s.worker, nil
}

func (s *manifoldSuite) newTarget(st *state.State, logFile io.Writer, logger raftforwarder.Logger) raftlease.NotifyTarget {
	s.stub.MethodCall(s, "NewTarget", st, logFile, logger)
	return s.target
}

func (s *manifoldSuite) TestValidate(c *gc.C) {
	c.Assert(s.config.Validate(), jc.ErrorIsNil)
	type test struct {
		f      func(cfg *raftforwarder.ManifoldConfig)
		expect string
	}
	tests := []test{{
		func(cfg *raftforwarder.ManifoldConfig) { cfg.AgentName = "" },
		"empty AgentName not valid",
	}, {
		func(cfg *raftforwarder.ManifoldConfig) { cfg.StateName = "" },
		"empty StateName not valid",
	}, {
		func(cfg *raftforwarder.ManifoldConfig) { cfg.CentralHubName = "" },
		"empty CentralHubName not valid",
	}, {
		func(cfg *raftforwarder.ManifoldConfig) { cfg.RequestTopic = "" },
		"empty RequestTopic not valid",
	}, {
		func(cfg *raftforwarder.ManifoldConfig) { cfg.Logger = nil },
		"nil Logger not valid",
	}, {
		func(cfg *raftforwarder.ManifoldConfig) { cfg.PrometheusRegisterer = nil },
		"nil PrometheusRegisterer not valid",
	}, {
		func(cfg *raftforwarder.ManifoldConfig) { cfg.NewWorker = nil },
		"nil NewWorker not valid",
	}, {
		func(cfg *raftforwarder.ManifoldConfig) { cfg.NewTarget = nil },
		"nil NewTarget not valid",
	}}
	for i, test := range tests {
		c.Logf("test #%d (%s)", i, test.expect)
		// Local copy before mutating.
		cfg := s.config
		test.f(&cfg)
		c.Assert(cfg.Validate(), gc.ErrorMatches, test.expect)
	}
}

var expectedInputs = []string{
	"agent", "raft", "state", "hub",
}

func (s *manifoldSuite) TestInputs(c *gc.C) {
	c.Assert(s.manifold.Inputs, jc.SameContents, expectedInputs)
}

func (s *manifoldSuite) TestMissingInputs(c *gc.C) {
	for _, input := range expectedInputs {
		context := s.newContext(map[string]interface{}{
			input: dependency.ErrMissing,
		})
		_, err := s.manifold.Start(context)
		c.Assert(errors.Cause(err), gc.Equals, dependency.ErrMissing)
	}
}

func (s *manifoldSuite) TestStart(c *gc.C) {
	w, err := s.manifold.Start(s.context)
	c.Assert(err, jc.ErrorIsNil)
	cleanupW, ok := w.(*common.CleanupWorker)
	c.Assert(ok, gc.Equals, true)
	c.Assert(cleanupW.Worker, gc.Equals, s.worker)

	s.stub.CheckCallNames(c, "NewTarget", "NewWorker")

	args := s.stub.Calls()[0].Args
	c.Assert(args, gc.HasLen, 3)
	c.Assert(args[0], gc.Equals, s.stateTracker.pool.SystemState())
	c.Assert(args[1], gc.FitsTypeOf, &lumberjack.Logger{})

	expectedPath := filepath.Join(s.logDir, "lease.log")
	c.Assert(args[1].(*lumberjack.Logger).Filename, gc.Equals, expectedPath)
	stat, err := os.Stat(expectedPath)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(stat.Mode(), gc.Equals, os.FileMode(0640))
	c.Assert(args[2], gc.Equals, &s.logger)

	args = s.stub.Calls()[1].Args
	c.Assert(args, gc.HasLen, 1)
	c.Assert(args[0], gc.FitsTypeOf, raftforwarder.Config{})
	config := args[0].(raftforwarder.Config)

	c.Assert(config, jc.DeepEquals, raftforwarder.Config{
		Raft:                 s.raft,
		Hub:                  s.hub,
		Logger:               &s.logger,
		Topic:                "test.request",
		Target:               s.target,
		PrometheusRegisterer: s.config.PrometheusRegisterer,
	})
}

func (s *manifoldSuite) TestStoppingWorkerReleasesState(c *gc.C) {
	w, err := s.manifold.Start(s.context)
	c.Assert(err, jc.ErrorIsNil)

	s.stateTracker.CheckCallNames(c, "Use")
	select {
	case <-s.stateTracker.done:
		c.Fatal("unexpected state release")
	case <-time.After(coretesting.ShortWait):
	}

	// Stopping the worker should cause the state to
	// eventually be released.
	workertest.CleanKill(c, w)

	s.stateTracker.waitDone(c)
	s.stateTracker.CheckCallNames(c, "Use", "Done")
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
	logDir string
	uuid   string
}

func (c *mockAgentConfig) LogDir() string {
	return c.logDir
}

func (c *mockAgentConfig) Controller() names.ControllerTag {
	return names.NewControllerTag(c.uuid)
}

type stubStateTracker struct {
	testing.Stub
	pool state.StatePool
	done chan struct{}
}

func (s *stubStateTracker) Use() (*state.StatePool, error) {
	s.MethodCall(s, "Use")
	return &s.pool, s.NextErr()
}

func (s *stubStateTracker) Done() error {
	s.MethodCall(s, "Done")
	err := s.NextErr()
	close(s.done)
	return err
}

func (s *stubStateTracker) Report() map[string]interface{} {
	return map[string]interface{}{"hey": "mum"}
}

func (s *stubStateTracker) waitDone(c *gc.C) {
	select {
	case <-s.done:
	case <-time.After(coretesting.LongWait):
		c.Fatal("timed out waiting for state to be released")
	}
}

type mockWorker struct{}

func (w *mockWorker) Kill() {}
func (w *mockWorker) Wait() error {
	return nil
}

type noopRegisterer struct {
	prometheus.Registerer
}

func (noopRegisterer) Register(prometheus.Collector) error {
	return nil
}

func (noopRegisterer) Unregister(prometheus.Collector) bool {
	return true
}
