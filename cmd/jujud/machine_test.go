// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"time"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/agent"
	"launchpad.net/juju-core/charm"
	"launchpad.net/juju-core/cmd"
	lxctesting "launchpad.net/juju-core/container/lxc/testing"
	"launchpad.net/juju-core/environs/config"
	envtesting "launchpad.net/juju-core/environs/testing"
	"launchpad.net/juju-core/errors"
	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/juju"
	"launchpad.net/juju-core/juju/osenv"
	jujutesting "launchpad.net/juju-core/juju/testing"
	"launchpad.net/juju-core/names"
	"launchpad.net/juju-core/provider/dummy"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api"
	apideployer "launchpad.net/juju-core/state/api/deployer"
	"launchpad.net/juju-core/state/api/params"
	apirsyslog "launchpad.net/juju-core/state/api/rsyslog"
	charmtesting "launchpad.net/juju-core/state/apiserver/charmrevisionupdater/testing"
	statetesting "launchpad.net/juju-core/state/testing"
	"launchpad.net/juju-core/state/watcher"
	"launchpad.net/juju-core/testing"
	coretesting "launchpad.net/juju-core/testing"
	jc "launchpad.net/juju-core/testing/checkers"
	"launchpad.net/juju-core/testing/testbase"
	"launchpad.net/juju-core/tools"
	"launchpad.net/juju-core/utils"
	"launchpad.net/juju-core/utils/ssh"
	sshtesting "launchpad.net/juju-core/utils/ssh/testing"
	"launchpad.net/juju-core/version"
	"launchpad.net/juju-core/worker"
	"launchpad.net/juju-core/worker/authenticationworker"
	"launchpad.net/juju-core/worker/deployer"
	"launchpad.net/juju-core/worker/instancepoller"
	"launchpad.net/juju-core/worker/machineenvironmentworker"
	"launchpad.net/juju-core/worker/rsyslog"
	"launchpad.net/juju-core/worker/upgrader"
)

type commonMachineSuite struct {
	agentSuite
	lxctesting.TestSuite
}

func (s *commonMachineSuite) SetUpSuite(c *gc.C) {
	s.agentSuite.SetUpSuite(c)
	s.TestSuite.SetUpSuite(c)
	restore := testbase.PatchValue(&charm.CacheDir, c.MkDir())
	s.AddSuiteCleanup(func(*gc.C) { restore() })
}

func (s *commonMachineSuite) TearDownSuite(c *gc.C) {
	s.TestSuite.TearDownSuite(c)
	s.agentSuite.TearDownSuite(c)
}

func (s *commonMachineSuite) SetUpTest(c *gc.C) {
	s.agentSuite.SetUpTest(c)
	s.TestSuite.SetUpTest(c)
	os.Remove(jujuRun) // ignore error; may not exist
	// Fake $HOME, and ssh user to avoid touching ~ubuntu/.ssh/authorized_keys.
	fakeHome := coretesting.MakeEmptyFakeHomeWithoutJuju(c)
	s.AddCleanup(func(*gc.C) { fakeHome.Restore() })
	s.PatchValue(&authenticationworker.SSHUser, "")
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
	jobs ...state.MachineJob) (m *state.Machine, config agent.Config, tools *tools.Tools) {

	// Add a machine and ensure it is provisioned.
	m, err := s.State.AddMachine("quantal", jobs...)
	c.Assert(err, gc.IsNil)
	inst, md := jujutesting.AssertStartInstance(c, s.Conn.Environ, m.Id())
	c.Assert(m.SetProvisioned(inst.Id(), state.BootstrapNonce, md), gc.IsNil)

	// Set up the new machine.
	err = m.SetAgentVersion(vers)
	c.Assert(err, gc.IsNil)
	err = m.SetPassword(initialMachinePassword)
	c.Assert(err, gc.IsNil)
	tag := names.MachineTag(m.Id())
	if m.IsManager() {
		err = m.SetMongoPassword(initialMachinePassword)
		c.Assert(err, gc.IsNil)
		config, tools = s.agentSuite.primeStateAgent(c, tag, initialMachinePassword, vers)
	} else {
		config, tools = s.agentSuite.primeAgent(c, tag, initialMachinePassword, vers)
	}
	err = config.Write()
	c.Assert(err, gc.IsNil)
	return m, config, tools
}

// newAgent returns a new MachineAgent instance
func (s *commonMachineSuite) newAgent(c *gc.C, m *state.Machine) *MachineAgent {
	a := &MachineAgent{}
	s.initAgent(c, a, "--machine-id", m.Id())
	return a
}

func (s *MachineSuite) TestParseSuccess(c *gc.C) {
	create := func() (cmd.Command, *AgentConf) {
		a := &MachineAgent{}
		return a, &a.Conf
	}
	a := CheckAgentCommand(c, create, []string{"--machine-id", "42"})
	c.Assert(a.(*MachineAgent).MachineId, gc.Equals, "42")
}

type MachineSuite struct {
	commonMachineSuite
}

var _ = gc.Suite(&MachineSuite{})

