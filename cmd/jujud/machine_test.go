package main

import (
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/dummy"
	"launchpad.net/juju-core/juju/testing"
	"launchpad.net/juju-core/state"
	"os"
	"reflect"
	"time"
)

type MachineSuite struct {
	testing.JujuConnSuite
}

var _ = Suite(&MachineSuite{})

func (s *MachineSuite) TestParseSuccess(c *C) {
	create := func() (cmd.Command, *AgentConf) {
		a := &MachineAgent{}
		return a, &a.Conf
	}
	a := CheckAgentCommand(c, create, []string{"--machine-id", "42"}, flagAll)
	c.Assert(a.(*MachineAgent).MachineId, Equals, 42)
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

func (s *MachineSuite) newAgent(c *C, mid int) *MachineAgent {
	a := &MachineAgent{
		Conf: AgentConf{
			DataDir:         c.MkDir(),
			StateInfo:       *s.StateInfo(c),
			InitialPassword: "machine-password",
		},
		MachineId: mid,
	}
	err := os.MkdirAll(environs.AgentDir(a.Conf.DataDir, state.MachineEntityName(mid)), 0777)
	c.Assert(err, IsNil)
	return a
}

func (s *MachineSuite) TestRunInvalidMachineId(c *C) {
	c.Skip("agents don't yet distinguish between temporary and permanent errors")
	err := s.newAgent(c, 2).Run(nil)
	c.Assert(err, ErrorMatches, "some error")
}

func addMachine(st *state.State, workers ...state.WorkerKind) *state.Machine {
	m, err := st.AddMachine(workers...)
	if err != nil {
		panic(err)
	}
	err = m.SetPassword("machine-password")
	if err != nil {
		panic(err)
	}
	return m
}

func (s *MachineSuite) TestRunStop(c *C) {
	m := addMachine(s.State, state.MachinerWorker)
	a := s.newAgent(c, m.Id())
	done := make(chan error)
	go func() {
		done <- a.Run(nil)
	}()
	err := a.Stop()
	c.Assert(err, IsNil)
	c.Assert(<-done, IsNil)
}

func (s *MachineSuite) TestWithDeadMachine(c *C) {
	m := addMachine(s.State, state.MachinerWorker)
	err := m.EnsureDead()
	c.Assert(err, IsNil)
	a := s.newAgent(c, m.Id())
	err = runWithTimeout(a)
	c.Assert(err, IsNil)

	// try again with the machine removed.
	err = s.State.RemoveMachine(m.Id())
	c.Assert(err, IsNil)
	a = s.newAgent(c, m.Id())
	err = runWithTimeout(a)
	c.Assert(err, IsNil)
}

func (s *MachineSuite) TestProvisionerFirewaller(c *C) {
	m := addMachine(s.State,
		state.MachinerWorker,
		state.ProvisionerWorker,
		state.FirewallerWorker)

	op := make(chan dummy.Operation, 200)
	dummy.Listen(op)

	a := s.newAgent(c, m.Id())
	done := make(chan error)
	go func() {
		done <- a.Run(nil)
	}()

	// Check that the provisioner and firewaller are alive by doing
	// a rudimentary check that it responds to state changes.

	// Add one unit to a service; it should get allocated a machine
	// and then its ports should be opened.
	charm := s.AddTestingCharm(c, "dummy")
	svc, err := s.Conn.AddService("test-service", charm)
	c.Assert(err, IsNil)
	err = svc.SetExposed()
	c.Assert(err, IsNil)
	units, err := s.Conn.AddUnits(svc, 1)
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
		_, err := m1.InstanceId()
		if state.IsNotFound(err) {
			continue
		}
		c.Assert(err, IsNil)
		break
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
		case <-time.After(5 * time.Second):
			c.Fatalf("time out wating for operation")
		}
	}
	panic("not reached")
}

func (s *MachineSuite) TestChangePasswordChanging(c *C) {
	m := addMachine(s.State, state.MachinerWorker)
	a := s.newAgent(c, m.Id())
	dataDir := a.Conf.DataDir
	newAgent := func(initialPassword string) runner {
		return &MachineAgent{
			Conf: AgentConf{
				DataDir:         dataDir,
				StateInfo:       *s.StateInfo(c),
				InitialPassword: initialPassword,
			},
			MachineId: m.Id(),
		}
	}
	testAgentPasswordChanging(&s.JujuConnSuite, c, m, dataDir, newAgent)
}
