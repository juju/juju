// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"sort"
	"strings"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	jc "github.com/juju/testing/checkers"
	jujutxn "github.com/juju/txn"
	"github.com/juju/version"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"
	"gopkg.in/juju/worker.v1"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"

	"github.com/juju/juju/constraints"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/mongo"
	"github.com/juju/juju/mongo/mongotest"
	"github.com/juju/juju/network"
	"github.com/juju/juju/provider/dummy"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/testing"
	"github.com/juju/juju/storage"
	"github.com/juju/juju/storage/poolmanager"
	"github.com/juju/juju/storage/provider"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/testing/factory"
)

type MachineSuite struct {
	ConnSuite
	machine0 *state.Machine
	machine  *state.Machine
}

var _ = gc.Suite(&MachineSuite{})

func (s *MachineSuite) SetUpTest(c *gc.C) {
	s.ConnSuite.SetUpTest(c)
	s.policy.GetConstraintsValidator = func() (constraints.Validator, error) {
		validator := constraints.NewValidator()
		validator.RegisterConflicts([]string{constraints.InstanceType}, []string{constraints.Mem})
		validator.RegisterUnsupported([]string{constraints.CpuPower})
		return validator, nil
	}
	var err error
	s.machine0, err = s.State.AddMachine("quantal", state.JobManageModel)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.machine0.SetHasVote(true), jc.ErrorIsNil)
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

func (s *MachineSuite) TestSetKeepInstance(c *gc.C) {
	err := s.machine.SetProvisioned("1234", "nonce", nil)
	c.Assert(err, jc.ErrorIsNil)
	err = s.machine.SetKeepInstance(true)
	c.Assert(err, jc.ErrorIsNil)

	m, err := s.State.Machine(s.machine.Id())
	c.Assert(err, jc.ErrorIsNil)
	keep, err := m.KeepInstance()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(keep, jc.IsTrue)
}

func (s *MachineSuite) TestSetCharmProfile(c *gc.C) {
	err := s.machine.SetProvisioned("1234", "nonce", nil)
	c.Assert(err, jc.ErrorIsNil)
	expectedProfiles := []string{"juju-default-lxd-profile-0", "juju-default-lxd-sub-0"}
	err = s.machine.SetCharmProfiles(expectedProfiles)
	c.Assert(err, jc.ErrorIsNil)

	m, err := s.State.Machine(s.machine.Id())
	c.Assert(err, jc.ErrorIsNil)
	obtainedProfiles, err := m.CharmProfiles()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(expectedProfiles, jc.SameContents, obtainedProfiles)
}

func (s *MachineSuite) TestAddMachineInsideMachineModelDying(c *gc.C) {
	model, err := s.State.Model()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(model.Destroy(state.DestroyModelParams{}), jc.ErrorIsNil)

	_, err = s.State.AddMachineInsideMachine(state.MachineTemplate{
		Series: "quantal",
		Jobs:   []state.MachineJob{state.JobHostUnits},
	}, s.machine.Id(), instance.LXD)
	c.Assert(err, gc.ErrorMatches, `model "testmodel" is no longer alive`)
}

func (s *MachineSuite) TestAddMachineInsideMachineModelMigrating(c *gc.C) {
	model, err := s.State.Model()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(model.SetMigrationMode(state.MigrationModeExporting), jc.ErrorIsNil)

	_, err = s.State.AddMachineInsideMachine(state.MachineTemplate{
		Series: "quantal",
		Jobs:   []state.MachineJob{state.JobHostUnits},
	}, s.machine.Id(), instance.LXD)
	c.Assert(err, gc.ErrorMatches, `model "testmodel" is being migrated`)
}

func (s *MachineSuite) TestShouldShutdownOrReboot(c *gc.C) {
	// Add first container.
	c1, err := s.State.AddMachineInsideMachine(state.MachineTemplate{
		Series: "quantal",
		Jobs:   []state.MachineJob{state.JobHostUnits},
	}, s.machine.Id(), instance.LXD)
	c.Assert(err, jc.ErrorIsNil)

	// Add second container.
	c2, err := s.State.AddMachineInsideMachine(state.MachineTemplate{
		Series: "quantal",
		Jobs:   []state.MachineJob{state.JobHostUnits},
	}, c1.Id(), instance.LXD)
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
	}, s.machine.Id(), instance.LXD)
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
	cfg, err := s.Model.ModelConfig()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cfg.Type(), gc.Not(gc.Equals), "null")
	c.Assert(s.machine.Id(), gc.Equals, "1")
	manual, err := s.machine0.IsManual()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(manual, jc.IsFalse)
	attrs := map[string]interface{}{"type": "null"}
	err = s.Model.UpdateModelConfig(attrs, nil)
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
	container, err := s.State.AddMachineInsideMachine(template, machine.Id(), instance.LXD)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(machine.IsContainer(), jc.IsFalse)
	c.Assert(container.IsContainer(), jc.IsTrue)
}

func (s *MachineSuite) TestLifeJobManageModel(c *gc.C) {
	m := s.machine0
	err := m.Destroy()
	c.Assert(err, gc.ErrorMatches, "machine 0 is the only controller machine")
	err = m.EnsureDead()
	c.Assert(err, gc.ErrorMatches, "machine 0 is still a voting controller member")
	// Since this is the only controller machine, we cannot even force destroy it
	err = m.ForceDestroy()
	c.Assert(err, gc.ErrorMatches, "machine 0 is the only controller machine")
	err = m.EnsureDead()
	c.Assert(err, gc.ErrorMatches, "machine 0 is still a voting controller member")
}

func (s *MachineSuite) TestLifeMachineWithContainer(c *gc.C) {
	// A machine hosting a container must not advance lifecycle.
	_, err := s.State.AddMachineInsideMachine(state.MachineTemplate{
		Series: "quantal",
		Jobs:   []state.MachineJob{state.JobHostUnits},
	}, s.machine.Id(), instance.LXD)
	c.Assert(err, jc.ErrorIsNil)
	err = s.machine.Destroy()
	c.Assert(err, gc.FitsTypeOf, &state.HasContainersError{})
	c.Assert(err, gc.ErrorMatches, `machine 1 is hosting containers "1/lxd/0"`)
	err1 := s.machine.EnsureDead()
	c.Assert(err1, gc.DeepEquals, err)
	c.Assert(s.machine.Life(), gc.Equals, state.Alive)
}

func (s *MachineSuite) TestLifeJobHostUnits(c *gc.C) {
	// A machine with an assigned unit must not advance lifecycle.
	app := s.AddTestingApplication(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	unit, err := app.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	err = unit.AssignToMachine(s.machine)
	c.Assert(err, jc.ErrorIsNil)
	err = s.machine.Destroy()
	c.Assert(err, jc.Satisfies, state.IsHasAssignedUnitsError)
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
	app := s.AddTestingApplication(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	unit, err := app.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	err = unit.AssignToMachine(s.machine)
	c.Assert(err, jc.ErrorIsNil)
	err = unit.OpenPort("tcp", 8080)
	c.Assert(err, jc.ErrorIsNil)
	ports, err := state.GetPorts(s.State, s.machine.Id(), "")
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
	ports, err = state.GetPorts(s.State, s.machine.Id(), "")
	c.Assert(ports, gc.IsNil)
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *MachineSuite) TestDestroyOps(c *gc.C) {
	m := s.Factory.MakeMachine(c, nil)
	ops, err := state.ForceDestroyMachineOps(m)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ops, gc.NotNil)
}

func (s *MachineSuite) TestDestroyOpsForManagerFails(c *gc.C) {
	// s.Factory does not allow us to make a manager machine, so we grab one
	// from State ...
	machines, err := s.State.AllMachines()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(machines), jc.GreaterThan, 0)
	m := machines[0]
	c.Assert(m.IsManager(), jc.IsTrue)

	// ... and assert that we cannot get the destroy ops for it.
	ops, err := state.ForceDestroyMachineOps(m)
	c.Assert(err, gc.ErrorMatches, `machine 0 is the only controller machine`)
	c.Assert(ops, gc.IsNil)
}