const initialMachinePassword = "machine-password-1234567890"

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
	err := m.Destroy()
	c.Assert(err, gc.IsNil)
	select {
	case err := <-done:
		c.Assert(err, gc.IsNil)
	case <-time.After(watcher.Period * 5 / 4):
		// TODO(rog) Fix this so it doesn't wait for so long.
		// https://bugs.launchpad.net/juju-core/+bug/1163983
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
	err = u0.SetStatus(params.StatusStarted, "", nil)
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
		c.Assert(err, jc.Satisfies, errors.IsNotFoundError)
	}

	// short-circuit-remove u1 after it's been deployed; check it's recalled
	// and removed from state.
	err = u1.Destroy()
	c.Assert(err, gc.IsNil)
	err = u1.Refresh()
	c.Assert(err, jc.Satisfies, errors.IsNotFoundError)
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

func (s *MachineSuite) setFakeMachineAddresses(c *gc.C, machine *state.Machine) {
	addrs := []instance.Address{
		instance.NewAddress("0.1.2.3"),
	}
	err := machine.SetAddresses(addrs)
	c.Assert(err, gc.IsNil)
	// Set the addresses in the environ instance as well so that if the instance poller
	// runs it won't overwrite them.
	instId, err := machine.InstanceId()
	c.Assert(err, gc.IsNil)
	insts, err := s.Conn.Environ.Instances([]instance.Id{instId})
	c.Assert(err, gc.IsNil)
	dummy.SetInstanceAddresses(insts[0], addrs)
}

func (s *MachineSuite) TestManageEnviron(c *gc.C) {
	usefulVersion := version.Current
	usefulVersion.Series = "quantal" // to match the charm created below
	envtesting.AssertUploadFakeToolsVersions(c, s.Conn.Environ.Storage(), usefulVersion)
	m, _, _ := s.primeAgent(c, version.Current, state.JobManageEnviron)
	s.setFakeMachineAddresses(c, m)
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

	err = a.Stop()
	c.Assert(err, gc.IsNil)

	select {
	case err := <-done:
		c.Assert(err, gc.IsNil)
	case <-time.After(5 * time.Second):
		c.Fatalf("timed out waiting for agent to terminate")
	}
}

func (s *MachineSuite) TestManageEnvironRunsInstancePoller(c *gc.C) {
	defer testbase.PatchValue(&instancepoller.ShortPoll, 500*time.Millisecond).Restore()
	usefulVersion := version.Current
	usefulVersion.Series = "quantal" // to match the charm created below
	envtesting.AssertUploadFakeToolsVersions(c, s.Conn.Environ.Storage(), usefulVersion)
	m, _, _ := s.primeAgent(c, version.Current, state.JobManageEnviron)
	s.setFakeMachineAddresses(c, m)
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
	insts, err := s.Conn.Environ.Instances([]instance.Id{instId})
	c.Assert(err, gc.IsNil)
	addrs := []instance.Address{instance.NewAddress("1.2.3.4")}
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
	panic("watcher died")
}

