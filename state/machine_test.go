// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"sort"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	jujutxn "github.com/juju/txn"
	gc "gopkg.in/check.v1"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"

	"github.com/juju/juju/constraints"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/network"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/testing"
	"github.com/juju/juju/storage/poolmanager"
	"github.com/juju/juju/storage/provider"
	"github.com/juju/juju/storage/provider/registry"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/version"
)

type MachineSuite struct {
	ConnSuite
	machine0 *state.Machine
	machine  *state.Machine
}

var _ = gc.Suite(&MachineSuite{})

func (s *MachineSuite) SetUpTest(c *gc.C) {
	s.ConnSuite.SetUpTest(c)
	s.policy.GetConstraintsValidator = func(*config.Config) (constraints.Validator, error) {
		validator := constraints.NewValidator()
		validator.RegisterConflicts([]string{constraints.InstanceType}, []string{constraints.Mem})
		validator.RegisterUnsupported([]string{constraints.CpuPower})
		return validator, nil
	}
	var err error
	s.machine0, err = s.State.AddMachine("quantal", state.JobManageEnviron)
	c.Assert(err, jc.ErrorIsNil)
	s.machine, err = s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *MachineSuite) TestSetRebootFlagDeadMachine(c *gc.C) {
	err := s.machine.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)

	err = s.machine.SetRebootFlag(true)
	c.Assert(err, gc.Equals, mgo.ErrNotFound)

	rFlag, err := s.machine.GetRebootFlag()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(rFlag, jc.IsFalse)

	err = s.machine.SetRebootFlag(false)
	c.Assert(err, jc.ErrorIsNil)

	action, err := s.machine.ShouldRebootOrShutdown()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(action, gc.Equals, state.ShouldDoNothing)
}

func (s *MachineSuite) TestSetRebootFlagDeadMachineRace(c *gc.C) {
	setFlag := jujutxn.TestHook{
		Before: func() {
			err := s.machine.EnsureDead()
			c.Assert(err, jc.ErrorIsNil)
		},
	}
	defer state.SetTestHooks(c, s.State, setFlag).Check()

	err := s.machine.SetRebootFlag(true)
	c.Assert(err, gc.Equals, mgo.ErrNotFound)
}

func (s *MachineSuite) TestSetRebootFlag(c *gc.C) {
	err := s.machine.SetRebootFlag(true)
	c.Assert(err, jc.ErrorIsNil)

	rebootFlag, err := s.machine.GetRebootFlag()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(rebootFlag, jc.IsTrue)
}

func (s *MachineSuite) TestSetUnsetRebootFlag(c *gc.C) {
	err := s.machine.SetRebootFlag(true)
	c.Assert(err, jc.ErrorIsNil)

	rebootFlag, err := s.machine.GetRebootFlag()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(rebootFlag, jc.IsTrue)

	err = s.machine.SetRebootFlag(false)
	c.Assert(err, jc.ErrorIsNil)

	rebootFlag, err = s.machine.GetRebootFlag()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(rebootFlag, jc.IsFalse)
}

func (s *MachineSuite) TestShouldShutdownOrReboot(c *gc.C) {
	// Add first container.
	c1, err := s.State.AddMachineInsideMachine(state.MachineTemplate{
		Series: "quantal",
		Jobs:   []state.MachineJob{state.JobHostUnits},
	}, s.machine.Id(), instance.LXC)
	c.Assert(err, jc.ErrorIsNil)

	// Add second container.
	c2, err := s.State.AddMachineInsideMachine(state.MachineTemplate{
		Series: "quantal",
		Jobs:   []state.MachineJob{state.JobHostUnits},
	}, c1.Id(), instance.LXC)
	c.Assert(err, jc.ErrorIsNil)

	err = c2.SetRebootFlag(true)
	c.Assert(err, jc.ErrorIsNil)

	rAction, err := s.machine.ShouldRebootOrShutdown()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(rAction, gc.Equals, state.ShouldDoNothing)

	rAction, err = c1.ShouldRebootOrShutdown()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(rAction, gc.Equals, state.ShouldDoNothing)

	rAction, err = c2.ShouldRebootOrShutdown()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(rAction, gc.Equals, state.ShouldReboot)

	// // Reboot happens on the root node
	err = c2.SetRebootFlag(false)
	c.Assert(err, jc.ErrorIsNil)

	err = s.machine.SetRebootFlag(true)
	c.Assert(err, jc.ErrorIsNil)

	rAction, err = s.machine.ShouldRebootOrShutdown()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(rAction, gc.Equals, state.ShouldReboot)

	rAction, err = c1.ShouldRebootOrShutdown()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(rAction, gc.Equals, state.ShouldShutdown)

	rAction, err = c2.ShouldRebootOrShutdown()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(rAction, gc.Equals, state.ShouldShutdown)
}

func (s *MachineSuite) TestContainerDefaults(c *gc.C) {
	c.Assert(string(s.machine.ContainerType()), gc.Equals, "")
	containers, err := s.machine.Containers()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(containers, gc.DeepEquals, []string(nil))
}

func (s *MachineSuite) TestParentId(c *gc.C) {
	parentId, ok := s.machine.ParentId()
	c.Assert(parentId, gc.Equals, "")
	c.Assert(ok, jc.IsFalse)
	container, err := s.State.AddMachineInsideMachine(state.MachineTemplate{
		Series: "quantal",
		Jobs:   []state.MachineJob{state.JobHostUnits},
	}, s.machine.Id(), instance.LXC)
	c.Assert(err, jc.ErrorIsNil)
	parentId, ok = container.ParentId()
	c.Assert(parentId, gc.Equals, s.machine.Id())
	c.Assert(ok, jc.IsTrue)
}

func (s *MachineSuite) TestMachineIsManager(c *gc.C) {
	c.Assert(s.machine0.IsManager(), jc.IsTrue)
	c.Assert(s.machine.IsManager(), jc.IsFalse)
}

func (s *MachineSuite) TestMachineIsManualBootstrap(c *gc.C) {
	cfg, err := s.State.EnvironConfig()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cfg.Type(), gc.Not(gc.Equals), "null")
	c.Assert(s.machine.Id(), gc.Equals, "1")
	manual, err := s.machine0.IsManual()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(manual, jc.IsFalse)
	attrs := map[string]interface{}{"type": "null"}
	err = s.State.UpdateEnvironConfig(attrs, nil, nil)
	c.Assert(err, jc.ErrorIsNil)
	manual, err = s.machine0.IsManual()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(manual, jc.IsTrue)
}

func (s *MachineSuite) TestMachineIsManual(c *gc.C) {
	tests := []struct {
		instanceId instance.Id
		nonce      string
		isManual   bool
	}{
		{instanceId: "x", nonce: "y", isManual: false},
		{instanceId: "manual:", nonce: "y", isManual: false},
		{instanceId: "x", nonce: "manual:", isManual: true},
		{instanceId: "x", nonce: "manual:y", isManual: true},
		{instanceId: "x", nonce: "manual", isManual: false},
	}
	for _, test := range tests {
		m, err := s.State.AddOneMachine(state.MachineTemplate{
			Series:     "quantal",
			Jobs:       []state.MachineJob{state.JobHostUnits},
			InstanceId: test.instanceId,
			Nonce:      test.nonce,
		})
		c.Assert(err, jc.ErrorIsNil)
		isManual, err := m.IsManual()
		c.Assert(isManual, gc.Equals, test.isManual)
	}
}

func (s *MachineSuite) TestMachineIsContainer(c *gc.C) {
	machine, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)

	template := state.MachineTemplate{
		Series: "quantal",
		Jobs:   []state.MachineJob{state.JobHostUnits},
	}
	container, err := s.State.AddMachineInsideMachine(template, machine.Id(), instance.LXC)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(machine.IsContainer(), jc.IsFalse)
	c.Assert(container.IsContainer(), jc.IsTrue)
}

func (s *MachineSuite) TestLifeJobManageEnviron(c *gc.C) {
	// A JobManageEnviron machine must never advance lifecycle.
	m := s.machine0
	err := m.Destroy()
	c.Assert(err, gc.ErrorMatches, "machine 0 is required by the environment")
	err = m.ForceDestroy()
	c.Assert(err, gc.ErrorMatches, "machine 0 is required by the environment")
	err = m.EnsureDead()
	c.Assert(err, gc.ErrorMatches, "machine 0 is required by the environment")
}

func (s *MachineSuite) TestLifeMachineWithContainer(c *gc.C) {
	// A machine hosting a container must not advance lifecycle.
	_, err := s.State.AddMachineInsideMachine(state.MachineTemplate{
		Series: "quantal",
		Jobs:   []state.MachineJob{state.JobHostUnits},
	}, s.machine.Id(), instance.LXC)
	c.Assert(err, jc.ErrorIsNil)
	err = s.machine.Destroy()
	c.Assert(err, gc.FitsTypeOf, &state.HasContainersError{})
	c.Assert(err, gc.ErrorMatches, `machine 1 is hosting containers "1/lxc/0"`)
	err1 := s.machine.EnsureDead()
	c.Assert(err1, gc.DeepEquals, err)
	c.Assert(s.machine.Life(), gc.Equals, state.Alive)
}

func (s *MachineSuite) TestLifeJobHostUnits(c *gc.C) {
	// A machine with an assigned unit must not advance lifecycle.
	svc := s.AddTestingService(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	unit, err := svc.AddUnit()
	c.Assert(err, jc.ErrorIsNil)
	err = unit.AssignToMachine(s.machine)
	c.Assert(err, jc.ErrorIsNil)
	err = s.machine.Destroy()
	c.Assert(err, gc.FitsTypeOf, &state.HasAssignedUnitsError{})
	c.Assert(err, gc.ErrorMatches, `machine 1 has unit "wordpress/0" assigned`)
	err1 := s.machine.EnsureDead()
	c.Assert(err1, gc.DeepEquals, err)
	c.Assert(s.machine.Life(), gc.Equals, state.Alive)

	// Once no unit is assigned, lifecycle can advance.
	err = unit.UnassignFromMachine()
	c.Assert(err, jc.ErrorIsNil)
	err = s.machine.Destroy()
	c.Assert(s.machine.Life(), gc.Equals, state.Dying)
	c.Assert(err, jc.ErrorIsNil)
	err = s.machine.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.machine.Life(), gc.Equals, state.Dead)

	// A machine that has never had units assigned can advance lifecycle.
	m, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	err = m.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(m.Life(), gc.Equals, state.Dying)
	err = m.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(m.Life(), gc.Equals, state.Dead)
}