func (s *MachineSuite) TestDestroyAbort(c *gc.C) {
	defer state.SetBeforeHooks(c, s.State, func() {
		c.Assert(s.machine.Destroy(), gc.IsNil)
	}).Check()
	err := s.machine.Destroy()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *MachineSuite) TestDestroyCancel(c *gc.C) {
	app := s.AddTestingApplication(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	unit, err := app.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)

	defer state.SetBeforeHooks(c, s.State, func() {
		c.Assert(unit.AssignToMachine(s.machine), gc.IsNil)
	}).Check()
	err = s.machine.Destroy()
	c.Assert(err, jc.Satisfies, state.IsHasAssignedUnitsError)
}

func (s *MachineSuite) TestDestroyContention(c *gc.C) {
	app := s.AddTestingApplication(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	unit, err := app.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)

	perturb := jujutxn.TestHook{
		Before: func() { c.Assert(unit.AssignToMachine(s.machine), gc.IsNil) },
		After:  func() { c.Assert(unit.UnassignFromMachine(), gc.IsNil) },
	}
	defer state.SetTestHooks(c, s.State, perturb, perturb, perturb).Check()

	err = s.machine.Destroy()
	c.Assert(err, gc.ErrorMatches, "machine 1 cannot advance lifecycle: state changing too quickly; try again soon")
}

