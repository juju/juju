// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"fmt"
	"path/filepath"
	"reflect"
	"time"

	. "launchpad.net/gocheck"

	"launchpad.net/juju-core/agent"
	"launchpad.net/juju-core/charm"
	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/constraints"
	"launchpad.net/juju-core/container/lxc"
	"launchpad.net/juju-core/environs/dummy"
	envtesting "launchpad.net/juju-core/environs/testing"
	"launchpad.net/juju-core/errors"
	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api"
	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/state/watcher"
	"launchpad.net/juju-core/testing"
	"launchpad.net/juju-core/testing/checkers"
	"launchpad.net/juju-core/version"
)

type MachineSuite struct {
	agentSuite
	lxc.TestSuite
	oldCacheDir string
}

var _ = Suite(&MachineSuite{})

func (s *MachineSuite) SetUpSuite(c *C) {
	s.agentSuite.SetUpSuite(c)
	s.TestSuite.SetUpSuite(c)
	s.oldCacheDir = charm.CacheDir
}

func (s *MachineSuite) TearDownSuite(c *C) {
	charm.CacheDir = s.oldCacheDir
	s.TestSuite.TearDownSuite(c)
	s.agentSuite.TearDownSuite(c)
}

func (s *MachineSuite) SetUpTest(c *C) {
	s.agentSuite.SetUpTest(c)
	s.TestSuite.SetUpTest(c)
}

func (s *MachineSuite) TearDownTest(c *C) {
	s.TestSuite.TearDownTest(c)
	s.agentSuite.TearDownTest(c)
}

// primeAgent adds a new Machine to run the given jobs, and sets up the
// machine agent's directory.  It returns the new machine, the
// agent's configuration and the tools currently running.
func (s *MachineSuite) primeAgent(c *C, jobs ...state.MachineJob) (*state.Machine, *agent.Conf, *state.Tools) {
	m, err := s.State.InjectMachine("series", constraints.Value{}, "ardbeg-0", instance.HardwareCharacteristics{}, jobs...)
	c.Assert(err, IsNil)
	err = m.SetMongoPassword("machine-password")
	c.Assert(err, IsNil)
	err = m.SetPassword("machine-password")
	c.Assert(err, IsNil)
	conf, tools := s.agentSuite.primeAgent(c, state.MachineTag(m.Id()), "machine-password")
	conf.MachineNonce = state.BootstrapNonce
	conf.APIInfo.Nonce = conf.MachineNonce
	err = conf.Write()
	c.Assert(err, IsNil)
	return m, conf, tools
}

// newAgent returns a new MachineAgent instance
func (s *MachineSuite) newAgent(c *C, m *state.Machine) *MachineAgent {
	a := &MachineAgent{}
	s.initAgent(c, a, "--machine-id", m.Id())
	return a
}

func (s *MachineSuite) TestParseSuccess(c *C) {
	create := func() (cmd.Command, *AgentConf) {
		a := &MachineAgent{}
		return a, &a.Conf
	}
	a := CheckAgentCommand(c, create, []string{"--machine-id", "42"})
	c.Assert(a.(*MachineAgent).MachineId, Equals, "42")
}

func (s *MachineSuite) TestParseNonsense(c *C) {
	for _, args := range [][]string{
		{},
		{"--machine-id", "-4004"},
	} {
		err := ParseAgentCommand(&MachineAgent{}, args)
		c.Assert(err, ErrorMatches, "--machine-id option must be set, and expects a non-negative integer")
	}
}

func (s *MachineSuite) TestParseUnknown(c *C) {
	a := &MachineAgent{}
	err := ParseAgentCommand(a, []string{"--machine-id", "42", "blistering barnacles"})
	c.Assert(err, ErrorMatches, `unrecognized args: \["blistering barnacles"\]`)
}

func (s *MachineSuite) TestRunInvalidMachineId(c *C) {
	c.Skip("agents don't yet distinguish between temporary and permanent errors")
	m, _, _ := s.primeAgent(c, state.JobHostUnits)
	err := s.newAgent(c, m).Run(nil)
	c.Assert(err, ErrorMatches, "some error")
}

func (s *MachineSuite) TestRunStop(c *C) {
	m, ac, _ := s.primeAgent(c, state.JobHostUnits)
	a := s.newAgent(c, m)
	done := make(chan error)
	go func() {
		done <- a.Run(nil)
	}()
	err := a.Stop()
	c.Assert(err, IsNil)
	c.Assert(<-done, IsNil)
	c.Assert(charm.CacheDir, Equals, filepath.Join(ac.DataDir, "charmcache"))
}

