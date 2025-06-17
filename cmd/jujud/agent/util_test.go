// Copyright 2012-2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agent

import (
	"fmt"
	"io/ioutil"
	"path/filepath"
	"sync"
	"time"

	"github.com/juju/cmd/v3"
	"github.com/juju/cmd/v3/cmdtesting"
	"github.com/juju/names/v4"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/v3/voyeur"
	"github.com/juju/version/v2"
	"github.com/juju/worker/v3"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/agent/addons"
	"github.com/juju/juju/api"
	"github.com/juju/juju/cmd/jujud/agent/agentconf"
	"github.com/juju/juju/cmd/jujud/agent/agenttest"
	agenterrors "github.com/juju/juju/cmd/jujud/agent/errors"
	"github.com/juju/juju/cmd/jujud/agent/mocks"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/lxdprofile"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/environs/instances"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/mongo/mongometrics"
	"github.com/juju/juju/mongo/mongotest"
	"github.com/juju/juju/provider/dummy"
	"github.com/juju/juju/service/upstart"
	"github.com/juju/juju/state"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/tools"
	jujuversion "github.com/juju/juju/version"
	jworker "github.com/juju/juju/worker"
	"github.com/juju/juju/worker/authenticationworker"
	"github.com/juju/juju/worker/gate"
	"github.com/juju/juju/worker/logsender"
)

const (
	initialMachinePassword = "machine-password-1234567890"
)

var fastDialOpts = api.DialOpts{
	Timeout:    coretesting.LongWait,
	RetryDelay: coretesting.ShortWait,
}

type commonMachineSuite struct {
	fakeEnsureMongo *agenttest.FakeEnsureMongo
	AgentSuite

	cmdRunner *mocks.MockCommandRunner
}

func (s *commonMachineSuite) SetUpSuite(c *gc.C) {
	s.AgentSuite.SetUpSuite(c)
	s.PatchValue(&jujuversion.Current, coretesting.FakeVersionNumber)
	s.PatchValue(&stateWorkerDialOpts, mongotest.DialOpts())
}

func (s *commonMachineSuite) SetUpTest(c *gc.C) {
	s.AgentSuite.SetUpTest(c)

	// Patch ssh user to avoid touching ~ubuntu/.ssh/authorized_keys.
	s.PatchValue(&authenticationworker.SSHUser, "")

	testpath := c.MkDir()
	s.PatchEnvPathPrepend(testpath)
	// mock out the start method so we can fake install services without sudo
	fakeCmd(filepath.Join(testpath, "start"))
	fakeCmd(filepath.Join(testpath, "stop"))

	s.PatchValue(&upstart.InitDir, c.MkDir())
	s.fakeEnsureMongo = agenttest.InstallFakeEnsureMongo(s)
}

func (s *commonMachineSuite) assertChannelActive(c *gc.C, aChannel chan struct{}, intent string) {
	// Wait for channel to be active.
	select {
	case <-aChannel:
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timeout while waiting for %v", intent)
	}
}

func fakeCmd(path string) {
	err := ioutil.WriteFile(path, []byte("#!/bin/bash --norc\nexit 0"), 0755)
	if err != nil {
		panic(err)
	}
}

func (s *commonMachineSuite) TearDownTest(c *gc.C) {
	s.AgentSuite.TearDownTest(c)
}

// primeAgent adds a new Machine to run the given jobs, and sets up the
// machine agent's directory.  It returns the new machine, the
// agent's configuration and the tools currently running.
func (s *commonMachineSuite) primeAgent(c *gc.C, jobs ...state.MachineJob) (m *state.Machine, agentConfig agent.ConfigSetterWriter, tools *tools.Tools) {
	vers := coretesting.CurrentVersion()
	return s.primeAgentVersion(c, vers, jobs...)
}

// primeAgentVersion is similar to primeAgent, but permits the
// caller to specify the version.Binary to prime with.
func (s *commonMachineSuite) primeAgentVersion(c *gc.C, vers version.Binary, jobs ...state.MachineJob) (m *state.Machine, agentConfig agent.ConfigSetterWriter, tools *tools.Tools) {
	m, err := s.State.AddMachine("quantal", jobs...)
	c.Assert(err, jc.ErrorIsNil)
	return s.primeAgentWithMachine(c, m, vers)
}

func (s *commonMachineSuite) primeAgentWithMachine(c *gc.C, m *state.Machine, vers version.Binary) (*state.Machine, agent.ConfigSetterWriter, *tools.Tools) {
	return s.configureMachine(c, m.Id(), vers)
}

