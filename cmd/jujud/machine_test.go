// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"path/filepath"
	"reflect"
	"time"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/agent"
	"launchpad.net/juju-core/charm"
	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/container/lxc"
	envtesting "launchpad.net/juju-core/environs/testing"
	"launchpad.net/juju-core/errors"
	"launchpad.net/juju-core/names"
	"launchpad.net/juju-core/provider/dummy"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api"
	apideployer "launchpad.net/juju-core/state/api/deployer"
	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/state/watcher"
	"launchpad.net/juju-core/testing"
	"launchpad.net/juju-core/testing/checkers"
	"launchpad.net/juju-core/tools"
	"launchpad.net/juju-core/version"
	"launchpad.net/juju-core/worker"
	"launchpad.net/juju-core/worker/deployer"
)

type MachineSuite struct {
	agentSuite
	lxc.TestSuite
	oldCacheDir string
}

var _ = gc.Suite(&MachineSuite{})

func (s *MachineSuite) SetUpSuite(c *gc.C) {
	s.agentSuite.SetUpSuite(c)
	s.TestSuite.SetUpSuite(c)
	s.oldCacheDir = charm.CacheDir
}

func (s *MachineSuite) TearDownSuite(c *gc.C) {
	charm.CacheDir = s.oldCacheDir
	s.TestSuite.TearDownSuite(c)
	s.agentSuite.TearDownSuite(c)
}

func (s *MachineSuite) SetUpTest(c *gc.C) {
	s.agentSuite.SetUpTest(c)
	s.TestSuite.SetUpTest(c)
	newRunner = newMockRunner
}

func (s *MachineSuite) TearDownTest(c *gc.C) {
	s.TestSuite.TearDownTest(c)
	s.agentSuite.TearDownTest(c)
}

const initialMachinePassword = "machine-password"

// primeAgent adds a new Machine to run the given jobs, and sets up the
// machine agent's directory.  It returns the new machine, the
// agent's configuration and the tools currently running.
func (s *MachineSuite) primeAgent(c *gc.C, jobs ...state.MachineJob) (m *state.Machine, config agent.Config, tools *tools.Tools) {
	m, err := s.State.InjectMachine(&state.AddMachineParams{
		Series:     "series",
		InstanceId: "ardbeg-0",
		Nonce:      state.BootstrapNonce,
		Jobs:       jobs,
	})
	c.Assert(err, gc.IsNil)
	err = m.SetMongoPassword(initialMachinePassword)
	c.Assert(err, gc.IsNil)
	err = m.SetPassword(initialMachinePassword)
	c.Assert(err, gc.IsNil)
	tag := names.MachineTag(m.Id())
	if m.IsStateServer() {
		config, tools = s.agentSuite.primeStateAgent(c, tag, initialMachinePassword)
	} else {
		config, tools = s.agentSuite.primeAgent(c, tag, initialMachinePassword)
	}
	err = config.Write()
	c.Assert(err, gc.IsNil)
	return m, config, tools
}

// newAgent returns a new MachineAgent instance
func (s *MachineSuite) newAgent(c *gc.C, m *state.Machine) *MachineAgent {
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
	m, _, _ := s.primeAgent(c, state.JobHostUnits)
	err := s.newAgent(c, m).Run(nil)
	c.Assert(err, gc.ErrorMatches, "some error")
}

func (s *MachineSuite) TestRunStop(c *gc.C) {
	m, ac, _ := s.primeAgent(c, state.JobHostUnits)
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
	m, _, _ := s.primeAgent(c, state.JobHostUnits, state.JobManageState)
	err := m.EnsureDead()
	c.Assert(err, gc.IsNil)
	a := s.newAgent(c, m)
	err = runWithTimeout(a)
	c.Assert(err, gc.IsNil)

	// try again with the machine removed.
	err = m.Remove()
	c.Assert(err, gc.IsNil)
	a = s.newAgent(c, m)
	err = runWithTimeout(a)
	c.Assert(err, gc.IsNil)
}