func (s *MachineSuite) TestDestroyRemovePorts(c *gc.C) {
	svc := s.AddTestingService(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	unit, err := svc.AddUnit()
	c.Assert(err, jc.ErrorIsNil)
	err = unit.AssignToMachine(s.machine)
	c.Assert(err, jc.ErrorIsNil)
	err = unit.OpenPort("tcp", 8080)
	c.Assert(err, jc.ErrorIsNil)
	ports, err := state.GetPorts(s.State, s.machine.Id(), network.DefaultPublic)
	c.Assert(ports, gc.NotNil)
	c.Assert(err, jc.ErrorIsNil)
	err = unit.UnassignFromMachine()
	c.Assert(err, jc.ErrorIsNil)
	err = s.machine.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	err = s.machine.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
	err = s.machine.Remove()
	c.Assert(err, jc.ErrorIsNil)
	// once the machine is destroyed, there should be no ports documents present for it
	ports, err = state.GetPorts(s.State, s.machine.Id(), network.DefaultPublic)
	c.Assert(ports, gc.IsNil)
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *MachineSuite) TestDestroyAbort(c *gc.C) {
	defer state.SetBeforeHooks(c, s.State, func() {
		c.Assert(s.machine.Destroy(), gc.IsNil)
	}).Check()
	err := s.machine.Destroy()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *MachineSuite) TestDestroyCancel(c *gc.C) {
	svc := s.AddTestingService(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	unit, err := svc.AddUnit()
	c.Assert(err, jc.ErrorIsNil)

	defer state.SetBeforeHooks(c, s.State, func() {
		c.Assert(unit.AssignToMachine(s.machine), gc.IsNil)
	}).Check()
	err = s.machine.Destroy()
	c.Assert(err, gc.FitsTypeOf, &state.HasAssignedUnitsError{})
}

func (s *MachineSuite) TestDestroyContention(c *gc.C) {
	svc := s.AddTestingService(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	unit, err := svc.AddUnit()
	c.Assert(err, jc.ErrorIsNil)

	perturb := jujutxn.TestHook{
		Before: func() { c.Assert(unit.AssignToMachine(s.machine), gc.IsNil) },
		After:  func() { c.Assert(unit.UnassignFromMachine(), gc.IsNil) },
	}
	defer state.SetTestHooks(c, s.State, perturb, perturb, perturb).Check()

	err = s.machine.Destroy()
	c.Assert(err, gc.ErrorMatches, "machine 1 cannot advance lifecycle: state changing too quickly; try again soon")
}

func (s *MachineSuite) TestRemove(c *gc.C) {
	err := s.machine.Remove()
	c.Assert(err, gc.ErrorMatches, "cannot remove machine 1: machine is not dead")
	err = s.machine.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
	err = s.machine.Remove()
	c.Assert(err, jc.ErrorIsNil)
	err = s.machine.Refresh()
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
	_, err = s.machine.HardwareCharacteristics()
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
	_, err = s.machine.Containers()
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
	networks, err := s.machine.RequestedNetworks()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(networks, gc.HasLen, 0)
	ifaces, err := s.machine.NetworkInterfaces()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ifaces, gc.HasLen, 0)
	err = s.machine.Remove()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *MachineSuite) TestHasVote(c *gc.C) {
	c.Assert(s.machine.HasVote(), jc.IsFalse)

	// Make another machine value so that
	// it won't have the cached HasVote value.
	m, err := s.State.Machine(s.machine.Id())
	c.Assert(err, jc.ErrorIsNil)

	err = s.machine.SetHasVote(true)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.machine.HasVote(), jc.IsTrue)
	c.Assert(m.HasVote(), jc.IsFalse)

	err = m.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(m.HasVote(), jc.IsTrue)

	err = m.SetHasVote(false)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(m.HasVote(), jc.IsFalse)

	c.Assert(s.machine.HasVote(), jc.IsTrue)
	err = s.machine.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.machine.HasVote(), jc.IsFalse)
}

func (s *MachineSuite) TestCannotDestroyMachineWithVote(c *gc.C) {
	err := s.machine.SetHasVote(true)
	c.Assert(err, jc.ErrorIsNil)

	// Make another machine value so that
	// it won't have the cached HasVote value.
	m, err := s.State.Machine(s.machine.Id())
	c.Assert(err, jc.ErrorIsNil)

	err = s.machine.Destroy()
	c.Assert(err, gc.ErrorMatches, "machine "+s.machine.Id()+" is a voting replica set member")

	err = m.Destroy()
	c.Assert(err, gc.ErrorMatches, "machine "+s.machine.Id()+" is a voting replica set member")
}

func (s *MachineSuite) TestRemoveAbort(c *gc.C) {
	err := s.machine.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)

	defer state.SetBeforeHooks(c, s.State, func() {
		c.Assert(s.machine.Remove(), gc.IsNil)
	}).Check()
	err = s.machine.Remove()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *MachineSuite) TestMachineSetAgentPresence(c *gc.C) {
	alive, err := s.machine.AgentPresence()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(alive, jc.IsFalse)

	pinger, err := s.machine.SetAgentPresence()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(pinger, gc.NotNil)
	defer pinger.Stop()

	s.State.StartSync()
	alive, err = s.machine.AgentPresence()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(alive, jc.IsTrue)
}

func (s *MachineSuite) TestTag(c *gc.C) {
	tag := s.machine.MachineTag()
	c.Assert(tag.Kind(), gc.Equals, names.MachineTagKind)
	c.Assert(tag.Id(), gc.Equals, "1")

	// To keep gccgo happy, don't compare an interface with a struct.
	var asTag names.Tag = tag
	c.Assert(s.machine.Tag(), gc.Equals, asTag)
}

func (s *MachineSuite) TestSetMongoPassword(c *gc.C) {
	info := testing.NewMongoInfo()
	st, err := state.Open(info, testing.NewDialOpts(), state.Policy(nil))
	c.Assert(err, jc.ErrorIsNil)
	defer st.Close()
	// Turn on fully-authenticated mode.
	err = st.SetAdminMongoPassword("admin-secret")
	c.Assert(err, jc.ErrorIsNil)
	err = st.MongoSession().DB("admin").Login("admin", "admin-secret")
	c.Assert(err, jc.ErrorIsNil)

	// Set the password for the entity
	ent, err := st.Machine("0")
	c.Assert(err, jc.ErrorIsNil)
	err = ent.SetMongoPassword("foo")
	c.Assert(err, jc.ErrorIsNil)

	// Check that we cannot log in with the wrong password.
	info.Tag = ent.Tag()
	info.Password = "bar"
	err = tryOpenState(info)
	c.Assert(err, jc.Satisfies, errors.IsUnauthorized)

	// Check that we can log in with the correct password.
	info.Password = "foo"
	st1, err := state.Open(info, testing.NewDialOpts(), state.Policy(nil))
	c.Assert(err, jc.ErrorIsNil)
	defer st1.Close()

	// Change the password with an entity derived from the newly
	// opened and authenticated state.
	ent, err = st.Machine("0")
	c.Assert(err, jc.ErrorIsNil)
	err = ent.SetMongoPassword("bar")
	c.Assert(err, jc.ErrorIsNil)

	// Check that we cannot log in with the old password.
	info.Password = "foo"
	err = tryOpenState(info)
	c.Assert(err, jc.Satisfies, errors.IsUnauthorized)

	// Check that we can log in with the correct password.
	info.Password = "bar"
	err = tryOpenState(info)
	c.Assert(err, jc.ErrorIsNil)

	// Check that the administrator can still log in.
	info.Tag, info.Password = nil, "admin-secret"
	err = tryOpenState(info)
	c.Assert(err, jc.ErrorIsNil)

	// Remove the admin password so that the test harness can reset the state.
	err = st.SetAdminMongoPassword("")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *MachineSuite) TestSetPassword(c *gc.C) {
	testSetPassword(c, func() (state.Authenticator, error) {
		return s.State.Machine(s.machine.Id())
	})
}

func (s *MachineSuite) TestSetAgentCompatPassword(c *gc.C) {
	e, err := s.State.Machine(s.machine.Id())
	c.Assert(err, jc.ErrorIsNil)
	testSetAgentCompatPassword(c, e)
}

func (s *MachineSuite) TestMachineWaitAgentPresence(c *gc.C) {
	alive, err := s.machine.AgentPresence()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(alive, jc.IsFalse)

	s.State.StartSync()
	err = s.machine.WaitAgentPresence(coretesting.ShortWait)
	c.Assert(err, gc.ErrorMatches, `waiting for agent of machine 1: still not alive after timeout`)

	pinger, err := s.machine.SetAgentPresence()
	c.Assert(err, jc.ErrorIsNil)

	s.State.StartSync()
	err = s.machine.WaitAgentPresence(coretesting.LongWait)
	c.Assert(err, jc.ErrorIsNil)

	alive, err = s.machine.AgentPresence()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(alive, jc.IsTrue)

	err = pinger.Kill()
	c.Assert(err, jc.ErrorIsNil)

	s.State.StartSync()
	alive, err = s.machine.AgentPresence()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(alive, jc.IsFalse)
}

func (s *MachineSuite) TestRequestedNetworks(c *gc.C) {
	// s.machine is created without requested networks, so check
	// they're empty when we read them.
	networks, err := s.machine.RequestedNetworks()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(networks, gc.HasLen, 0)

	// Now create a machine with networks and read them back.
	machine, err := s.State.AddOneMachine(state.MachineTemplate{
		Series:            "quantal",
		Jobs:              []state.MachineJob{state.JobHostUnits},
		Constraints:       constraints.MustParse("networks=mynet,^private-net,^logging"),
		RequestedNetworks: []string{"net1", "net2"},
	})
	c.Assert(err, jc.ErrorIsNil)
	networks, err = machine.RequestedNetworks()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(networks, jc.DeepEquals, []string{"net1", "net2"})
	cons, err := machine.Constraints()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cons.IncludeNetworks(), jc.DeepEquals, []string{"mynet"})
	c.Assert(cons.ExcludeNetworks(), jc.DeepEquals, []string{"private-net", "logging"})

	// Finally, networks should be removed with the machine.
	err = machine.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
	err = machine.Remove()
	c.Assert(err, jc.ErrorIsNil)
	networks, err = machine.RequestedNetworks()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(networks, gc.HasLen, 0)
}

