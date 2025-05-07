// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package certupdater_test

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/dependency"
	dt "github.com/juju/worker/v4/dependency/testing"
	"github.com/juju/worker/v4/workertest"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/logger"
	controllerconfigservice "github.com/juju/juju/domain/controllerconfig/service"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/pki"
	pkitest "github.com/juju/juju/internal/pki/test"
	"github.com/juju/juju/internal/services"
	"github.com/juju/juju/internal/worker/certupdater"
	"github.com/juju/juju/state"
)

type ManifoldSuite struct {
	testing.IsolationSuite

	authority              pki.Authority
	manifold               dependency.Manifold
	getter                 dependency.Getter
	agent                  *mockAgent
	stateTracker           stubStateTracker
	addressWatcher         fakeAddressWatcher
	domainServices         services.DomainServices
	controllerConfigGetter *controllerconfigservice.WatchableService
	logger                 logger.Logger

	stub testing.Stub
}

var _ = tc.Suite(&ManifoldSuite{})

func (s *ManifoldSuite) SetUpTest(c *tc.C) {
	s.IsolationSuite.SetUpTest(c)

	s.agent = &mockAgent{}
	s.stateTracker = stubStateTracker{}
	s.controllerConfigGetter = &controllerconfigservice.WatchableService{}
	s.domainServices = stubDomainServices{
		controllerConfigGetter: s.controllerConfigGetter,
	}
	s.logger = loggertesting.WrapCheckLog(c)
	s.stub.ResetCalls()

	authority, err := pkitest.NewTestAuthority()
	c.Assert(err, jc.ErrorIsNil)
	s.authority = authority

	s.getter = s.newGetter(nil)
	s.manifold = certupdater.Manifold(certupdater.ManifoldConfig{
		AgentName:                "agent",
		AuthorityName:            "authority",
		StateName:                "state",
		DomainServicesName:       "domain-services",
		NewWorker:                s.newWorker,
		NewMachineAddressWatcher: s.newMachineAddressWatcher,
		Logger:                   s.logger,
	})
}

func (s *ManifoldSuite) newGetter(overlay map[string]any) dependency.Getter {
	resources := map[string]any{
		"agent":           s.agent,
		"authority":       s.authority,
		"state":           &s.stateTracker,
		"domain-services": s.domainServices,
	}
	for k, v := range overlay {
		resources[k] = v
	}
	return dt.StubGetter(resources)
}

func (s *ManifoldSuite) newWorker(config certupdater.Config) (worker.Worker, error) {
	s.stub.MethodCall(s, "NewWorker", config)
	w := worker.NewRunner(worker.RunnerParams{})
	s.AddCleanup(func(c *tc.C) { workertest.DirtyKill(c, w) })
	return w, nil
}

func (s *ManifoldSuite) newMachineAddressWatcher(st *state.State, machineId string) (certupdater.AddressWatcher, error) {
	s.stub.MethodCall(s, "NewMachineAddressWatcher", st, machineId)
	if err := s.stub.NextErr(); err != nil {
		return nil, err
	}
	return &s.addressWatcher, nil
}

var expectedInputs = []string{"agent", "authority", "state", "domain-services"}

func (s *ManifoldSuite) TestInputs(c *tc.C) {
	c.Assert(s.manifold.Inputs, jc.SameContents, expectedInputs)
}

func (s *ManifoldSuite) TestMissingInputs(c *tc.C) {
	for _, input := range expectedInputs {
		getter := s.newGetter(map[string]any{
			input: dependency.ErrMissing,
		})
		_, err := s.manifold.Start(context.Background(), getter)
		c.Assert(errors.Cause(err), tc.Equals, dependency.ErrMissing)
	}
}

func (s *ManifoldSuite) TestStart(c *tc.C) {
	w, err := s.manifold.Start(context.Background(), s.getter)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.CleanKill(c, w)

	s.stub.CheckCallNames(c, "NewMachineAddressWatcher", "NewWorker")
	s.stub.CheckCall(c, 0, "NewMachineAddressWatcher", &s.stateTracker.state, "123")

	args := s.stub.Calls()[1].Args
	c.Assert(args, tc.HasLen, 1)
	c.Assert(args[0], tc.FitsTypeOf, certupdater.Config{})
	config := args[0].(certupdater.Config)

	c.Assert(config, jc.DeepEquals, certupdater.Config{
		AddressWatcher:         &s.addressWatcher,
		Authority:              s.authority,
		APIHostPortsGetter:     &s.stateTracker.state,
		Logger:                 s.logger,
		ControllerConfigGetter: s.controllerConfigGetter,
	})
}

func (s *ManifoldSuite) TestStartErrorClosesState(c *tc.C) {
	s.stub.SetErrors(errors.New("boom"))

	_, err := s.manifold.Start(context.Background(), s.getter)
	c.Assert(err, tc.ErrorMatches, "boom")

	s.stateTracker.CheckCallNames(c, "Use", "Done")
}

func (s *ManifoldSuite) TestStopWorkerClosesState(c *tc.C) {
	w, err := s.manifold.Start(context.Background(), s.getter)
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

func (s *stubStateTracker) Report() map[string]any {
	s.MethodCall(s, "Report")
	return nil
}

type fakeAddressWatcher struct {
	certupdater.AddressWatcher
}

type stubDomainServices struct {
	services.DomainServices
	controllerConfigGetter *controllerconfigservice.WatchableService
}

func (s stubDomainServices) ControllerConfig() *controllerconfigservice.WatchableService {
	return s.controllerConfigGetter
}
