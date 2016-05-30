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
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/arch"
	"github.com/juju/utils/series"
	"github.com/juju/utils/set"
	"github.com/juju/version"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charmrepo.v2-unstable"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api"
	apideployer "github.com/juju/juju/api/deployer"
	"github.com/juju/juju/cmd/jujud/agent/agenttest"
	cmdutil "github.com/juju/juju/cmd/jujud/util"
	lxctesting "github.com/juju/juju/container/lxc/testing"
	"github.com/juju/juju/instance"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/mongo/mongotest"
	"github.com/juju/juju/network"
	"github.com/juju/juju/provider/dummy"
	"github.com/juju/juju/service/upstart"
	"github.com/juju/juju/state"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/tools"
	jujuversion "github.com/juju/juju/version"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/authenticationworker"
	"github.com/juju/juju/worker/deployer"
	"github.com/juju/juju/worker/logsender"
	"github.com/juju/juju/worker/singular"
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
	singularRecord *singularRunnerRecord
	lxctesting.TestSuite
	fakeEnsureMongo *agenttest.FakeEnsureMongo
	AgentSuite
}

func (s *commonMachineSuite) SetUpSuite(c *gc.C) {
	s.AgentSuite.SetUpSuite(c)
	s.TestSuite.SetUpSuite(c)
	s.AgentSuite.PatchValue(&jujuversion.Current, coretesting.FakeVersionNumber)
	s.AgentSuite.PatchValue(&stateWorkerDialOpts, mongotest.DialOpts())
}

func (s *commonMachineSuite) TearDownSuite(c *gc.C) {
	s.TestSuite.TearDownSuite(c)
	s.AgentSuite.TearDownSuite(c)
}

func (s *commonMachineSuite) SetUpTest(c *gc.C) {
	s.AgentSuite.SetUpTest(c)
	s.TestSuite.SetUpTest(c)
	s.AgentSuite.PatchValue(&charmrepo.CacheDir, c.MkDir())

	// Patch ssh user to avoid touching ~ubuntu/.ssh/authorized_keys.
	s.AgentSuite.PatchValue(&authenticationworker.SSHUser, "")

	testpath := c.MkDir()
	s.AgentSuite.PatchEnvPathPrepend(testpath)
	// mock out the start method so we can fake install services without sudo
	fakeCmd(filepath.Join(testpath, "start"))
	fakeCmd(filepath.Join(testpath, "stop"))

	s.AgentSuite.PatchValue(&upstart.InitDir, c.MkDir())

	s.singularRecord = newSingularRunnerRecord()
	s.AgentSuite.PatchValue(&newSingularRunner, s.singularRecord.newSingularRunner)
	s.AgentSuite.PatchValue(&peergrouperNew, func(st *state.State) (worker.Worker, error) {
		return newDummyWorker(), nil
	})

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
	s.TestSuite.TearDownTest(c)
	s.AgentSuite.TearDownTest(c)
}

// primeAgent adds a new Machine to run the given jobs, and sets up the
// machine agent's directory.  It returns the new machine, the
// agent's configuration and the tools currently running.
func (s *commonMachineSuite) primeAgent(c *gc.C, jobs ...state.MachineJob) (m *state.Machine, agentConfig agent.ConfigSetterWriter, tools *tools.Tools) {
	vers := version.Binary{
		Number: jujuversion.Current,
		Arch:   arch.HostArch(),
		Series: series.HostSeries(),
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
		err := pinger.Stop()
		c.Check(err, jc.ErrorIsNil)
	})
	return s.configureMachine(c, m.Id(), vers)
}

func (s *commonMachineSuite) configureMachine(c *gc.C, machineId string, vers version.Binary) (
	machine *state.Machine, agentConfig agent.ConfigSetterWriter, tools *tools.Tools,
) {
	m, err := s.State.Machine(machineId)
	c.Assert(err, jc.ErrorIsNil)

	// Add a machine and ensure it is provisioned.
	inst, md := jujutesting.AssertStartInstance(c, s.Environ, machineId)
	c.Assert(m.SetProvisioned(inst.Id(), agent.BootstrapNonce, md), jc.ErrorIsNil)

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
	bufferedLogs logsender.LogRecordCh,
	rootDir string,
) func(string) *MachineAgent {
	return func(machineId string) *MachineAgent {
		return NewMachineAgent(
			machineId,
			agentConfWriter,
			bufferedLogs,
			worker.NewRunner(cmdutil.IsFatal, cmdutil.MoreImportant, worker.RestartDelay),
			&mockLoopDeviceManager{},
			rootDir,
		)
	}
}

// newAgent returns a new MachineAgent instance
func (s *commonMachineSuite) newAgent(c *gc.C, m *state.Machine) *MachineAgent {
	agentConf := agentConf{dataDir: s.DataDir()}
	agentConf.ReadConfig(names.NewMachineTag(m.Id()).String())
	machineAgentFactory := NewTestMachineAgentFactory(&agentConf, nil, c.MkDir())
	return machineAgentFactory(m.Id())
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
	addrs := network.NewAddresses("0.1.2.3")
	err := machine.SetProviderAddresses(addrs...)
	c.Assert(err, jc.ErrorIsNil)
	// Set the addresses in the environ instance as well so that if the instance poller
	// runs it won't overwrite them.
	instId, err := machine.InstanceId()
	c.Assert(err, jc.ErrorIsNil)
	insts, err := s.Environ.Instances([]instance.Id{instId})
	c.Assert(err, jc.ErrorIsNil)
	dummy.SetInstanceAddresses(insts[0], addrs)
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
			c.Fatalf("time out wating for operation")
		}
	}
}