func (s *MachineSuite) TestWithDeadMachine(c *C) {
	m, _, _ := s.primeAgent(c, state.JobHostUnits, state.JobManageState)
	err := m.EnsureDead()
	c.Assert(err, IsNil)
	a := s.newAgent(c, m)
	err = runWithTimeout(a)
	c.Assert(err, IsNil)

	// try again with the machine removed.
	err = m.Remove()
	c.Assert(err, IsNil)
	a = s.newAgent(c, m)
	err = runWithTimeout(a)
	c.Assert(err, IsNil)
}

func (s *MachineSuite) TestDyingMachine(c *C) {
	m, _, _ := s.primeAgent(c, state.JobHostUnits)
	a := s.newAgent(c, m)
	done := make(chan error)
	go func() {
		done <- a.Run(nil)
	}()
	defer func() {
		c.Check(a.Stop(), IsNil)
	}()
	err := m.Destroy()
	c.Assert(err, IsNil)
	select {
	case err := <-done:
		c.Assert(err, IsNil)
	case <-time.After(watcher.Period * 5 / 4):
		// TODO(rog) Fix this so it doesn't wait for so long.
		// https://bugs.launchpad.net/juju-core/+bug/1163983
		c.Fatalf("timed out waiting for agent to terminate")
	}
	err = m.Refresh()
	c.Assert(err, IsNil)
	c.Assert(m.Life(), Equals, state.Dead)
}

func (s *MachineSuite) TestHostUnits(c *C) {
	m, conf, _ := s.primeAgent(c, state.JobHostUnits)
	a := s.newAgent(c, m)
	ctx, reset := patchDeployContext(c, conf.StateInfo, conf.DataDir)
	defer reset()
	go func() { c.Check(a.Run(nil), IsNil) }()
	defer func() { c.Check(a.Stop(), IsNil) }()

	// check that unassigned units don't trigger any deployments.
	svc, err := s.State.AddService("wordpress", s.AddTestingCharm(c, "wordpress"))
	c.Assert(err, IsNil)
	u0, err := svc.AddUnit()
	c.Assert(err, IsNil)
	u1, err := svc.AddUnit()
	c.Assert(err, IsNil)
	ctx.waitDeployed(c)

	// assign u0, check it's deployed.
	err = u0.AssignToMachine(m)
	c.Assert(err, IsNil)
	ctx.waitDeployed(c, u0.Name())

	// "start the agent" for u0 to prevent short-circuited remove-on-destroy;
	// check that it's kept deployed despite being Dying.
	err = u0.SetStatus(params.StatusStarted, "")
	c.Assert(err, IsNil)
	err = u0.Destroy()
	c.Assert(err, IsNil)
	ctx.waitDeployed(c, u0.Name())

	// add u1 to the machine, check it's deployed.
	err = u1.AssignToMachine(m)
	c.Assert(err, IsNil)
	ctx.waitDeployed(c, u0.Name(), u1.Name())

	// make u0 dead; check the deployer recalls the unit and removes it from
	// state.
	err = u0.EnsureDead()
	c.Assert(err, IsNil)
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
	c.Assert(err, IsNil)
	err = u1.Refresh()
	c.Assert(err, checkers.Satisfies, errors.IsNotFoundError)
	ctx.waitDeployed(c)
}

func (s *MachineSuite) TestManageEnviron(c *C) {
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
	c.Assert(err, IsNil)
	err = svc.SetExposed()
	c.Assert(err, IsNil)
	units, err := s.Conn.AddUnits(svc, 1, "")
	c.Assert(err, IsNil)
	c.Check(opRecvTimeout(c, s.State, op, dummy.OpStartInstance{}), NotNil)

	// Wait for the instance id to show up in the state.
	id1, err := units[0].AssignedMachineId()
	c.Assert(err, IsNil)
	m1, err := s.State.Machine(id1)
	c.Assert(err, IsNil)
	w := m1.Watch()
	defer w.Stop()
	for _ = range w.Changes() {
		err = m1.Refresh()
		c.Assert(err, IsNil)
		if _, err := m1.InstanceId(); err == nil {
			break
		} else {
			c.Check(err, FitsTypeOf, (*state.NotProvisionedError)(nil))
		}
	}
	err = units[0].OpenPort("tcp", 999)
	c.Assert(err, IsNil)

	c.Check(opRecvTimeout(c, s.State, op, dummy.OpOpenPorts{}), NotNil)

	err = a.Stop()
	c.Assert(err, IsNil)

	select {
	case err := <-done:
		c.Assert(err, IsNil)
	case <-time.After(5 * time.Second):
		c.Fatalf("timed out waiting for agent to terminate")
	}
}