func (s *MachineSuite) TestDestroyWithApplicationDestroyPending(c *gc.C) {
	app := s.AddTestingApplication(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	unit, err := app.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	err = unit.AssignToMachine(s.machine)
	c.Assert(err, jc.ErrorIsNil)

	err = app.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	err = s.machine.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	// Machine is still advanced to Dying.
	life := s.machine.Life()
	c.Assert(life, gc.Equals, state.Dying)
}

func (s *MachineSuite) TestDestroyFailsWhenNewUnitAdded(c *gc.C) {
	app := s.AddTestingApplication(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	unit, err := app.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	err = unit.AssignToMachine(s.machine)
	c.Assert(err, jc.ErrorIsNil)

	err = app.Destroy()
	c.Assert(err, jc.ErrorIsNil)

	defer state.SetBeforeHooks(c, s.State, func() {
		anotherApp := s.AddTestingApplication(c, "mysql", s.AddTestingCharm(c, "mysql"))
		anotherUnit, err := anotherApp.AddUnit(state.AddUnitParams{})
		c.Assert(err, jc.ErrorIsNil)
		err = anotherUnit.AssignToMachine(s.machine)
		c.Assert(err, jc.ErrorIsNil)
	}).Check()

	err = s.machine.Destroy()
	c.Assert(err, jc.Satisfies, state.IsHasAssignedUnitsError)
	life := s.machine.Life()
	c.Assert(life, gc.Equals, state.Alive)
}

func (s *MachineSuite) TestDestroyWithUnitDestroyPending(c *gc.C) {
	app := s.AddTestingApplication(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	unit, err := app.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	err = unit.AssignToMachine(s.machine)
	c.Assert(err, jc.ErrorIsNil)

	err = unit.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	err = s.machine.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	// Machine is still advanced to Dying.
	life := s.machine.Life()
	c.Assert(life, gc.Equals, state.Dying)
}

func (s *MachineSuite) TestDestroyFailsWhenNewContainerAdded(c *gc.C) {
	app := s.AddTestingApplication(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	unit, err := app.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	err = unit.AssignToMachine(s.machine)
	c.Assert(err, jc.ErrorIsNil)

	err = app.Destroy()
	c.Assert(err, jc.ErrorIsNil)

	defer state.SetBeforeHooks(c, s.State, func() {
		_, err := s.State.AddMachineInsideMachine(state.MachineTemplate{
			Series: "quantal",
			Jobs:   []state.MachineJob{state.JobHostUnits},
		}, s.machine.Id(), instance.LXD)
		c.Assert(err, jc.ErrorIsNil)
	}).Check()

	err = s.machine.Destroy()
	c.Assert(err, jc.Satisfies, state.IsHasAssignedUnitsError)
	life := s.machine.Life()
	c.Assert(life, gc.Equals, state.Alive)
}

func (s *MachineSuite) TestRemove(c *gc.C) {
	err := s.State.SetSSHHostKeys(s.machine.MachineTag(), state.SSHHostKeys{"rsa", "dsa"})
	c.Assert(err, jc.ErrorIsNil)

	err = s.machine.Remove()
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

	_, err = s.State.GetSSHHostKeys(s.machine.MachineTag())
	c.Assert(errors.IsNotFound(err), jc.IsTrue)

	// Removing an already removed machine is OK.
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
	defer func() {
		c.Assert(worker.Stop(pinger), jc.ErrorIsNil)
	}()

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
	pool, err := state.OpenStatePool(state.OpenParams{
		Clock:              clock.WallClock,
		ControllerTag:      s.State.ControllerTag(),
		ControllerModelTag: s.modelTag,
		MongoSession:       s.Session,
	})
	c.Assert(err, jc.ErrorIsNil)
	st := pool.SystemState()
	defer func() {
		// Remove the admin password so that the test harness can reset the state.
		err := st.SetAdminMongoPassword("")
		c.Check(err, jc.ErrorIsNil)
		err = pool.Close()
		c.Check(err, jc.ErrorIsNil)
	}()

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
	info := testing.NewMongoInfo()
	info.Tag = ent.Tag()
	info.Password = "bar"
	err = tryOpenState(s.modelTag, s.State.ControllerTag(), info)
	c.Check(errors.Cause(err), jc.Satisfies, errors.IsUnauthorized)
	c.Check(err, gc.ErrorMatches, `cannot log in to admin database as "machine-0": unauthorized mongo access: .*`)

	// Check that we can log in with the correct password.
	info.Password = "foo"
	session, err := mongo.DialWithInfo(*info, mongotest.DialOpts())
	c.Assert(err, jc.ErrorIsNil)
	defer session.Close()

	pool1, err := state.OpenStatePool(state.OpenParams{
		Clock:              clock.WallClock,
		ControllerTag:      s.State.ControllerTag(),
		ControllerModelTag: s.modelTag,
		MongoSession:       session,
	})
	c.Assert(err, jc.ErrorIsNil)
	defer pool1.Close()
	st1 := pool1.SystemState()

	// Change the password with an entity derived from the newly
	// opened and authenticated state.
	ent, err = st1.Machine("0")
	c.Assert(err, jc.ErrorIsNil)
	err = ent.SetMongoPassword("bar")
	c.Assert(err, jc.ErrorIsNil)

	// Check that we cannot log in with the old password.
	info.Password = "foo"
	err = tryOpenState(s.modelTag, s.State.ControllerTag(), info)
	c.Check(errors.Cause(err), jc.Satisfies, errors.IsUnauthorized)
	c.Check(err, gc.ErrorMatches, `cannot log in to admin database as "machine-0": unauthorized mongo access: .*`)

	// Check that we can log in with the correct password.
	info.Password = "bar"
	err = tryOpenState(s.modelTag, s.State.ControllerTag(), info)
	c.Assert(err, jc.ErrorIsNil)

	// Check that the administrator can still log in.
	info.Tag, info.Password = nil, "admin-secret"
	err = tryOpenState(s.modelTag, s.State.ControllerTag(), info)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *MachineSuite) TestSetPassword(c *gc.C) {
	testSetPassword(c, func() (state.Authenticator, error) {
		return s.State.Machine(s.machine.Id())
	})
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

	err = pinger.KillForTesting()
	c.Assert(err, jc.ErrorIsNil)

	s.State.StartSync()
	alive, err = s.machine.AgentPresence()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(alive, jc.IsFalse)
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

func (s *MachineSuite) TestDesiredSpacesNone(c *gc.C) {
	machine, err := s.State.AddMachine("quantal", state.JobHostUnits)
	spaces, err := machine.DesiredSpaces()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(spaces.SortedValues(), gc.DeepEquals, []string{})
}

func (s *MachineSuite) TestDesiredSpacesSimpleConstraints(c *gc.C) {
	machine, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	err = machine.SetConstraints(constraints.Value{
		Spaces: &[]string{"foo", "bar", "^baz"},
	})
	c.Assert(err, jc.ErrorIsNil)
	spaces, err := machine.DesiredSpaces()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(spaces.SortedValues(), gc.DeepEquals, []string{"bar", "foo"})
}

func (s *MachineSuite) TestDesiredSpacesEndpoints(c *gc.C) {
	_, err := s.State.AddSpace("db", "", nil, true)
	c.Assert(err, jc.ErrorIsNil)
	machine, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	app := s.AddTestingApplicationWithBindings(c, "mysql",
		s.AddTestingCharm(c, "mysql"), map[string]string{"server": "db"})
	unit, err := app.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	err = unit.AssignToMachine(machine)
	c.Assert(err, jc.ErrorIsNil)
	spaces, err := machine.DesiredSpaces()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(spaces.SortedValues(), gc.DeepEquals, []string{"db"})
}

func (s *MachineSuite) TestDesiredSpacesEndpointsAndConstraints(c *gc.C) {
	_, err := s.State.AddSpace("foo", "", nil, true)
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.State.AddSpace("db", "", nil, true)
	c.Assert(err, jc.ErrorIsNil)
	machine, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	err = machine.SetConstraints(constraints.Value{
		Spaces: &[]string{"foo"},
	})
	c.Assert(err, jc.ErrorIsNil)
	app := s.AddTestingApplicationWithBindings(c, "mysql",
		s.AddTestingCharm(c, "mysql"), map[string]string{"server": "db"})
	unit, err := app.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	err = unit.AssignToMachine(machine)
	c.Assert(err, jc.ErrorIsNil)
	spaces, err := machine.DesiredSpaces()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(spaces.SortedValues(), gc.DeepEquals, []string{"db", "foo"})
}

func (s *MachineSuite) TestDesiredSpacesNegativeConstraints(c *gc.C) {
	_, err := s.State.AddSpace("foo", "", nil, true)
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.State.AddSpace("db", "", nil, true)
	c.Assert(err, jc.ErrorIsNil)
	machine, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	err = machine.SetConstraints(constraints.Value{
		Spaces: &[]string{"^foo,^db"},
	})
	c.Assert(err, jc.ErrorIsNil)
	spaces, err := machine.DesiredSpaces()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(spaces.SortedValues(), gc.DeepEquals, []string{})
}

func (s *MachineSuite) TestDesiredSpacesNothingRequested(c *gc.C) {
	_, err := s.State.AddSpace("foo", "", nil, true)
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.State.AddSpace("db", "", nil, true)
	c.Assert(err, jc.ErrorIsNil)
	// No space constraints
	machine, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	// And empty bindings
	app := s.AddTestingApplicationWithBindings(c, "mysql",
		s.AddTestingCharm(c, "mysql"), map[string]string{})
	unit, err := app.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	err = unit.AssignToMachine(machine)
	c.Assert(err, jc.ErrorIsNil)
	spaces, err := machine.DesiredSpaces()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(spaces.SortedValues(), gc.DeepEquals, []string{})
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

func (s *MachineSuite) TestSetProvisionedDupInstanceId(c *gc.C) {
	var logWriter loggo.TestWriter
	c.Assert(loggo.RegisterWriter("dupe-test", &logWriter), gc.IsNil)
	s.AddCleanup(func(*gc.C) {
		loggo.RemoveWriter("dupe-test")
	})

	err := s.machine.SetProvisioned("umbrella/0", "fake_nonce", nil)
	c.Assert(err, jc.ErrorIsNil)

	anotherMachine, _ := s.Factory.MakeUnprovisionedMachineReturningPassword(c, &factory.MachineParams{})
	err = anotherMachine.SetProvisioned("umbrella/0", "another_nonce", nil)
	c.Assert(err, jc.ErrorIsNil)
	found := false
	for _, le := range logWriter.Log() {
		if found = strings.Contains(le.Message, "duplicate instance id"); found == true {
			break
		}
	}
	c.Assert(found, jc.IsTrue)
}

func (s *MachineSuite) TestMachineSetInstanceInfoFailureDoesNotProvision(c *gc.C) {
	assertNotProvisioned := func() {
		c.Assert(s.machine.CheckProvisioned("fake_nonce"), jc.IsFalse)
	}

	assertNotProvisioned()

	invalidVolumes := map[names.VolumeTag]state.VolumeInfo{
		names.NewVolumeTag("1065"): {VolumeId: "vol-ume"},
	}
	err := s.machine.SetInstanceInfo("umbrella/0", "fake_nonce", nil, nil, nil, invalidVolumes, nil, nil)
	c.Assert(err, gc.ErrorMatches, `cannot set info for volume \"1065\": volume \"1065\" not found`)
	assertNotProvisioned()

	invalidVolumes = map[names.VolumeTag]state.VolumeInfo{
		names.NewVolumeTag("1065"): {},
	}
	err = s.machine.SetInstanceInfo("umbrella/0", "fake_nonce", nil, nil, nil, invalidVolumes, nil, nil)
	c.Assert(err, gc.ErrorMatches, `cannot set info for volume \"1065\": volume ID not set`)
	assertNotProvisioned()

	// TODO(axw) test invalid volume attachment
}

func (s *MachineSuite) addVolume(c *gc.C, params state.VolumeParams, machineId string) names.VolumeTag {
	ops, tag, err := state.AddVolumeOps(s.State, params, machineId)
	c.Assert(err, jc.ErrorIsNil)
	err = state.RunTransaction(s.State, ops)
	c.Assert(err, jc.ErrorIsNil)
	return tag
}

func (s *MachineSuite) TestMachineSetInstanceInfoSuccess(c *gc.C) {
	pm := poolmanager.New(state.NewStateSettings(s.State), storage.ChainedProviderRegistry{
		dummy.StorageProviders(),
		provider.CommonStorageProviders(),
	})
	_, err := pm.Create("loop-pool", provider.LoopProviderType, map[string]interface{}{})
	c.Assert(err, jc.ErrorIsNil)

	// Must create the requested block device prior to SetInstanceInfo.
	volumeTag := s.addVolume(c, state.VolumeParams{Size: 1000, Pool: "loop-pool"}, "123")
	c.Assert(volumeTag, gc.Equals, names.NewVolumeTag("123/0"))

	c.Assert(s.machine.CheckProvisioned("fake_nonce"), jc.IsFalse)
	volumeInfo := state.VolumeInfo{
		VolumeId: "storage-123",
		Size:     1234,
	}
	volumes := map[names.VolumeTag]state.VolumeInfo{volumeTag: volumeInfo}
	err = s.machine.SetInstanceInfo("umbrella/0", "fake_nonce", nil, nil, nil, volumes, nil, nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.machine.CheckProvisioned("fake_nonce"), jc.IsTrue)

	sb, err := state.NewStorageBackend(s.State)
	c.Assert(err, jc.ErrorIsNil)
	volume, err := sb.Volume(volumeTag)
	c.Assert(err, jc.ErrorIsNil)
	info, err := volume.Info()
	c.Assert(err, jc.ErrorIsNil)
	volumeInfo.Pool = "loop-pool" // taken from params
	c.Assert(info, gc.Equals, volumeInfo)
	volumeStatus, err := volume.Status()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(volumeStatus.Status, gc.Equals, status.Attaching)
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

	now := coretesting.ZeroTime()
	sInfo := status.StatusInfo{
		Status:  status.Running,
		Message: "alive",
		Since:   &now,
	}
	err = s.machine.SetInstanceStatus(sInfo)
	c.Assert(err, jc.ErrorIsNil)

	// Reload machine and check result.
	err = s.machine.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	machineStatus, err := s.machine.InstanceStatus()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(machineStatus.Status, gc.DeepEquals, status.Running)
	c.Assert(machineStatus.Message, gc.DeepEquals, "alive")
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

	// Make three machines, three applications and three units for each application;
	// variously assign units to machines and check that Machine.Units
	// tells us the right thing.

	m1 := s.machine
	m2, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	m3, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)

	dummy := s.AddTestingCharm(c, "dummy")
	logging := s.AddTestingCharm(c, "logging")
	s0 := s.AddTestingApplication(c, "s0", dummy)
	s1 := s.AddTestingApplication(c, "s1", dummy)
	s2 := s.AddTestingApplication(c, "s2", dummy)
	s3 := s.AddTestingApplication(c, "s3", logging)

	units := make([][]*state.Unit, 4)
	for i, app := range []*state.Application{s0, s1, s2} {
		units[i] = make([]*state.Unit, 3)
		for j := range units[i] {
			units[i][j], err = app.AddUnit(state.AddUnitParams{})
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

func (s *MachineSuite) assertMachineDirtyAfterAddingUnit(c *gc.C) (*state.Machine, *state.Application, *state.Unit) {
	m, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(m.Clean(), jc.IsTrue)

	app := s.AddTestingApplication(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	unit, err := app.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	err = unit.AssignToMachine(m)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(m.Clean(), jc.IsFalse)
	return m, app, unit
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
	m, app, unit := s.assertMachineDirtyAfterAddingUnit(c)
	err := unit.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
	err = unit.Remove()
	c.Assert(err, jc.ErrorIsNil)
	err = app.Destroy()
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
	//  Application.Watch
	//  Unit.Watch
	//  State.WatchForModelConfigChanges
	//  Unit.WatchConfigSettings
	testWatcherDiesWhenStateCloses(c, s.Session, s.modelTag, s.State.ControllerTag(), func(c *gc.C, st *state.State) waiter {
		m, err := st.Machine(s.machine.Id())
		c.Assert(err, jc.ErrorIsNil)
		w := m.Watch()
		<-w.Changes()
		return w
	})
}

func (s *MachineSuite) TestWatchPrincipalUnits(c *gc.C) {
	// TODO(mjs) - MODELUUID - test with multiple models with
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
	mysql := s.AddTestingApplication(c, "mysql", s.AddTestingCharm(c, "mysql"))
	mysql0, err := mysql.AddUnit(state.AddUnitParams{})
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
	now := coretesting.ZeroTime()
	sInfo := status.StatusInfo{
		Status:  status.Idle,
		Message: "",
		Since:   &now,
	}
	err = mysql0.SetAgentStatus(sInfo)
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertNoChange()

	// Assign another unit and make the first Dying; check both changes detected.
	mysql1, err := mysql.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	err = mysql1.AssignToMachine(machine)
	c.Assert(err, jc.ErrorIsNil)
	err = mysql0.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertChange("mysql/0", "mysql/1")
	wc.AssertNoChange()

	// Add a subordinate to the Alive unit; no change.
	s.AddTestingApplication(c, "logging", s.AddTestingCharm(c, "logging"))
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
	sInfo = status.StatusInfo{
		Status:  status.Idle,
		Message: "",
		Since:   &now,
	}
	err = logging0.SetAgentStatus(sInfo)
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
	testWatcherDiesWhenStateCloses(c, s.Session, s.modelTag, s.State.ControllerTag(), func(c *gc.C, st *state.State) waiter {
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
	mysql := s.AddTestingApplication(c, "mysql", s.AddTestingCharm(c, "mysql"))
	mysql0, err := mysql.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	machine, err := s.State.Machine(s.machine.Id())
	c.Assert(err, jc.ErrorIsNil)
	err = mysql0.AssignToMachine(machine)
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertChange("mysql/0")
	wc.AssertNoChange()

	// Change the unit; no change.
	now := coretesting.ZeroTime()
	sInfo := status.StatusInfo{
		Status:  status.Idle,
		Message: "",
		Since:   &now,
	}
	err = mysql0.SetAgentStatus(sInfo)
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertNoChange()

	// Assign another unit and make the first Dying; check both changes detected.
	mysql1, err := mysql.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	err = mysql1.AssignToMachine(machine)
	c.Assert(err, jc.ErrorIsNil)
	err = mysql0.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertChange("mysql/0", "mysql/1")
	wc.AssertNoChange()

	// Add a subordinate to the Alive unit; change detected.
	s.AddTestingApplication(c, "logging", s.AddTestingCharm(c, "logging"))
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
	sInfo = status.StatusInfo{
		Status:  status.Idle,
		Message: "",
		Since:   &now,
	}
	err = logging0.SetAgentStatus(sInfo)
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
	testWatcherDiesWhenStateCloses(c, s.Session, s.modelTag, s.State.ControllerTag(), func(c *gc.C, st *state.State) waiter {
		m, err := st.Machine(s.machine.Id())
		c.Assert(err, jc.ErrorIsNil)
		w := m.WatchUnits()
		<-w.Changes()
		return w
	})
}

func (s *MachineSuite) TestConstraintsFromModel(c *gc.C) {
	econs1 := constraints.MustParse("mem=1G")
	econs2 := constraints.MustParse("mem=2G")

	// A newly-created machine gets a copy of the model constraints.
	err := s.State.SetModelConstraints(econs1)
	c.Assert(err, jc.ErrorIsNil)
	machine1, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	mcons1, err := machine1.Constraints()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(mcons1, gc.DeepEquals, econs1)

	// Change model constraints and add a new machine.
	err = s.State.SetModelConstraints(econs2)
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
	c.Assert(err, gc.ErrorMatches, `updating machine "2": cannot set constraints: machine is already provisioned`)

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
	c.Assert(err, gc.ErrorMatches, `updating machine "2": cannot set constraints: ambiguous constraints: "instance-type" overlaps with "mem"`)
}

func (s *MachineSuite) TestSetUnsupportedConstraintsWarning(c *gc.C) {
	defer loggo.ResetWriters()
	logger := loggo.GetLogger("test")
	logger.SetLogLevel(loggo.DEBUG)
	var tw loggo.TestWriter
	c.Assert(loggo.RegisterWriter("constraints-tester", &tw), gc.IsNil)

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
	cannotSet := `updating machine "1": cannot set constraints: machine is not found or not alive`
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

func (s *MachineSuite) TestSetProviderAddresses(c *gc.C) {
	machine, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(machine.Addresses(), gc.HasLen, 0)

	addresses := network.NewAddresses("127.0.0.1", "8.8.8.8")
	err = machine.SetProviderAddresses(addresses...)
	c.Assert(err, jc.ErrorIsNil)
	err = machine.Refresh()
	c.Assert(err, jc.ErrorIsNil)

	expectedAddresses := network.NewAddresses("8.8.8.8", "127.0.0.1")
	c.Assert(machine.Addresses(), jc.DeepEquals, expectedAddresses)
}

func (s *MachineSuite) TestSetProviderAddressesWithContainers(c *gc.C) {
	machine, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(machine.Addresses(), gc.HasLen, 0)

	// When setting all addresses the subnet addresses have to be
	// filtered out.
	addresses := network.NewAddresses(
		"127.0.0.1",
		"8.8.8.8",
	)
	err = machine.SetProviderAddresses(addresses...)
	c.Assert(err, jc.ErrorIsNil)
	err = machine.Refresh()
	c.Assert(err, jc.ErrorIsNil)

	expectedAddresses := network.NewAddresses("8.8.8.8", "127.0.0.1")
	c.Assert(machine.Addresses(), jc.DeepEquals, expectedAddresses)
}

func (s *MachineSuite) TestSetProviderAddressesOnContainer(c *gc.C) {
	machine, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(machine.Addresses(), gc.HasLen, 0)

	// Create an LXC container inside the machine.
	template := state.MachineTemplate{
		Series: "quantal",
		Jobs:   []state.MachineJob{state.JobHostUnits},
	}
	container, err := s.State.AddMachineInsideMachine(template, machine.Id(), instance.LXD)
	c.Assert(err, jc.ErrorIsNil)

	// When setting all addresses the subnet address has to accepted.
	addresses := network.NewAddresses("127.0.0.1")
	err = container.SetProviderAddresses(addresses...)
	c.Assert(err, jc.ErrorIsNil)
	err = container.Refresh()
	c.Assert(err, jc.ErrorIsNil)

	expectedAddresses := network.NewAddresses("127.0.0.1")
	c.Assert(container.Addresses(), jc.DeepEquals, expectedAddresses)
}

func (s *MachineSuite) TestSetMachineAddresses(c *gc.C) {
	machine, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(machine.Addresses(), gc.HasLen, 0)

	addresses := network.NewAddresses("127.0.0.1", "8.8.8.8")
	err = machine.SetMachineAddresses(addresses...)
	c.Assert(err, jc.ErrorIsNil)
	err = machine.Refresh()
	c.Assert(err, jc.ErrorIsNil)

	expectedAddresses := network.NewAddresses("8.8.8.8", "127.0.0.1")
	c.Assert(machine.MachineAddresses(), jc.DeepEquals, expectedAddresses)
}

func (s *MachineSuite) TestSetEmptyMachineAddresses(c *gc.C) {
	machine, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(machine.Addresses(), gc.HasLen, 0)

	// Add some machine addresses initially to make sure they're removed.
	addresses := network.NewAddresses("127.0.0.1", "8.8.8.8")
	err = machine.SetMachineAddresses(addresses...)
	c.Assert(err, jc.ErrorIsNil)
	err = machine.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(machine.MachineAddresses(), gc.HasLen, 2)

	// Make call with empty address list.
	err = machine.SetMachineAddresses()
	c.Assert(err, jc.ErrorIsNil)
	err = machine.Refresh()
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(machine.MachineAddresses(), gc.HasLen, 0)
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
	err = machine.SetProviderAddresses(providerAddresses...)
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
}

func (s *MachineSuite) TestSetProviderAddressesConcurrentChangeDifferent(c *gc.C) {
	machine, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(machine.Addresses(), gc.HasLen, 0)

	addr0 := network.NewAddress("127.0.0.1")
	addr1 := network.NewAddress("8.8.8.8")

	defer state.SetBeforeHooks(c, s.State, func() {
		machine, err := s.State.Machine(machine.Id())
		c.Assert(err, jc.ErrorIsNil)
		err = machine.SetProviderAddresses(addr1, addr0)
		c.Assert(err, jc.ErrorIsNil)
	}).Check()

	err = machine.SetProviderAddresses(addr0, addr1)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(machine.Addresses(), jc.SameContents, []network.Address{addr0, addr1})
}

func (s *MachineSuite) TestSetProviderAddressesConcurrentChangeEqual(c *gc.C) {
	machine, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(machine.Addresses(), gc.HasLen, 0)
	machineDocID := state.DocID(s.State, machine.Id())
	revno0, err := state.TxnRevno(s.State, "machines", machineDocID)
	c.Assert(err, jc.ErrorIsNil)

	addr0 := network.NewAddress("127.0.0.1")
	addr1 := network.NewAddress("8.8.8.8")

	var revno1 int64
	defer state.SetBeforeHooks(c, s.State, func() {
		machine, err := s.State.Machine(machine.Id())
		c.Assert(err, jc.ErrorIsNil)
		err = machine.SetProviderAddresses(addr0, addr1)
		c.Assert(err, jc.ErrorIsNil)
		revno1, err = state.TxnRevno(s.State, "machines", machineDocID)
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(revno1, jc.GreaterThan, revno0)
	}).Check()

	err = machine.SetProviderAddresses(addr0, addr1)
	c.Assert(err, jc.ErrorIsNil)

	// Doc will be updated; concurrent changes are explicitly ignored.
	revno2, err := state.TxnRevno(s.State, "machines", machineDocID)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(revno2, jc.GreaterThan, revno1)
	c.Assert(machine.Addresses(), jc.SameContents, []network.Address{addr0, addr1})
}

func (s *MachineSuite) TestSetProviderAddressesInvalidateMemory(c *gc.C) {
	machine, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(machine.Addresses(), gc.HasLen, 0)
	machineDocID := state.DocID(s.State, machine.Id())

	addr0 := network.NewAddress("127.0.0.1")
	addr1 := network.NewAddress("8.8.8.8")

	// Set addresses to [addr0] initially. We'll get a separate Machine
	// object to update addresses, to ensure that the in-memory cache of
	// addresses does not prevent the initial Machine from updating
	// addresses back to the original value.
	err = machine.SetProviderAddresses(addr0)
	c.Assert(err, jc.ErrorIsNil)
	revno0, err := state.TxnRevno(s.State, "machines", machineDocID)
	c.Assert(err, jc.ErrorIsNil)

	machine2, err := s.State.Machine(machine.Id())
	c.Assert(err, jc.ErrorIsNil)
	err = machine2.SetProviderAddresses(addr1)
	c.Assert(err, jc.ErrorIsNil)
	revno1, err := state.TxnRevno(s.State, "machines", machineDocID)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(revno1, jc.GreaterThan, revno0)
	c.Assert(machine.Addresses(), jc.SameContents, []network.Address{addr0})
	c.Assert(machine2.Addresses(), jc.SameContents, []network.Address{addr1})

	err = machine.SetProviderAddresses(addr0)
	c.Assert(err, jc.ErrorIsNil)
	revno2, err := state.TxnRevno(s.State, "machines", machineDocID)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(revno2, jc.GreaterThan, revno1)
	c.Assert(machine.Addresses(), jc.SameContents, []network.Address{addr0})
}

func (s *MachineSuite) TestPublicAddressSetOnNewMachine(c *gc.C) {
	m, err := s.State.AddOneMachine(state.MachineTemplate{
		Series:    "quantal",
		Jobs:      []state.MachineJob{state.JobHostUnits},
		Addresses: network.NewAddresses("10.0.0.1", "8.8.8.8"),
	})
	c.Assert(err, jc.ErrorIsNil)
	addr, err := m.PublicAddress()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(addr, jc.DeepEquals, network.NewAddress("8.8.8.8"))
}

func (s *MachineSuite) TestPrivateAddressSetOnNewMachine(c *gc.C) {
	m, err := s.State.AddOneMachine(state.MachineTemplate{
		Series:    "quantal",
		Jobs:      []state.MachineJob{state.JobHostUnits},
		Addresses: network.NewAddresses("10.0.0.1", "8.8.8.8"),
	})
	c.Assert(err, jc.ErrorIsNil)
	addr, err := m.PrivateAddress()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(addr, jc.DeepEquals, network.NewAddress("10.0.0.1"))
}

func (s *MachineSuite) TestPublicAddressEmptyAddresses(c *gc.C) {
	machine, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(machine.Addresses(), gc.HasLen, 0)

	addr, err := machine.PublicAddress()
	c.Assert(err, jc.Satisfies, network.IsNoAddressError)
	c.Assert(addr.Value, gc.Equals, "")
}

func (s *MachineSuite) TestPrivateAddressEmptyAddresses(c *gc.C) {
	machine, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(machine.Addresses(), gc.HasLen, 0)

	addr, err := machine.PrivateAddress()
	c.Assert(err, jc.Satisfies, network.IsNoAddressError)
	c.Assert(addr.Value, gc.Equals, "")
}

func (s *MachineSuite) TestPublicAddress(c *gc.C) {
	machine, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)

	err = machine.SetProviderAddresses(network.NewAddress("8.8.8.8"))
	c.Assert(err, jc.ErrorIsNil)

	addr, err := machine.PublicAddress()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(addr.Value, gc.Equals, "8.8.8.8")
}

func (s *MachineSuite) TestPrivateAddress(c *gc.C) {
	machine, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)

	err = machine.SetMachineAddresses(network.NewAddress("10.0.0.1"))
	c.Assert(err, jc.ErrorIsNil)

	addr, err := machine.PrivateAddress()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(addr.Value, gc.Equals, "10.0.0.1")
}

func (s *MachineSuite) TestPublicAddressBetterMatch(c *gc.C) {
	machine, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)

	err = machine.SetMachineAddresses(network.NewAddress("10.0.0.1"))
	c.Assert(err, jc.ErrorIsNil)

	addr, err := machine.PublicAddress()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(addr.Value, gc.Equals, "10.0.0.1")

	err = machine.SetProviderAddresses(network.NewAddress("8.8.8.8"))
	c.Assert(err, jc.ErrorIsNil)

	addr, err = machine.PublicAddress()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(addr.Value, gc.Equals, "8.8.8.8")
}

func (s *MachineSuite) TestPrivateAddressBetterMatch(c *gc.C) {
	machine, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)

	err = machine.SetProviderAddresses(network.NewAddress("8.8.8.8"))
	c.Assert(err, jc.ErrorIsNil)

	addr, err := machine.PrivateAddress()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(addr.Value, gc.Equals, "8.8.8.8")

	err = machine.SetProviderAddresses(network.NewAddress("8.8.8.8"), network.NewAddress("10.0.0.1"))
	c.Assert(err, jc.ErrorIsNil)

	addr, err = machine.PrivateAddress()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(addr.Value, gc.Equals, "10.0.0.1")
}

func (s *MachineSuite) TestPublicAddressChanges(c *gc.C) {
	machine, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)

	err = machine.SetProviderAddresses(network.NewAddress("8.8.8.8"))
	c.Assert(err, jc.ErrorIsNil)

	addr, err := machine.PublicAddress()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(addr.Value, gc.Equals, "8.8.8.8")

	err = machine.SetProviderAddresses(network.NewAddress("8.8.4.4"))
	c.Assert(err, jc.ErrorIsNil)

	addr, err = machine.PublicAddress()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(addr.Value, gc.Equals, "8.8.4.4")
}

func (s *MachineSuite) TestPrivateAddressChanges(c *gc.C) {
	machine, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)

	err = machine.SetMachineAddresses(network.NewAddress("10.0.0.2"))
	c.Assert(err, jc.ErrorIsNil)

	addr, err := machine.PrivateAddress()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(addr.Value, gc.Equals, "10.0.0.2")

	err = machine.SetMachineAddresses(network.NewAddress("10.0.0.1"))
	c.Assert(err, jc.ErrorIsNil)

	addr, err = machine.PrivateAddress()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(addr.Value, gc.Equals, "10.0.0.1")
}

func (s *MachineSuite) TestAddressesDeadMachine(c *gc.C) {
	machine, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)

	err = machine.SetProviderAddresses(network.NewAddress("10.0.0.2"), network.NewAddress("8.8.4.4"))
	c.Assert(err, jc.ErrorIsNil)

	addr, err := machine.PrivateAddress()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(addr.Value, gc.Equals, "10.0.0.2")

	addr, err = machine.PublicAddress()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(addr.Value, gc.Equals, "8.8.4.4")

	err = machine.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)

	// A dead machine should still report the last known addresses.
	addr, err = machine.PrivateAddress()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(addr.Value, gc.Equals, "10.0.0.2")

	addr, err = machine.PublicAddress()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(addr.Value, gc.Equals, "8.8.4.4")
}

func (s *MachineSuite) TestStablePrivateAddress(c *gc.C) {
	machine, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)

	err = machine.SetMachineAddresses(network.NewAddress("10.0.0.2"))
	c.Assert(err, jc.ErrorIsNil)

	addr, err := machine.PrivateAddress()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(addr.Value, gc.Equals, "10.0.0.2")

	// Now add an address that would previously have sorted before the
	// default.
	err = machine.SetMachineAddresses(network.NewAddress("10.0.0.1"), network.NewAddress("10.0.0.2"))
	c.Assert(err, jc.ErrorIsNil)

	// Assert the address is unchanged.
	addr, err = machine.PrivateAddress()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(addr.Value, gc.Equals, "10.0.0.2")
}

func (s *MachineSuite) TestStablePublicAddress(c *gc.C) {
	machine, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)

	err = machine.SetProviderAddresses(network.NewAddress("8.8.8.8"))
	c.Assert(err, jc.ErrorIsNil)

	addr, err := machine.PublicAddress()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(addr.Value, gc.Equals, "8.8.8.8")

	// Now add an address that would previously have sorted before the
	// default.
	err = machine.SetProviderAddresses(network.NewAddress("8.8.4.4"), network.NewAddress("8.8.8.8"))
	c.Assert(err, jc.ErrorIsNil)

	// Assert the address is unchanged.
	addr, err = machine.PublicAddress()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(addr.Value, gc.Equals, "8.8.8.8")
}

func (s *MachineSuite) TestAddressesRaceMachineFirst(c *gc.C) {
	machine, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)

	changeAddresses := jujutxn.TestHook{
		Before: func() {
			err = machine.SetProviderAddresses(network.NewAddress("8.8.8.8"))
			c.Assert(err, jc.ErrorIsNil)
			address, err := machine.PublicAddress()
			c.Assert(err, jc.ErrorIsNil)
			c.Assert(address, jc.DeepEquals, network.NewAddress("8.8.8.8"))
			address, err = machine.PrivateAddress()
			c.Assert(err, jc.ErrorIsNil)
			c.Assert(address, jc.DeepEquals, network.NewAddress("8.8.8.8"))
		},
	}
	defer state.SetTestHooks(c, s.State, changeAddresses).Check()

	err = machine.SetMachineAddresses(network.NewAddress("8.8.4.4"))
	c.Assert(err, jc.ErrorIsNil)

	machine, err = s.State.Machine(machine.Id())
	c.Assert(err, jc.ErrorIsNil)
	address, err := machine.PublicAddress()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(address, jc.DeepEquals, network.NewAddress("8.8.8.8"))
	address, err = machine.PrivateAddress()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(address, jc.DeepEquals, network.NewAddress("8.8.8.8"))
}

func (s *MachineSuite) TestAddressesRaceProviderFirst(c *gc.C) {
	machine, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)

	changeAddresses := jujutxn.TestHook{
		Before: func() {
			err = machine.SetMachineAddresses(network.NewAddress("10.0.0.1"))
			c.Assert(err, jc.ErrorIsNil)
			address, err := machine.PublicAddress()
			c.Assert(err, jc.ErrorIsNil)
			c.Assert(address, jc.DeepEquals, network.NewAddress("10.0.0.1"))
			address, err = machine.PrivateAddress()
			c.Assert(err, jc.ErrorIsNil)
			c.Assert(address, jc.DeepEquals, network.NewAddress("10.0.0.1"))
		},
	}
	defer state.SetTestHooks(c, s.State, changeAddresses).Check()

	err = machine.SetProviderAddresses(network.NewAddress("8.8.4.4"))
	c.Assert(err, jc.ErrorIsNil)

	machine, err = s.State.Machine(machine.Id())
	c.Assert(err, jc.ErrorIsNil)
	address, err := machine.PublicAddress()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(address, jc.DeepEquals, network.NewAddress("8.8.4.4"))
	address, err = machine.PrivateAddress()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(address, jc.DeepEquals, network.NewAddress("8.8.4.4"))
}