func addNetworkAndInterface(c *gc.C, st *state.State, machine *state.Machine,
	networkName, providerId, cidr string, vlanTag int, isVirtual bool,
	mac, ifaceName string,
) (*state.Network, *state.NetworkInterface) {
	net, err := st.AddNetwork(state.NetworkInfo{
		Name:       networkName,
		ProviderId: network.Id(providerId),
		CIDR:       cidr,
		VLANTag:    vlanTag,
	})
	c.Assert(err, jc.ErrorIsNil)
	iface, err := machine.AddNetworkInterface(state.NetworkInterfaceInfo{
		MACAddress:    mac,
		InterfaceName: ifaceName,
		NetworkName:   networkName,
		IsVirtual:     isVirtual,
	})
	c.Assert(err, jc.ErrorIsNil)
	return net, iface
}

func (s *MachineSuite) TestNetworks(c *gc.C) {
	// s.machine is created without networks, so check
	// they're empty when we read them.
	nets, err := s.machine.Networks()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(nets, gc.HasLen, 0)

	// Now create a testing machine with requested networks, because
	// Networks() uses them to determine which networks are bound to
	// the machine.
	machine, err := s.State.AddOneMachine(state.MachineTemplate{
		Series:            "quantal",
		Jobs:              []state.MachineJob{state.JobHostUnits},
		RequestedNetworks: []string{"net1", "net2"},
	})
	c.Assert(err, jc.ErrorIsNil)

	net1, _ := addNetworkAndInterface(
		c, s.State, machine,
		"net1", "net1", "0.1.2.0/24", 0, false,
		"aa:bb:cc:dd:ee:f0", "eth0")
	net2, _ := addNetworkAndInterface(
		c, s.State, machine,
		"net2", "net2", "0.2.2.0/24", 0, false,
		"aa:bb:cc:dd:ee:f1", "eth1")

	nets, err = machine.Networks()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(nets, jc.DeepEquals, []*state.Network{net1, net2})
}

func (s *MachineSuite) TestMachineNetworkInterfaces(c *gc.C) {
	// s.machine is created without network interfaces, so check
	// they're empty when we read them.
	ifaces, err := s.machine.NetworkInterfaces()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ifaces, gc.HasLen, 0)

	machine, err := s.State.AddOneMachine(state.MachineTemplate{
		Series:            "quantal",
		Jobs:              []state.MachineJob{state.JobHostUnits},
		RequestedNetworks: []string{"net1", "vlan42", "net2"},
	})
	c.Assert(err, jc.ErrorIsNil)

	// And a few networks and NICs.
	_, iface0 := addNetworkAndInterface(
		c, s.State, machine,
		"net1", "net1", "0.1.2.0/24", 0, false,
		"aa:bb:cc:dd:ee:f0", "eth0")
	_, iface1 := addNetworkAndInterface(
		c, s.State, machine,
		"vlan42", "vlan42", "0.1.2.0/30", 42, true,
		"aa:bb:cc:dd:ee:f1", "eth0.42")
	_, iface2 := addNetworkAndInterface(
		c, s.State, machine,
		"net2", "net2", "0.2.2.0/24", 0, false,
		"aa:bb:cc:dd:ee:f2", "eth1")

	ifaces, err = machine.NetworkInterfaces()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ifaces, jc.DeepEquals, []*state.NetworkInterface{
		iface0, iface1, iface2,
	})

	// Make sure interfaces get removed with the machine.
	err = machine.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
	err = machine.Remove()
	c.Assert(err, jc.ErrorIsNil)
	ifaces, err = machine.NetworkInterfaces()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ifaces, gc.HasLen, 0)
}

var addNetworkInterfaceErrorsTests = []struct {
	args         state.NetworkInterfaceInfo
	beforeAdding func(*gc.C, *state.Machine)
	expectErr    string
}{{
	state.NetworkInterfaceInfo{"", "eth1", "net1", false, false},
	nil,
	`cannot add network interface "eth1" to machine "2": MAC address must be not empty`,
}, {
	state.NetworkInterfaceInfo{"invalid", "eth1", "net1", false, false},
	nil,
	`cannot add network interface "eth1" to machine "2": invalid MAC address: invalid`,
}, {
	state.NetworkInterfaceInfo{"aa:bb:cc:dd:ee:f0", "eth1", "net1", false, false},
	nil,
	`cannot add network interface "eth1" to machine "2": MAC address "aa:bb:cc:dd:ee:f0" on network "net1" already exists`,
}, {
	state.NetworkInterfaceInfo{"aa:bb:cc:dd:ee:ff", "", "net1", false, false},
	nil,
	`cannot add network interface "" to machine "2": interface name must be not empty`,
}, {
	state.NetworkInterfaceInfo{"aa:bb:cc:dd:ee:ff", "eth0", "net1", false, false},
	nil,
	`cannot add network interface "eth0" to machine "2": "eth0" on machine "2" already exists`,
}, {
	state.NetworkInterfaceInfo{"aa:bb:cc:dd:ee:ff", "eth1", "missing", false, false},
	nil,
	`cannot add network interface "eth1" to machine "2": network "missing" not found`,
}, {
	state.NetworkInterfaceInfo{"aa:bb:cc:dd:ee:f1", "eth1", "net1", false, false},
	func(c *gc.C, m *state.Machine) {
		c.Check(m.EnsureDead(), gc.IsNil)
	},
	`cannot add network interface "eth1" to machine "2": machine is not alive`,
}, {
	state.NetworkInterfaceInfo{"aa:bb:cc:dd:ee:f1", "eth1", "net1", false, false},
	func(c *gc.C, m *state.Machine) {
		c.Check(m.Remove(), gc.IsNil)
	},
	`cannot add network interface "eth1" to machine "2": machine 2 not found`,
}}

func (s *MachineSuite) TestAddNetworkInterfaceErrors(c *gc.C) {
	machine, err := s.State.AddOneMachine(state.MachineTemplate{
		Series:            "quantal",
		Jobs:              []state.MachineJob{state.JobHostUnits},
		RequestedNetworks: []string{"net1"},
	})
	c.Assert(err, jc.ErrorIsNil)
	addNetworkAndInterface(
		c, s.State, machine,
		"net1", "provider-net1", "0.1.2.0/24", 0, false,
		"aa:bb:cc:dd:ee:f0", "eth0")
	ifaces, err := machine.NetworkInterfaces()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ifaces, gc.HasLen, 1)

	for i, test := range addNetworkInterfaceErrorsTests {
		c.Logf("test %d: %#v", i, test.args)

		if test.beforeAdding != nil {
			test.beforeAdding(c, machine)
		}

		_, err = machine.AddNetworkInterface(test.args)
		c.Check(err, gc.ErrorMatches, test.expectErr)
		if strings.Contains(test.expectErr, "not found") {
			c.Check(err, jc.Satisfies, errors.IsNotFound)
		}
		if strings.Contains(test.expectErr, "already exists") {
			c.Check(err, jc.Satisfies, errors.IsAlreadyExists)
		}
	}
}

func (s *MachineSuite) TestMachineInstanceIdCorrupt(c *gc.C) {
	machine, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	err = s.machines.Update(
		bson.D{{"_id", state.DocID(s.State, machine.Id())}},
		bson.D{{"$set", bson.D{{"instanceid", bson.D{{"foo", "bar"}}}}}},
	)
	c.Assert(err, jc.ErrorIsNil)

	err = machine.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	iid, err := machine.InstanceId()
	c.Assert(err, jc.Satisfies, errors.IsNotProvisioned)
	c.Assert(iid, gc.Equals, instance.Id(""))
}

func (s *MachineSuite) TestMachineInstanceIdMissing(c *gc.C) {
	iid, err := s.machine.InstanceId()
	c.Assert(err, jc.Satisfies, errors.IsNotProvisioned)
	c.Assert(string(iid), gc.Equals, "")
}

func (s *MachineSuite) TestMachineInstanceIdBlank(c *gc.C) {
	machine, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	err = s.machines.Update(
		bson.D{{"_id", state.DocID(s.State, machine.Id())}},
		bson.D{{"$set", bson.D{{"instanceid", ""}}}},
	)
	c.Assert(err, jc.ErrorIsNil)

	err = machine.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	iid, err := machine.InstanceId()
	c.Assert(err, jc.Satisfies, errors.IsNotProvisioned)
	c.Assert(string(iid), gc.Equals, "")
}

func (s *MachineSuite) TestMachineSetProvisionedUpdatesCharacteristics(c *gc.C) {
	// Before provisioning, there is no hardware characteristics.
	_, err := s.machine.HardwareCharacteristics()
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
	arch := "amd64"
	mem := uint64(4096)
	expected := &instance.HardwareCharacteristics{
		Arch: &arch,
		Mem:  &mem,
	}
	err = s.machine.SetProvisioned("umbrella/0", "fake_nonce", expected)
	c.Assert(err, jc.ErrorIsNil)
	md, err := s.machine.HardwareCharacteristics()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(*md, gc.DeepEquals, *expected)

	// Reload machine and check again.
	err = s.machine.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	md, err = s.machine.HardwareCharacteristics()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(*md, gc.DeepEquals, *expected)
}

func (s *MachineSuite) TestMachineAvailabilityZone(c *gc.C) {
	zone := "a_zone"
	hwc := &instance.HardwareCharacteristics{
		AvailabilityZone: &zone,
	}
	err := s.machine.SetProvisioned("umbrella/0", "fake_nonce", hwc)
	c.Assert(err, jc.ErrorIsNil)

	zone, err = s.machine.AvailabilityZone()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(zone, gc.Equals, "a_zone")
}

func (s *MachineSuite) TestMachineAvailabilityZoneEmpty(c *gc.C) {
	zone := ""
	hwc := &instance.HardwareCharacteristics{
		AvailabilityZone: &zone,
	}
	err := s.machine.SetProvisioned("umbrella/0", "fake_nonce", hwc)
	c.Assert(err, jc.ErrorIsNil)

	zone, err = s.machine.AvailabilityZone()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(zone, gc.Equals, "")
}

func (s *MachineSuite) TestMachineAvailabilityZoneMissing(c *gc.C) {
	zone := "a_zone"
	hwc := &instance.HardwareCharacteristics{}
	err := s.machine.SetProvisioned("umbrella/0", "fake_nonce", hwc)
	c.Assert(err, jc.ErrorIsNil)

	zone, err = s.machine.AvailabilityZone()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(zone, gc.Equals, "")
}

