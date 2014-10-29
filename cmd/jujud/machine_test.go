// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/names"
	gitjujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/apt"
	"github.com/juju/utils/proxy"
	"github.com/juju/utils/set"
	"github.com/juju/utils/symlink"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v4"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api"
	apideployer "github.com/juju/juju/api/deployer"
	apifirewaller "github.com/juju/juju/api/firewaller"
	apimetricsmanager "github.com/juju/juju/api/metricsmanager"
	apinetworker "github.com/juju/juju/api/networker"
	apirsyslog "github.com/juju/juju/api/rsyslog"
	charmtesting "github.com/juju/juju/apiserver/charmrevisionupdater/testing"
	"github.com/juju/juju/apiserver/params"
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
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/tools"
	"github.com/juju/juju/upgrades"
	"github.com/juju/juju/utils/ssh"
	sshtesting "github.com/juju/juju/utils/ssh/testing"
	"github.com/juju/juju/version"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/authenticationworker"
	"github.com/juju/juju/worker/deployer"
	"github.com/juju/juju/worker/instancepoller"
	"github.com/juju/juju/worker/machineenvironmentworker"
	"github.com/juju/juju/worker/networker"
	"github.com/juju/juju/worker/peergrouper"
	"github.com/juju/juju/worker/rsyslog"
	"github.com/juju/juju/worker/singular"
	"github.com/juju/juju/worker/upgrader"
)

type commonMachineSuite struct {
	agentSuite
	singularRecord *singularRunnerRecord
	lxctesting.TestSuite
	fakeEnsureMongo fakeEnsure
}

func (s *commonMachineSuite) SetUpSuite(c *gc.C) {
	s.agentSuite.SetUpSuite(c)
	s.TestSuite.SetUpSuite(c)
}

func (s *commonMachineSuite) TearDownSuite(c *gc.C) {
	s.TestSuite.TearDownSuite(c)
	s.agentSuite.TearDownSuite(c)
}

func (s *commonMachineSuite) SetUpTest(c *gc.C) {
	s.agentSuite.SetUpTest(c)
	s.TestSuite.SetUpTest(c)
	s.agentSuite.PatchValue(&charm.CacheDir, c.MkDir())
	s.agentSuite.PatchValue(&stateWorkerDialOpts, mongo.DialOpts{})

	os.Remove(jujuRun) // ignore error; may not exist
	// Patch ssh user to avoid touching ~ubuntu/.ssh/authorized_keys.
	s.agentSuite.PatchValue(&authenticationworker.SSHUser, "")

	testpath := c.MkDir()
	s.agentSuite.PatchEnvPathPrepend(testpath)
	// mock out the start method so we can fake install services without sudo
	fakeCmd(filepath.Join(testpath, "start"))
	fakeCmd(filepath.Join(testpath, "stop"))

	s.agentSuite.PatchValue(&upstart.InitDir, c.MkDir())

	s.singularRecord = &singularRunnerRecord{}
	s.agentSuite.PatchValue(&newSingularRunner, s.singularRecord.newSingularRunner)
	s.agentSuite.PatchValue(&peergrouperNew, func(st *state.State) (worker.Worker, error) {
		return newDummyWorker(), nil
	})

	s.fakeEnsureMongo = fakeEnsure{}
	s.agentSuite.PatchValue(&ensureMongoServer, s.fakeEnsureMongo.fakeEnsureMongo)
	s.agentSuite.PatchValue(&maybeInitiateMongoServer, s.fakeEnsureMongo.fakeInitiateMongo)
}

func fakeCmd(path string) {
	err := ioutil.WriteFile(path, []byte("#!/bin/bash --norc\nexit 0"), 0755)
	if err != nil {
		panic(err)
	}
}

func (s *commonMachineSuite) TearDownTest(c *gc.C) {
	s.TestSuite.TearDownTest(c)
	s.agentSuite.TearDownTest(c)
}

