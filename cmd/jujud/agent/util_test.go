// Copyright 2012-2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agent

import (
	"fmt"

	"io/ioutil"
	"path/filepath"
	"reflect"
	"sync"
	"time"

	"github.com/juju/cmd"
	"github.com/juju/collections/set"
	"github.com/juju/os/series"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/arch"
	"github.com/juju/version"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v6"
	"gopkg.in/juju/charmrepo.v3"
	"gopkg.in/juju/names.v3"
	"gopkg.in/juju/worker.v1"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api"
	apideployer "github.com/juju/juju/api/deployer"
	"github.com/juju/juju/cmd/jujud/agent/agenttest"
	cmdutil "github.com/juju/juju/cmd/jujud/util"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/lxdprofile"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/environs/instances"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/mongo/mongotest"
	"github.com/juju/juju/provider/dummy"
	"github.com/juju/juju/service/upstart"
	"github.com/juju/juju/state"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/tools"
	jujuversion "github.com/juju/juju/version"
	jworker "github.com/juju/juju/worker"
	"github.com/juju/juju/worker/authenticationworker"
	"github.com/juju/juju/worker/deployer"
	"github.com/juju/juju/worker/logsender"
)

const (
	initialMachinePassword = "machine-password-1234567890"
	initialUnitPassword    = "unit-password-1234567890"
	startWorkerWait        = 250 * time.Millisecond
)

var fastDialOpts = api.DialOpts{
	Timeout:    coretesting.LongWait,
	RetryDelay: coretesting.ShortWait,
}

type commonMachineSuite struct {
	fakeEnsureMongo *agenttest.FakeEnsureMongo
	AgentSuite
}

func (s *commonMachineSuite) SetUpSuite(c *gc.C) {
	s.AgentSuite.SetUpSuite(c)
	s.PatchValue(&jujuversion.Current, coretesting.FakeVersionNumber)
	s.PatchValue(&stateWorkerDialOpts, mongotest.DialOpts())
}