func (s *MachineSuite) TestDyingMachine(c *gc.C) {
	c.Skip("Disabled as breaks test isolation somehow, see lp:1206195")
	m, _, _ := s.primeAgent(c, state.JobHostUnits)
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
	m, _, _ := s.primeAgent(c, state.JobHostUnits)
	a := s.newAgent(c, m)
	ctx, reset := patchDeployContext(c, s.BackingState)
	defer reset()
	go func() { c.Check(a.Run(nil), gc.IsNil) }()
	defer func() { c.Check(a.Stop(), gc.IsNil) }()

	// check that unassigned units don't trigger any deployments.
	svc, err := s.State.AddService("wordpress", s.AddTestingCharm(c, "wordpress"))
	c.Assert(err, gc.IsNil)
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
	err = u0.SetStatus(params.StatusStarted, "")
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
	for attempt := testing.LongAttempt.Start(); attempt.Next(); {
		err := u0.Refresh()
		if err == nil && attempt.HasNext() {
			continue
		}
		c.Assert(err, checkers.Satisfies, errors.IsNotFoundError)
	}

	// short-circuit-remove u1 after it's been deployed; check it's recalled
	// and removed from state.
	err = u1.Destroy()
	c.Assert(err, gc.IsNil)
	err = u1.Refresh()
	c.Assert(err, checkers.Satisfies, errors.IsNotFoundError)
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

func (s *MachineSuite) TestManageEnviron(c *gc.C) {
	usefulVersion := version.Current
	usefulVersion.Series = "series" // to match the charm created below
	envtesting.UploadFakeToolsVersion(c, s.Conn.Environ.Storage(), usefulVersion)
	m, _, _ := s.primeAgent(c, state.JobManageEnviron)
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
	svc, err := s.State.AddService("test-service", charm)
	c.Assert(err, gc.IsNil)
	err = svc.SetExposed()
	c.Assert(err, gc.IsNil)
	units, err := s.Conn.AddUnits(svc, 1, "")
	c.Assert(err, gc.IsNil)
	c.Check(opRecvTimeout(c, s.State, op, dummy.OpStartInstance{}), gc.NotNil)

	// Wait for the instance id to show up in the state.
	id1, err := units[0].AssignedMachineId()
	c.Assert(err, gc.IsNil)
	m1, err := s.State.Machine(id1)
	c.Assert(err, gc.IsNil)
	w := m1.Watch()
	defer w.Stop()
	for _ = range w.Changes() {
		err = m1.Refresh()
		c.Assert(err, gc.IsNil)
		if _, err := m1.InstanceId(); err == nil {
			break
		} else {
			c.Check(err, gc.FitsTypeOf, (*state.NotProvisionedError)(nil))
		}
	}
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

func (s *MachineSuite) TestUpgrade(c *gc.C) {
	m, _, currentTools := s.primeAgent(c, state.JobManageState, state.JobManageEnviron, state.JobHostUnits)
	a := s.newAgent(c, m)
	s.testUpgrade(c, a, currentTools)
}

var fastDialOpts = api.DialOpts{
	Timeout:    testing.LongWait,
	RetryDelay: testing.ShortWait,
}

func (s *MachineSuite) assertJobWithState(c *gc.C, job state.MachineJob, test func(agent.Config, *state.State)) {
	stm, conf, _ := s.primeAgent(c, job)
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
	case <-time.After(testing.LongWait):
		c.Fatalf("state not opened")
	}

	err := a.Stop()
	if job == state.JobManageState {
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

// TODO(jam): 2013-09-02 http://pad.lv/1219661
// This test has been failing regularly on the Bot. Until someone fixes it so
// it doesn't crash, it isn't worth having as we can't tell when someone
// actually breaks something.
func (s *MachineSuite) TestManageStateServesAPI(c *gc.C) {
	c.Skip("does not pass reliably on the bot (http://pad.lv/1219661")
	s.assertJobWithState(c, state.JobManageState, func(conf agent.Config, agentState *state.State) {
		st, _, err := conf.OpenAPI(fastDialOpts)
		c.Assert(err, gc.IsNil)
		defer st.Close()
		m, err := st.Machiner().Machine(conf.Tag())
		c.Assert(err, gc.IsNil)
		c.Assert(m.Life(), gc.Equals, params.Alive)
	})
}

func (s *MachineSuite) TestManageStateRunsCleaner(c *gc.C) {
	s.assertJobWithState(c, state.JobManageState, func(conf agent.Config, agentState *state.State) {
		// Create a service and unit, and destroy the service.
		service, err := s.State.AddService("wordpress", s.AddTestingCharm(c, "wordpress"))
		c.Assert(err, gc.IsNil)
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
		timeout := time.After(testing.LongWait)
		for done := false; !done; {
			select {
			case <-timeout:
				c.Fatalf("unit not cleaned up")
			case <-time.After(testing.ShortWait):
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

func (s *MachineSuite) TestManageStateRunsMinUnitsWorker(c *gc.C) {
	s.assertJobWithState(c, state.JobManageState, func(conf agent.Config, agentState *state.State) {
		// Ensure that the MinUnits worker is alive by doing a simple check
		// that it responds to state changes: add a service, set its minimum
		// number of units to one, wait for the worker to add the missing unit.
		service, err := s.State.AddService("wordpress", s.AddTestingCharm(c, "wordpress"))
		c.Assert(err, gc.IsNil)
		err = service.SetMinUnits(1)
		c.Assert(err, gc.IsNil)
		w := service.Watch()
		defer w.Stop()

		// Trigger a sync on the state used by the agent, and wait for the unit
		// to be created.
		agentState.StartSync()
		timeout := time.After(testing.LongWait)
		for {
			select {
			case <-timeout:
				c.Fatalf("unit not created")
			case <-time.After(testing.ShortWait):
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

func (s *MachineSuite) TestOpenAPIState(c *gc.C) {
	m, _, _ := s.primeAgent(c, state.JobHostUnits)
	s.testOpenAPIState(c, m, s.newAgent(c, m), initialMachinePassword)
}

type mockRunner struct {
	*worker.Runner
}

func newMockRunner(isFatal func(error) bool, moreImportant func(e0, e1 error) bool) workerRunner {
	return &mockRunner{worker.NewRunner(isFatal, moreImportant)}
}