// primeAgent adds a new Machine to run the given jobs, and sets up the
// machine agent's directory.  It returns the new machine, the
// agent's configuration and the tools currently running.
func (s *commonMachineSuite) primeAgent(
	c *gc.C, vers version.Binary,
	jobs ...state.MachineJob) (m *state.Machine, agentConfig agent.ConfigSetterWriter, tools *tools.Tools) {

	m, err := s.State.AddMachine("quantal", jobs...)
	c.Assert(err, gc.IsNil)

	pinger, err := m.SetAgentPresence()
	c.Assert(err, gc.IsNil)
	s.AddCleanup(func(c *gc.C) {
		err := pinger.Stop()
		c.Check(err, gc.IsNil)
	})

	return s.configureMachine(c, m.Id(), vers)
}

func (s *commonMachineSuite) configureMachine(c *gc.C, machineId string, vers version.Binary) (
	machine *state.Machine, agentConfig agent.ConfigSetterWriter, tools *tools.Tools,
) {
	m, err := s.State.Machine(machineId)
	c.Assert(err, gc.IsNil)

	// Add a machine and ensure it is provisioned.
	inst, md := jujutesting.AssertStartInstance(c, s.Environ, machineId)
	c.Assert(m.SetProvisioned(inst.Id(), agent.BootstrapNonce, md), gc.IsNil)

	// Add an address for the tests in case the maybeInitiateMongoServer
	// codepath is exercised.
	s.setFakeMachineAddresses(c, m)

	// Set up the new machine.
	err = m.SetAgentVersion(vers)
	c.Assert(err, gc.IsNil)
	err = m.SetPassword(initialMachinePassword)
	c.Assert(err, gc.IsNil)
	tag := m.Tag()
	if m.IsManager() {
		err = m.SetMongoPassword(initialMachinePassword)
		c.Assert(err, gc.IsNil)
		agentConfig, tools = s.agentSuite.primeStateAgent(c, tag, initialMachinePassword, vers)
		info, ok := agentConfig.StateServingInfo()
		c.Assert(ok, jc.IsTrue)
		ssi := paramsStateServingInfoToStateStateServingInfo(info)
		err = s.State.SetStateServingInfo(ssi)
		c.Assert(err, gc.IsNil)
	} else {
		agentConfig, tools = s.agentSuite.primeAgent(c, tag, initialMachinePassword, vers)
	}
	err = agentConfig.Write()
	c.Assert(err, gc.IsNil)
	return m, agentConfig, tools
}

// newAgent returns a new MachineAgent instance
func (s *commonMachineSuite) newAgent(c *gc.C, m *state.Machine) *MachineAgent {
	a := &MachineAgent{}
	s.initAgent(c, a, "--machine-id", m.Id())
	err := a.ReadConfig(m.Tag().String())
	c.Assert(err, gc.IsNil)
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

var _ = gc.Suite(&MachineSuite{})

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
	c.Assert(err, gc.IsNil)
	c.Assert(<-done, gc.IsNil)
	c.Assert(charm.CacheDir, gc.Equals, filepath.Join(ac.DataDir(), "charmcache"))
}

func (s *MachineSuite) TestWithDeadMachine(c *gc.C) {
	m, _, _ := s.primeAgent(c, version.Current, state.JobHostUnits)
	err := m.EnsureDead()
	c.Assert(err, gc.IsNil)
	a := s.newAgent(c, m)
	err = runWithTimeout(a)
	c.Assert(err, gc.IsNil)
}

func (s *MachineSuite) TestWithRemovedMachine(c *gc.C) {
	m, _, _ := s.primeAgent(c, version.Current, state.JobHostUnits)
	err := m.EnsureDead()
	c.Assert(err, gc.IsNil)
	err = m.Remove()
	c.Assert(err, gc.IsNil)
	a := s.newAgent(c, m)
	err = runWithTimeout(a)
	c.Assert(err, gc.IsNil)
}

func (s *MachineSuite) TestDyingMachine(c *gc.C) {
	m, _, _ := s.primeAgent(c, version.Current, state.JobHostUnits)
	a := s.newAgent(c, m)
	done := make(chan error)
	go func() {
		done <- a.Run(nil)
	}()
	defer func() {
		c.Check(a.Stop(), gc.IsNil)
	}()
	// Wait for configuration to be finished
	<-a.WorkersStarted()
	err := m.Destroy()
	c.Assert(err, gc.IsNil)
	select {
	case err := <-done:
		c.Assert(err, gc.IsNil)
	case <-time.After(watcher.Period * 5 / 4):
		// TODO(rog) Fix this so it doesn't wait for so long.
		// https://bugs.github.com/juju/juju/+bug/1163983
		c.Fatalf("timed out waiting for agent to terminate")
	}
	err = m.Refresh()
	c.Assert(err, gc.IsNil)
	c.Assert(m.Life(), gc.Equals, state.Dead)
}