func (s *MachineSuite) TestPrivateAddressPrefersProvider(c *gc.C) {
	machine, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)

	err = machine.SetMachineAddresses(network.NewAddress("8.8.8.8"), network.NewAddress("10.0.0.2"))
	c.Assert(err, jc.ErrorIsNil)

	addr, err := machine.PublicAddress()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(addr.Value, gc.Equals, "8.8.8.8")
	addr, err = machine.PrivateAddress()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(addr.Value, gc.Equals, "10.0.0.2")

	err = machine.SetProviderAddresses(network.NewAddress("10.0.0.1"))
	c.Assert(err, jc.ErrorIsNil)

	addr, err = machine.PublicAddress()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(addr.Value, gc.Equals, "10.0.0.1")
	addr, err = machine.PrivateAddress()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(addr.Value, gc.Equals, "10.0.0.1")
}

func (s *MachineSuite) TestPublicAddressPrefersProvider(c *gc.C) {
	machine, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)

	err = machine.SetMachineAddresses(network.NewAddress("8.8.8.8"), network.NewAddress("10.0.0.2"))
	c.Assert(err, jc.ErrorIsNil)

	addr, err := machine.PublicAddress()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(addr.Value, gc.Equals, "8.8.8.8")
	addr, err = machine.PrivateAddress()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(addr.Value, gc.Equals, "10.0.0.2")

	err = machine.SetProviderAddresses(network.NewAddress("8.8.4.4"))
	c.Assert(err, jc.ErrorIsNil)

	addr, err = machine.PublicAddress()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(addr.Value, gc.Equals, "8.8.4.4")
	addr, err = machine.PrivateAddress()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(addr.Value, gc.Equals, "8.8.4.4")
}