func (s *MachineSuite) TestMachineSetCheckProvisioned(c *gc.C) {
	// Check before provisioning.
	c.Assert(s.machine.CheckProvisioned("fake_nonce"), jc.IsFalse)

	// Either one should not be empty.
	err := s.machine.SetProvisioned("umbrella/0", "", nil)
	c.Assert(err, gc.ErrorMatches, `cannot set instance data for machine "1": instance id and nonce cannot be empty`)
	err = s.machine.SetProvisioned("", "fake_nonce", nil)
	c.Assert(err, gc.ErrorMatches, `cannot set instance data for machine "1": instance id and nonce cannot be empty`)
	err = s.machine.SetProvisioned("", "", nil)
	c.Assert(err, gc.ErrorMatches, `cannot set instance data for machine "1": instance id and nonce cannot be empty`)

	err = s.machine.SetProvisioned("umbrella/0", "fake_nonce", nil)
	c.Assert(err, jc.ErrorIsNil)

	m, err := s.State.Machine(s.machine.Id())
	c.Assert(err, jc.ErrorIsNil)
	id, err := m.InstanceId()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(string(id), gc.Equals, "umbrella/0")
	c.Assert(s.machine.CheckProvisioned("fake_nonce"), jc.IsTrue)
	id, err = s.machine.InstanceId()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(string(id), gc.Equals, "umbrella/0")
	c.Assert(s.machine.CheckProvisioned("fake_nonce"), jc.IsTrue)

	// Try it twice, it should fail.
	err = s.machine.SetProvisioned("doesn't-matter", "phony", nil)
	c.Assert(err, gc.ErrorMatches, `cannot set instance data for machine "1": already set`)

	// Check it with invalid nonce.
	c.Assert(s.machine.CheckProvisioned("not-really"), jc.IsFalse)
}

func (s *MachineSuite) TestMachineSetInstanceInfoFailureDoesNotProvision(c *gc.C) {
	assertNotProvisioned := func() {
		c.Assert(s.machine.CheckProvisioned("fake_nonce"), jc.IsFalse)
	}

	assertNotProvisioned()
	invalidNetworks := []state.NetworkInfo{{Name: ""}}
	err := s.machine.SetInstanceInfo("umbrella/0", "fake_nonce", nil, invalidNetworks, nil, nil, nil)
	c.Assert(err, gc.ErrorMatches, `cannot add network "": name must be not empty`)
	assertNotProvisioned()

	invalidInterfaces := []state.NetworkInterfaceInfo{{MACAddress: ""}}
	err = s.machine.SetInstanceInfo("umbrella/0", "fake_nonce", nil, nil, invalidInterfaces, nil, nil)
	c.Assert(err, gc.ErrorMatches, `cannot add network interface "" to machine "1": MAC address must be not empty`)
	assertNotProvisioned()

	invalidVolumes := map[names.VolumeTag]state.VolumeInfo{names.NewVolumeTag("1065"): state.VolumeInfo{}}
	err = s.machine.SetInstanceInfo("umbrella/0", "fake_nonce", nil, nil, nil, invalidVolumes, nil)
	c.Assert(err, gc.ErrorMatches, `cannot set info for volume \"1065\": volume \"1065\" not found`)
	assertNotProvisioned()

	// TODO(axw) test invalid volume attachment
}

func (s *MachineSuite) addVolume(c *gc.C, params state.VolumeParams) names.VolumeTag {
	op, tag, err := state.AddVolumeOp(s.State, params)
	c.Assert(err, jc.ErrorIsNil)
	err = state.RunTransaction(s.State, []txn.Op{op})
	c.Assert(err, jc.ErrorIsNil)
	return tag
}

func (s *MachineSuite) TestMachineSetInstanceInfoSuccess(c *gc.C) {
	pm := poolmanager.New(state.NewStateSettings(s.State))
	_, err := pm.Create("loop-pool", provider.LoopProviderType, map[string]interface{}{})
	c.Assert(err, jc.ErrorIsNil)
	registry.RegisterEnvironStorageProviders("someprovider", provider.LoopProviderType)

	// Must create the requested block device prior to SetInstanceInfo.
	volumeTag := s.addVolume(c, state.VolumeParams{Size: 1000, Pool: "loop-pool"})

	c.Assert(s.machine.CheckProvisioned("fake_nonce"), jc.IsFalse)
	networks := []state.NetworkInfo{
		{Name: "net1", ProviderId: "net1", CIDR: "0.1.2.0/24", VLANTag: 0},
	}
	interfaces := []state.NetworkInterfaceInfo{
		{MACAddress: "aa:bb:cc:dd:ee:ff", NetworkName: "net1", InterfaceName: "eth0", IsVirtual: false},
	}
	volumes := map[names.VolumeTag]state.VolumeInfo{
		volumeTag: state.VolumeInfo{
			VolumeId: "storage-123",
			Size:     1234,
		},
	}
	err = s.machine.SetInstanceInfo("umbrella/0", "fake_nonce", nil, networks, interfaces, volumes, nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.machine.CheckProvisioned("fake_nonce"), jc.IsTrue)
	network, err := s.State.Network(networks[0].Name)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(network.Name(), gc.Equals, networks[0].Name)
	c.Check(network.ProviderId(), gc.Equals, networks[0].ProviderId)
	c.Check(network.VLANTag(), gc.Equals, networks[0].VLANTag)
	c.Check(network.CIDR(), gc.Equals, networks[0].CIDR)
	ifaces, err := s.machine.NetworkInterfaces()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ifaces, gc.HasLen, 1)
	c.Check(ifaces[0].InterfaceName(), gc.Equals, interfaces[0].InterfaceName)
	c.Check(ifaces[0].NetworkName(), gc.Equals, interfaces[0].NetworkName)
	c.Check(ifaces[0].MACAddress(), gc.Equals, interfaces[0].MACAddress)
	c.Check(ifaces[0].MachineTag(), gc.Equals, s.machine.Tag())
	c.Check(ifaces[0].IsVirtual(), gc.Equals, interfaces[0].IsVirtual)

	volume, err := s.State.Volume(volumeTag)
	c.Assert(err, jc.ErrorIsNil)
	info, err := volume.Info()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(info, gc.Equals, volumes[volumeTag])
}

func (s *MachineSuite) TestMachineSetProvisionedWhenNotAlive(c *gc.C) {
	testWhenDying(c, s.machine, notAliveErr, notAliveErr, func() error {
		return s.machine.SetProvisioned("umbrella/0", "fake_nonce", nil)
	})
}

func (s *MachineSuite) TestMachineSetInstanceStatus(c *gc.C) {
	// Machine needs to be provisioned first.
	err := s.machine.SetProvisioned("umbrella/0", "fake_nonce", nil)
	c.Assert(err, jc.ErrorIsNil)

	err = s.machine.SetInstanceStatus("ALIVE")
	c.Assert(err, jc.ErrorIsNil)

	// Reload machine and check result.
	err = s.machine.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	status, err := s.machine.InstanceStatus()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(status, gc.DeepEquals, "ALIVE")
}

func (s *MachineSuite) TestNotProvisionedMachineSetInstanceStatus(c *gc.C) {
	err := s.machine.SetInstanceStatus("ALIVE")
	c.Assert(err, gc.ErrorMatches, ".* not provisioned")
}

func (s *MachineSuite) TestNotProvisionedMachineInstanceStatus(c *gc.C) {
	_, err := s.machine.InstanceStatus()
	c.Assert(err, jc.Satisfies, errors.IsNotProvisioned)
}

func (s *MachineSuite) TestMachineRefresh(c *gc.C) {
	m0, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	oldTools, _ := m0.AgentTools()
	m1, err := s.State.Machine(m0.Id())
	c.Assert(err, jc.ErrorIsNil)
	err = m0.SetAgentVersion(version.MustParseBinary("0.0.3-quantal-amd64"))
	c.Assert(err, jc.ErrorIsNil)
	newTools, _ := m0.AgentTools()

	m1Tools, _ := m1.AgentTools()
	c.Assert(m1Tools, gc.DeepEquals, oldTools)
	err = m1.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	m1Tools, _ = m1.AgentTools()
	c.Assert(*m1Tools, gc.Equals, *newTools)

	err = m0.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
	err = m0.Remove()
	c.Assert(err, jc.ErrorIsNil)
	err = m0.Refresh()
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *MachineSuite) TestRefreshWhenNotAlive(c *gc.C) {
	// Refresh should work regardless of liveness status.
	testWhenDying(c, s.machine, noErr, noErr, func() error {
		return s.machine.Refresh()
	})
}

func (s *MachineSuite) TestMachinePrincipalUnits(c *gc.C) {
	// Check that Machine.Units and st.UnitsFor work correctly.

	// Make three machines, three services and three units for each service;
	// variously assign units to machines and check that Machine.Units
	// tells us the right thing.

	m1 := s.machine
	m2, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	m3, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)

	dummy := s.AddTestingCharm(c, "dummy")
	logging := s.AddTestingCharm(c, "logging")
	s0 := s.AddTestingService(c, "s0", dummy)
	s1 := s.AddTestingService(c, "s1", dummy)
	s2 := s.AddTestingService(c, "s2", dummy)
	s3 := s.AddTestingService(c, "s3", logging)

	units := make([][]*state.Unit, 4)
	for i, svc := range []*state.Service{s0, s1, s2} {
		units[i] = make([]*state.Unit, 3)
		for j := range units[i] {
			units[i][j], err = svc.AddUnit()
			c.Assert(err, jc.ErrorIsNil)
		}
	}
	// Add the logging units subordinate to the s2 units.
	eps, err := s.State.InferEndpoints("s2", "s3")
	c.Assert(err, jc.ErrorIsNil)
	rel, err := s.State.AddRelation(eps...)
	c.Assert(err, jc.ErrorIsNil)
	for _, u := range units[2] {
		ru, err := rel.Unit(u)
		c.Assert(err, jc.ErrorIsNil)
		err = ru.EnterScope(nil)
		c.Assert(err, jc.ErrorIsNil)
	}
	units[3], err = s3.AllUnits()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(sortedUnitNames(units[3]), jc.DeepEquals, []string{"s3/0", "s3/1", "s3/2"})

	assignments := []struct {
		machine      *state.Machine
		units        []*state.Unit
		subordinates []*state.Unit
	}{
		{m1, []*state.Unit{units[0][0]}, nil},
		{m2, []*state.Unit{units[0][1], units[1][0], units[1][1], units[2][0]}, []*state.Unit{units[3][0]}},
		{m3, []*state.Unit{units[2][2]}, []*state.Unit{units[3][2]}},
	}

	for _, a := range assignments {
		for _, u := range a.units {
			err := u.AssignToMachine(a.machine)
			c.Assert(err, jc.ErrorIsNil)
		}
	}

	for i, a := range assignments {
		c.Logf("test %d", i)
		expect := sortedUnitNames(append(a.units, a.subordinates...))

		// The units can be retrieved from the machine model.
		got, err := a.machine.Units()
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(sortedUnitNames(got), jc.DeepEquals, expect)

		// The units can be retrieved from the machine id.
		got, err = s.State.UnitsFor(a.machine.Id())
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(sortedUnitNames(got), jc.DeepEquals, expect)
	}
}