func (s *MachineSuite) TestUpgrade(c *C) {
	m, conf, currentTools := s.primeAgent(c, state.JobManageState, state.JobManageEnviron, state.JobHostUnits)
	addAPIInfo(conf, m)
	err := conf.Write()
	c.Assert(err, IsNil)
	a := s.newAgent(c, m)
	s.testUpgrade(c, a, currentTools)
}

// addAPIInfo adds information to the agent's configuration
// for serving the API.
func addAPIInfo(conf *agent.Conf, m *state.Machine) {
	port := testing.FindTCPPort()
	conf.APIInfo.Addrs = []string{fmt.Sprintf("localhost:%d", port)}
	conf.APIInfo.CACert = []byte(testing.CACert)
	conf.StateServerCert = []byte(testing.ServerCert)
	conf.StateServerKey = []byte(testing.ServerKey)
	conf.APIPort = port
}

var fastDialOpts = api.DialOpts{
	Timeout:    1 * time.Second,
	RetryDelay: 10 * time.Millisecond,
}

func (s *MachineSuite) assertJobWithState(c *C, job state.MachineJob, test func(*agent.Conf, *state.State)) {
	stm, conf, _ := s.primeAgent(c, job)
	addAPIInfo(conf, stm)
	err := conf.Write()
	c.Assert(err, IsNil)
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
		c.Assert(agentState, NotNil)
		test(conf, agentState)
	case <-time.After(testing.LongWait):
		c.Fatalf("state not opened")
	}

	err = a.Stop()
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
		c.Assert(err, IsNil)
	}

	select {
	case err := <-done:
		c.Assert(err, IsNil)
	case <-time.After(5 * time.Second):
		c.Fatalf("timed out waiting for agent to terminate")
	}
}

func (s *MachineSuite) TestManageStateServesAPI(c *C) {
	s.assertJobWithState(c, state.JobManageState, func(conf *agent.Conf, agentState *state.State) {
		st, err := api.Open(conf.APIInfo, fastDialOpts)
		c.Assert(err, IsNil)
		defer st.Close()
		m, err := st.Machiner().Machine(conf.APIInfo.Tag)
		c.Assert(err, IsNil)
		c.Assert(m.Life(), Equals, params.Alive)
	})
}

func (s *MachineSuite) TestManageStateRunsCleaner(c *C) {
	s.assertJobWithState(c, state.JobManageState, func(conf *agent.Conf, agentState *state.State) {
		// Create a service and unit, and destroy the service.
		service, err := s.State.AddService("wordpress", s.AddTestingCharm(c, "wordpress"))
		c.Assert(err, IsNil)
		unit, err := service.AddUnit()
		c.Assert(err, IsNil)
		err = service.Destroy()
		c.Assert(err, IsNil)

		// Check the unit was not yet removed.
		err = unit.Refresh()
		c.Assert(err, IsNil)
		w := unit.Watch()
		defer w.Stop()

		// Trigger a sync on the state used by the agent, and wait
		// for the unit to be removed.
		agentState.Sync()
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
					c.Assert(err, IsNil)
				}
			}
		}
	})
}

var serveAPIWithBadConfTests = []struct {
	change func(c *agent.Conf)
	err    string
}{{
	func(c *agent.Conf) {
		c.StateServerCert = nil
	},
	"configuration does not have state server cert/key",
}, {
	func(c *agent.Conf) {
		c.StateServerKey = nil
	},
	"configuration does not have state server cert/key",
}}

func (s *MachineSuite) TestServeAPIWithBadConf(c *C) {
	m, conf, _ := s.primeAgent(c, state.JobManageState)
	addAPIInfo(conf, m)
	for i, t := range serveAPIWithBadConfTests {
		c.Logf("test %d: %q", i, t.err)
		conf1 := *conf
		t.change(&conf1)
		err := conf1.Write()
		c.Assert(err, IsNil)
		a := s.newAgent(c, m)
		err = runWithTimeout(a)
		c.Assert(err, ErrorMatches, t.err)
		err = refreshConfig(conf)
		c.Assert(err, IsNil)
	}
}

// opRecvTimeout waits for any of the given kinds of operation to
// be received from ops, and times out if not.
func opRecvTimeout(c *C, st *state.State, opc <-chan dummy.Operation, kinds ...dummy.Operation) dummy.Operation {
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
	panic("not reached")
}

func (s *MachineSuite) TestOpenAPIState(c *C) {
	m, _, _ := s.primeAgent(c, state.JobHostUnits)
	s.testOpenAPIState(c, m, s.newAgent(c, m))
}