func (s *MachineSuite) TestAddressesPrefersProviderBoth(c *gc.C) {
	machine, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)

	err = machine.SetMachineAddresses(network.NewAddress("8.8.8.8"), network.NewAddress("10.0.0.1"))
	c.Assert(err, jc.ErrorIsNil)

	addr, err := machine.PublicAddress()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(addr.Value, gc.Equals, "8.8.8.8")
	addr, err = machine.PrivateAddress()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(addr.Value, gc.Equals, "10.0.0.1")

	err = machine.SetProviderAddresses(network.NewAddress("8.8.4.4"), network.NewAddress("10.0.0.2"))
	c.Assert(err, jc.ErrorIsNil)

	addr, err = machine.PublicAddress()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(addr.Value, gc.Equals, "8.8.4.4")
	addr, err = machine.PrivateAddress()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(addr.Value, gc.Equals, "10.0.0.2")
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

	err = machine.SetSupportedContainers([]instance.ContainerType{instance.LXD, instance.NONE})
	c.Assert(err, gc.ErrorMatches, `"none" is not a valid container type`)
	assertSupportedContainersUnknown(c, machine)
	err = machine.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	assertSupportedContainersUnknown(c, machine)
}

func (s *MachineSuite) TestSupportsNoContainersOverwritesExisting(c *gc.C) {
	machine := s.addMachineWithSupportedContainer(c, instance.LXD)

	err := machine.SupportsNoContainers()
	c.Assert(err, jc.ErrorIsNil)
	assertSupportedContainers(c, machine, []instance.ContainerType{})
}