func sortedUnitNames(units []*state.Unit) []string {
	names := make([]string, len(units))
	for i, u := range units {
		names[i] = u.Name()
	}
	sort.Strings(names)
	return names
}

func (s *MachineSuite) assertMachineDirtyAfterAddingUnit(c *gc.C) (*state.Machine, *state.Service, *state.Unit) {
	m, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(m.Clean(), jc.IsTrue)

	svc := s.AddTestingService(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	unit, err := svc.AddUnit()
	c.Assert(err, jc.ErrorIsNil)
	err = unit.AssignToMachine(m)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(m.Clean(), jc.IsFalse)
	return m, svc, unit
}

func (s *MachineSuite) TestMachineDirtyAfterAddingUnit(c *gc.C) {
	s.assertMachineDirtyAfterAddingUnit(c)
}

func (s *MachineSuite) TestMachineDirtyAfterUnassigningUnit(c *gc.C) {
	m, _, unit := s.assertMachineDirtyAfterAddingUnit(c)
	err := unit.UnassignFromMachine()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(m.Clean(), jc.IsFalse)
}

func (s *MachineSuite) TestMachineDirtyAfterRemovingUnit(c *gc.C) {
	m, svc, unit := s.assertMachineDirtyAfterAddingUnit(c)
	err := unit.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
	err = unit.Remove()
	c.Assert(err, jc.ErrorIsNil)
	err = svc.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(m.Clean(), jc.IsFalse)
}

func (s *MachineSuite) TestWatchMachine(c *gc.C) {
	w := s.machine.Watch()
	defer testing.AssertStop(c, w)

	// Initial event.
	wc := testing.NewNotifyWatcherC(c, s.State, w)
	wc.AssertOneChange()

	// Make one change (to a separate instance), check one event.
	machine, err := s.State.Machine(s.machine.Id())
	c.Assert(err, jc.ErrorIsNil)
	err = machine.SetProvisioned("m-foo", "fake_nonce", nil)
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertOneChange()

	// Make two changes, check one event.
	err = machine.SetAgentVersion(version.MustParseBinary("0.0.3-quantal-amd64"))
	c.Assert(err, jc.ErrorIsNil)
	err = machine.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertOneChange()

	// Stop, check closed.
	testing.AssertStop(c, w)
	wc.AssertClosed()

	// Remove machine, start new watch, check single event.
	err = machine.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
	err = machine.Remove()
	c.Assert(err, jc.ErrorIsNil)
	w = s.machine.Watch()
	defer testing.AssertStop(c, w)
	testing.NewNotifyWatcherC(c, s.State, w).AssertOneChange()
}

func (s *MachineSuite) TestWatchDiesOnStateClose(c *gc.C) {
	// This test is testing logic in watcher.entityWatcher, which
	// is also used by:
	//  Machine.WatchHardwareCharacteristics
	//  Service.Watch
	//  Unit.Watch
	//  State.WatchForEnvironConfigChanges
	//  Unit.WatchConfigSettings
	testWatcherDiesWhenStateCloses(c, func(c *gc.C, st *state.State) waiter {
		m, err := st.Machine(s.machine.Id())
		c.Assert(err, jc.ErrorIsNil)
		w := m.Watch()
		<-w.Changes()
		return w
	})
}

func (s *MachineSuite) TestWatchPrincipalUnits(c *gc.C) {
	// TODO(mjs) - ENVUUID - test with multiple environments with
	// identically named units and ensure there's no leakage.

	// Start a watch on an empty machine; check no units reported.
	w := s.machine.WatchPrincipalUnits()
	defer testing.AssertStop(c, w)
	wc := testing.NewStringsWatcherC(c, s.State, w)
	wc.AssertChange()
	wc.AssertNoChange()

	// Change machine, and create a unit independently; no change.
	err := s.machine.SetProvisioned("cheese", "fake_nonce", nil)
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertNoChange()
	mysql := s.AddTestingService(c, "mysql", s.AddTestingCharm(c, "mysql"))
	mysql0, err := mysql.AddUnit()
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertNoChange()

	// Assign that unit (to a separate machine instance); change detected.
	machine, err := s.State.Machine(s.machine.Id())
	c.Assert(err, jc.ErrorIsNil)
	err = mysql0.AssignToMachine(machine)
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertChange("mysql/0")
	wc.AssertNoChange()

	// Change the unit; no change.
	err = mysql0.SetAgentStatus(state.StatusActive, "", nil)
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertNoChange()

	// Assign another unit and make the first Dying; check both changes detected.
	mysql1, err := mysql.AddUnit()
	c.Assert(err, jc.ErrorIsNil)
	err = mysql1.AssignToMachine(machine)
	c.Assert(err, jc.ErrorIsNil)
	err = mysql0.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertChange("mysql/0", "mysql/1")
	wc.AssertNoChange()

	// Add a subordinate to the Alive unit; no change.
	s.AddTestingService(c, "logging", s.AddTestingCharm(c, "logging"))
	eps, err := s.State.InferEndpoints("mysql", "logging")
	c.Assert(err, jc.ErrorIsNil)
	rel, err := s.State.AddRelation(eps...)
	c.Assert(err, jc.ErrorIsNil)
	mysqlru1, err := rel.Unit(mysql1)
	c.Assert(err, jc.ErrorIsNil)
	err = mysqlru1.EnterScope(nil)
	c.Assert(err, jc.ErrorIsNil)
	logging0, err := s.State.Unit("logging/0")
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertNoChange()

	// Change the subordinate; no change.
	err = logging0.SetAgentStatus(state.StatusActive, "", nil)
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertNoChange()

	// Make the Dying unit Dead; change detected.
	err = mysql0.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertChange("mysql/0")
	wc.AssertNoChange()

	// Stop watcher; check Changes chan closed.
	testing.AssertStop(c, w)
	wc.AssertClosed()

	// Start a fresh watcher; check both principals reported.
	w = s.machine.WatchPrincipalUnits()
	defer testing.AssertStop(c, w)
	wc = testing.NewStringsWatcherC(c, s.State, w)
	wc.AssertChange("mysql/0", "mysql/1")
	wc.AssertNoChange()

	// Remove the Dead unit; no change.
	err = mysql0.Remove()
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertNoChange()

	// Destroy the subordinate; no change.
	err = logging0.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertNoChange()

	// Unassign the unit; check change.
	err = mysql1.UnassignFromMachine()
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertChange("mysql/1")
	wc.AssertNoChange()
}

func (s *MachineSuite) TestWatchPrincipalUnitsDiesOnStateClose(c *gc.C) {
	// This test is testing logic in watcher.unitsWatcher, which
	// is also used by Unit.WatchSubordinateUnits.
	testWatcherDiesWhenStateCloses(c, func(c *gc.C, st *state.State) waiter {
		m, err := st.Machine(s.machine.Id())
		c.Assert(err, jc.ErrorIsNil)
		w := m.WatchPrincipalUnits()
		<-w.Changes()
		return w
	})
}

func (s *MachineSuite) TestWatchUnits(c *gc.C) {
	// Start a watch on an empty machine; check no units reported.
	w := s.machine.WatchUnits()
	defer testing.AssertStop(c, w)
	wc := testing.NewStringsWatcherC(c, s.State, w)
	wc.AssertChange()
	wc.AssertNoChange()

	// Change machine; no change.
	err := s.machine.SetProvisioned("cheese", "fake_nonce", nil)
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertNoChange()

	// Assign a unit (to a separate instance); change detected.
	mysql := s.AddTestingService(c, "mysql", s.AddTestingCharm(c, "mysql"))
	mysql0, err := mysql.AddUnit()
	c.Assert(err, jc.ErrorIsNil)
	machine, err := s.State.Machine(s.machine.Id())
	c.Assert(err, jc.ErrorIsNil)
	err = mysql0.AssignToMachine(machine)
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertChange("mysql/0")
	wc.AssertNoChange()

	// Change the unit; no change.
	err = mysql0.SetAgentStatus(state.StatusActive, "", nil)
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertNoChange()

	// Assign another unit and make the first Dying; check both changes detected.
	mysql1, err := mysql.AddUnit()
	c.Assert(err, jc.ErrorIsNil)
	err = mysql1.AssignToMachine(machine)
	c.Assert(err, jc.ErrorIsNil)
	err = mysql0.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertChange("mysql/0", "mysql/1")
	wc.AssertNoChange()

	// Add a subordinate to the Alive unit; change detected.
	s.AddTestingService(c, "logging", s.AddTestingCharm(c, "logging"))
	eps, err := s.State.InferEndpoints("mysql", "logging")
	c.Assert(err, jc.ErrorIsNil)
	rel, err := s.State.AddRelation(eps...)
	c.Assert(err, jc.ErrorIsNil)
	mysqlru1, err := rel.Unit(mysql1)
	c.Assert(err, jc.ErrorIsNil)
	err = mysqlru1.EnterScope(nil)
	c.Assert(err, jc.ErrorIsNil)
	logging0, err := s.State.Unit("logging/0")
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertChange("logging/0")
	wc.AssertNoChange()

	// Change the subordinate; no change.
	err = logging0.SetAgentStatus(state.StatusActive, "", nil)
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertNoChange()

	// Make the Dying unit Dead; change detected.
	err = mysql0.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertChange("mysql/0")
	wc.AssertNoChange()

	// Stop watcher; check Changes chan closed.
	testing.AssertStop(c, w)
	wc.AssertClosed()

	// Start a fresh watcher; check all units reported.
	w = s.machine.WatchUnits()
	defer testing.AssertStop(c, w)
	wc = testing.NewStringsWatcherC(c, s.State, w)
	wc.AssertChange("mysql/0", "mysql/1", "logging/0")
	wc.AssertNoChange()

	// Remove the Dead unit; no change.
	err = mysql0.Remove()
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertNoChange()

	// Destroy the subordinate; change detected.
	err = logging0.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertChange("logging/0")
	wc.AssertNoChange()

	// Unassign the principal; check subordinate departure also reported.
	err = mysql1.UnassignFromMachine()
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertChange("mysql/1", "logging/0")
	wc.AssertNoChange()
}

func (s *MachineSuite) TestWatchUnitsDiesOnStateClose(c *gc.C) {
	testWatcherDiesWhenStateCloses(c, func(c *gc.C, st *state.State) waiter {
		m, err := st.Machine(s.machine.Id())
		c.Assert(err, jc.ErrorIsNil)
		w := m.WatchUnits()
		<-w.Changes()
		return w
	})
}

func (s *MachineSuite) TestConstraintsFromEnvironment(c *gc.C) {
	econs1 := constraints.MustParse("mem=1G")
	econs2 := constraints.MustParse("mem=2G")

	// A newly-created machine gets a copy of the environment constraints.
	err := s.State.SetEnvironConstraints(econs1)
	c.Assert(err, jc.ErrorIsNil)
	machine1, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	mcons1, err := machine1.Constraints()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(mcons1, gc.DeepEquals, econs1)

	// Change environment constraints and add a new machine.
	err = s.State.SetEnvironConstraints(econs2)
	c.Assert(err, jc.ErrorIsNil)
	machine2, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	mcons2, err := machine2.Constraints()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(mcons2, gc.DeepEquals, econs2)

	// Check the original machine has its original constraints.
	mcons1, err = machine1.Constraints()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(mcons1, gc.DeepEquals, econs1)
}

func (s *MachineSuite) TestSetConstraints(c *gc.C) {
	machine, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)

	// Constraints can be set...
	cons1 := constraints.MustParse("mem=1G")
	err = machine.SetConstraints(cons1)
	c.Assert(err, jc.ErrorIsNil)
	mcons, err := machine.Constraints()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(mcons, gc.DeepEquals, cons1)

	// ...until the machine is provisioned, at which point they stick.
	err = machine.SetProvisioned("i-mstuck", "fake_nonce", nil)
	c.Assert(err, jc.ErrorIsNil)
	cons2 := constraints.MustParse("mem=2G")
	err = machine.SetConstraints(cons2)
	c.Assert(err, gc.ErrorMatches, "cannot set constraints: machine is already provisioned")

	// Check the failed set had no effect.
	mcons, err = machine.Constraints()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(mcons, gc.DeepEquals, cons1)
}

