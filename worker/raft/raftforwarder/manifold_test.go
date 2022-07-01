// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package raftforwarder_test

import (
	"time"

	"github.com/hashicorp/raft"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/pubsub/v2"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v3"
	"github.com/juju/worker/v3/dependency"
	dt "github.com/juju/worker/v3/dependency/testing"
	"github.com/juju/worker/v3/workertest"
	"github.com/prometheus/client_golang/prometheus"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/v2/core/raftlease"
	"github.com/juju/juju/v2/state"
	raftleasestore "github.com/juju/juju/v2/state/raftlease"
	statetesting "github.com/juju/juju/v2/state/testing"
	coretesting "github.com/juju/juju/v2/testing"
	"github.com/juju/juju/v2/worker/common"
	"github.com/juju/juju/v2/worker/raft/raftforwarder"
)

type manifoldSuite struct {
	testing.IsolationSuite
	statetesting.StateSuite

	context  dependency.Context
	manifold dependency.Manifold
	config   raftforwarder.ManifoldConfig

	raft         *raft.Raft
	stateTracker *stubStateTracker
	hub          *pubsub.StructuredHub
	logger       loggo.Logger
	worker       worker.Worker
	target       raftlease.NotifyTarget

	stub testing.Stub
}

var _ = gc.Suite(&manifoldSuite{})

func (s *manifoldSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)

	s.stub.ResetCalls()

	s.raft = &raft.Raft{}
	s.hub = &pubsub.StructuredHub{}

	s.StateSuite.SetUpTest(c)

	s.stateTracker = &stubStateTracker{
		pool: s.StatePool,
		done: make(chan struct{}),
	}
	s.worker = &mockWorker{}
	s.logger = loggo.GetLogger("juju.worker.raftforwarder_test")
	s.target = &struct{ raftlease.NotifyTarget }{}

	s.context = s.newContext(nil)
	s.config = raftforwarder.ManifoldConfig{
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

func (s *manifoldSuite) SetUpSuite(c *gc.C) {
	s.IsolationSuite.SetUpSuite(c)

	err := testing.MgoServer.Start(nil)
	c.Assert(err, jc.ErrorIsNil)
	s.IsolationSuite.AddCleanup(func(*gc.C) { testing.MgoServer.Destroy() })

	s.StateSuite.SetUpSuite(c)
}

func (s *manifoldSuite) TearDownSuite(c *gc.C) {
	s.StateSuite.TearDownSuite(c)
	s.IsolationSuite.TearDownSuite(c)
}

func (s *manifoldSuite) TearDownTest(c *gc.C) {
	s.StateSuite.TearDownTest(c)
	s.IsolationSuite.TearDownTest(c)
}

func (s *manifoldSuite) newContext(overlay map[string]interface{}) dependency.Context {
	resources := map[string]interface{}{
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

func (s *manifoldSuite) newTarget(st *state.State, logger raftleasestore.Logger) raftlease.NotifyTarget {
	s.stub.MethodCall(s, "NewTarget", st, logger)
	return s.target
}

func (s *manifoldSuite) TestValidate(c *gc.C) {
	c.Assert(s.config.Validate(), jc.ErrorIsNil)
	type test struct {
		f      func(cfg *raftforwarder.ManifoldConfig)
		expect string
	}
	tests := []test{{
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
	"raft", "state", "hub",
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
	c.Assert(args, gc.HasLen, 2)
	systemState, err := s.stateTracker.pool.SystemState()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(args[0], gc.Equals, systemState)
	var logWriter raftleasestore.Logger
	c.Assert(args[1], gc.Implements, &logWriter)

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

type stubStateTracker struct {
	testing.Stub
	pool *state.StatePool
	done chan struct{}
}

func (s *stubStateTracker) Use() (*state.StatePool, error) {
	s.MethodCall(s, "Use")
	return s.pool, s.NextErr()
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
