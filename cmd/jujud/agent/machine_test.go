// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agent

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/names"
	gitjujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/proxy"
	"github.com/juju/utils/set"
	"github.com/juju/utils/symlink"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v4"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api"
	apideployer "github.com/juju/juju/api/deployer"
	apienvironment "github.com/juju/juju/api/environment"
	apifirewaller "github.com/juju/juju/api/firewaller"
	apimetricsmanager "github.com/juju/juju/api/metricsmanager"
	apinetworker "github.com/juju/juju/api/networker"
	apirsyslog "github.com/juju/juju/api/rsyslog"
	charmtesting "github.com/juju/juju/apiserver/charmrevisionupdater/testing"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cert"
	agenttesting "github.com/juju/juju/cmd/jujud/agent/testing"
	cmdutil "github.com/juju/juju/cmd/jujud/util"
	lxctesting "github.com/juju/juju/container/lxc/testing"
	"github.com/juju/juju/environs/config"
	envtesting "github.com/juju/juju/environs/testing"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/juju"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/mongo"
	"github.com/juju/juju/network"
	"github.com/juju/juju/provider/dummy"
	"github.com/juju/juju/service/upstart"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/watcher"
	"github.com/juju/juju/storage"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/tools"
	"github.com/juju/juju/utils/ssh"
	sshtesting "github.com/juju/juju/utils/ssh/testing"
	"github.com/juju/juju/version"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/authenticationworker"
	"github.com/juju/juju/worker/certupdater"
	"github.com/juju/juju/worker/deployer"
	"github.com/juju/juju/worker/diskmanager"
	"github.com/juju/juju/worker/instancepoller"
	"github.com/juju/juju/worker/networker"
	"github.com/juju/juju/worker/peergrouper"
	"github.com/juju/juju/worker/proxyupdater"
	"github.com/juju/juju/worker/rsyslog"
	"github.com/juju/juju/worker/singular"
	"github.com/juju/juju/worker/upgrader"
)

var (
	_ = gc.Suite(&MachineSuite{})
	_ = gc.Suite(&MachineWithCharmsSuite{})
	_ = gc.Suite(&mongoSuite{})
)