func (s *MachineSuite) TestSetSupportedContainersSingle(c *gc.C) {
	machine, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)

	err = machine.SetSupportedContainers([]instance.ContainerType{instance.LXD})
	c.Assert(err, jc.ErrorIsNil)
	assertSupportedContainers(c, machine, []instance.ContainerType{instance.LXD})
}

func (s *MachineSuite) TestSetSupportedContainersSame(c *gc.C) {
	machine := s.addMachineWithSupportedContainer(c, instance.LXD)

	err := machine.SetSupportedContainers([]instance.ContainerType{instance.LXD})
	c.Assert(err, jc.ErrorIsNil)
	assertSupportedContainers(c, machine, []instance.ContainerType{instance.LXD})
}

func (s *MachineSuite) TestSetSupportedContainersNew(c *gc.C) {
	machine := s.addMachineWithSupportedContainer(c, instance.LXD)

	err := machine.SetSupportedContainers([]instance.ContainerType{instance.LXD, instance.KVM})
	c.Assert(err, jc.ErrorIsNil)
	assertSupportedContainers(c, machine, []instance.ContainerType{instance.LXD, instance.KVM})
}

func (s *MachineSuite) TestSetSupportedContainersMultipeNew(c *gc.C) {
	machine, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)

	err = machine.SetSupportedContainers([]instance.ContainerType{instance.LXD, instance.KVM})
	c.Assert(err, jc.ErrorIsNil)
	assertSupportedContainers(c, machine, []instance.ContainerType{instance.LXD, instance.KVM})
}