type mockAgentConfig struct {
	agent.Config
	providerType string
	tag          names.Tag
}

func (m *mockAgentConfig) Tag() names.Tag {
	return m.tag
}

func (m *mockAgentConfig) Value(key string) string {
	if key == agent.ProviderType {
		return m.providerType
	}
	return ""
}

type singularRunnerRecord struct {
	runnerC chan *fakeSingularRunner
}

func newSingularRunnerRecord() *singularRunnerRecord {
	return &singularRunnerRecord{
		runnerC: make(chan *fakeSingularRunner, 64),
	}
}

func (r *singularRunnerRecord) newSingularRunner(runner worker.Runner, conn singular.Conn) (worker.Runner, error) {
	sr, err := singular.New(runner, conn)
	if err != nil {
		return nil, err
	}
	fakeRunner := &fakeSingularRunner{
		Runner: sr,
		startC: make(chan string, 64),
	}
	r.runnerC <- fakeRunner
	return fakeRunner, nil
}

// nextRunner blocks until a new singular runner is created.
func (r *singularRunnerRecord) nextRunner(c *gc.C) *fakeSingularRunner {
	timeout := time.After(coretesting.LongWait)
	for {
		select {
		case r := <-r.runnerC:
			return r
		case <-timeout:
			c.Fatal("timed out waiting for singular runner to be created")
		}
	}
}

type fakeSingularRunner struct {
	worker.Runner
	startC chan string
}

func (r *fakeSingularRunner) StartWorker(name string, start func() (worker.Worker, error)) error {
	logger.Infof("starting fake worker %q", name)
	r.startC <- name
	return r.Runner.StartWorker(name, start)
}

// waitForWorker waits for a given worker to be started, returning all
// workers started while waiting.
func (r *fakeSingularRunner) waitForWorker(c *gc.C, target string) []string {
	var seen []string
	timeout := time.After(coretesting.LongWait)
	for {
		select {
		case <-time.After(coretesting.ShortWait):
			c.Logf("still waiting for %q; workers seen so far: %+v", target, seen)
		case workerName := <-r.startC:
			seen = append(seen, workerName)
			if workerName == target {
				c.Logf("target worker %q started; workers seen so far: %+v", workerName, seen)
				return seen
			}
			c.Logf("worker %q started; still waiting for %q; workers seen so far: %+v", workerName, target, seen)
		case <-timeout:
			c.Fatal("timed out waiting for " + target)
		}
	}
}

// waitForWorkers waits for a given worker to be started, returning all
// workers started while waiting.
func (r *fakeSingularRunner) waitForWorkers(c *gc.C, targets []string) []string {
	var seen []string
	seenTargets := make(map[string]bool)
	numSeenTargets := 0
	timeout := time.After(coretesting.LongWait)
	for {
		select {
		case workerName := <-r.startC:
			c.Logf("worker %q started; workers seen so far: %+v (len: %d, len(targets): %d)", workerName, seen, len(seen), len(targets))
			if seenTargets[workerName] == true {
				c.Fatal("worker started twice: " + workerName)
			}
			seenTargets[workerName] = true
			numSeenTargets++
			seen = append(seen, workerName)
			if numSeenTargets == len(targets) {
				c.Logf("all expected target workers started: %+v", seen)
				return seen
			}
			c.Logf("still waiting for workers %+v to start; numSeenTargets=%d", targets, numSeenTargets)
		case <-timeout:
			c.Fatalf("timed out waiting for %v", targets)
		}
	}
}

type mockMetricAPI struct {
	stop          chan struct{}
	cleanUpCalled chan struct{}
	sendCalled    chan struct{}
}

func newMockMetricAPI() *mockMetricAPI {
	return &mockMetricAPI{
		stop:          make(chan struct{}),
		cleanUpCalled: make(chan struct{}),
		sendCalled:    make(chan struct{}),
	}
}

func (m *mockMetricAPI) CleanupOldMetrics() error {
	go func() {
		select {
		case m.cleanUpCalled <- struct{}{}:
		case <-m.stop:
			break
		}
	}()
	return nil
}

func (m *mockMetricAPI) SendMetrics() error {
	go func() {
		select {
		case m.sendCalled <- struct{}{}:
		case <-m.stop:
			break
		}
	}()
	return nil
}

func (m *mockMetricAPI) SendCalled() <-chan struct{} {
	return m.sendCalled
}

func (m *mockMetricAPI) CleanupCalled() <-chan struct{} {
	return m.cleanUpCalled
}

func (m *mockMetricAPI) Stop() {
	close(m.stop)
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
	return worker.NewSimpleWorker(func(stop <-chan struct{}) error {
		<-stop
		return nil
	})
}

type FakeConfig struct {
	agent.ConfigSetter
}

func (FakeConfig) LogDir() string {
	return filepath.FromSlash("/var/log/juju/")
}

func (FakeConfig) Tag() names.Tag {
	return names.NewMachineTag("42")
}

type FakeAgentConfig struct {
	AgentConf
}

func (FakeAgentConfig) ReadConfig(string) error { return nil }

func (FakeAgentConfig) CurrentConfig() agent.Config {
	return FakeConfig{}
}

func (FakeAgentConfig) ChangeConfig(mutate agent.ConfigMutator) error {
	return mutate(FakeConfig{})
}

func (FakeAgentConfig) CheckArgs([]string) error { return nil }