func TestPackage(t *testing.T) {
	// Change the default init dir in worker/deployer,
	// so the deployer doesn't try to remove upstart
	// jobs from tests.
	restore := gitjujutesting.PatchValue(&deployer.InitDir, mkdtemp("juju-worker-deployer"))
	defer restore()

	// TODO(waigani) 2014-03-19 bug 1294458
	// Refactor to use base suites

	// Change the path to "juju-run", so that the
	// tests don't try to write to /usr/local/bin.
	JujuRun = mktemp("juju-run", "")
	defer os.Remove(JujuRun)

	coretesting.MgoTestPackage(t)
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

type commonMachineSuite struct {
	singularRecord *singularRunnerRecord
	lxctesting.TestSuite
	fakeEnsureMongo agenttesting.FakeEnsure
	AgentSuite
}

func (s *commonMachineSuite) SetUpSuite(c *gc.C) {
	s.AgentSuite.SetUpSuite(c)
	s.TestSuite.SetUpSuite(c)
}

func (s *commonMachineSuite) TearDownSuite(c *gc.C) {
	s.TestSuite.TearDownSuite(c)
	s.AgentSuite.TearDownSuite(c)
}

func (s *commonMachineSuite) SetUpTest(c *gc.C) {
	s.AgentSuite.SetUpTest(c)
	s.TestSuite.SetUpTest(c)
	s.AgentSuite.PatchValue(&charm.CacheDir, c.MkDir())
	s.AgentSuite.PatchValue(&stateWorkerDialOpts, mongo.DialOpts{})

	os.Remove(JujuRun) // ignore error; may not exist
	// Patch ssh user to avoid touching ~ubuntu/.ssh/authorized_keys.
	s.AgentSuite.PatchValue(&authenticationworker.SSHUser, "")

	testpath := c.MkDir()
	s.AgentSuite.PatchEnvPathPrepend(testpath)
	// mock out the start method so we can fake install services without sudo
	fakeCmd(filepath.Join(testpath, "start"))
	fakeCmd(filepath.Join(testpath, "stop"))

	s.AgentSuite.PatchValue(&upstart.InitDir, c.MkDir())

	s.singularRecord = &singularRunnerRecord{startedWorkers: make(set.Strings)}
	s.AgentSuite.PatchValue(&newSingularRunner, s.singularRecord.newSingularRunner)
	s.AgentSuite.PatchValue(&peergrouperNew, func(st *state.State) (worker.Worker, error) {
		return newDummyWorker(), nil
	})

	s.fakeEnsureMongo = agenttesting.FakeEnsure{}
	s.AgentSuite.PatchValue(&cmdutil.EnsureMongoServer, s.fakeEnsureMongo.FakeEnsureMongo)
	s.AgentSuite.PatchValue(&maybeInitiateMongoServer, s.fakeEnsureMongo.FakeInitiateMongo)
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
func (s *commonMachineSuite) primeAgent(
	c *gc.C, vers version.Binary,
	jobs ...state.MachineJob) (m *state.Machine, agentConfig agent.ConfigSetterWriter, tools *tools.Tools) {

	m, err := s.State.AddMachine("quantal", jobs...)
	c.Assert(err, jc.ErrorIsNil)

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

	// Add an address for the tests in case the maybeInitiateMongoServer
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
		agentConfig, tools = s.AgentSuite.PrimeStateAgent(c, tag, initialMachinePassword, vers)
		info, ok := agentConfig.StateServingInfo()
		c.Assert(ok, jc.IsTrue)
		ssi := cmdutil.ParamsStateServingInfoToStateStateServingInfo(info)
		err = s.State.SetStateServingInfo(ssi)
		c.Assert(err, jc.ErrorIsNil)
	} else {
		agentConfig, tools = s.PrimeAgent(c, tag, initialMachinePassword, vers)
	}
	err = agentConfig.Write()
	c.Assert(err, jc.ErrorIsNil)
	return m, agentConfig, tools
}

// newAgent returns a new MachineAgent instance
func (s *commonMachineSuite) newAgent(c *gc.C, m *state.Machine) *MachineAgent {
	a := &MachineAgent{}
	s.InitAgent(c, a, "--machine-id", m.Id(), "--log-to-stderr=true")
	err := a.ReadConfig(m.Tag().String())
	c.Assert(err, jc.ErrorIsNil)
	return a
}

func (s *MachineSuite) TestParseSuccess(c *gc.C) {
	create := func() (cmd.Command, *AgentConf) {
		a := &MachineAgent{}
		return a, &a.AgentConf
	}
	a := CheckAgentCommand(c, create, []string{"--machine-id", "42"})
	c.Assert(a.(*MachineAgent).MachineId, gc.Equals, "42")
}

type MachineSuite struct {
	commonMachineSuite
	metricAPI *mockMetricAPI
}

const initialMachinePassword = "machine-password-1234567890"

func (s *MachineSuite) NewMockMetricAPI() *mockMetricAPI {
	cleanup := make(chan struct{})
	sender := make(chan struct{})
	return &mockMetricAPI{cleanup, sender}
}

func (s *MachineSuite) SetUpTest(c *gc.C) {
	s.metricAPI = s.NewMockMetricAPI()
	s.PatchValue(&getMetricAPI, func(_ *api.State) apimetricsmanager.MetricsManagerClient {
		return s.metricAPI
	})
	s.commonMachineSuite.SetUpTest(c)
}

func (s *MachineSuite) TestParseNonsense(c *gc.C) {
	for _, args := range [][]string{
		{},
		{"--machine-id", "-4004"},
	} {
		err := ParseAgentCommand(&MachineAgent{}, args)
		c.Assert(err, gc.ErrorMatches, "--machine-id option must be set, and expects a non-negative integer")
	}
}

func (s *MachineSuite) TestParseUnknown(c *gc.C) {
	a := &MachineAgent{}
	err := ParseAgentCommand(a, []string{"--machine-id", "42", "blistering barnacles"})
	c.Assert(err, gc.ErrorMatches, `unrecognized args: \["blistering barnacles"\]`)
}

func (s *MachineSuite) TestRunInvalidMachineId(c *gc.C) {
	c.Skip("agents don't yet distinguish between temporary and permanent errors")
	m, _, _ := s.primeAgent(c, version.Current, state.JobHostUnits)
	err := s.newAgent(c, m).Run(nil)
	c.Assert(err, gc.ErrorMatches, "some error")
}

func (s *MachineSuite) TestRunStop(c *gc.C) {
	m, ac, _ := s.primeAgent(c, version.Current, state.JobHostUnits)
	a := s.newAgent(c, m)
	done := make(chan error)
	go func() {
		done <- a.Run(nil)
	}()
	err := a.Stop()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(<-done, jc.ErrorIsNil)
	c.Assert(charm.CacheDir, gc.Equals, filepath.Join(ac.DataDir(), "charmcache"))
}

func (s *MachineSuite) TestWithDeadMachine(c *gc.C) {
	m, _, _ := s.primeAgent(c, version.Current, state.JobHostUnits)
	err := m.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
	a := s.newAgent(c, m)
	err = runWithTimeout(a)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *MachineSuite) TestWithRemovedMachine(c *gc.C) {
	m, _, _ := s.primeAgent(c, version.Current, state.JobHostUnits)
	err := m.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
	err = m.Remove()
	c.Assert(err, jc.ErrorIsNil)
	a := s.newAgent(c, m)
	err = runWithTimeout(a)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *MachineSuite) TestDyingMachine(c *gc.C) {
	m, _, _ := s.primeAgent(c, version.Current, state.JobHostUnits)
	a := s.newAgent(c, m)
	done := make(chan error)
	go func() {
		done <- a.Run(nil)
	}()
	defer func() {
		c.Check(a.Stop(), jc.ErrorIsNil)
	}()
	// Wait for configuration to be finished
	<-a.WorkersStarted()
	err := m.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	select {
	case err := <-done:
		c.Assert(err, jc.ErrorIsNil)
	case <-time.After(watcher.Period * 5 / 4):
		// TODO(rog) Fix this so it doesn't wait for so long.
		// https://bugs.github.com/juju/juju/+bug/1163983
		c.Fatalf("timed out waiting for agent to terminate")
	}
	err = m.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(m.Life(), gc.Equals, state.Dead)
}

func (s *MachineSuite) TestHostUnits(c *gc.C) {
	m, _, _ := s.primeAgent(c, version.Current, state.JobHostUnits)
	a := s.newAgent(c, m)
	ctx, reset := patchDeployContext(c, s.BackingState)
	defer reset()
	go func() { c.Check(a.Run(nil), jc.ErrorIsNil) }()
	defer func() { c.Check(a.Stop(), jc.ErrorIsNil) }()

	// check that unassigned units don't trigger any deployments.
	svc := s.AddTestingService(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	u0, err := svc.AddUnit()
	c.Assert(err, jc.ErrorIsNil)
	u1, err := svc.AddUnit()
	c.Assert(err, jc.ErrorIsNil)

	ctx.waitDeployed(c)

	// assign u0, check it's deployed.
	err = u0.AssignToMachine(m)
	c.Assert(err, jc.ErrorIsNil)
	ctx.waitDeployed(c, u0.Name())

	// "start the agent" for u0 to prevent short-circuited remove-on-destroy;
	// check that it's kept deployed despite being Dying.
	err = u0.SetStatus(state.StatusStarted, "", nil)
	c.Assert(err, jc.ErrorIsNil)
	err = u0.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	ctx.waitDeployed(c, u0.Name())

	// add u1 to the machine, check it's deployed.
	err = u1.AssignToMachine(m)
	c.Assert(err, jc.ErrorIsNil)
	ctx.waitDeployed(c, u0.Name(), u1.Name())

	// make u0 dead; check the deployer recalls the unit and removes it from
	// state.
	err = u0.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
	ctx.waitDeployed(c, u1.Name())

	// The deployer actually removes the unit just after
	// removing its deployment, so we need to poll here
	// until it actually happens.
	for attempt := coretesting.LongAttempt.Start(); attempt.Next(); {
		err := u0.Refresh()
		if err == nil && attempt.HasNext() {
			continue
		}
		c.Assert(err, jc.Satisfies, errors.IsNotFound)
	}

	// short-circuit-remove u1 after it's been deployed; check it's recalled
	// and removed from state.
	err = u1.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	err = u1.Refresh()
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
	ctx.waitDeployed(c)
}

func patchDeployContext(c *gc.C, st *state.State) (*fakeContext, func()) {
	ctx := &fakeContext{
		inited:   make(chan struct{}),
		deployed: make(set.Strings),
	}
	orig := newDeployContext
	newDeployContext = func(dst *apideployer.State, agentConfig agent.Config) deployer.Context {
		ctx.st = st
		ctx.agentConfig = agentConfig
		close(ctx.inited)
		return ctx
	}
	return ctx, func() { newDeployContext = orig }
}

func (s *commonMachineSuite) setFakeMachineAddresses(c *gc.C, machine *state.Machine) {
	addrs := []network.Address{
		network.NewAddress("0.1.2.3", network.ScopeUnknown),
	}
	err := machine.SetAddresses(addrs...)
	c.Assert(err, jc.ErrorIsNil)
	// Set the addresses in the environ instance as well so that if the instance poller
	// runs it won't overwrite them.
	instId, err := machine.InstanceId()
	c.Assert(err, jc.ErrorIsNil)
	insts, err := s.Environ.Instances([]instance.Id{instId})
	c.Assert(err, jc.ErrorIsNil)
	dummy.SetInstanceAddresses(insts[0], addrs)
}

func (s *MachineSuite) TestManageEnviron(c *gc.C) {
	usefulVersion := version.Current
	usefulVersion.Series = "quantal" // to match the charm created below
	envtesting.AssertUploadFakeToolsVersions(
		c, s.DefaultToolsStorage, s.Environ.Config().AgentStream(), s.Environ.Config().AgentStream(), usefulVersion)
	m, _, _ := s.primeAgent(c, version.Current, state.JobManageEnviron)
	op := make(chan dummy.Operation, 200)
	dummy.Listen(op)

	a := s.newAgent(c, m)
	// Make sure the agent is stopped even if the test fails.
	defer a.Stop()
	done := make(chan error)
	go func() {
		done <- a.Run(nil)
	}()

	// Check that the provisioner and firewaller are alive by doing
	// a rudimentary check that it responds to state changes.

	// Add one unit to a service; it should get allocated a machine
	// and then its ports should be opened.
	charm := s.AddTestingCharm(c, "dummy")
	svc := s.AddTestingService(c, "test-service", charm)
	err := svc.SetExposed()
	c.Assert(err, jc.ErrorIsNil)
	units, err := juju.AddUnits(s.State, svc, 1, "")
	c.Assert(err, jc.ErrorIsNil)
	c.Check(opRecvTimeout(c, s.State, op, dummy.OpStartInstance{}), gc.NotNil)

	// Wait for the instance id to show up in the state.
	s.waitProvisioned(c, units[0])
	err = units[0].OpenPort("tcp", 999)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(opRecvTimeout(c, s.State, op, dummy.OpOpenPorts{}), gc.NotNil)

	// Check that the metrics workers have started by adding metrics
	select {
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timed out waiting for metric cleanup API to be called")
	case <-s.metricAPI.CleanupCalled():
	}
	select {
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timed out waiting for metric sender API to be called")
	case <-s.metricAPI.SendCalled():
	}

	err = a.Stop()
	c.Assert(err, jc.ErrorIsNil)

	select {
	case err := <-done:
		c.Assert(err, jc.ErrorIsNil)
	case <-time.After(5 * time.Second):
		c.Fatalf("timed out waiting for agent to terminate")
	}

	c.Assert(s.singularRecord.started(), jc.DeepEquals, []string{
		"charm-revision-updater",
		"cleaner",
		"environ-provisioner",
		"firewaller",
		"minunitsworker",
		"resumer",
	})
}

func (s *MachineSuite) TestManageEnvironDoesNotRunFirewallerWhenModeIsNone(c *gc.C) {
	started := make(chan struct{}, 1)
	s.AgentSuite.PatchValue(&newFirewaller, func(st *apifirewaller.State) (worker.Worker, error) {
		select {
		case started <- struct{}{}:
		default:
		}
		return newDummyWorker(), nil
	})
	m, _, _ := s.primeAgent(c, version.Current, state.JobManageEnviron)
	a := s.newAgent(c, m)
	defer a.Stop()
	go func() {
		c.Check(a.Run(nil), jc.ErrorIsNil)
	}()
	select {
	case <-started:
		c.Fatalf("firewaller worker unexpectedly started")
	case <-time.After(coretesting.ShortWait):
	}
}

func (s *MachineSuite) TestManageEnvironRunsInstancePoller(c *gc.C) {
	s.AgentSuite.PatchValue(&instancepoller.ShortPoll, 500*time.Millisecond)
	usefulVersion := version.Current
	usefulVersion.Series = "quantal" // to match the charm created below
	envtesting.AssertUploadFakeToolsVersions(
		c, s.DefaultToolsStorage, s.Environ.Config().AgentStream(), s.Environ.Config().AgentStream(), usefulVersion)
	m, _, _ := s.primeAgent(c, version.Current, state.JobManageEnviron)
	a := s.newAgent(c, m)
	defer a.Stop()
	go func() {
		c.Check(a.Run(nil), jc.ErrorIsNil)
	}()

	// Add one unit to a service;
	charm := s.AddTestingCharm(c, "dummy")
	svc := s.AddTestingService(c, "test-service", charm)
	units, err := juju.AddUnits(s.State, svc, 1, "")
	c.Assert(err, jc.ErrorIsNil)

	m, instId := s.waitProvisioned(c, units[0])
	insts, err := s.Environ.Instances([]instance.Id{instId})
	c.Assert(err, jc.ErrorIsNil)
	addrs := []network.Address{network.NewAddress("1.2.3.4", network.ScopeUnknown)}
	dummy.SetInstanceAddresses(insts[0], addrs)
	dummy.SetInstanceStatus(insts[0], "running")

	for a := coretesting.LongAttempt.Start(); a.Next(); {
		if !a.HasNext() {
			c.Logf("final machine addresses: %#v", m.Addresses())
			c.Fatalf("timed out waiting for machine to get address")
		}
		err := m.Refresh()
		c.Assert(err, jc.ErrorIsNil)
		instStatus, err := m.InstanceStatus()
		c.Assert(err, jc.ErrorIsNil)
		if reflect.DeepEqual(m.Addresses(), addrs) && instStatus == "running" {
			break
		}
	}
}

func (s *MachineSuite) TestManageEnvironRunsPeergrouper(c *gc.C) {
	started := make(chan struct{}, 1)
	s.AgentSuite.PatchValue(&peergrouperNew, func(st *state.State) (worker.Worker, error) {
		c.Check(st, gc.NotNil)
		select {
		case started <- struct{}{}:
		default:
		}
		return newDummyWorker(), nil
	})
	m, _, _ := s.primeAgent(c, version.Current, state.JobManageEnviron)
	a := s.newAgent(c, m)
	defer a.Stop()
	go func() {
		c.Check(a.Run(nil), jc.ErrorIsNil)
	}()
	select {
	case <-started:
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timed out waiting for peergrouper worker to be started")
	}
}

func (s *MachineSuite) TestManageEnvironCallsUseMultipleCPUs(c *gc.C) {
	// If it has been enabled, the JobManageEnviron agent should call utils.UseMultipleCPUs
	usefulVersion := version.Current
	usefulVersion.Series = "quantal"
	envtesting.AssertUploadFakeToolsVersions(
		c, s.DefaultToolsStorage, s.Environ.Config().AgentStream(), s.Environ.Config().AgentStream(), usefulVersion)
	m, _, _ := s.primeAgent(c, version.Current, state.JobManageEnviron)
	calledChan := make(chan struct{}, 1)
	s.AgentSuite.PatchValue(&useMultipleCPUs, func() { calledChan <- struct{}{} })
	// Now, start the agent, and observe that a JobManageEnviron agent
	// calls UseMultipleCPUs
	a := s.newAgent(c, m)
	defer a.Stop()
	go func() {
		c.Check(a.Run(nil), jc.ErrorIsNil)
	}()
	// Wait for configuration to be finished
	<-a.WorkersStarted()
	select {
	case <-calledChan:
	case <-time.After(coretesting.LongWait):
		c.Errorf("we failed to call UseMultipleCPUs()")
	}
	c.Check(a.Stop(), jc.ErrorIsNil)
	// However, an agent that just JobHostUnits doesn't call UseMultipleCPUs
	m2, _, _ := s.primeAgent(c, version.Current, state.JobHostUnits)
	a2 := s.newAgent(c, m2)
	defer a2.Stop()
	go func() {
		c.Check(a2.Run(nil), jc.ErrorIsNil)
	}()
	// Wait until all the workers have been started, and then kill everything
	<-a2.workersStarted
	c.Check(a2.Stop(), jc.ErrorIsNil)
	select {
	case <-calledChan:
		c.Errorf("we should not have called UseMultipleCPUs()")
	case <-time.After(coretesting.ShortWait):
	}
}

func (s *MachineSuite) waitProvisioned(c *gc.C, unit *state.Unit) (*state.Machine, instance.Id) {
	c.Logf("waiting for unit %q to be provisioned", unit)
	machineId, err := unit.AssignedMachineId()
	c.Assert(err, jc.ErrorIsNil)
	m, err := s.State.Machine(machineId)
	c.Assert(err, jc.ErrorIsNil)
	w := m.Watch()
	defer w.Stop()
	timeout := time.After(coretesting.LongWait)
	for {
		select {
		case <-timeout:
			c.Fatalf("timed out waiting for provisioning")
		case _, ok := <-w.Changes():
			c.Assert(ok, jc.IsTrue)
			err := m.Refresh()
			c.Assert(err, jc.ErrorIsNil)
			if instId, err := m.InstanceId(); err == nil {
				c.Logf("unit provisioned with instance %s", instId)
				return m, instId
			} else {
				c.Check(err, jc.Satisfies, state.IsNotProvisionedError)
			}
		}
	}
}

func (s *MachineSuite) testUpgradeRequest(c *gc.C, agent runner, tag string, currentTools *tools.Tools) {
	newVers := version.Current
	newVers.Patch++
	newTools := envtesting.AssertUploadFakeToolsVersions(
		c, s.DefaultToolsStorage, s.Environ.Config().AgentStream(), s.Environ.Config().AgentStream(), newVers)[0]
	err := s.State.SetEnvironAgentVersion(newVers.Number)
	c.Assert(err, jc.ErrorIsNil)
	err = runWithTimeout(agent)
	envtesting.CheckUpgraderReadyError(c, err, &upgrader.UpgradeReadyError{
		AgentName: tag,
		OldTools:  currentTools.Version,
		NewTools:  newTools.Version,
		DataDir:   s.DataDir(),
	})
}

func (s *MachineSuite) TestUpgradeRequest(c *gc.C) {
	m, _, currentTools := s.primeAgent(c, version.Current, state.JobManageEnviron, state.JobHostUnits)
	a := s.newAgent(c, m)
	s.testUpgradeRequest(c, a, m.Tag().String(), currentTools)
}

var fastDialOpts = api.DialOpts{
	Timeout:    coretesting.LongWait,
	RetryDelay: coretesting.ShortWait,
}

func (s *MachineSuite) waitStopped(c *gc.C, job state.MachineJob, a *MachineAgent, done chan error) {
	err := a.Stop()
	if job == state.JobManageEnviron {
		// When shutting down, the API server can be shut down before
		// the other workers that connect to it, so they get an error so
		// they then die, causing Stop to return an error.  It's not
		// easy to control the actual error that's received in this
		// circumstance so we just log it rather than asserting that it
		// is not nil.
		if err != nil {
			c.Logf("error shutting down state manager: %v", err)
		}
	} else {
		c.Assert(err, jc.ErrorIsNil)
	}

	select {
	case err := <-done:
		c.Assert(err, jc.ErrorIsNil)
	case <-time.After(5 * time.Second):
		c.Fatalf("timed out waiting for agent to terminate")
	}
}

func (s *MachineSuite) assertJobWithAPI(
	c *gc.C,
	job state.MachineJob,
	test func(agent.Config, *api.State),
) {
	s.assertAgentOpensState(c, &reportOpenedAPI, job, func(cfg agent.Config, st eitherState) {
		test(cfg, st.(*api.State))
	})
}

func (s *MachineSuite) assertJobWithState(
	c *gc.C,
	job state.MachineJob,
	test func(agent.Config, *state.State),
) {
	paramsJob := job.ToParams()
	if !paramsJob.NeedsState() {
		c.Fatalf("%v does not use state", paramsJob)
	}
	s.assertAgentOpensState(c, &reportOpenedState, job, func(cfg agent.Config, st eitherState) {
		test(cfg, st.(*state.State))
	})
}

// assertAgentOpensState asserts that a machine agent started with the
// given job will call the function pointed to by reportOpened. The
// agent's configuration and the value passed to reportOpened are then
// passed to the test function for further checking.
func (s *MachineSuite) assertAgentOpensState(
	c *gc.C,
	reportOpened *func(eitherState),
	job state.MachineJob,
	test func(agent.Config, eitherState),
) {
	stm, conf, _ := s.primeAgent(c, version.Current, job)
	a := s.newAgent(c, stm)
	defer a.Stop()
	logger.Debugf("new agent %#v", a)

	// All state jobs currently also run an APIWorker, so no
	// need to check for that here, like in assertJobWithState.

	agentAPIs := make(chan eitherState, 1)
	s.AgentSuite.PatchValue(reportOpened, func(st eitherState) {
		select {
		case agentAPIs <- st:
		default:
		}
	})

	done := make(chan error)
	go func() {
		done <- a.Run(nil)
	}()

	select {
	case agentAPI := <-agentAPIs:
		c.Assert(agentAPI, gc.NotNil)
		test(conf, agentAPI)
	case <-time.After(coretesting.LongWait):
		c.Fatalf("API not opened")
	}

	s.waitStopped(c, job, a, done)
}

func (s *MachineSuite) TestManageEnvironServesAPI(c *gc.C) {
	s.assertJobWithState(c, state.JobManageEnviron, func(conf agent.Config, agentState *state.State) {
		st, err := api.Open(conf.APIInfo(), fastDialOpts)
		c.Assert(err, jc.ErrorIsNil)
		defer st.Close()
		m, err := st.Machiner().Machine(conf.Tag().(names.MachineTag))
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(m.Life(), gc.Equals, params.Alive)
	})
}

func (s *MachineSuite) assertAgentSetsToolsVersion(c *gc.C, job state.MachineJob) {
	vers := version.Current
	vers.Minor = version.Current.Minor + 1
	m, _, _ := s.primeAgent(c, vers, job)
	a := s.newAgent(c, m)
	go func() { c.Check(a.Run(nil), jc.ErrorIsNil) }()
	defer func() { c.Check(a.Stop(), jc.ErrorIsNil) }()

	timeout := time.After(coretesting.LongWait)
	for done := false; !done; {
		select {
		case <-timeout:
			c.Fatalf("timeout while waiting for agent version to be set")
		case <-time.After(coretesting.ShortWait):
			c.Log("Refreshing")
			err := m.Refresh()
			c.Assert(err, jc.ErrorIsNil)
			c.Log("Fetching agent tools")
			agentTools, err := m.AgentTools()
			c.Assert(err, jc.ErrorIsNil)
			c.Logf("(%v vs. %v)", agentTools.Version, version.Current)
			if agentTools.Version.Minor != version.Current.Minor {
				continue
			}
			c.Assert(agentTools.Version, gc.DeepEquals, version.Current)
			done = true
		}
	}
}

func (s *MachineSuite) TestAgentSetsToolsVersionManageEnviron(c *gc.C) {
	s.assertAgentSetsToolsVersion(c, state.JobManageEnviron)
}

func (s *MachineSuite) TestAgentSetsToolsVersionHostUnits(c *gc.C) {
	s.assertAgentSetsToolsVersion(c, state.JobHostUnits)
}

func (s *MachineSuite) TestManageEnvironRunsCleaner(c *gc.C) {
	s.assertJobWithState(c, state.JobManageEnviron, func(conf agent.Config, agentState *state.State) {
		// Create a service and unit, and destroy the service.
		service := s.AddTestingService(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
		unit, err := service.AddUnit()
		c.Assert(err, jc.ErrorIsNil)
		err = service.Destroy()
		c.Assert(err, jc.ErrorIsNil)

		// Check the unit was not yet removed.
		err = unit.Refresh()
		c.Assert(err, jc.ErrorIsNil)
		w := unit.Watch()
		defer w.Stop()

		// Trigger a sync on the state used by the agent, and wait
		// for the unit to be removed.
		agentState.StartSync()
		timeout := time.After(coretesting.LongWait)
		for done := false; !done; {
			select {
			case <-timeout:
				c.Fatalf("unit not cleaned up")
			case <-time.After(coretesting.ShortWait):
				s.State.StartSync()
			case <-w.Changes():
				err := unit.Refresh()
				if errors.IsNotFound(err) {
					done = true
				} else {
					c.Assert(err, jc.ErrorIsNil)
				}
			}
		}
	})
}

func (s *MachineSuite) TestJobManageEnvironRunsMinUnitsWorker(c *gc.C) {
	s.assertJobWithState(c, state.JobManageEnviron, func(conf agent.Config, agentState *state.State) {
		// Ensure that the MinUnits worker is alive by doing a simple check
		// that it responds to state changes: add a service, set its minimum
		// number of units to one, wait for the worker to add the missing unit.
		service := s.AddTestingService(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
		err := service.SetMinUnits(1)
		c.Assert(err, jc.ErrorIsNil)
		w := service.Watch()
		defer w.Stop()

		// Trigger a sync on the state used by the agent, and wait for the unit
		// to be created.
		agentState.StartSync()
		timeout := time.After(coretesting.LongWait)
		for {
			select {
			case <-timeout:
				c.Fatalf("unit not created")
			case <-time.After(coretesting.ShortWait):
				s.State.StartSync()
			case <-w.Changes():
				units, err := service.AllUnits()
				c.Assert(err, jc.ErrorIsNil)
				if len(units) == 1 {
					return
				}
			}
		}
	})
}

func (s *MachineSuite) TestMachineAgentRunsAuthorisedKeysWorker(c *gc.C) {
	// Start the machine agent.
	m, _, _ := s.primeAgent(c, version.Current, state.JobHostUnits)
	a := s.newAgent(c, m)
	go func() { c.Check(a.Run(nil), jc.ErrorIsNil) }()
	defer func() { c.Check(a.Stop(), jc.ErrorIsNil) }()

	// Update the keys in the environment.
	sshKey := sshtesting.ValidKeyOne.Key + " user@host"
	err := s.BackingState.UpdateEnvironConfig(map[string]interface{}{"authorized-keys": sshKey}, nil, nil)
	c.Assert(err, jc.ErrorIsNil)

	// Wait for ssh keys file to be updated.
	s.State.StartSync()
	timeout := time.After(coretesting.LongWait)
	sshKeyWithCommentPrefix := sshtesting.ValidKeyOne.Key + " Juju:user@host"
	for {
		select {
		case <-timeout:
			c.Fatalf("timeout while waiting for authorised ssh keys to change")
		case <-time.After(coretesting.ShortWait):
			keys, err := ssh.ListKeys(authenticationworker.SSHUser, ssh.FullKeys)
			c.Assert(err, jc.ErrorIsNil)
			keysStr := strings.Join(keys, "\n")
			if sshKeyWithCommentPrefix != keysStr {
				continue
			}
			return
		}
	}
}

// opRecvTimeout waits for any of the given kinds of operation to
// be received from ops, and times out if not.
func opRecvTimeout(c *gc.C, st *state.State, opc <-chan dummy.Operation, kinds ...dummy.Operation) dummy.Operation {
	st.StartSync()
	for {
		select {
		case op := <-opc:
			for _, k := range kinds {
				if reflect.TypeOf(op) == reflect.TypeOf(k) {
					return op
				}
			}
			c.Logf("discarding unknown event %#v", op)
		case <-time.After(15 * time.Second):
			c.Fatalf("time out wating for operation")
		}
	}
}

func (s *MachineSuite) TestOpenStateFailsForJobHostUnitsButOpenAPIWorks(c *gc.C) {
	m, _, _ := s.primeAgent(c, version.Current, state.JobHostUnits)
	s.RunTestOpenAPIState(c, m, s.newAgent(c, m), initialMachinePassword)
	s.assertJobWithAPI(c, state.JobHostUnits, func(conf agent.Config, st *api.State) {
		s.AssertCannotOpenState(c, conf.Tag(), conf.DataDir())
	})
}

func (s *MachineSuite) TestOpenStateWorksForJobManageEnviron(c *gc.C) {
	s.assertJobWithAPI(c, state.JobManageEnviron, func(conf agent.Config, st *api.State) {
		s.AssertCanOpenState(c, conf.Tag(), conf.DataDir())
	})
}

func (s *MachineSuite) TestMachineAgentSymlinkJujuRun(c *gc.C) {
	_, err := os.Stat(JujuRun)
	c.Assert(err, jc.Satisfies, os.IsNotExist)
	s.assertJobWithAPI(c, state.JobManageEnviron, func(conf agent.Config, st *api.State) {
		// juju-run should have been created
		_, err := os.Stat(JujuRun)
		c.Assert(err, jc.ErrorIsNil)
	})
}

func (s *MachineSuite) TestMachineAgentSymlinkJujuRunExists(c *gc.C) {
	err := symlink.New("/nowhere/special", JujuRun)
	c.Assert(err, jc.ErrorIsNil)
	_, err = os.Stat(JujuRun)
	c.Assert(err, jc.Satisfies, os.IsNotExist)
	s.assertJobWithAPI(c, state.JobManageEnviron, func(conf agent.Config, st *api.State) {
		// juju-run should have been recreated
		_, err := os.Stat(JujuRun)
		c.Assert(err, jc.ErrorIsNil)
		link, err := symlink.Read(JujuRun)
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(link, gc.Not(gc.Equals), "/nowhere/special")
	})
}

func (s *MachineSuite) TestProxyUpdater(c *gc.C) {
	s.assertProxyUpdater(c, true)
	s.assertProxyUpdater(c, false)
}

func (s *MachineSuite) assertProxyUpdater(c *gc.C, expectWriteSystemFiles bool) {
	// Patch out the func that decides whether we should write system files.
	var gotConf agent.Config
	s.AgentSuite.PatchValue(&shouldWriteProxyFiles, func(conf agent.Config) bool {
		gotConf = conf
		return expectWriteSystemFiles
	})

	// Make sure there are some proxy settings to write.
	expectSettings := proxy.Settings{
		Http:  "http proxy",
		Https: "https proxy",
		Ftp:   "ftp proxy",
	}
	updateAttrs := config.ProxyConfigMap(expectSettings)
	err := s.State.UpdateEnvironConfig(updateAttrs, nil, nil)
	c.Assert(err, jc.ErrorIsNil)

	// Patch out the actual worker func.
	started := make(chan struct{})
	mockNew := func(api *apienvironment.Facade, writeSystemFiles bool) worker.Worker {
		// Direct check of the behaviour flag.
		c.Check(writeSystemFiles, gc.Equals, expectWriteSystemFiles)
		// Indirect check that we get a functional API.
		conf, err := api.EnvironConfig()
		if c.Check(err, jc.ErrorIsNil) {
			actualSettings := conf.ProxySettings()
			c.Check(actualSettings, jc.DeepEquals, expectSettings)
		}
		return worker.NewSimpleWorker(func(_ <-chan struct{}) error {
			close(started)
			return nil
		})
	}
	s.AgentSuite.PatchValue(&proxyupdater.New, mockNew)

	s.primeAgent(c, version.Current, state.JobHostUnits)
	s.assertJobWithAPI(c, state.JobHostUnits, func(conf agent.Config, st *api.State) {
		for {
			select {
			case <-time.After(coretesting.LongWait):
				c.Fatalf("timeout while waiting for proxy updater to start")
			case <-started:
				c.Assert(gotConf, jc.DeepEquals, conf)
				return
			}
		}
	})
}

func (s *MachineSuite) TestMachineAgentUninstall(c *gc.C) {
	m, ac, _ := s.primeAgent(c, version.Current, state.JobHostUnits)
	err := m.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
	a := s.newAgent(c, m)
	err = runWithTimeout(a)
	c.Assert(err, jc.ErrorIsNil)
	// juju-run should have been removed on termination
	_, err = os.Stat(JujuRun)
	c.Assert(err, jc.Satisfies, os.IsNotExist)
	// data-dir should have been removed on termination
	_, err = os.Stat(ac.DataDir())
	c.Assert(err, jc.Satisfies, os.IsNotExist)
}

func (s *MachineSuite) TestMachineAgentRsyslogManageEnviron(c *gc.C) {
	s.testMachineAgentRsyslogConfigWorker(c, state.JobManageEnviron, rsyslog.RsyslogModeAccumulate)
}

func (s *MachineSuite) TestMachineAgentRsyslogHostUnits(c *gc.C) {
	s.testMachineAgentRsyslogConfigWorker(c, state.JobHostUnits, rsyslog.RsyslogModeForwarding)
}

func (s *MachineSuite) testMachineAgentRsyslogConfigWorker(c *gc.C, job state.MachineJob, expectedMode rsyslog.RsyslogMode) {
	created := make(chan rsyslog.RsyslogMode, 1)
	s.AgentSuite.PatchValue(&cmdutil.NewRsyslogConfigWorker, func(_ *apirsyslog.State, _ agent.Config, mode rsyslog.RsyslogMode) (worker.Worker, error) {
		created <- mode
		return newDummyWorker(), nil
	})
	s.assertJobWithAPI(c, job, func(conf agent.Config, st *api.State) {
		select {
		case <-time.After(coretesting.LongWait):
			c.Fatalf("timeout while waiting for rsyslog worker to be created")
		case mode := <-created:
			c.Assert(mode, gc.Equals, expectedMode)
		}
	})
}

func (s *MachineSuite) TestMachineAgentRunsAPIAddressUpdaterWorker(c *gc.C) {
	// Start the machine agent.
	m, _, _ := s.primeAgent(c, version.Current, state.JobHostUnits)
	a := s.newAgent(c, m)
	go func() { c.Check(a.Run(nil), jc.ErrorIsNil) }()
	defer func() { c.Check(a.Stop(), jc.ErrorIsNil) }()

	// Update the API addresses.
	updatedServers := [][]network.HostPort{
		network.NewHostPorts(1234, "localhost"),
	}
	err := s.BackingState.SetAPIHostPorts(updatedServers)
	c.Assert(err, jc.ErrorIsNil)

	// Wait for config to be updated.
	s.BackingState.StartSync()
	for attempt := coretesting.LongAttempt.Start(); attempt.Next(); {
		addrs, err := a.CurrentConfig().APIAddresses()
		c.Assert(err, jc.ErrorIsNil)
		if reflect.DeepEqual(addrs, []string{"localhost:1234"}) {
			return
		}
	}
	c.Fatalf("timeout while waiting for agent config to change")
}

func (s *MachineSuite) TestMachineAgentRunsDiskManagerWorker(c *gc.C) {
	// Start the machine agent.
	m, _, _ := s.primeAgent(c, version.Current, state.JobHostUnits)
	a := s.newAgent(c, m)
	go func() { c.Check(a.Run(nil), jc.ErrorIsNil) }()
	defer func() { c.Check(a.Stop(), jc.ErrorIsNil) }()

	started := make(chan struct{})
	newWorker := func(diskmanager.ListBlockDevicesFunc, diskmanager.BlockDeviceSetter) worker.Worker {
		close(started)
		return worker.NewNoOpWorker()
	}
	s.PatchValue(&newDiskManager, newWorker)

	// Wait for worker to be started.
	select {
	case <-started:
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timeout while waiting for diskmanager worker to start")
	}
}

func (s *MachineSuite) TestDiskManagerWorkerUpdatesState(c *gc.C) {
	expected := []storage.BlockDevice{{DeviceName: "whatever"}}
	s.PatchValue(&diskmanager.DefaultListBlockDevices, func() ([]storage.BlockDevice, error) {
		return expected, nil
	})

	// Start the machine agent.
	m, _, _ := s.primeAgent(c, version.Current, state.JobHostUnits)
	a := s.newAgent(c, m)
	go func() { c.Check(a.Run(nil), jc.ErrorIsNil) }()
	defer func() { c.Check(a.Stop(), jc.ErrorIsNil) }()

	// Wait for state to be updated.
	s.BackingState.StartSync()
	for attempt := coretesting.LongAttempt.Start(); attempt.Next(); {
		devices, err := m.BlockDevices()
		c.Assert(err, jc.ErrorIsNil)
		if len(devices) > 0 {
			c.Assert(devices, gc.DeepEquals, expected)
			return
		}
	}
	c.Fatalf("timeout while waiting for block devices to be recorded")
}

func (s *MachineSuite) TestMachineAgentRunsCertificateUpdateWorkerForStateServer(c *gc.C) {
	started := make(chan struct{})
	newUpdater := func(certupdater.AddressWatcher, certupdater.StateServingInfoGetter, certupdater.EnvironConfigGetter,
		certupdater.StateServingInfoSetter,
	) worker.Worker {
		close(started)
		return worker.NewNoOpWorker()
	}
	s.PatchValue(&newCertificateUpdater, newUpdater)

	// Start the machine agent.
	m, _, _ := s.primeAgent(c, version.Current, state.JobManageEnviron)
	a := s.newAgent(c, m)
	go func() { c.Check(a.Run(nil), jc.ErrorIsNil) }()
	defer func() { c.Check(a.Stop(), jc.ErrorIsNil) }()

	// Wait for worker to be started.
	select {
	case <-started:
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timeout while waiting for certificate update worker to start")
	}
}

func (s *MachineSuite) TestMachineAgentDoesNotRunsCertificateUpdateWorkerForNonStateServer(c *gc.C) {
	started := make(chan struct{})
	newUpdater := func(certupdater.AddressWatcher, certupdater.StateServingInfoGetter, certupdater.EnvironConfigGetter,
		certupdater.StateServingInfoSetter,
	) worker.Worker {
		close(started)
		return worker.NewNoOpWorker()
	}
	s.PatchValue(&newCertificateUpdater, newUpdater)

	// Start the machine agent.
	m, _, _ := s.primeAgent(c, version.Current, state.JobHostUnits)
	a := s.newAgent(c, m)
	go func() { c.Check(a.Run(nil), jc.ErrorIsNil) }()
	defer func() { c.Check(a.Stop(), jc.ErrorIsNil) }()

	// Ensure the worker is not started.
	select {
	case <-started:
		c.Fatalf("certificate update worker unexpectedly started")
	case <-time.After(coretesting.ShortWait):
	}
}

func (s *MachineSuite) TestCertificateUpdateWorkerUpdatesCertificate(c *gc.C) {
	// Set up the machine agent.
	m, _, _ := s.primeAgent(c, version.Current, state.JobManageEnviron)
	a := s.newAgent(c, m)

	// Set up check that certificate has been updated.
	updated := make(chan struct{})
	go func() {
		for {
			stateInfo, _ := a.CurrentConfig().StateServingInfo()
			srvCert, err := cert.ParseCert(stateInfo.Cert)
			c.Assert(err, jc.ErrorIsNil)
			sanIPs := make([]string, len(srvCert.IPAddresses))
			for i, ip := range srvCert.IPAddresses {
				sanIPs[i] = ip.String()
			}
			if len(sanIPs) == 1 && sanIPs[0] == "0.1.2.3" {
				close(updated)
				break
			}
			time.Sleep(10 * time.Millisecond)
		}
	}()

	go func() { c.Check(a.Run(nil), jc.ErrorIsNil) }()
	defer func() { c.Check(a.Stop(), jc.ErrorIsNil) }()
	// Wait for certificate to be updated.
	select {
	case <-updated:
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timeout while waiting for certificate to be updated")
	}
}

func (s *MachineSuite) TestMachineAgentNetworkerMode(c *gc.C) {
	tests := []struct {
		about          string
		managedNetwork bool
		jobs           []state.MachineJob
		intrusiveMode  bool
	}{{
		about:          "network management enabled, network management job set",
		managedNetwork: true,
		jobs:           []state.MachineJob{state.JobHostUnits, state.JobManageNetworking},
		intrusiveMode:  true,
	}, {
		about:          "network management disabled, network management job set",
		managedNetwork: false,
		jobs:           []state.MachineJob{state.JobHostUnits, state.JobManageNetworking},
		intrusiveMode:  false,
	}, {
		about:          "network management enabled, network management job not set",
		managedNetwork: true,
		jobs:           []state.MachineJob{state.JobHostUnits},
		intrusiveMode:  false,
	}, {
		about:          "network management disabled, network management job not set",
		managedNetwork: false,
		jobs:           []state.MachineJob{state.JobHostUnits},
		intrusiveMode:  false,
	}}
	// Perform tests.
	for i, test := range tests {
		c.Logf("test #%d: %s", i, test.about)

		modeCh := make(chan bool, 1)
		s.AgentSuite.PatchValue(&newNetworker, func(
			st *apinetworker.State,
			conf agent.Config,
			intrusiveMode bool,
			configBaseDir string,
		) (*networker.Networker, error) {
			select {
			case modeCh <- intrusiveMode:
			default:
			}
			return networker.NewNetworker(st, conf, intrusiveMode, configBaseDir)
		})

		attrs := coretesting.Attrs{"disable-network-management": !test.managedNetwork}
		err := s.BackingState.UpdateEnvironConfig(attrs, nil, nil)
		c.Assert(err, jc.ErrorIsNil)

		m, _, _ := s.primeAgent(c, version.Current, test.jobs...)
		a := s.newAgent(c, m)
		defer a.Stop()
		doneCh := make(chan error)
		go func() {
			doneCh <- a.Run(nil)
		}()

		select {
		case intrusiveMode := <-modeCh:
			if intrusiveMode != test.intrusiveMode {
				c.Fatalf("expected networker intrusive mode = %v, got mode = %v", test.intrusiveMode, intrusiveMode)
			}
		case <-time.After(coretesting.LongWait):
			c.Fatalf("timed out waiting for the networker to start")
		}
		s.waitStopped(c, state.JobManageNetworking, a, doneCh)
	}
}

func (s *MachineSuite) TestMachineAgentUpgradeMongo(c *gc.C) {
	m, agentConfig, _ := s.primeAgent(c, version.Current, state.JobManageEnviron)
	agentConfig.SetUpgradedToVersion(version.MustParse("1.18.0"))
	err := agentConfig.Write()
	c.Assert(err, jc.ErrorIsNil)
	err = s.State.MongoSession().DB("admin").RemoveUser(m.Tag().String())
	c.Assert(err, jc.ErrorIsNil)

	s.AgentSuite.PatchValue(&ensureMongoAdminUser, func(p mongo.EnsureAdminUserParams) (bool, error) {
		err := s.State.MongoSession().DB("admin").AddUser(p.User, p.Password, false)
		c.Assert(err, jc.ErrorIsNil)
		return true, nil
	})

	stateOpened := make(chan eitherState, 1)
	s.AgentSuite.PatchValue(&reportOpenedState, func(st eitherState) {
		select {
		case stateOpened <- st:
		default:
		}
	})

	// Start the machine agent, and wait for state to be opened.
	a := s.newAgent(c, m)
	done := make(chan error)
	go func() { done <- a.Run(nil) }()
	defer a.Stop() // in case of failure
	select {
	case st := <-stateOpened:
		c.Assert(st, gc.NotNil)
	case <-time.After(coretesting.LongWait):
		c.Fatalf("state not opened")
	}
	s.waitStopped(c, state.JobManageEnviron, a, done)
	c.Assert(s.fakeEnsureMongo.EnsureCount, gc.Equals, 1)
	c.Assert(s.fakeEnsureMongo.InitiateCount, gc.Equals, 1)
}

func (s *MachineSuite) TestMachineAgentSetsPrepareRestore(c *gc.C) {
	// Start the machine agent.
	m, _, _ := s.primeAgent(c, version.Current, state.JobHostUnits)
	a := s.newAgent(c, m)
	go func() { c.Check(a.Run(nil), jc.ErrorIsNil) }()
	defer func() { c.Check(a.Stop(), jc.ErrorIsNil) }()
	c.Check(a.IsRestorePreparing(), jc.IsFalse)
	c.Check(a.IsRestoreRunning(), jc.IsFalse)
	err := a.PrepareRestore()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(a.IsRestorePreparing(), jc.IsTrue)
	c.Assert(a.IsRestoreRunning(), jc.IsFalse)
	err = a.PrepareRestore()
	c.Assert(err, gc.ErrorMatches, "already in restore mode")
}

func (s *MachineSuite) TestMachineAgentSetsRestoreInProgress(c *gc.C) {
	// Start the machine agent.
	m, _, _ := s.primeAgent(c, version.Current, state.JobHostUnits)
	a := s.newAgent(c, m)
	go func() { c.Check(a.Run(nil), jc.ErrorIsNil) }()
	defer func() { c.Check(a.Stop(), jc.ErrorIsNil) }()
	c.Check(a.IsRestorePreparing(), jc.IsFalse)
	c.Check(a.IsRestoreRunning(), jc.IsFalse)
	err := a.PrepareRestore()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(a.IsRestorePreparing(), jc.IsTrue)
	err = a.BeginRestore()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(a.IsRestoreRunning(), jc.IsTrue)
	err = a.BeginRestore()
	c.Assert(err, gc.ErrorMatches, "already restoring")
}

func (s *MachineSuite) TestMachineAgentRestoreRequiresPrepare(c *gc.C) {
	// Start the machine agent.
	m, _, _ := s.primeAgent(c, version.Current, state.JobHostUnits)
	a := s.newAgent(c, m)
	go func() { c.Check(a.Run(nil), jc.ErrorIsNil) }()
	defer func() { c.Check(a.Stop(), jc.ErrorIsNil) }()
	c.Check(a.IsRestorePreparing(), jc.IsFalse)
	c.Check(a.IsRestoreRunning(), jc.IsFalse)
	err := a.BeginRestore()
	c.Assert(err, gc.ErrorMatches, "not in restore mode, cannot begin restoration")
	c.Assert(a.IsRestoreRunning(), jc.IsFalse)
}

// MachineWithCharmsSuite provides infrastructure for tests which need to
// work with charms.
type MachineWithCharmsSuite struct {
	commonMachineSuite
	charmtesting.CharmSuite

	machine *state.Machine
}

func (s *MachineWithCharmsSuite) SetUpSuite(c *gc.C) {
	s.commonMachineSuite.SetUpSuite(c)
	s.CharmSuite.SetUpSuite(c, &s.commonMachineSuite.JujuConnSuite)
}

func (s *MachineWithCharmsSuite) TearDownSuite(c *gc.C) {
	s.commonMachineSuite.TearDownSuite(c)
	s.CharmSuite.TearDownSuite(c)
}

func (s *MachineWithCharmsSuite) SetUpTest(c *gc.C) {
	s.commonMachineSuite.SetUpTest(c)
	s.CharmSuite.SetUpTest(c)
}

func (s *MachineWithCharmsSuite) TearDownTest(c *gc.C) {
	s.commonMachineSuite.TearDownTest(c)
	s.CharmSuite.TearDownTest(c)
}

func (s *MachineWithCharmsSuite) TestManageEnvironRunsCharmRevisionUpdater(c *gc.C) {
	m, _, _ := s.primeAgent(c, version.Current, state.JobManageEnviron)

	s.SetupScenario(c)

	a := s.newAgent(c, m)
	go func() {
		c.Check(a.Run(nil), jc.ErrorIsNil)
	}()
	defer func() { c.Check(a.Stop(), jc.ErrorIsNil) }()

	checkRevision := func() bool {
		curl := charm.MustParseURL("cs:quantal/mysql")
		placeholder, err := s.State.LatestPlaceholderCharm(curl)
		return err == nil && placeholder.String() == curl.WithRevision(23).String()
	}
	success := false
	for attempt := coretesting.LongAttempt.Start(); attempt.Next(); {
		if success = checkRevision(); success {
			break
		}
	}
	c.Assert(success, jc.IsTrue)
}

type mongoSuite struct {
	coretesting.BaseSuite
}

func (s *mongoSuite) TestStateWorkerDialSetsWriteMajority(c *gc.C) {
	s.testStateWorkerDialSetsWriteMajority(c, true)
}

func (s *mongoSuite) TestStateWorkerDialDoesNotSetWriteMajorityWithoutReplsetConfig(c *gc.C) {
	s.testStateWorkerDialSetsWriteMajority(c, false)
}

func (s *mongoSuite) testStateWorkerDialSetsWriteMajority(c *gc.C, configureReplset bool) {
	inst := gitjujutesting.MgoInstance{
		EnableJournal: true,
		Params:        []string{"--replSet", "juju"},
	}
	err := inst.Start(coretesting.Certs)
	c.Assert(err, jc.ErrorIsNil)
	defer inst.Destroy()

	var expectedWMode string
	dialOpts := stateWorkerDialOpts
	if configureReplset {
		info := inst.DialInfo()
		args := peergrouper.InitiateMongoParams{
			DialInfo:       info,
			MemberHostPort: inst.Addr(),
		}
		err = peergrouper.MaybeInitiateMongoServer(args)
		c.Assert(err, jc.ErrorIsNil)
		expectedWMode = "majority"
	} else {
		dialOpts.Direct = true
	}

	mongoInfo := mongo.Info{
		Addrs:  []string{inst.Addr()},
		CACert: coretesting.CACert,
	}
	session, err := mongo.DialWithInfo(mongoInfo, dialOpts)
	c.Assert(err, jc.ErrorIsNil)
	defer session.Close()

	safe := session.Safe()
	c.Assert(safe, gc.NotNil)
	c.Assert(safe.WMode, gc.Equals, expectedWMode)
	c.Assert(safe.J, jc.IsTrue) // always enabled
}

type shouldWriteProxyFilesSuite struct {
	coretesting.BaseSuite
}

var _ = gc.Suite(&shouldWriteProxyFilesSuite{})

func (s *shouldWriteProxyFilesSuite) TestAll(c *gc.C) {
	tests := []struct {
		description  string
		providerType string
		machineId    string
		expect       bool
	}{{
		description:  "local provider machine 0 must not write",
		providerType: "local",
		machineId:    "0",
		expect:       false,
	}, {
		description:  "local provider other machine must write 1",
		providerType: "local",
		machineId:    "0/kvm/0",
		expect:       true,
	}, {
		description:  "local provider other machine must write 2",
		providerType: "local",
		machineId:    "123",
		expect:       true,
	}, {
		description:  "other provider machine 0 must write",
		providerType: "anything",
		machineId:    "0",
		expect:       true,
	}, {
		description:  "other provider other machine must write 1",
		providerType: "dummy",
		machineId:    "0/kvm/0",
		expect:       true,
	}, {
		description:  "other provider other machine must write 2",
		providerType: "blahblahblah",
		machineId:    "123",
		expect:       true,
	}}
	for i, test := range tests {
		c.Logf("test %d: %s", i, test.description)
		mockConf := &mockAgentConfig{
			providerType: test.providerType,
			tag:          names.NewMachineTag(test.machineId),
		}
		c.Check(shouldWriteProxyFiles(mockConf), gc.Equals, test.expect)
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
	mu             sync.Mutex
	startedWorkers set.Strings
}

func (r *singularRunnerRecord) newSingularRunner(runner worker.Runner, conn singular.Conn) (worker.Runner, error) {
	sr, err := singular.New(runner, conn)
	if err != nil {
		return nil, err
	}
	return &fakeSingularRunner{
		Runner: sr,
		record: r,
	}, nil
}

// started returns the names of all singular-started workers.
func (r *singularRunnerRecord) started() []string {
	return r.startedWorkers.SortedValues()
}

type fakeSingularRunner struct {
	worker.Runner
	record *singularRunnerRecord
}

func (r *fakeSingularRunner) StartWorker(name string, start func() (worker.Worker, error)) error {
	r.record.mu.Lock()
	defer r.record.mu.Unlock()
	r.record.startedWorkers.Add(name)
	return r.Runner.StartWorker(name, start)
}

func newDummyWorker() worker.Worker {
	return worker.NewSimpleWorker(func(stop <-chan struct{}) error {
		<-stop
		return nil
	})
}

type mockMetricAPI struct {
	cleanUpCalled chan struct{}
	sendCalled    chan struct{}
}

func (m *mockMetricAPI) CleanupOldMetrics() error {
	go func() {
		m.cleanUpCalled <- struct{}{}
	}()
	return nil
}
func (m *mockMetricAPI) SendMetrics() error {
	go func() {
		m.sendCalled <- struct{}{}
	}()
	return nil
}

func (m *mockMetricAPI) SendCalled() <-chan struct{} {
	return m.sendCalled
}

func (m *mockMetricAPI) CleanupCalled() <-chan struct{} {
	return m.cleanUpCalled
}

func mkdtemp(prefix string) string {
	d, err := ioutil.TempDir("", prefix)
	if err != nil {
		panic(err)
	}
	return d
}

func mktemp(prefix string, content string) string {
	f, err := ioutil.TempFile("", prefix)
	if err != nil {
		panic(err)
	}
	_, err = f.WriteString(content)
	if err != nil {
		panic(err)
	}
	f.Close()
	return f.Name()
}