func (s *MachineSuite) TestSetSupportedContainersMultipleExisting(c *gc.C) {
	machine := s.addMachineWithSupportedContainer(c, instance.LXD)

	err := machine.SetSupportedContainers([]instance.ContainerType{instance.LXD, instance.KVM})
	c.Assert(err, jc.ErrorIsNil)
	assertSupportedContainers(c, machine, []instance.ContainerType{instance.LXD, instance.KVM})
}

func (s *MachineSuite) TestSetSupportedContainersSetsUnknownToError(c *gc.C) {
	// Create a machine and add lxd and kvm containers prior to calling SetSupportedContainers
	machine, err := s.State.AddMachine("quantal", state.JobHostUnits)
	template := state.MachineTemplate{
		Series: "quantal",
		Jobs:   []state.MachineJob{state.JobHostUnits},
	}
	container, err := s.State.AddMachineInsideMachine(template, machine.Id(), instance.LXD)
	c.Assert(err, jc.ErrorIsNil)
	supportedContainer, err := s.State.AddMachineInsideMachine(template, machine.Id(), instance.KVM)
	c.Assert(err, jc.ErrorIsNil)
	err = machine.SetSupportedContainers([]instance.ContainerType{instance.KVM})
	c.Assert(err, jc.ErrorIsNil)

	// A supported (kvm) container will have a pending status.
	err = supportedContainer.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	statusInfo, err := supportedContainer.Status()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(statusInfo.Status, gc.Equals, status.Pending)

	// An unsupported (lxd) container will have an error status.
	err = container.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	statusInfo, err = container.Status()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(statusInfo.Status, gc.Equals, status.Error)
	c.Assert(statusInfo.Message, gc.Equals, "unsupported container")
	c.Assert(statusInfo.Data, gc.DeepEquals, map[string]interface{}{"type": "lxd"})
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
		statusInfo, err := container.Status()
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(statusInfo.Status, gc.Equals, status.Error)
		c.Assert(statusInfo.Message, gc.Equals, "unsupported container")
		containerType := state.ContainerTypeFromId(container.Id())
		c.Assert(statusInfo.Data, gc.DeepEquals, map[string]interface{}{"type": string(containerType)})
	}
}

func (s *MachineSuite) TestMachineAgentTools(c *gc.C) {
	m, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	testAgentTools(c, m, "machine "+m.Id())
}

func (s *MachineSuite) TestMachineValidActions(c *gc.C) {
	m, err := s.State.AddMachine("trusty", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)

	var tests = []struct {
		actionName      string
		errString       string
		givenPayload    map[string]interface{}
		expectedPayload map[string]interface{}
	}{
		{
			actionName: "juju-run",
			errString:  `validation failed: (root) : "command" property is missing and required, given {}; (root) : "timeout" property is missing and required, given {}`,
		},
		{
			actionName:      "juju-run",
			givenPayload:    map[string]interface{}{"command": "allyourbasearebelongtous", "timeout": 5.0},
			expectedPayload: map[string]interface{}{"command": "allyourbasearebelongtous", "timeout": 5.0},
		},
		{
			actionName: "baiku",
			errString:  `cannot add action "baiku" to a machine; only predefined actions allowed`,
		},
	}

	for i, t := range tests {
		c.Logf("running test %d", i)
		action, err := m.AddAction(t.actionName, t.givenPayload)
		if t.errString != "" {
			c.Assert(err.Error(), gc.Equals, t.errString)
			continue
		} else {
			c.Assert(err, jc.ErrorIsNil)
			c.Assert(action.Parameters(), jc.DeepEquals, t.expectedPayload)
		}
	}
}

func (s *MachineSuite) TestMachineAddDifferentAction(c *gc.C) {
	m, err := s.State.AddMachine("trusty", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)

	_, err = m.AddAction("benchmark", nil)
	c.Assert(err, gc.ErrorMatches, `cannot add action "benchmark" to a machine; only predefined actions allowed`)
}

func (s *MachineSuite) setupTestUpdateMachineSeries(c *gc.C) *state.Machine {
	mach, err := s.State.AddMachine("precise", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)

	ch := state.AddTestingCharmMultiSeries(c, s.State, "multi-series")
	app := state.AddTestingApplicationForSeries(c, s.State, "precise", "multi-series", ch)
	subCh := state.AddTestingCharmMultiSeries(c, s.State, "multi-series-subordinate")
	_ = state.AddTestingApplicationForSeries(c, s.State, "precise", "multi-series-subordinate", subCh)

	eps, err := s.State.InferEndpoints("multi-series", "multi-series-subordinate")
	c.Assert(err, jc.ErrorIsNil)
	rel, err := s.State.AddRelation(eps...)
	c.Assert(err, jc.ErrorIsNil)

	unit, err := app.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	err = unit.AssignToMachine(mach)
	c.Assert(err, jc.ErrorIsNil)

	ru, err := rel.Unit(unit)
	c.Assert(err, jc.ErrorIsNil)
	err = ru.EnterScope(nil)
	c.Assert(err, jc.ErrorIsNil)

	ch2 := state.AddTestingCharmMultiSeries(c, s.State, "wordpress")
	app2 := state.AddTestingApplicationForSeries(c, s.State, "precise", "wordpress", ch2)
	unit2, err := app2.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	err = unit2.AssignToMachine(mach)
	c.Assert(err, jc.ErrorIsNil)

	return mach
}