func (s *commonMachineSuite) configureMachine(c *gc.C, machineId string, vers version.Binary) (
	machine *state.Machine, agentConfig agent.ConfigSetterWriter, tools *tools.Tools,
) {
	m, err := s.State.Machine(machineId)
	c.Assert(err, jc.ErrorIsNil)

	// Add a machine and ensure it is provisioned.
	inst, md := jujutesting.AssertStartInstance(c, s.Environ, context.NewEmptyCloudCallContext(), s.ControllerConfig.ControllerUUID(), machineId)
	c.Assert(m.SetProvisioned(inst.Id(), "", agent.BootstrapNonce, md), jc.ErrorIsNil)

	// Add an address for the tests in case the initiateMongoServer
	// codepath is exercised.
	s.setFakeMachineAddresses(c, m)

	// Set up the new machine.
	err = m.SetAgentVersion(vers)
	c.Assert(err, jc.ErrorIsNil)
	err = m.SetPassword(initialMachinePassword)
	c.Assert(err, jc.ErrorIsNil)
	tag := m.Tag()
	if m.IsManager() {
		err = m.SetMongoPassword(initialMachinePassword)
		c.Assert(err, jc.ErrorIsNil)
		agentConfig, tools = s.PrimeStateAgentVersion(c, tag, initialMachinePassword, vers)
		info, ok := agentConfig.StateServingInfo()
		c.Assert(ok, jc.IsTrue)
		err = s.State.SetStateServingInfo(info)
		c.Assert(err, jc.ErrorIsNil)
	} else {
		agentConfig, tools = s.PrimeAgentVersion(c, tag, initialMachinePassword, vers)
	}
	err = agentConfig.Write()
	c.Assert(err, jc.ErrorIsNil)
	return m, agentConfig, tools
}

func NewTestMachineAgentFactory(
	c *gc.C,
	agentConfWriter agentconf.AgentConfigWriter,
	bufferedLogger *logsender.BufferedLogWriter,
	rootDir string,
	cmdRunner CommandRunner,
) machineAgentFactoryFnType {
	preUpgradeSteps := func(_ *state.StatePool, _ agent.Config, isController, isCaas bool) error {
		return nil
	}

	return func(agentTag names.Tag, isCAAS bool) (*MachineAgent, error) {
		prometheusRegistry, err := addons.NewPrometheusRegistry()
		c.Assert(err, jc.ErrorIsNil)
		a := &MachineAgent{
			agentTag:          agentTag,
			AgentConfigWriter: agentConfWriter,
			configChangedVal:  voyeur.NewValue(true),
			bufferedLogger:    bufferedLogger,
			workersStarted:    make(chan struct{}),
			dead:              make(chan struct{}),
			runner: worker.NewRunner(worker.RunnerParams{
				IsFatal:       agenterrors.IsFatal,
				MoreImportant: agenterrors.MoreImportant,
				RestartDelay:  jworker.RestartDelay,
			}),
			rootDir:                     rootDir,
			initialUpgradeCheckComplete: gate.NewLock(),
			loopDeviceManager:           &mockLoopDeviceManager{},
			prometheusRegistry:          prometheusRegistry,
			mongoTxnCollector:           mongometrics.NewTxnCollector(),
			mongoDialCollector:          mongometrics.NewDialCollector(),
			preUpgradeSteps:             preUpgradeSteps,
			isCaasAgent:                 isCAAS,
			cmdRunner:                   cmdRunner,
		}
		return a, nil
	}
}

// newAgent returns a new MachineAgent instance
func (s *commonMachineSuite) newAgent(c *gc.C, m *state.Machine) (*gomock.Controller, *MachineAgent) {
	ctrl := gomock.NewController(c)
	s.cmdRunner = mocks.NewMockCommandRunner(ctrl)

	agentConf := agentconf.NewAgentConf(s.DataDir())
	agentConf.ReadConfig(names.NewMachineTag(m.Id()).String())
	logger := s.newBufferedLogWriter()
	machineAgentFactory := NewTestMachineAgentFactory(c, agentConf, logger, c.MkDir(), s.cmdRunner)
	machineAgent, err := machineAgentFactory(m.Tag(), false)
	c.Assert(err, jc.ErrorIsNil)
	return ctrl, machineAgent
}

func (s *commonMachineSuite) newBufferedLogWriter() *logsender.BufferedLogWriter {
	logger := logsender.NewBufferedLogWriter(1024)
	s.AddCleanup(func(*gc.C) { logger.Close() })
	return logger
}