func (s *MachineSuite) TestSetAmbiguousConstraints(c *gc.C) {
	machine, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	cons := constraints.MustParse("mem=4G instance-type=foo")
	err = machine.SetConstraints(cons)
	c.Assert(err, gc.ErrorMatches, `cannot set constraints: ambiguous constraints: "instance-type" overlaps with "mem"`)
}

func (s *MachineSuite) TestSetUnsupportedConstraintsWarning(c *gc.C) {
	defer loggo.ResetWriters()
	logger := loggo.GetLogger("test")
	logger.SetLogLevel(loggo.DEBUG)
	var tw loggo.TestWriter
	c.Assert(loggo.RegisterWriter("constraints-tester", &tw, loggo.DEBUG), gc.IsNil)

	machine, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	cons := constraints.MustParse("mem=4G cpu-power=10")
	err = machine.SetConstraints(cons)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(tw.Log(), jc.LogMatches, jc.SimpleMessages{{
		loggo.WARNING,
		`setting constraints on machine "2": unsupported constraints: cpu-power`},
	})
	mcons, err := machine.Constraints()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(mcons, gc.DeepEquals, cons)
}

func (s *MachineSuite) TestConstraintsLifecycle(c *gc.C) {
	cons := constraints.MustParse("mem=1G")
	cannotSet := `cannot set constraints: not found or not alive`
	testWhenDying(c, s.machine, cannotSet, cannotSet, func() error {
		err := s.machine.SetConstraints(cons)
		mcons, err1 := s.machine.Constraints()
		c.Assert(err1, gc.IsNil)
		c.Assert(&mcons, jc.Satisfies, constraints.IsEmpty)
		return err
	})

	err := s.machine.Remove()
	c.Assert(err, jc.ErrorIsNil)
	err = s.machine.SetConstraints(cons)
	c.Assert(err, gc.ErrorMatches, cannotSet)
	_, err = s.machine.Constraints()
	c.Assert(err, gc.ErrorMatches, `constraints not found`)
}

func (s *MachineSuite) TestGetSetStatusWhileAlive(c *gc.C) {
	err := s.machine.SetStatus(state.StatusError, "", nil)
	c.Assert(err, gc.ErrorMatches, `cannot set status "error" without info`)
	err = s.machine.SetStatus(state.StatusDown, "", nil)
	c.Assert(err, gc.ErrorMatches, `cannot set status "down"`)
	err = s.machine.SetStatus(state.Status("vliegkat"), "orville", nil)
	c.Assert(err, gc.ErrorMatches, `cannot set invalid status "vliegkat"`)

	status, info, data, err := s.machine.Status()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(status, gc.Equals, state.StatusPending)
	c.Assert(info, gc.Equals, "")
	c.Assert(data, gc.HasLen, 0)

	err = s.machine.SetStatus(state.StatusStarted, "", nil)
	c.Assert(err, jc.ErrorIsNil)
	status, info, data, err = s.machine.Status()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(status, gc.Equals, state.StatusStarted)
	c.Assert(info, gc.Equals, "")
	c.Assert(data, gc.HasLen, 0)

	err = s.machine.SetStatus(state.StatusError, "provisioning failed", map[string]interface{}{
		"foo": "bar",
	})
	c.Assert(err, jc.ErrorIsNil)
	status, info, data, err = s.machine.Status()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(status, gc.Equals, state.StatusError)
	c.Assert(info, gc.Equals, "provisioning failed")
	c.Assert(data, gc.DeepEquals, map[string]interface{}{
		"foo": "bar",
	})
}

func (s *MachineSuite) TestSetStatusPending(c *gc.C) {
	err := s.machine.SetStatus(state.StatusPending, "", nil)
	c.Assert(err, jc.ErrorIsNil)
	// Cannot set status to pending once a machine is provisioned.
	err = s.machine.SetProvisioned("umbrella/0", "fake_nonce", nil)
	c.Assert(err, jc.ErrorIsNil)
	err = s.machine.SetStatus(state.StatusPending, "", nil)
	c.Assert(err, gc.ErrorMatches, `cannot set status "pending"`)
}

func (s *MachineSuite) TestGetSetStatusWhileNotAlive(c *gc.C) {
	// When Dying set/get should work.
	err := s.machine.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	err = s.machine.SetStatus(state.StatusStopped, "", nil)
	c.Assert(err, jc.ErrorIsNil)
	status, info, data, err := s.machine.Status()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(status, gc.Equals, state.StatusStopped)
	c.Assert(info, gc.Equals, "")
	c.Assert(data, gc.HasLen, 0)

	// When Dead set should fail, but get will work.
	err = s.machine.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
	err = s.machine.SetStatus(state.StatusStarted, "not really", nil)
	c.Assert(err, gc.ErrorMatches, `cannot set status of machine "1": not found or not alive`)
	status, info, data, err = s.machine.Status()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(status, gc.Equals, state.StatusStopped)
	c.Assert(info, gc.Equals, "")
	c.Assert(data, gc.HasLen, 0)

	err = s.machine.Remove()
	c.Assert(err, jc.ErrorIsNil)
	err = s.machine.SetStatus(state.StatusStarted, "not really", nil)
	c.Assert(err, gc.ErrorMatches, `cannot set status of machine "1": not found or not alive`)
	_, _, _, err = s.machine.Status()
	c.Assert(err, gc.ErrorMatches, "status not found")
}

func (s *MachineSuite) TestGetSetStatusDataStandard(c *gc.C) {
	err := s.machine.SetStatus(state.StatusStarted, "", nil)
	c.Assert(err, jc.ErrorIsNil)
	_, _, _, err = s.machine.Status()
	c.Assert(err, jc.ErrorIsNil)

	// Regular status setting with data.
	err = s.machine.SetStatus(state.StatusError, "provisioning failed", map[string]interface{}{
		"1st-key": "one",
		"2nd-key": 2,
		"3rd-key": true,
	})
	c.Assert(err, jc.ErrorIsNil)
	status, info, data, err := s.machine.Status()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(status, gc.Equals, state.StatusError)
	c.Assert(info, gc.Equals, "provisioning failed")
	c.Assert(data, gc.DeepEquals, map[string]interface{}{
		"1st-key": "one",
		"2nd-key": 2,
		"3rd-key": true,
	})
}

func (s *MachineSuite) TestGetSetStatusDataMongo(c *gc.C) {
	err := s.machine.SetStatus(state.StatusStarted, "", nil)
	c.Assert(err, jc.ErrorIsNil)
	_, _, _, err = s.machine.Status()
	c.Assert(err, jc.ErrorIsNil)

	// Status setting with MongoDB special values.
	err = s.machine.SetStatus(state.StatusError, "mongo", map[string]interface{}{
		`{name: "Joe"}`: "$where",
		"eval":          `eval(function(foo) { return foo; }, "bar")`,
		"mapReduce":     "mapReduce",
		"group":         "group",
	})
	c.Assert(err, jc.ErrorIsNil)
	status, info, data, err := s.machine.Status()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(status, gc.Equals, state.StatusError)
	c.Assert(info, gc.Equals, "mongo")
	c.Assert(data, gc.DeepEquals, map[string]interface{}{
		`{name: "Joe"}`: "$where",
		"eval":          `eval(function(foo) { return foo; }, "bar")`,
		"mapReduce":     "mapReduce",
		"group":         "group",
	})
}

func (s *MachineSuite) TestGetSetStatusDataChange(c *gc.C) {
	err := s.machine.SetStatus(state.StatusStarted, "", nil)
	c.Assert(err, jc.ErrorIsNil)
	_, _, _, err = s.machine.Status()
	c.Assert(err, jc.ErrorIsNil)

	// Status setting and changing data afterwards.
	data := map[string]interface{}{
		"1st-key": "one",
		"2nd-key": 2,
		"3rd-key": true,
	}
	err = s.machine.SetStatus(state.StatusError, "provisioning failed", data)
	c.Assert(err, jc.ErrorIsNil)
	data["4th-key"] = 4.0

	status, info, data, err := s.machine.Status()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(status, gc.Equals, state.StatusError)
	c.Assert(info, gc.Equals, "provisioning failed")
	c.Assert(data, gc.DeepEquals, map[string]interface{}{
		"1st-key": "one",
		"2nd-key": 2,
		"3rd-key": true,
	})

	// Set status data to nil, so an empty map will be returned.
	err = s.machine.SetStatus(state.StatusStarted, "", nil)
	c.Assert(err, jc.ErrorIsNil)

	status, info, data, err = s.machine.Status()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(status, gc.Equals, state.StatusStarted)
	c.Assert(info, gc.Equals, "")
	c.Assert(data, gc.HasLen, 0)
}