func (s *MachineSuite) testUpgradeRequest(c *gc.C, agent runner, tag string, currentTools *tools.Tools) {
	newVers := version.Current
	newVers.Patch++
	newTools := envtesting.AssertUploadFakeToolsVersions(c, s.Conn.Environ.Storage(), newVers)[0]
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

func (s *MachineSuite) TestUpgradeRequest(c *gc.C) {
	m, _, currentTools := s.primeAgent(c, version.Current, state.JobManageEnviron, state.JobHostUnits)
	a := s.newAgent(c, m)
	s.testUpgradeRequest(c, a, m.Tag(), currentTools)
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
	stm, conf, _ := s.primeAgent(c, version.Current, job)
	a := s.newAgent(c, stm)
	defer a.Stop()

	// All state jobs currently also run an APIWorker, so no
	// need to check for that here, like in assertJobWithState.

	agentAPIs := make(chan *api.State, 1000)
	undo := sendOpenedAPIs(agentAPIs)
	defer undo()

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

func (s *MachineSuite) assertJobWithState(
	c *gc.C,
	job state.MachineJob,
	test func(agent.Config, *state.State),
) {
	paramsJob := job.ToParams()
	if !paramsJob.NeedsState() {
		c.Fatalf("%v does not use state", paramsJob)
	}
	stm, conf, _ := s.primeAgent(c, version.Current, job)
	a := s.newAgent(c, stm)
	defer a.Stop()

	agentStates := make(chan *state.State, 1000)
	undo := sendOpenedStates(agentStates)
	defer undo()

	done := make(chan error)
	go func() {
		done <- a.Run(nil)
	}()

	select {
	case agentState := <-agentStates:
		c.Assert(agentState, gc.NotNil)
		test(conf, agentState)
	case <-time.After(coretesting.LongWait):
		c.Fatalf("state not opened")
	}

	s.waitStopped(c, job, a, done)
}

// TODO(jam): 2013-09-02 http://pad.lv/1219661
// This test has been failing regularly on the Bot. Until someone fixes it so
// it doesn't crash, it isn't worth having as we can't tell when someone
// actually breaks something.
func (s *MachineSuite) TestManageEnvironServesAPI(c *gc.C) {
	c.Skip("does not pass reliably on the bot (http://pad.lv/1219661")
	s.assertJobWithState(c, state.JobManageEnviron, func(conf agent.Config, agentState *state.State) {
		st, _, err := conf.OpenAPI(fastDialOpts)
		c.Assert(err, gc.IsNil)
		defer st.Close()
		m, err := st.Machiner().Machine(conf.Tag())
		c.Assert(err, gc.IsNil)
		c.Assert(m.Life(), gc.Equals, params.Alive)
	})
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
				if errors.IsNotFoundError(err) {
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
	err := statetesting.UpdateConfig(s.BackingState, map[string]interface{}{"authorized-keys": sshKey})
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
	err := os.Symlink("/nowhere/special", jujuRun)
	c.Assert(err, gc.IsNil)
	_, err = os.Stat(jujuRun)
	c.Assert(err, jc.Satisfies, os.IsNotExist)
	s.assertJobWithAPI(c, state.JobManageEnviron, func(conf agent.Config, st *api.State) {
		// juju-run should have been recreated
		_, err := os.Stat(jujuRun)
		c.Assert(err, gc.IsNil)
		link, err := os.Readlink(jujuRun)
		c.Assert(err, gc.IsNil)
		c.Assert(link, gc.Not(gc.Equals), "/nowhere/special")
	})
}

func (s *MachineSuite) TestMachineEnvirnWorker(c *gc.C) {
	proxyDir := c.MkDir()
	s.PatchValue(&machineenvironmentworker.ProxyDirectory, proxyDir)
	s.PatchValue(&utils.AptConfFile, filepath.Join(proxyDir, "juju-apt-proxy"))

	s.primeAgent(c, version.Current, state.JobHostUnits)
	// Make sure there are some proxy settings to write.
	oldConfig, err := s.State.EnvironConfig()
	c.Assert(err, gc.IsNil)

	proxySettings := osenv.ProxySettings{
		Http:  "http proxy",
		Https: "https proxy",
		Ftp:   "ftp proxy",
	}

	envConfig, err := oldConfig.Apply(config.ProxyConfigMap(proxySettings))
	c.Assert(err, gc.IsNil)

	err = s.State.SetEnvironConfig(envConfig, oldConfig)
	c.Assert(err, gc.IsNil)

	s.assertJobWithAPI(c, state.JobHostUnits, func(conf agent.Config, st *api.State) {
		for {
			select {
			case <-time.After(testing.LongWait):
				c.Fatalf("timeout while waiting for proxy settings to change")
			case <-time.After(10 * time.Millisecond):
				_, err := os.Stat(utils.AptConfFile)
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
	s.PatchValue(&newRsyslogConfigWorker, func(_ *apirsyslog.State, _ agent.Config, mode rsyslog.RsyslogMode) (worker.Worker, error) {
		created <- mode
		return worker.NewRunner(isFatal, moreImportant), nil
	})
	s.assertJobWithAPI(c, job, func(conf agent.Config, st *api.State) {
		select {
		case <-time.After(testing.LongWait):
			c.Fatalf("timeout while waiting for rsyslog worker to be created")
		case mode := <-created:
			c.Assert(mode, gc.Equals, expectedMode)
		}
	})
}

// MachineWithCharmsSuite provides infrastructure for tests which need to
// work with charms.
type MachineWithCharmsSuite struct {
	charmtesting.CharmSuite

	machine *state.Machine
}

var _ = gc.Suite(&MachineWithCharmsSuite{})

func (s *MachineWithCharmsSuite) SetUpTest(c *gc.C) {
	s.CharmSuite.SetUpTest(c)

	// Create a state server machine.
	var err error
	s.machine, err = s.State.AddOneMachine(state.MachineTemplate{
		Series:     "quantal",
		InstanceId: "ardbeg-0",
		Nonce:      state.BootstrapNonce,
		Jobs:       []state.MachineJob{state.JobManageEnviron},
	})
	c.Assert(err, gc.IsNil)
	err = s.machine.SetPassword(initialMachinePassword)
	c.Assert(err, gc.IsNil)
	tag := names.MachineTag(s.machine.Id())
	err = s.machine.SetMongoPassword(initialMachinePassword)
	c.Assert(err, gc.IsNil)

	// Set up the agent configuration.
	stateInfo := s.StateInfo(c)
	writeStateAgentConfig(c, stateInfo, s.DataDir(), tag, initialMachinePassword, version.Current)
}

func (s *MachineWithCharmsSuite) TestManageEnvironRunsCharmRevisionUpdater(c *gc.C) {
	s.SetupScenario(c)

	// Start the machine agent.
	a := &MachineAgent{}
	args := []string{"--data-dir", s.DataDir(), "--machine-id", s.machine.Id()}
	err := coretesting.InitCommand(a, args)
	c.Assert(err, gc.IsNil)

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