func (s *commonMachineSuite) setFakeMachineAddresses(c *gc.C, machine *state.Machine) {
	addrs := network.NewSpaceAddresses("0.1.2.3")
	err := machine.SetProviderAddresses(addrs...)
	c.Assert(err, jc.ErrorIsNil)
	// Set the addresses in the environ instance as well so that if the instance poller
	// runs it won't overwrite them.
	instId, err := machine.InstanceId()
	c.Assert(err, jc.ErrorIsNil)
	insts, err := s.Environ.Instances(context.NewEmptyCloudCallContext(), []instance.Id{instId})
	c.Assert(err, jc.ErrorIsNil)
	dummy.SetInstanceAddresses(insts[0], network.NewMachineAddresses([]string{"0.1.2.3"}).AsProviderAddresses())
}

type mockLoopDeviceManager struct {
	detachLoopDevicesArgRootfs string
	detachLoopDevicesArgPrefix string
}

func (m *mockLoopDeviceManager) DetachLoopDevices(rootfs, prefix string) error {
	m.detachLoopDevicesArgRootfs = rootfs
	m.detachLoopDevicesArgPrefix = prefix
	return nil
}

func newSignal() *signal {
	return &signal{ch: make(chan struct{})}
}

type signal struct {
	mu sync.Mutex
	ch chan struct{}
}

func (s *signal) triggered() <-chan struct{} {
	return s.ch
}

func (s *signal) assertTriggered(c *gc.C, thing string) {
	select {
	case <-s.triggered():
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timed out waiting for %s", thing)
	}
}

func (s *signal) trigger() {
	s.mu.Lock()
	defer s.mu.Unlock()

	select {
	case <-s.ch:
		// Already closed.
	default:
		close(s.ch)
	}
}

type runner interface {
	Run(*cmd.Context) error
	Stop() error
}

// runWithTimeout runs an agent and waits
// for it to complete within a reasonable time.
func runWithTimeout(c *gc.C, r runner) error {
	done := make(chan error)
	go func() {
		done <- r.Run(cmdtesting.Context(c))
	}()
	select {
	case err := <-done:
		return err
	case <-time.After(coretesting.LongWait):
	}
	err := r.Stop()
	return fmt.Errorf("timed out waiting for agent to finish; stop error: %v", err)
}

func newDummyWorker() worker.Worker {
	return jworker.NewSimpleWorker(func(stop <-chan struct{}) error {
		<-stop
		return nil
	})
}

type FakeConfig struct {
	agent.ConfigSetter
	values map[string]string
}

func (FakeConfig) LogDir() string {
	return filepath.FromSlash("/var/log/juju/")
}

func (FakeConfig) Tag() names.Tag {
	return names.NewMachineTag("42")
}

func (f FakeConfig) Value(key string) string {
	if f.values == nil {
		return ""
	}
	return f.values[key]
}

func (f FakeConfig) AgentLogfileMaxSizeMB() int {
	return 100
}

func (f FakeConfig) AgentLogfileMaxBackups() int {
	return 2
}

type FakeAgentConfig struct {
	agentconf.AgentConf
	values map[string]string
}

func (FakeAgentConfig) ReadConfig(string) error { return nil }

func (a FakeAgentConfig) CurrentConfig() agent.Config {
	return FakeConfig{values: a.values}
}

func (FakeAgentConfig) ChangeConfig(mutate agent.ConfigMutator) error {
	return mutate(FakeConfig{})
}

func (FakeAgentConfig) CheckArgs([]string) error { return nil }

// minModelWorkersEnviron implements just enough of environs.Environ
// to allow model workers to run.
type minModelWorkersEnviron struct {
	environs.Environ
	environs.LXDProfiler
}

func (e *minModelWorkersEnviron) Config() *config.Config {
	attrs := coretesting.FakeConfig()
	cfg, err := config.New(config.UseDefaults, attrs)
	if err != nil {
		panic(err)
	}
	return cfg
}

func (e *minModelWorkersEnviron) SetConfig(*config.Config) error {
	return nil
}

func (e *minModelWorkersEnviron) AllRunningInstances(context.ProviderCallContext) ([]instances.Instance, error) {
	return nil, nil
}

func (e *minModelWorkersEnviron) Instances(ctx context.ProviderCallContext, ids []instance.Id) ([]instances.Instance, error) {
	return nil, environs.ErrNoInstances
}

func (*minModelWorkersEnviron) MaybeWriteLXDProfile(pName string, put lxdprofile.Profile) error {
	return nil
}

func (*minModelWorkersEnviron) LXDProfileNames(containerName string) ([]string, error) {
	return nil, nil
}

func (*minModelWorkersEnviron) AssignLXDProfiles(instId string, profilesNames []string, profilePosts []lxdprofile.ProfilePost) (current []string, err error) {
	return profilesNames, nil
}
