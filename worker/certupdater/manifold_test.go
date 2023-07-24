// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package certupdater_test

import (
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names/v4"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v3"
	"github.com/juju/worker/v3/dependency"
	dt "github.com/juju/worker/v3/dependency/testing"
	"github.com/juju/worker/v3/workertest"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/changestream"
	"github.com/juju/juju/pki"
	pkitest "github.com/juju/juju/pki/test"
	"github.com/juju/juju/state"
	statetesting "github.com/juju/juju/state/testing"
	"github.com/juju/juju/worker/certupdater"
	"github.com/juju/juju/worker/certupdater/mocks"
)

type ManifoldSuite struct {
	statetesting.StateSuite

	authority         pki.Authority
	manifold          dependency.Manifold
	context           dependency.Context
	agent             *mockAgent
	stateTracker      stubStateTracker
	addressWatcher    fakeAddressWatcher
	watchableDBGetter *mocks.MockWatchableDBGetter
	ctrlConfigService *mocks.MockControllerConfigService

	stub testing.Stub
}

var _ = gc.Suite(&ManifoldSuite{})

func (s *ManifoldSuite) SetUpTest(c *gc.C) {
	s.StateSuite.SetUpTest(c)

	ctrl := gomock.NewController(c)
	s.watchableDBGetter = mocks.NewMockWatchableDBGetter(ctrl)
	s.ctrlConfigService = mocks.NewMockControllerConfigService(ctrl)

	s.agent = &mockAgent{}
	s.stateTracker = stubStateTracker{
		pool: s.StatePool,
	}
	s.stub.ResetCalls()

	authority, err := pkitest.NewTestAuthority()
	c.Assert(err, jc.ErrorIsNil)
	s.authority = authority

	s.context = s.newContext(nil)
	s.manifold = certupdater.Manifold(certupdater.ManifoldConfig{
		AgentName:                  "agent",
		AuthorityName:              "authority",
		StateName:                  "state",
		ChangeStreamName:           "change-stream",
		Logger:                     loggo.GetLogger("test"),
		NewWorker:                  s.newWorker,
		NewMachineAddressWatcher:   s.newMachineAddressWatcher,
		NewControllerConfigService: s.newControllerConfigService,
	})
}

func (s *ManifoldSuite) newContext(overlay map[string]interface{}) dependency.Context {
	resources := map[string]interface{}{
		"agent":         s.agent,
		"authority":     s.authority,
		"state":         &s.stateTracker,
		"change-stream": s.watchableDBGetter,
	}
	for k, v := range overlay {
		resources[k] = v
	}
	return dt.StubContext(nil, resources)
}

func (s *ManifoldSuite) newWorker(config certupdater.Config) (worker.Worker, error) {
	s.stub.MethodCall(s, "NewWorker", config)
	w := worker.NewRunner(worker.RunnerParams{})
	s.AddCleanup(func(c *gc.C) { workertest.DirtyKill(c, w) })
	return w, nil
}

func (s *ManifoldSuite) newMachineAddressWatcher(st *state.State, machineId string) (certupdater.AddressWatcher, error) {
	s.stub.MethodCall(s, "NewMachineAddressWatcher", st, machineId)
	if err := s.stub.NextErr(); err != nil {
		return nil, err
	}
	return &s.addressWatcher, nil
}

func (s *ManifoldSuite) newControllerConfigService(changestream.WatchableDBGetter, certupdater.Logger) certupdater.ControllerConfigService {
	return s.ctrlConfigService
}

var expectedInputs = []string{"agent", "authority", "state", "change-stream"}

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
	s.stub.CheckCall(c, 0, "NewMachineAddressWatcher", s.State, "123")

	args := s.stub.Calls()[1].Args
	c.Assert(args, gc.HasLen, 1)
	c.Assert(args[0], gc.FitsTypeOf, certupdater.Config{})
	config := args[0].(certupdater.Config)

	c.Assert(config, jc.DeepEquals, certupdater.Config{
		AddressWatcher:     &s.addressWatcher,
		Authority:          s.authority,
		APIHostPortsGetter: s.State,
		CtrlConfigService:  s.ctrlConfigService,
	})
}

func (s *ManifoldSuite) TestStartErrorClosesState(c *gc.C) {
	s.stub.SetErrors(errors.New("boom"))

	_, err := s.manifold.Start(s.context)
	c.Assert(err, gc.ErrorMatches, "boom")

	s.stateTracker.CheckCallNames(c, "Use", "Done")
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

func (s *stubStateTracker) Report() map[string]interface{} {
	s.MethodCall(s, "Report")
	return nil
}

type fakeAddressWatcher struct {
	certupdater.AddressWatcher
}