func (s *MachineSuite) TestSetAddresses(c *gc.C) {
	machine, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(machine.Addresses(), gc.HasLen, 0)

	addresses := []network.Address{
		network.NewAddress("127.0.0.1", network.ScopeUnknown),
		network.NewAddress("8.8.8.8", network.ScopeUnknown),
	}
	err = machine.SetAddresses(addresses...)
	c.Assert(err, jc.ErrorIsNil)
	err = machine.Refresh()
	c.Assert(err, jc.ErrorIsNil)

	expectedAddresses := []network.Address{
		network.NewAddress("8.8.8.8", network.ScopeUnknown),
		network.NewAddress("127.0.0.1", network.ScopeUnknown),
	}
	c.Assert(machine.Addresses(), jc.DeepEquals, expectedAddresses)
}

func (s *MachineSuite) TestSetAddressesWithContainers(c *gc.C) {
	machine, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(machine.Addresses(), gc.HasLen, 0)

	// Create subnet and pick two addresses.
	subnetInfo := state.SubnetInfo{
		CIDR:              "192.168.1.0/24",
		AllocatableIPLow:  "192.168.1.0",
		AllocatableIPHigh: "192.168.1.10",
	}
	subnet, err := s.State.AddSubnet(subnetInfo)
	c.Assert(err, jc.ErrorIsNil)

	ipAddr1, err := subnet.PickNewAddress()
	c.Assert(err, jc.ErrorIsNil)
	err = ipAddr1.SetState(state.AddressStateAllocated)
	c.Assert(err, jc.ErrorIsNil)
	ipAddr2, err := subnet.PickNewAddress()
	c.Assert(err, jc.ErrorIsNil)
	err = ipAddr2.SetState(state.AddressStateAllocated)
	c.Assert(err, jc.ErrorIsNil)

	// When setting all addresses the subnet addresses have to be
	// filtered out.
	addresses := []network.Address{
		network.NewAddress("127.0.0.1", network.ScopeUnknown),
		network.NewAddress("8.8.8.8", network.ScopeUnknown),
		ipAddr1.Address(),
		ipAddr2.Address(),
	}
	err = machine.SetAddresses(addresses...)
	c.Assert(err, jc.ErrorIsNil)
	err = machine.Refresh()
	c.Assert(err, jc.ErrorIsNil)

	expectedAddresses := []network.Address{
		network.NewAddress("8.8.8.8", network.ScopeUnknown),
		network.NewAddress("127.0.0.1", network.ScopeUnknown),
	}
	c.Assert(machine.Addresses(), jc.DeepEquals, expectedAddresses)
}

func (s *MachineSuite) TestSetAddressesOnContainer(c *gc.C) {
	machine, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(machine.Addresses(), gc.HasLen, 0)

	// Create subnet and pick two addresses.
	subnetInfo := state.SubnetInfo{
		CIDR:              "192.168.1.0/24",
		AllocatableIPLow:  "192.168.1.0",
		AllocatableIPHigh: "192.168.1.10",
	}
	subnet, err := s.State.AddSubnet(subnetInfo)
	c.Assert(err, jc.ErrorIsNil)

	ipAddr, err := subnet.PickNewAddress()
	c.Assert(err, jc.ErrorIsNil)
	err = ipAddr.SetState(state.AddressStateAllocated)
	c.Assert(err, jc.ErrorIsNil)

	// Create an LXC container inside the machine.
	template := state.MachineTemplate{
		Series: "quantal",
		Jobs:   []state.MachineJob{state.JobHostUnits},
	}
	container, err := s.State.AddMachineInsideMachine(template, machine.Id(), instance.LXC)
	c.Assert(err, jc.ErrorIsNil)

	// When setting all addresses the subnet address has to accepted.
	addresses := []network.Address{
		network.NewAddress("127.0.0.1", network.ScopeUnknown),
		ipAddr.Address(),
	}
	err = container.SetAddresses(addresses...)
	c.Assert(err, jc.ErrorIsNil)
	err = container.Refresh()
	c.Assert(err, jc.ErrorIsNil)

	expectedAddresses := []network.Address{
		ipAddr.Address(),
		network.NewAddress("127.0.0.1", network.ScopeUnknown),
	}
	c.Assert(container.Addresses(), jc.DeepEquals, expectedAddresses)
}

func (s *MachineSuite) TestSetMachineAddresses(c *gc.C) {
	machine, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(machine.Addresses(), gc.HasLen, 0)

	addresses := []network.Address{
		network.NewAddress("127.0.0.1", network.ScopeUnknown),
		network.NewAddress("8.8.8.8", network.ScopeUnknown),
	}
	err = machine.SetMachineAddresses(addresses...)
	c.Assert(err, jc.ErrorIsNil)
	err = machine.Refresh()
	c.Assert(err, jc.ErrorIsNil)

	expectedAddresses := []network.Address{
		network.NewAddress("8.8.8.8", network.ScopeUnknown),
		network.NewAddress("127.0.0.1", network.ScopeUnknown),
	}
	c.Assert(machine.MachineAddresses(), jc.DeepEquals, expectedAddresses)
}

func (s *MachineSuite) TestMergedAddresses(c *gc.C) {
	machine, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(machine.Addresses(), gc.HasLen, 0)

	providerAddresses := network.NewAddresses(
		"127.0.0.2",
		"8.8.8.8",
		"fc00::1",
		"::1",
		"",
		"2001:db8::1",
		"127.0.0.2",
		"example.org",
	)
	err = machine.SetAddresses(providerAddresses...)
	c.Assert(err, jc.ErrorIsNil)

	machineAddresses := network.NewAddresses(
		"127.0.0.1",
		"localhost",
		"2001:db8::1",
		"192.168.0.1",
		"fe80::1",
		"::1",
		"fd00::1",
	)
	err = machine.SetMachineAddresses(machineAddresses...)
	c.Assert(err, jc.ErrorIsNil)
	err = machine.Refresh()
	c.Assert(err, jc.ErrorIsNil)

	// Before setting the addresses coming from either the provider or
	// the machine itself, they are sorted to prefer public IPs on
	// top, then hostnames, cloud-local, machine-local, link-local.
	// Duplicates are removed, then when calling Addresses() both
	// sources are merged while preservig the provider addresses
	// order.
	c.Assert(machine.Addresses(), jc.DeepEquals, network.NewAddresses(
		"8.8.8.8",
		"2001:db8::1",
		"example.org",
		"fc00::1",
		"127.0.0.2",
		"::1",
		"localhost",
		"192.168.0.1",
		"fd00::1",
		"127.0.0.1",
		"fe80::1",
	))

	// Now simulate prefer-ipv6: true
	c.Assert(
		s.State.UpdateEnvironConfig(
			map[string]interface{}{"prefer-ipv6": true},
			nil, nil,
		),
		gc.IsNil,
	)

	err = machine.SetAddresses(providerAddresses...)
	c.Assert(err, jc.ErrorIsNil)
	err = machine.SetMachineAddresses(machineAddresses...)
	c.Assert(err, jc.ErrorIsNil)
	err = machine.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(machine.Addresses(), jc.DeepEquals, network.NewAddresses(
		"2001:db8::1",
		"8.8.8.8",
		"example.org",
		"fc00::1",
		"::1",
		"127.0.0.2",
		"localhost",
		"fd00::1",
		"192.168.0.1",
		"127.0.0.1",
		"fe80::1",
	))
}

func (s *MachineSuite) TestSetAddressesConcurrentChangeDifferent(c *gc.C) {
	machine, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(machine.Addresses(), gc.HasLen, 0)

	addr0 := network.NewAddress("127.0.0.1", network.ScopeUnknown)
	addr1 := network.NewAddress("8.8.8.8", network.ScopeUnknown)

	defer state.SetBeforeHooks(c, s.State, func() {
		machine, err := s.State.Machine(machine.Id())
		c.Assert(err, jc.ErrorIsNil)
		err = machine.SetAddresses(addr1, addr0)
		c.Assert(err, jc.ErrorIsNil)
	}).Check()

	err = machine.SetAddresses(addr0, addr1)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(machine.Addresses(), jc.SameContents, []network.Address{addr0, addr1})
}

func (s *MachineSuite) TestSetAddressesConcurrentChangeEqual(c *gc.C) {
	machine, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(machine.Addresses(), gc.HasLen, 0)
	machineDocID := state.DocID(s.State, machine.Id())
	revno0, err := state.TxnRevno(s.State, "machines", machineDocID)
	c.Assert(err, jc.ErrorIsNil)

	addr0 := network.NewAddress("127.0.0.1", network.ScopeUnknown)
	addr1 := network.NewAddress("8.8.8.8", network.ScopeUnknown)

	var revno1 int64
	defer state.SetBeforeHooks(c, s.State, func() {
		machine, err := s.State.Machine(machine.Id())
		c.Assert(err, jc.ErrorIsNil)
		err = machine.SetAddresses(addr0, addr1)
		c.Assert(err, jc.ErrorIsNil)
		revno1, err = state.TxnRevno(s.State, "machines", machineDocID)
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(revno1, gc.Equals, revno0+1)
	}).Check()

	err = machine.SetAddresses(addr0, addr1)
	c.Assert(err, jc.ErrorIsNil)

	// Doc should not have been updated, but Machine object's view should be.
	revno2, err := state.TxnRevno(s.State, "machines", machineDocID)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(revno2, gc.Equals, revno1)
	c.Assert(machine.Addresses(), jc.SameContents, []network.Address{addr0, addr1})
}

func (s *MachineSuite) addMachineWithSupportedContainer(c *gc.C, container instance.ContainerType) *state.Machine {
	machine, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	containers := []instance.ContainerType{container}
	err = machine.SetSupportedContainers(containers)
	c.Assert(err, jc.ErrorIsNil)
	assertSupportedContainers(c, machine, containers)
	return machine
}

// assertSupportedContainers checks the document in memory has the specified
// containers and then reloads the document from the database to assert saved
// values match also.
func assertSupportedContainers(c *gc.C, machine *state.Machine, containers []instance.ContainerType) {
	supportedContainers, known := machine.SupportedContainers()
	c.Assert(known, jc.IsTrue)
	c.Assert(supportedContainers, gc.DeepEquals, containers)
	// Reload so we can check the saved values.
	err := machine.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	supportedContainers, known = machine.SupportedContainers()
	c.Assert(known, jc.IsTrue)
	c.Assert(supportedContainers, gc.DeepEquals, containers)
}