func (s *MachineSuite) TestHostUnits(c *gc.C) {
	m, _, _ := s.primeAgent(c, version.Current, state.JobHostUnits)
	a := s.newAgent(c, m)
	ctx, reset := patchDeployContext(c, s.BackingState)
	defer reset()
	go func() { c.Check(a.Run(nil), gc.IsNil) }()
	defer func() { c.Check(a.Stop(), gc.IsNil) }()

	// check that unassigned units don't trigger any deployments.
	svc := s.AddTestingService(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	u0, err := svc.AddUnit()
	c.Assert(err, gc.IsNil)
	u1, err := svc.AddUnit()
	c.Assert(err, gc.IsNil)

	ctx.waitDeployed(c)

	// assign u0, check it's deployed.
	err = u0.AssignToMachine(m)
	c.Assert(err, gc.IsNil)
	ctx.waitDeployed(c, u0.Name())

	// "start the agent" for u0 to prevent short-circuited remove-on-destroy;
	// check that it's kept deployed despite being Dying.
	err = u0.SetStatus(state.StatusStarted, "", nil)
	c.Assert(err, gc.IsNil)
	err = u0.Destroy()
	c.Assert(err, gc.IsNil)
	ctx.waitDeployed(c, u0.Name())

	// add u1 to the machine, check it's deployed.
	err = u1.AssignToMachine(m)
	c.Assert(err, gc.IsNil)
	ctx.waitDeployed(c, u0.Name(), u1.Name())

	// make u0 dead; check the deployer recalls the unit and removes it from
	// state.
	err = u0.EnsureDead()
	c.Assert(err, gc.IsNil)
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
	c.Assert(err, gc.IsNil)
	err = u1.Refresh()
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
	ctx.waitDeployed(c)
}

func patchDeployContext(c *gc.C, st *state.State) (*fakeContext, func()) {
	ctx := &fakeContext{
		inited: make(chan struct{}),
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
	c.Assert(err, gc.IsNil)
	// Set the addresses in the environ instance as well so that if the instance poller
	// runs it won't overwrite them.
	instId, err := machine.InstanceId()
	c.Assert(err, gc.IsNil)
	insts, err := s.Environ.Instances([]instance.Id{instId})
	c.Assert(err, gc.IsNil)
	dummy.SetInstanceAddresses(insts[0], addrs)
}

func (s *MachineSuite) TestManageEnviron(c *gc.C) {
	usefulVersion := version.Current
	usefulVersion.Series = "quantal" // to match the charm created below
	envtesting.AssertUploadFakeToolsVersions(c, s.DefaultToolsStorage, usefulVersion)
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
	c.Assert(err, gc.IsNil)
	units, err := juju.AddUnits(s.State, svc, 1, "")
	c.Assert(err, gc.IsNil)
	c.Check(opRecvTimeout(c, s.State, op, dummy.OpStartInstance{}), gc.NotNil)

	// Wait for the instance id to show up in the state.
	s.waitProvisioned(c, units[0])
	err = units[0].OpenPort("tcp", 999)
	c.Assert(err, gc.IsNil)

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
	c.Assert(err, gc.IsNil)

	select {
	case err := <-done:
		c.Assert(err, gc.IsNil)
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
	s.agentSuite.PatchValue(&newFirewaller, func(st *apifirewaller.State) (worker.Worker, error) {
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
		c.Check(a.Run(nil), gc.IsNil)
	}()
	select {
	case <-started:
		c.Fatalf("firewaller worker unexpectedly started")
	case <-time.After(coretesting.ShortWait):
	}
}

func (s *MachineSuite) TestManageEnvironRunsInstancePoller(c *gc.C) {
	s.agentSuite.PatchValue(&instancepoller.ShortPoll, 500*time.Millisecond)
	usefulVersion := version.Current
	usefulVersion.Series = "quantal" // to match the charm created below
	envtesting.AssertUploadFakeToolsVersions(c, s.DefaultToolsStorage, usefulVersion)
	m, _, _ := s.primeAgent(c, version.Current, state.JobManageEnviron)
	a := s.newAgent(c, m)
	defer a.Stop()
	go func() {
		c.Check(a.Run(nil), gc.IsNil)
	}()

	// Add one unit to a service;
	charm := s.AddTestingCharm(c, "dummy")
	svc := s.AddTestingService(c, "test-service", charm)
	units, err := juju.AddUnits(s.State, svc, 1, "")
	c.Assert(err, gc.IsNil)

	m, instId := s.waitProvisioned(c, units[0])
	insts, err := s.Environ.Instances([]instance.Id{instId})
	c.Assert(err, gc.IsNil)
	addrs := []network.Address{network.NewAddress("1.2.3.4", network.ScopeUnknown)}
	dummy.SetInstanceAddresses(insts[0], addrs)
	dummy.SetInstanceStatus(insts[0], "running")

	for a := coretesting.LongAttempt.Start(); a.Next(); {
		if !a.HasNext() {
			c.Logf("final machine addresses: %#v", m.Addresses())
			c.Fatalf("timed out waiting for machine to get address")
		}
		err := m.Refresh()
		c.Assert(err, gc.IsNil)
		instStatus, err := m.InstanceStatus()
		c.Assert(err, gc.IsNil)
		if reflect.DeepEqual(m.Addresses(), addrs) && instStatus == "running" {
			break
		}
	}
}

func (s *MachineSuite) TestManageEnvironRunsPeergrouper(c *gc.C) {
	started := make(chan struct{}, 1)
	s.agentSuite.PatchValue(&peergrouperNew, func(st *state.State) (worker.Worker, error) {
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
		c.Check(a.Run(nil), gc.IsNil)
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
	envtesting.AssertUploadFakeToolsVersions(c, s.DefaultToolsStorage, usefulVersion)
	m, _, _ := s.primeAgent(c, version.Current, state.JobManageEnviron)
	calledChan := make(chan struct{}, 1)
	s.agentSuite.PatchValue(&useMultipleCPUs, func() { calledChan <- struct{}{} })
	// Now, start the agent, and observe that a JobManageEnviron agent
	// calls UseMultipleCPUs
	a := s.newAgent(c, m)
	defer a.Stop()
	go func() {
		c.Check(a.Run(nil), gc.IsNil)
	}()
	// Wait for configuration to be finished
	<-a.WorkersStarted()
	select {
	case <-calledChan:
	case <-time.After(coretesting.LongWait):
		c.Errorf("we failed to call UseMultipleCPUs()")
	}
	c.Check(a.Stop(), gc.IsNil)
	// However, an agent that just JobHostUnits doesn't call UseMultipleCPUs
	m2, _, _ := s.primeAgent(c, version.Current, state.JobHostUnits)
	a2 := s.newAgent(c, m2)
	defer a2.Stop()
	go func() {
		c.Check(a2.Run(nil), gc.IsNil)
	}()
	// Wait until all the workers have been started, and then kill everything
	<-a2.workersStarted
	c.Check(a2.Stop(), gc.IsNil)
	select {
	case <-calledChan:
		c.Errorf("we should not have called UseMultipleCPUs()")
	case <-time.After(coretesting.ShortWait):
	}
}

func (s *MachineSuite) waitProvisioned(c *gc.C, unit *state.Unit) (*state.Machine, instance.Id) {
	c.Logf("waiting for unit %q to be provisioned", unit)
	machineId, err := unit.AssignedMachineId()
	c.Assert(err, gc.IsNil)
	m, err := s.State.Machine(machineId)
	c.Assert(err, gc.IsNil)
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
			c.Assert(err, gc.IsNil)
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
	newTools := envtesting.AssertUploadFakeToolsVersions(c, s.DefaultToolsStorage, newVers)[0]
	err := s.State.SetEnvironAgentVersion(newVers.Number)
	c.Assert(err, gc.IsNil)
	err = runWithTimeout(agent)
	envtesting.CheckUpgraderReadyError(c, err, &upgrader.UpgradeReadyError{
		AgentName: tag,
		OldTools:  currentTools.Version,
		NewTools:  newTools.Version,
		DataDir:   s.DataDir(),
	})
}

func (s *MachineSuite) TestUpgradeTarget(c *gc.C) {
	for i, test := range []struct {
		job      params.MachineJob
		master   bool
		expected upgrades.Target
	}{
		{
		// empty gives empty
		}, {
			job:      params.JobManageEnviron,
			expected: upgrades.StateServer,
		}, {
			job:      params.JobManageEnviron,
			master:   true,
			expected: upgrades.DatabaseMaster,
		}, {
			job:      params.JobHostUnits,
			expected: upgrades.HostMachine,
		}, {
			job:      params.JobHostUnits,
			master:   true,
			expected: upgrades.HostMachine,
		},
	} {
		c.Logf("Test %v", i)
		c.Assert(upgradeTarget(test.job, test.master), gc.Equals, test.expected)
	}
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
		c.Assert(err, gc.IsNil)
	}

	select {
	case err := <-done:
		c.Assert(err, gc.IsNil)
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
	s.agentSuite.PatchValue(reportOpened, func(st eitherState) {
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
		c.Assert(err, gc.IsNil)
		defer st.Close()
		m, err := st.Machiner().Machine(conf.Tag().(names.MachineTag))
		c.Assert(err, gc.IsNil)
		c.Assert(m.Life(), gc.Equals, params.Alive)
	})
}

func (s *MachineSuite) assertAgentSetsToolsVersion(c *gc.C, job state.MachineJob) {
	vers := version.Current
	vers.Minor = version.Current.Minor + 1
	m, _, _ := s.primeAgent(c, vers, job)
	a := s.newAgent(c, m)
	go func() { c.Check(a.Run(nil), gc.IsNil) }()
	defer func() { c.Check(a.Stop(), gc.IsNil) }()

	timeout := time.After(coretesting.LongWait)
	for done := false; !done; {
		select {
		case <-timeout:
			c.Fatalf("timeout while waiting for agent version to be set")
		case <-time.After(coretesting.ShortWait):
			err := m.Refresh()
			c.Assert(err, gc.IsNil)
			agentTools, err := m.AgentTools()
			c.Assert(err, gc.IsNil)
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
		c.Assert(err, gc.IsNil)
		err = service.Destroy()
		c.Assert(err, gc.IsNil)

		// Check the unit was not yet removed.
		err = unit.Refresh()
		c.Assert(err, gc.IsNil)
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
					c.Assert(err, gc.IsNil)
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
		c.Assert(err, gc.IsNil)
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
				c.Assert(err, gc.IsNil)
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
	go func() { c.Check(a.Run(nil), gc.IsNil) }()
	defer func() { c.Check(a.Stop(), gc.IsNil) }()

	// Update the keys in the environment.
	sshKey := sshtesting.ValidKeyOne.Key + " user@host"
	err := s.BackingState.UpdateEnvironConfig(map[string]interface{}{"authorized-keys": sshKey}, nil, nil)
	c.Assert(err, gc.IsNil)

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
			c.Assert(err, gc.IsNil)
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
	s.testOpenAPIState(c, m, s.newAgent(c, m), initialMachinePassword)
	s.assertJobWithAPI(c, state.JobHostUnits, func(conf agent.Config, st *api.State) {
		s.assertCannotOpenState(c, conf.Tag(), conf.DataDir())
	})
}

func (s *MachineSuite) TestOpenStateWorksForJobManageEnviron(c *gc.C) {
	s.assertJobWithAPI(c, state.JobManageEnviron, func(conf agent.Config, st *api.State) {
		s.assertCanOpenState(c, conf.Tag(), conf.DataDir())
	})
}

func (s *MachineSuite) TestMachineAgentSymlinkJujuRun(c *gc.C) {
	_, err := os.Stat(jujuRun)
	c.Assert(err, jc.Satisfies, os.IsNotExist)
	s.assertJobWithAPI(c, state.JobManageEnviron, func(conf agent.Config, st *api.State) {
		// juju-run should have been created
		_, err := os.Stat(jujuRun)
		c.Assert(err, gc.IsNil)
	})
}

func (s *MachineSuite) TestMachineAgentSymlinkJujuRunExists(c *gc.C) {
	err := symlink.New("/nowhere/special", jujuRun)
	c.Assert(err, gc.IsNil)
	_, err = os.Stat(jujuRun)
	c.Assert(err, jc.Satisfies, os.IsNotExist)
	s.assertJobWithAPI(c, state.JobManageEnviron, func(conf agent.Config, st *api.State) {
		// juju-run should have been recreated
		_, err := os.Stat(jujuRun)
		c.Assert(err, gc.IsNil)
		link, err := symlink.Read(jujuRun)
		c.Assert(err, gc.IsNil)
		c.Assert(link, gc.Not(gc.Equals), "/nowhere/special")
	})
}

func (s *MachineSuite) TestMachineEnvironWorker(c *gc.C) {
	proxyDir := c.MkDir()
	s.agentSuite.PatchValue(&machineenvironmentworker.ProxyDirectory, proxyDir)
	s.agentSuite.PatchValue(&apt.ConfFile, filepath.Join(proxyDir, "juju-apt-proxy"))

	s.primeAgent(c, version.Current, state.JobHostUnits)
	// Make sure there are some proxy settings to write.
	proxySettings := proxy.Settings{
		Http:  "http proxy",
		Https: "https proxy",
		Ftp:   "ftp proxy",
	}

	updateAttrs := config.ProxyConfigMap(proxySettings)

	err := s.State.UpdateEnvironConfig(updateAttrs, nil, nil)
	c.Assert(err, gc.IsNil)

	s.assertJobWithAPI(c, state.JobHostUnits, func(conf agent.Config, st *api.State) {
		for {
			select {
			case <-time.After(coretesting.LongWait):
				c.Fatalf("timeout while waiting for proxy settings to change")
			case <-time.After(10 * time.Millisecond):
				_, err := os.Stat(apt.ConfFile)
				if os.IsNotExist(err) {
					continue
				}
				c.Assert(err, gc.IsNil)
				return
			}
		}
	})
}

func (s *MachineSuite) TestMachineAgentUninstall(c *gc.C) {
	m, ac, _ := s.primeAgent(c, version.Current, state.JobHostUnits)
	err := m.EnsureDead()
	c.Assert(err, gc.IsNil)
	a := s.newAgent(c, m)
	err = runWithTimeout(a)
	c.Assert(err, gc.IsNil)
	// juju-run should have been removed on termination
	_, err = os.Stat(jujuRun)
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
	s.agentSuite.PatchValue(&newRsyslogConfigWorker, func(_ *apirsyslog.State, _ agent.Config, mode rsyslog.RsyslogMode) (worker.Worker, error) {
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
	go func() { c.Check(a.Run(nil), gc.IsNil) }()
	defer func() { c.Check(a.Stop(), gc.IsNil) }()

	// Update the API addresses.
	updatedServers := [][]network.HostPort{network.AddressesWithPort(
		network.NewAddresses("localhost"), 1234,
	)}
	err := s.BackingState.SetAPIHostPorts(updatedServers)
	c.Assert(err, gc.IsNil)

	// Wait for config to be updated.
	s.BackingState.StartSync()
	for attempt := coretesting.LongAttempt.Start(); attempt.Next(); {
		addrs, err := a.CurrentConfig().APIAddresses()
		c.Assert(err, gc.IsNil)
		if reflect.DeepEqual(addrs, []string{"localhost:1234"}) {
			return
		}
	}
	c.Fatalf("timeout while waiting for agent config to change")
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
		s.agentSuite.PatchValue(&newNetworker, func(
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
		c.Assert(err, gc.IsNil)

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
	c.Assert(err, gc.IsNil)
	err = s.State.MongoSession().DB("admin").RemoveUser(m.Tag().String())
	c.Assert(err, gc.IsNil)

	s.agentSuite.PatchValue(&ensureMongoAdminUser, func(p mongo.EnsureAdminUserParams) (bool, error) {
		err := s.State.MongoSession().DB("admin").AddUser(p.User, p.Password, false)
		c.Assert(err, gc.IsNil)
		return true, nil
	})

	stateOpened := make(chan eitherState, 1)
	s.agentSuite.PatchValue(&reportOpenedState, func(st eitherState) {
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
	c.Assert(s.fakeEnsureMongo.ensureCount, gc.Equals, 1)
	c.Assert(s.fakeEnsureMongo.initiateCount, gc.Equals, 1)
}

func (s *MachineSuite) TestMachineAgentSetsPrepareRestore(c *gc.C) {
	// Start the machine agent.
	m, _, _ := s.primeAgent(c, version.Current, state.JobHostUnits)
	a := s.newAgent(c, m)
	go func() { c.Check(a.Run(nil), gc.IsNil) }()
	defer func() { c.Check(a.Stop(), gc.IsNil) }()
	c.Check(a.IsRestorePreparing(), gc.Equals, false)
	c.Check(a.IsRestoreRunning(), gc.Equals, false)
	err := a.PrepareRestore()
	c.Assert(err, gc.IsNil)
	c.Assert(a.IsRestorePreparing(), gc.Equals, true)
	c.Assert(a.IsRestoreRunning(), gc.Equals, false)
	err = a.PrepareRestore()
	c.Assert(err, gc.ErrorMatches, "already in restore mode")
}

func (s *MachineSuite) TestMachineAgentSetsRestoreInProgress(c *gc.C) {
	// Start the machine agent.
	m, _, _ := s.primeAgent(c, version.Current, state.JobHostUnits)
	a := s.newAgent(c, m)
	go func() { c.Check(a.Run(nil), gc.IsNil) }()
	defer func() { c.Check(a.Stop(), gc.IsNil) }()
	c.Check(a.IsRestorePreparing(), gc.Equals, false)
	c.Check(a.IsRestoreRunning(), gc.Equals, false)
	err := a.PrepareRestore()
	c.Assert(err, gc.IsNil)
	c.Assert(a.IsRestorePreparing(), gc.Equals, true)
	err = a.BeginRestore()
	c.Assert(err, gc.IsNil)
	c.Assert(a.IsRestoreRunning(), gc.Equals, true)
	err = a.BeginRestore()
	c.Assert(err, gc.ErrorMatches, "already restoring")
}

func (s *MachineSuite) TestMachineAgentRestoreRequiresPrepare(c *gc.C) {
	// Start the machine agent.
	m, _, _ := s.primeAgent(c, version.Current, state.JobHostUnits)
	a := s.newAgent(c, m)
	go func() { c.Check(a.Run(nil), gc.IsNil) }()
	defer func() { c.Check(a.Stop(), gc.IsNil) }()
	c.Check(a.IsRestorePreparing(), gc.Equals, false)
	c.Check(a.IsRestoreRunning(), gc.Equals, false)
	err := a.BeginRestore()
	c.Assert(err, gc.ErrorMatches, "not in restore mode, cannot begin restoration")
	c.Assert(a.IsRestoreRunning(), gc.Equals, false)
}

// MachineWithCharmsSuite provides infrastructure for tests which need to
// work with charms.
type MachineWithCharmsSuite struct {
	commonMachineSuite
	charmtesting.CharmSuite

	machine *state.Machine
}

var _ = gc.Suite(&MachineWithCharmsSuite{})

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
		c.Check(a.Run(nil), gc.IsNil)
	}()
	defer func() { c.Check(a.Stop(), gc.IsNil) }()

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
	c.Assert(success, gc.Equals, true)
}

type mongoSuite struct {
	coretesting.BaseSuite
}

var _ = gc.Suite(&mongoSuite{})

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
	c.Assert(err, gc.IsNil)
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
		c.Assert(err, gc.IsNil)
		expectedWMode = "majority"
	} else {
		dialOpts.Direct = true
	}

	mongoInfo := mongo.Info{
		Addrs:  []string{inst.Addr()},
		CACert: coretesting.CACert,
	}
	session, err := mongo.DialWithInfo(mongoInfo, dialOpts)
	c.Assert(err, gc.IsNil)
	defer session.Close()

	safe := session.Safe()
	c.Assert(safe, gc.NotNil)
	c.Assert(safe.WMode, gc.Equals, expectedWMode)
	c.Assert(safe.J, jc.IsTrue) // always enabled
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