func (s *MachineSuite) assertMachineAndUnitSeriesChanged(c *gc.C, mach *state.Machine, series string) {
	err := mach.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(mach.Series(), gc.Equals, series)
	principals := mach.Principals()
	for _, p := range principals {
		u, err := s.State.Unit(p)
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(u.Series(), gc.Equals, series)
		subs := u.SubordinateNames()
		for _, sn := range subs {
			u, err := s.State.Unit(sn)
			c.Assert(err, jc.ErrorIsNil)
			c.Assert(u.Series(), gc.Equals, series)
		}
	}
}

func (s *MachineSuite) TestUpdateMachineSeries(c *gc.C) {
	mach := s.setupTestUpdateMachineSeries(c)
	err := mach.UpdateMachineSeries("trusty", false)
	c.Assert(err, jc.ErrorIsNil)
	s.assertMachineAndUnitSeriesChanged(c, mach, "trusty")
}

func (s *MachineSuite) TestUpdateMachineSeriesFail(c *gc.C) {
	mach := s.setupTestUpdateMachineSeries(c)
	err := mach.UpdateMachineSeries("xenial", false)
	c.Assert(err, jc.Satisfies, state.IsIncompatibleSeriesError)
	s.assertMachineAndUnitSeriesChanged(c, mach, "precise")
}

func (s *MachineSuite) TestUpdateMachineSeriesForce(c *gc.C) {
	mach := s.setupTestUpdateMachineSeries(c)
	err := mach.UpdateMachineSeries("xenial", true)
	c.Assert(err, jc.ErrorIsNil)
	s.assertMachineAndUnitSeriesChanged(c, mach, "xenial")
}

func (s *MachineSuite) TestUpdateMachineSeriesSameSeriesToStart(c *gc.C) {
	mach := s.setupTestUpdateMachineSeries(c)
	err := mach.UpdateMachineSeries("precise", false)
	c.Assert(err, jc.ErrorIsNil)
	s.assertMachineAndUnitSeriesChanged(c, mach, "precise")
}

func (s *MachineSuite) TestUpdateMachineSeriesSameSeriesAfterStart(c *gc.C) {
	mach := s.setupTestUpdateMachineSeries(c)

	defer state.SetTestHooks(c, s.State,
		jujutxn.TestHook{
			Before: func() {
				ops := []txn.Op{{
					C:      state.MachinesC,
					Id:     state.DocID(s.State, mach.Id()),
					Update: bson.D{{"$set", bson.D{{"series", "trusty"}}}},
				}}
				err := state.RunTransaction(s.State, ops)
				c.Assert(err, jc.ErrorIsNil)
			},
			After: func() {
				err := mach.Refresh()
				c.Assert(err, jc.ErrorIsNil)
				c.Assert(mach.Series(), gc.Equals, "trusty")
			},
		},
	).Check()

	err := mach.UpdateMachineSeries("trusty", false)
	c.Assert(err, jc.ErrorIsNil)
	err = mach.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(mach.Series(), gc.Equals, "trusty")
}

func (s *MachineSuite) TestUpdateMachineSeriesCharmURLChangedSeriesFail(c *gc.C) {
	mach := s.setupTestUpdateMachineSeries(c)

	defer state.SetTestHooks(c, s.State,
		jujutxn.TestHook{
			Before: func() {
				v2 := state.AddTestingCharmMultiSeries(c, s.State, "multi-seriesv2")
				cfg := state.SetCharmConfig{Charm: v2}
				app, err := s.State.Application("multi-series")
				c.Assert(err, jc.ErrorIsNil)
				err = app.SetCharm(cfg)
				c.Assert(err, jc.ErrorIsNil)
			},
		},
	).Check()

	// Trusty is listed in only version 1 of the charm.
	err := mach.UpdateMachineSeries("trusty", false)
	c.Assert(err, gc.ErrorMatches, "cannot update series for \"2\" to trusty: series \"trusty\" not supported by charm, supported series are: precise,xenial")
}

func (s *MachineSuite) TestUpdateMachineSeriesPrincipalsListChange(c *gc.C) {
	mach := s.setupTestUpdateMachineSeries(c)
	err := mach.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(mach.Principals()), gc.Equals, 2)

	defer state.SetTestHooks(c, s.State,
		jujutxn.TestHook{
			Before: func() {
				app, err := s.State.Application("wordpress")
				c.Assert(err, jc.ErrorIsNil)
				unit, err := app.AddUnit(state.AddUnitParams{})
				c.Assert(err, jc.ErrorIsNil)
				err = unit.AssignToMachine(mach)
				c.Assert(err, jc.ErrorIsNil)
			},
		},
	).Check()

	err = mach.UpdateMachineSeries("trusty", false)
	c.Assert(err, jc.ErrorIsNil)
	s.assertMachineAndUnitSeriesChanged(c, mach, "trusty")
	c.Assert(len(mach.Principals()), gc.Equals, 3)
}

func (s *MachineSuite) TestUpdateMachineSeriesSubordinateListChangeIncompatibleSeries(c *gc.C) {
	mach := s.setupTestUpdateMachineSeries(c)
	err := mach.Refresh()

	unit, err := s.State.Unit("multi-series/0")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(unit.SubordinateNames(), gc.DeepEquals, []string{"multi-series-subordinate/0"})

	defer state.SetTestHooks(c, s.State,
		jujutxn.TestHook{
			Before: func() {
				subCh2 := state.AddTestingCharmMultiSeries(c, s.State, "multi-series-subordinate2")
				subApp2 := state.AddTestingApplicationForSeries(c, s.State, "precise", "multi-series-subordinate2", subCh2)
				c.Assert(subApp2.Series(), gc.Equals, "precise")

				eps, err := s.State.InferEndpoints("multi-series", "multi-series-subordinate2")
				c.Assert(err, jc.ErrorIsNil)
				rel, err := s.State.AddRelation(eps...)
				c.Assert(err, jc.ErrorIsNil)

				err = unit.Refresh()
				c.Assert(err, jc.ErrorIsNil)
				relUnit, err := rel.Unit(unit)
				c.Assert(err, jc.ErrorIsNil)
				err = relUnit.EnterScope(nil)
				c.Assert(err, jc.ErrorIsNil)
			},
		},
	).Check()

	err = mach.UpdateMachineSeries("yakkety", false)
	c.Assert(err, jc.Satisfies, state.IsIncompatibleSeriesError)
	s.assertMachineAndUnitSeriesChanged(c, mach, "precise")
}

func (s *MachineSuite) addMachineUnit(c *gc.C, mach *state.Machine) *state.Unit {
	units, err := mach.Units()
	c.Assert(err, jc.ErrorIsNil)

	var app *state.Application
	if len(units) == 0 {
		ch := state.AddTestingCharmMultiSeries(c, s.State, "multi-series")
		app = state.AddTestingApplicationForSeries(c, s.State, mach.Series(), "multi-series", ch)
		subCh := state.AddTestingCharmMultiSeries(c, s.State, "multi-series-subordinate")
		_ = state.AddTestingApplicationForSeries(c, s.State, mach.Series(), "multi-series-subordinate", subCh)
	} else {
		app, err = units[0].Application()
	}

	unit, err := app.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	err = unit.AssignToMachine(mach)
	c.Assert(err, jc.ErrorIsNil)
	return unit
}

// VerifyUnitsSeries is also tested via TestUpdateMachineSeries*
func (s *MachineSuite) TestVerifyUnitsSeries(c *gc.C) {
	mach := s.setupTestUpdateMachineSeries(c)
	err := mach.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	expectedUnits, err := mach.Units()
	c.Assert(err, jc.ErrorIsNil)
	obtainedUnits, err := mach.VerifyUnitsSeries([]string{"wordpress/0", "multi-series/0"}, "trusty", false)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(unitNames(obtainedUnits), jc.SameContents, unitNames(expectedUnits))
}

func unitNames(units []*state.Unit) []string {
	names := make([]string, len(units))
	for i := range units {
		names[i] = units[i].Name()
	}
	return names
}
