// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package manifold_test

import (
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/juju/clock/testclock"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/pubsub"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"
	"gopkg.in/juju/worker.v1"
	"gopkg.in/juju/worker.v1/dependency"
	dt "gopkg.in/juju/worker.v1/dependency/testing"
	"gopkg.in/juju/worker.v1/workertest"
	"gopkg.in/natefinch/lumberjack.v2"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/core/globalclock"
	corelease "github.com/juju/juju/core/lease"
	"github.com/juju/juju/core/raftlease"
	"github.com/juju/juju/state"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker/common"
	"github.com/juju/juju/worker/lease"
	leasemanager "github.com/juju/juju/worker/lease/manifold"
)

type manifoldSuite struct {
	testing.IsolationSuite

	context  dependency.Context
	manifold dependency.Manifold

	agent        *mockAgent
	clock        *testclock.Clock
	stateTracker *stubStateTracker
	hub          *pubsub.StructuredHub

	fsm    *raftlease.FSM
	logger loggo.Logger

	worker worker.Worker
	store  *raftlease.Store
	target raftlease.NotifyTarget

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
	s.clock = testclock.NewClock(time.Now())
	s.stateTracker = &stubStateTracker{
		done: make(chan struct{}),
	}
	s.hub = pubsub.NewStructuredHub(nil)
	s.fsm = raftlease.NewFSM()
	s.logger = loggo.GetLogger("lease.manifold_test")

	s.worker = &mockWorker{}
	s.store = &raftlease.Store{}
	s.target = &struct{ raftlease.NotifyTarget }{}

	s.context = s.newContext(nil)
	s.manifold = leasemanager.Manifold(leasemanager.ManifoldConfig{
		AgentName:      "agent",
		ClockName:      "clock",
		StateName:      "state",
		CentralHubName: "hub",
		FSM:            s.fsm,
		RequestTopic:   "lease.manifold_test",
		Logger:         &s.logger,
		NewWorker:      s.newWorker,
		NewStore:       s.newStore,
		NewTarget:      s.newTarget,
	})
}

func (s *manifoldSuite) newContext(overlay map[string]interface{}) dependency.Context {
	resources := map[string]interface{}{
		"agent": s.agent,
		"clock": s.clock,
		"state": s.stateTracker,
		"hub":   s.hub,
	}
	for k, v := range overlay {
		resources[k] = v
	}
	return dt.StubContext(nil, resources)
}

func (s *manifoldSuite) newWorker(config lease.ManagerConfig) (worker.Worker, error) {
	s.stub.MethodCall(s, "NewWorker", config)
	if err := s.stub.NextErr(); err != nil {
		return nil, err
	}
	return s.worker, nil
}

func (s *manifoldSuite) newStore(config raftlease.StoreConfig) *raftlease.Store {
	s.stub.MethodCall(s, "NewStore", config)
	return s.store
}

func (s *manifoldSuite) newTarget(st *state.State, logFile io.Writer, logger lease.Logger) raftlease.NotifyTarget {
	s.stub.MethodCall(s, "NewTarget", st, logFile, logger)
	return s.target
}

var expectedInputs = []string{
	"agent", "clock", "state", "hub",
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

	s.stub.CheckCallNames(c, "NewTarget", "NewStore", "NewWorker")

	args := s.stub.Calls()[0].Args
	c.Assert(args, gc.HasLen, 3)
	c.Assert(args[0], gc.Equals, s.stateTracker.pool.SystemState())
	c.Assert(args[1], gc.FitsTypeOf, &lumberjack.Logger{})

	expectedPath := filepath.Join(s.logDir, "lease.log")
	c.Assert(args[1].(*lumberjack.Logger).Filename, gc.Equals, expectedPath)
	stat, err := os.Stat(expectedPath)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(stat.Mode(), gc.Equals, os.FileMode(0600))
	c.Assert(args[2], gc.Equals, &s.logger)

	args = s.stub.Calls()[1].Args
	c.Assert(args, gc.HasLen, 1)
	c.Assert(args[0], gc.FitsTypeOf, raftlease.StoreConfig{})
	storeConfig := args[0].(raftlease.StoreConfig)
	c.Assert(storeConfig.ResponseTopic(1234), gc.Matches, "lease.manifold_test.[0-9a-f]{8}.1234")
	storeConfig.ResponseTopic = nil
	c.Assert(storeConfig, gc.DeepEquals, raftlease.StoreConfig{
		FSM:            s.fsm,
		Hub:            s.hub,
		Target:         s.target,
		RequestTopic:   "lease.manifold_test",
		Clock:          s.clock,
		ForwardTimeout: 5 * time.Second,
	})

	args = s.stub.Calls()[2].Args
	c.Assert(args, gc.HasLen, 1)
	c.Assert(args[0], gc.FitsTypeOf, lease.ManagerConfig{})
	config := args[0].(lease.ManagerConfig)

	secretary, err := config.Secretary(corelease.SingularControllerNamespace)
	c.Assert(err, jc.ErrorIsNil)
	// Check that this secretary knows the controller uuid.
	err = secretary.CheckLease(corelease.Key{"", "", "controller-uuid"})
	c.Assert(err, jc.ErrorIsNil)
	config.Secretary = nil

	c.Assert(config, jc.DeepEquals, lease.ManagerConfig{
		Store:      s.store,
		Clock:      s.clock,
		Logger:     &s.logger,
		MaxSleep:   time.Minute,
		EntityUUID: "controller-uuid",
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

func (s *manifoldSuite) TestOutput(c *gc.C) {
	s.worker = &lease.Manager{}
	w, err := s.manifold.Start(s.context)
	c.Assert(err, jc.ErrorIsNil)

	var updater globalclock.Updater
	err = s.manifold.Output(w, &updater)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(updater, gc.Equals, s.store)

	var manager corelease.Manager
	err = s.manifold.Output(w, &manager)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(manager, gc.Equals, s.worker)

	var other io.Writer
	err = s.manifold.Output(w, &other)
	c.Assert(err, gc.ErrorMatches, `expected output of type \*globalclock.Updater or \*core/lease.Manager, got \*io.Writer`)
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
