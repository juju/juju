// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package certupdater_test

import (
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"
	worker "gopkg.in/juju/worker.v1"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state"
	"github.com/juju/juju/worker/certupdater"
	"github.com/juju/juju/worker/dependency"
	dt "github.com/juju/juju/worker/dependency/testing"
	"github.com/juju/juju/worker/workertest"
)

type ManifoldSuite struct {
	testing.IsolationSuite

	manifold       dependency.Manifold
	context        dependency.Context
	agent          *mockAgent
	st             state.State
	stateTracker   stubStateTracker
	addressWatcher fakeAddressWatcher

	stub testing.Stub
}

var _ = gc.Suite(&ManifoldSuite{})

func (s *ManifoldSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)

	s.agent = &mockAgent{}
	s.stateTracker = stubStateTracker{
		pool: state.NewStatePool(&s.st),
	}
	s.stub.ResetCalls()

	s.context = s.newContext(nil)
	s.manifold = certupdater.Manifold(certupdater.ManifoldConfig{
		AgentName:                "agent",
		StateName:                "state",
		NewWorker:                s.newWorker,
		NewMachineAddressWatcher: s.newMachineAddressWatcher,
	})
}

func (s *ManifoldSuite) newContext(overlay map[string]interface{}) dependency.Context {
	resources := map[string]interface{}{
		"agent": s.agent,
		"state": &s.stateTracker,
	}
	for k, v := range overlay {
		resources[k] = v
	}
	return dt.StubContext(nil, resources)
}

func (s *ManifoldSuite) newWorker(config certupdater.Config) worker.Worker {
	s.stub.MethodCall(s, "NewWorker", config)
	w := worker.NewRunner(worker.RunnerParams{})
	s.AddCleanup(func(c *gc.C) { workertest.DirtyKill(c, w) })
	return w
}

func (s *ManifoldSuite) newMachineAddressWatcher(st *state.State, machineId string) (certupdater.AddressWatcher, error) {
	s.stub.MethodCall(s, "NewMachineAddressWatcher", st, machineId)
	if err := s.stub.NextErr(); err != nil {
		return nil, err
	}
	return &s.addressWatcher, nil
}

var expectedInputs = []string{"agent", "state"}

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
	w, err := s.manifold.Start(s.context)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.CleanKill(c, w)

	s.stub.CheckCallNames(c, "NewMachineAddressWatcher", "NewWorker")
	s.stub.CheckCall(c, 0, "NewMachineAddressWatcher", &s.st, "123")

	args := s.stub.Calls()[1].Args
	c.Assert(args, gc.HasLen, 1)
	c.Assert(args[0], gc.FitsTypeOf, certupdater.Config{})
	config := args[0].(certupdater.Config)

	c.Assert(config.StateServingInfoSetter, gc.NotNil)
	config.StateServingInfoSetter = nil

	c.Assert(config, jc.DeepEquals, certupdater.Config{
		AddressWatcher:         &s.addressWatcher,
		StateServingInfoGetter: &s.agent.conf,
		ControllerConfigGetter: &s.st,
		APIHostPortsGetter:     &s.st,
	})
}

func (s *ManifoldSuite) TestStopWorkerClosesState(c *gc.C) {
	w, err := s.manifold.Start(s.context)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.CleanKill(c, w)

	s.stateTracker.CheckCallNames(c, "Use")

	workertest.CleanKill(c, w)
	s.stateTracker.CheckCallNames(c, "Use", "Done")
}

type mockAgent struct {
	agent.Agent
	conf mockAgentConfig
}

func (ma *mockAgent) CurrentConfig() agent.Config {
	return &ma.conf
}

func (ma *mockAgent) ChangeConfig(agent.ConfigMutator) error {
	// TODO(axw)
	return nil
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
	pool *state.StatePool
}

func (s *stubStateTracker) Use() (*state.StatePool, error) {
	s.MethodCall(s, "Use")
	return s.pool, s.NextErr()
}

func (s *stubStateTracker) Done() error {
	s.MethodCall(s, "Done")
	return s.NextErr()
}

type fakeAddressWatcher struct {
	certupdater.AddressWatcher
}