func (s *commonMachineSuite) SetUpTest(c *gc.C) {
	s.AgentSuite.SetUpTest(c)
	s.PatchValue(&charmrepo.CacheDir, c.MkDir())

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

func (s *commonMachineSuite) assertChannelInactive(c *gc.C, aChannel chan struct{}, intent string) {
	// Now make sure the channel is not active.
	select {
	case <-aChannel:
		c.Fatalf("%v unexpectedly", intent)
	case <-time.After(startWorkerWait):
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
	vers := version.Binary{
		Number: jujuversion.Current,
		Arch:   arch.HostArch(),
		Series: series.MustHostSeries(),
	}
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
	pinger, err := m.SetAgentPresence()
	c.Assert(err, jc.ErrorIsNil)
	s.AddCleanup(func(c *gc.C) {
		c.Assert(worker.Stop(pinger), jc.ErrorIsNil)
	})
	return s.configureMachine(c, m.Id(), vers)
}

func (s *commonMachineSuite) configureMachine(c *gc.C, machineId string, vers version.Binary) (
	machine *state.Machine, agentConfig agent.ConfigSetterWriter, tools *tools.Tools,
) {
	m, err := s.State.Machine(machineId)
	c.Assert(err, jc.ErrorIsNil)

	// Add a machine and ensure it is provisioned.
	inst, md := jujutesting.AssertStartInstance(c, s.Environ, context.NewCloudCallContext(), s.ControllerConfig.ControllerUUID(), machineId)
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
		ssi := cmdutil.ParamsStateServingInfoToStateStateServingInfo(info)
		err = s.State.SetStateServingInfo(ssi)
		c.Assert(err, jc.ErrorIsNil)
	} else {
		agentConfig, tools = s.PrimeAgentVersion(c, tag, initialMachinePassword, vers)
	}
	err = agentConfig.Write()
	c.Assert(err, jc.ErrorIsNil)
	return m, agentConfig, tools
}

func NewTestMachineAgentFactory(
	agentConfWriter AgentConfigWriter,
	bufferedLogger *logsender.BufferedLogWriter,
	rootDir string,
) machineAgentFactoryFnType {
	preUpgradeSteps := func(_ *state.StatePool, _ agent.Config, isController, isMaster, isCaas bool) error {
		return nil
	}
	return func(agentTag names.Tag, isCAAS bool) (*MachineAgent, error) {
		return NewMachineAgent(
			agentTag,
			agentConfWriter,
			bufferedLogger,
			worker.NewRunner(worker.RunnerParams{
				IsFatal:       cmdutil.IsFatal,
				MoreImportant: cmdutil.MoreImportant,
				RestartDelay:  jworker.RestartDelay,
			}),
			&mockLoopDeviceManager{},
			DefaultIntrospectionSocketName,
			preUpgradeSteps,
			rootDir,
			isCAAS,
		)
	}
}

// newAgent returns a new MachineAgent instance
func (s *commonMachineSuite) newAgent(c *gc.C, m *state.Machine) *MachineAgent {
	agentConf := agentConf{dataDir: s.DataDir()}
	agentConf.ReadConfig(names.NewMachineTag(m.Id()).String())
	logger := s.newBufferedLogWriter()
	machineAgentFactory := NewTestMachineAgentFactory(&agentConf, logger, c.MkDir())
	machineAgent, err := machineAgentFactory(m.Tag(), false)
	c.Assert(err, jc.ErrorIsNil)
	return machineAgent
}

func (s *commonMachineSuite) newBufferedLogWriter() *logsender.BufferedLogWriter {
	logger := logsender.NewBufferedLogWriter(1024)
	s.AddCleanup(func(*gc.C) { logger.Close() })
	return logger
}

func patchDeployContext(c *gc.C, st *state.State) (*fakeContext, func()) {
	ctx := &fakeContext{
		inited:   newSignal(),
		deployed: make(set.Strings),
	}
	orig := newDeployContext
	newDeployContext = func(dst *apideployer.State, agentConfig agent.Config) deployer.Context {
		ctx.st = st
		ctx.agentConfig = agentConfig
		ctx.inited.trigger()
		return ctx
	}
	return ctx, func() { newDeployContext = orig }
}

func (s *commonMachineSuite) setFakeMachineAddresses(c *gc.C, machine *state.Machine) {
	addrs := network.NewSpaceAddresses("0.1.2.3")
	err := machine.SetProviderAddresses(addrs...)
	c.Assert(err, jc.ErrorIsNil)
	// Set the addresses in the environ instance as well so that if the instance poller
	// runs it won't overwrite them.
	instId, err := machine.InstanceId()
	c.Assert(err, jc.ErrorIsNil)
	insts, err := s.Environ.Instances(context.NewCloudCallContext(), []instance.Id{instId})
	c.Assert(err, jc.ErrorIsNil)
	dummy.SetInstanceAddresses(insts[0], network.NewProviderAddresses("0.1.2.3"))
}

// WithAliveAgent starts the agent, wait till it becomes alive and then invokes
// the provided cb. Once the callback returns, WithAliveAgent will block until
// the agent either exits or exceeds its run timeout. In both cases, any error
// returned by the agent's Run method will be captured and returned to the
// caller.
func (s *commonMachineSuite) WithAliveAgent(m *state.Machine, a *MachineAgent, cb func() error) error {
	// achilleasa: the agent usually takes a around 30 seconds
	waitTime := coretesting.LongWait * 3

	errCh := make(chan error, 1)
	go func() {
		select {
		case errCh <- a.Run(nil):
		case <-time.After(waitTime):
			errCh <- fmt.Errorf("time out waiting for agent to complete its run")
		}
		a.Stop()
		close(errCh)
	}()

	if err := m.WaitAgentPresence(waitTime); err != nil {
		return err
	}

	if cb != nil {
		if err := cb(); err != nil {
			return err
		}
	}

	// Wait for agent to exit or timeout
	for err := range errCh {
		return err
	}

	return nil
}

// opRecvTimeout waits for any of the given kinds of operation to
// be received from ops, and times out if not.
func opRecvTimeout(c *gc.C, st *state.State, opc <-chan dummy.Operation, kinds ...dummy.Operation) dummy.Operation {
	st.StartSync()
	timeout := time.After(coretesting.LongWait)
	for {
		select {
		case op := <-opc:
			for _, k := range kinds {
				if reflect.TypeOf(op) == reflect.TypeOf(k) {
					return op
				}
			}
			c.Logf("discarding unknown event %#v", op)
		case <-time.After(coretesting.ShortWait):
			st.StartSync()
		case <-timeout:
			c.Fatalf("time out waiting for operation")
		}
	}
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
		c.Fatalf("timed out waiting for " + thing)
	}
}

func (s *signal) assertNotTriggered(c *gc.C, wait time.Duration, thing string) {
	select {
	case <-s.triggered():
		c.Fatalf("%v unexpectedly", thing)
	case <-time.After(wait):
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
func runWithTimeout(r runner) error {
	done := make(chan error)
	go func() {
		done <- r.Run(nil)
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

type FakeAgentConfig struct {
	AgentConf
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
	return nil, nil
}

func (env *minModelWorkersEnviron) MaybeWriteLXDProfile(pName string, put *charm.LXDProfile) error {
	return nil
}

func (env *minModelWorkersEnviron) LXDProfileNames(containerName string) ([]string, error) {
	return nil, nil
}

func (env *minModelWorkersEnviron) AssignLXDProfiles(instId string, profilesNames []string, profilePosts []lxdprofile.ProfilePost) (current []string, err error) {
	return profilesNames, nil
}