func assertSupportedContainersUnknown(c *gc.C, machine *state.Machine) {
	containers, known := machine.SupportedContainers()
	c.Assert(known, jc.IsFalse)
	c.Assert(containers, gc.HasLen, 0)
}

func (s *MachineSuite) TestSupportedContainersInitiallyUnknown(c *gc.C) {
	machine, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	assertSupportedContainersUnknown(c, machine)
}

func (s *MachineSuite) TestSupportsNoContainers(c *gc.C) {
	machine, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)

	err = machine.SupportsNoContainers()
	c.Assert(err, jc.ErrorIsNil)
	assertSupportedContainers(c, machine, []instance.ContainerType{})
}

func (s *MachineSuite) TestSetSupportedContainerTypeNoneIsError(c *gc.C) {
	machine, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)

	err = machine.SetSupportedContainers([]instance.ContainerType{instance.LXC, instance.NONE})
	c.Assert(err, gc.ErrorMatches, `"none" is not a valid container type`)
	assertSupportedContainersUnknown(c, machine)
	err = machine.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	assertSupportedContainersUnknown(c, machine)
}

func (s *MachineSuite) TestSupportsNoContainersOverwritesExisting(c *gc.C) {
	machine := s.addMachineWithSupportedContainer(c, instance.LXC)

	err := machine.SupportsNoContainers()
	c.Assert(err, jc.ErrorIsNil)
	assertSupportedContainers(c, machine, []instance.ContainerType{})
}

func (s *MachineSuite) TestSetSupportedContainersSingle(c *gc.C) {
	machine, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)

	err = machine.SetSupportedContainers([]instance.ContainerType{instance.LXC})
	c.Assert(err, jc.ErrorIsNil)
	assertSupportedContainers(c, machine, []instance.ContainerType{instance.LXC})
}

func (s *MachineSuite) TestSetSupportedContainersSame(c *gc.C) {
	machine := s.addMachineWithSupportedContainer(c, instance.LXC)

	err := machine.SetSupportedContainers([]instance.ContainerType{instance.LXC})
	c.Assert(err, jc.ErrorIsNil)
	assertSupportedContainers(c, machine, []instance.ContainerType{instance.LXC})
}

func (s *MachineSuite) TestSetSupportedContainersNew(c *gc.C) {
	machine := s.addMachineWithSupportedContainer(c, instance.LXC)

	err := machine.SetSupportedContainers([]instance.ContainerType{instance.LXC, instance.KVM})
	c.Assert(err, jc.ErrorIsNil)
	assertSupportedContainers(c, machine, []instance.ContainerType{instance.LXC, instance.KVM})
}

func (s *MachineSuite) TestSetSupportedContainersMultipeNew(c *gc.C) {
	machine, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)

	err = machine.SetSupportedContainers([]instance.ContainerType{instance.LXC, instance.KVM})
	c.Assert(err, jc.ErrorIsNil)
	assertSupportedContainers(c, machine, []instance.ContainerType{instance.LXC, instance.KVM})
}

func (s *MachineSuite) TestSetSupportedContainersMultipleExisting(c *gc.C) {
	machine := s.addMachineWithSupportedContainer(c, instance.LXC)

	err := machine.SetSupportedContainers([]instance.ContainerType{instance.LXC, instance.KVM})
	c.Assert(err, jc.ErrorIsNil)
	assertSupportedContainers(c, machine, []instance.ContainerType{instance.LXC, instance.KVM})
}

func (s *MachineSuite) TestSetSupportedContainersSetsUnknownToError(c *gc.C) {
	// Create a machine and add lxc and kvm containers prior to calling SetSupportedContainers
	machine, err := s.State.AddMachine("quantal", state.JobHostUnits)
	template := state.MachineTemplate{
		Series: "quantal",
		Jobs:   []state.MachineJob{state.JobHostUnits},
	}
	container, err := s.State.AddMachineInsideMachine(template, machine.Id(), instance.LXC)
	c.Assert(err, jc.ErrorIsNil)
	supportedContainer, err := s.State.AddMachineInsideMachine(template, machine.Id(), instance.KVM)
	c.Assert(err, jc.ErrorIsNil)
	err = machine.SetSupportedContainers([]instance.ContainerType{instance.KVM})
	c.Assert(err, jc.ErrorIsNil)

	// A supported (kvm) container will have a pending status.
	err = supportedContainer.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	status, info, data, err := supportedContainer.Status()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(status, gc.Equals, state.StatusPending)

	// An unsupported (lxc) container will have an error status.
	err = container.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	status, info, data, err = container.Status()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(status, gc.Equals, state.StatusError)
	c.Assert(info, gc.Equals, "unsupported container")
	c.Assert(data, gc.DeepEquals, map[string]interface{}{"type": "lxc"})
}

func (s *MachineSuite) TestSupportsNoContainersSetsAllToError(c *gc.C) {
	// Create a machine and add all container types prior to calling SupportsNoContainers
	machine, err := s.State.AddMachine("quantal", state.JobHostUnits)
	var containers []*state.Machine
	template := state.MachineTemplate{
		Series: "quantal",
		Jobs:   []state.MachineJob{state.JobHostUnits},
	}
	for _, containerType := range instance.ContainerTypes {
		container, err := s.State.AddMachineInsideMachine(template, machine.Id(), containerType)
		c.Assert(err, jc.ErrorIsNil)
		containers = append(containers, container)
	}

	err = machine.SupportsNoContainers()
	c.Assert(err, jc.ErrorIsNil)

	// All containers should be in error state.
	for _, container := range containers {
		err = container.Refresh()
		c.Assert(err, jc.ErrorIsNil)
		status, info, data, err := container.Status()
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(status, gc.Equals, state.StatusError)
		c.Assert(info, gc.Equals, "unsupported container")
		containerType := state.ContainerTypeFromId(container.Id())
		c.Assert(data, gc.DeepEquals, map[string]interface{}{"type": string(containerType)})
	}
}

func (s *MachineSuite) TestWatchInterfaces(c *gc.C) {
	// Provision the machine.
	networks := []state.NetworkInfo{{
		Name:       "net1",
		ProviderId: "net1",
		CIDR:       "0.1.2.0/24",
		VLANTag:    0,
	}, {
		Name:       "vlan42",
		ProviderId: "vlan42",
		CIDR:       "0.2.2.0/24",
		VLANTag:    42,
	}}
	interfaces := []state.NetworkInterfaceInfo{{
		MACAddress:    "aa:bb:cc:dd:ee:f0",
		InterfaceName: "eth0",
		NetworkName:   "net1",
		IsVirtual:     false,
	}, {
		MACAddress:    "aa:bb:cc:dd:ee:f1",
		InterfaceName: "eth1",
		NetworkName:   "net1",
		IsVirtual:     false,
		Disabled:      true,
	}, {
		MACAddress:    "aa:bb:cc:dd:ee:f1",
		InterfaceName: "eth1.42",
		NetworkName:   "vlan42",
		IsVirtual:     true,
	}}
	err := s.machine.SetInstanceInfo("umbrella/0", "fake_nonce", nil, networks, interfaces, nil, nil)
	c.Assert(err, jc.ErrorIsNil)

	// Read dynamically generated document Ids.
	ifaces, err := s.machine.NetworkInterfaces()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ifaces, gc.HasLen, 3)

	// Start network interface watcher.
	w := s.machine.WatchInterfaces()
	defer testing.AssertStop(c, w)
	wc := testing.NewNotifyWatcherC(c, s.State, w)
	wc.AssertOneChange()

	// Disable the first interface.
	err = ifaces[0].Disable()
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertOneChange()

	// Disable the first interface again, should not report.
	err = ifaces[0].Disable()
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertNoChange()

	// Enable the second interface, should report, because it was initially disabled.
	err = ifaces[1].Enable()
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertOneChange()

	// Disable two interfaces at once, check that both are reported.
	err = ifaces[1].Disable()
	c.Assert(err, jc.ErrorIsNil)
	err = ifaces[2].Disable()
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertOneChange()

	// Enable the first interface.
	err = ifaces[0].Enable()
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertOneChange()

	// Enable the first interface again, should not report.
	err = ifaces[0].Enable()
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertNoChange()

	// Remove the network interface.
	err = ifaces[0].Remove()
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertOneChange()

	// Add the new interface.
	_, _ = addNetworkAndInterface(
		c, s.State, s.machine,
		"net2", "net2", "0.5.2.0/24", 0, false,
		"aa:bb:cc:dd:ee:f2", "eth2")
	wc.AssertOneChange()

	// Provision another machine, should not report
	machine2, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	interfaces2 := []state.NetworkInterfaceInfo{{
		MACAddress:    "aa:bb:cc:dd:ee:e0",
		InterfaceName: "eth0",
		NetworkName:   "net1",
		IsVirtual:     false,
	}, {
		MACAddress:    "aa:bb:cc:dd:ee:e1",
		InterfaceName: "eth1",
		NetworkName:   "net1",
		IsVirtual:     false,
	}, {
		MACAddress:    "aa:bb:cc:dd:ee:e1",
		InterfaceName: "eth1.42",
		NetworkName:   "vlan42",
		IsVirtual:     true,
	}}
	err = machine2.SetInstanceInfo("m-too", "fake_nonce", nil, networks, interfaces2, nil, nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ifaces, gc.HasLen, 3)
	wc.AssertNoChange()

	// Read dynamically generated document Ids.
	ifaces2, err := machine2.NetworkInterfaces()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ifaces2, gc.HasLen, 3)

	// Disable the first interface on the second machine, should not report.
	err = ifaces2[0].Disable()
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertNoChange()

	// Remove the network interface on the second machine, should not report.
	err = ifaces2[0].Remove()
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertNoChange()

	// Stop watcher; check Changes chan closed.
	testing.AssertStop(c, w)
	wc.AssertClosed()
}

func (s *MachineSuite) TestWatchInterfacesDiesOnStateClose(c *gc.C) {
	testWatcherDiesWhenStateCloses(c, func(c *gc.C, st *state.State) waiter {
		m, err := st.Machine(s.machine.Id())
		c.Assert(err, jc.ErrorIsNil)
		w := m.WatchInterfaces()
		<-w.Changes()
		return w
	})
}

func (s *MachineSuite) TestMachineAgentTools(c *gc.C) {
	m, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	testAgentTools(c, m, "machine "+m.Id())
}
