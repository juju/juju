// Copyright 2014-2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"bytes"
	"sort"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v6"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/caas"
	"github.com/juju/juju/constraints"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/resource/resourcetesting"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/stateenvirons"
	"github.com/juju/juju/state/storage"
	"github.com/juju/juju/state/testing"
	corestorage "github.com/juju/juju/storage"
	"github.com/juju/juju/testing/factory"
)

type CleanupSuite struct {
	ConnSuite
	storageBackend *state.StorageBackend
}

var _ = gc.Suite(&CleanupSuite{})

func (s *CleanupSuite) SetUpTest(c *gc.C) {
	s.ConnSuite.SetUpTest(c)
	s.assertDoesNotNeedCleanup(c)
	var err error
	s.storageBackend, err = state.NewStorageBackend(s.State)
	c.Assert(err, jc.ErrorIsNil)

}

func (s *CleanupSuite) TestCleanupDyingApplicationUnits(c *gc.C) {
	// Create a application with some units.
	mysql := s.AddTestingApplication(c, "mysql", s.AddTestingCharm(c, "mysql"))
	units := make([]*state.Unit, 3)
	for i := range units {
		unit, err := mysql.AddUnit(state.AddUnitParams{})
		c.Assert(err, jc.ErrorIsNil)
		units[i] = unit
	}
	preventUnitDestroyRemove(c, units[0])
	s.assertDoesNotNeedCleanup(c)

	// Destroy the application and check the units are unaffected, but a cleanup
	// has been scheduled.
	err := mysql.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	for _, unit := range units {
		err := unit.Refresh()
		c.Assert(err, jc.ErrorIsNil)
	}
	s.assertNeedsCleanup(c)

	// Run the cleanup, and check that units are all destroyed as appropriate.
	s.assertCleanupRuns(c)
	err = units[0].Refresh()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(units[0].Life(), gc.Equals, state.Dying)
	err = units[1].Refresh()
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
	err = units[2].Refresh()
	c.Assert(err, jc.Satisfies, errors.IsNotFound)

	// Run a final cleanup to clear the cleanup scheduled for the unit that
	// became dying.
	s.assertCleanupCount(c, 1)
}

func (s *CleanupSuite) TestCleanupDyingApplicationCharm(c *gc.C) {
	// Create a application and a charm.
	ch := s.AddTestingCharm(c, "mysql")
	mysql := s.AddTestingApplication(c, "mysql", ch)

	// Create a dummy archive blob.
	stor := storage.NewStorage(s.State.ModelUUID(), s.State.MongoSession())
	storagePath := "dummy-path"
	err := stor.Put(storagePath, bytes.NewReader([]byte("data")), 4)
	c.Assert(err, jc.ErrorIsNil)

	// Destroy the application and check that a cleanup has been scheduled.
	err = mysql.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	s.assertNeedsCleanup(c)

	// Run the cleanup, and check that the charm is removed.
	s.assertCleanupRuns(c)
	_, _, err = stor.Get(storagePath)
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *CleanupSuite) TestCleanupRemoteApplication(c *gc.C) {
	app, err := s.State.AddRemoteApplication(state.AddRemoteApplicationParams{
		Name:        "remote-app",
		SourceModel: names.NewModelTag("test"),
		Token:       "token",
	})
	c.Assert(err, jc.ErrorIsNil)

	err = app.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	// Removed immediately since there are no relations yet.
	s.assertDoesNotNeedCleanup(c)
	_, err = s.State.RemoteApplication("remote-app")
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *CleanupSuite) TestCleanupRemoteApplicationWithRelations(c *gc.C) {
	mysqlEps := []charm.Relation{
		{
			Interface: "mysql",
			Name:      "db",
			Role:      charm.RoleProvider,
			Scope:     charm.ScopeGlobal,
		},
	}
	remoteApp, err := s.State.AddRemoteApplication(state.AddRemoteApplicationParams{
		Name:        "mysql",
		SourceModel: s.Model.ModelTag(),
		Token:       "t0",
		Endpoints:   mysqlEps,
	})
	c.Assert(err, jc.ErrorIsNil)

	s.AddTestingApplication(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	eps, err := s.State.InferEndpoints("wordpress", "mysql")
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.State.AddRelation(eps[0], eps[1])
	c.Assert(err, jc.ErrorIsNil)

	err = remoteApp.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	s.assertNeedsCleanup(c)

	// Run the cleanup, and check that the remote app is removed.
	s.assertCleanupRuns(c)
	_, err = s.State.RemoteApplication("mysql")
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *CleanupSuite) TestCleanupControllerModels(c *gc.C) {
	s.assertDoesNotNeedCleanup(c)

	// Create a non-empty hosted model.
	otherSt := s.Factory.MakeModel(c, nil)
	defer otherSt.Close()
	factory.NewFactory(otherSt, s.StatePool).MakeApplication(c, nil)
	otherModel, err := otherSt.Model()
	c.Assert(err, jc.ErrorIsNil)

	s.assertDoesNotNeedCleanup(c)

	// Destroy the controller and check the model is unaffected, but a
	// cleanup for the model and applications has been scheduled.
	controllerModel, err := s.State.Model()
	c.Assert(err, jc.ErrorIsNil)
	err = controllerModel.Destroy(state.DestroyModelParams{
		DestroyHostedModels: true,
	})
	c.Assert(err, jc.ErrorIsNil)

	// Two cleanups should be scheduled. One to destroy the hosted
	// models, the other to destroy the controller model's
	// applications.
	s.assertCleanupCount(c, 1)
	err = otherModel.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(otherModel.Life(), gc.Equals, state.Dying)

	s.assertDoesNotNeedCleanup(c)
}

func (s *CleanupSuite) TestCleanupModelMachines(c *gc.C) {
	// Create a controller machine, and manual and non-manual
	// workload machine.
	stateMachine, err := s.State.AddMachine("quantal", state.JobManageModel)
	c.Assert(err, jc.ErrorIsNil)
	machine, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	manualMachine, err := s.State.AddOneMachine(state.MachineTemplate{
		Series:     "quantal",
		Jobs:       []state.MachineJob{state.JobHostUnits},
		InstanceId: "inst-ance",
		Nonce:      "manual:foo",
	})
	c.Assert(err, jc.ErrorIsNil)

	// Create a relation with a unit in scope and assigned to the hosted machine.
	pr := newPeerRelation(c, s.State)
	err = pr.u0.AssignToMachine(machine)
	c.Assert(err, jc.ErrorIsNil)
	preventPeerUnitsDestroyRemove(c, pr)

	err = pr.ru0.EnterScope(nil)
	c.Assert(err, jc.ErrorIsNil)
	s.assertDoesNotNeedCleanup(c)

	// Destroy model, check cleanup queued.
	model, err := s.State.Model()
	c.Assert(err, jc.ErrorIsNil)
	err = model.Destroy(state.DestroyModelParams{})
	c.Assert(err, jc.ErrorIsNil)
	s.assertNeedsCleanup(c)

	// Clean up, and check that the unit has been removed...
	s.assertCleanupCount(c, 3)
	assertRemoved(c, pr.u0)

	// ...and the unit has departed relation scope...
	assertNotJoined(c, pr.ru0)

	// ...but that the machine remains, and is Dead, ready for removal by the
	// provisioner.
	assertLife(c, machine, state.Dead)
	assertLife(c, manualMachine, state.Dying)
	assertLife(c, stateMachine, state.Alive)
}

func (s *CleanupSuite) TestCleanupModelApplications(c *gc.C) {
	s.assertDoesNotNeedCleanup(c)

	// Create a application with some units.
	mysql := s.AddTestingApplication(c, "mysql", s.AddTestingCharm(c, "mysql"))
	units := make([]*state.Unit, 3)
	for i := range units {
		unit, err := mysql.AddUnit(state.AddUnitParams{})
		c.Assert(err, jc.ErrorIsNil)
		units[i] = unit
	}
	s.assertDoesNotNeedCleanup(c)

	// Destroy the model and check the application and units are
	// unaffected, but a cleanup for the application has been scheduled.
	model, err := s.State.Model()
	c.Assert(err, jc.ErrorIsNil)
	err = model.Destroy(state.DestroyModelParams{})
	c.Assert(err, jc.ErrorIsNil)
	s.assertNeedsCleanup(c)
	s.assertCleanupRuns(c)
	err = mysql.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(mysql.Life(), gc.Equals, state.Dying)
	for _, unit := range units {
		err = unit.Refresh()
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(unit.Life(), gc.Equals, state.Alive)
	}

	// The first cleanup Destroys the application, which
	// schedules another cleanup to destroy the units,
	// then we need another pass for the actions cleanup
	// which is queued on the next pass
	s.assertCleanupCount(c, 2)
	for _, unit := range units {
		err = unit.Refresh()
		c.Assert(err, jc.Satisfies, errors.IsNotFound)
	}

	// Now we should have all the cleanups done
	s.assertDoesNotNeedCleanup(c)
}

func (s *CleanupSuite) TestCleanupRelationSettings(c *gc.C) {
	// Create a relation with a unit in scope.
	pr := newPeerRelation(c, s.State)
	preventPeerUnitsDestroyRemove(c, pr)
	rel := pr.ru0.Relation()
	err := pr.ru0.EnterScope(map[string]interface{}{"some": "settings"})
	c.Assert(err, jc.ErrorIsNil)
	s.assertDoesNotNeedCleanup(c)

	// Destroy the application, check the relation's still around.
	err = pr.app.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	s.assertCleanupCount(c, 2)
	err = rel.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(rel.Life(), gc.Equals, state.Dying)

	// The unit leaves scope, triggering relation removal.
	err = pr.ru0.LeaveScope()
	c.Assert(err, jc.ErrorIsNil)
	s.assertNeedsCleanup(c)

	// Settings are not destroyed yet...
	settings, err := pr.ru1.ReadSettings("riak/0")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(settings, gc.DeepEquals, map[string]interface{}{"some": "settings"})

	// ...but they are on cleanup.
	s.assertCleanupCount(c, 1)
	_, err = pr.ru1.ReadSettings("riak/0")
	c.Assert(err, gc.ErrorMatches, `cannot read settings for unit "riak/0" in relation "riak:ring": unit "riak/0": settings not found`)
}

func (s *CleanupSuite) TestDestroyControllerMachineErrors(c *gc.C) {
	manager, err := s.State.AddMachine("quantal", state.JobManageModel)
	manager.SetHasVote(true)
	c.Assert(err, jc.ErrorIsNil)
	s.assertDoesNotNeedCleanup(c)
	err = manager.Destroy()
	c.Assert(err, gc.ErrorMatches, "machine 0 is the only controller machine")
	s.assertDoesNotNeedCleanup(c)
	assertLife(c, manager, state.Alive)
}

func (s *CleanupSuite) TestCleanupForceDestroyedMachineUnit(c *gc.C) {
	// Create a machine.
	machine, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)

	// Create a relation with a unit in scope and assigned to the machine.
	pr := newPeerRelation(c, s.State)
	err = pr.u0.AssignToMachine(machine)
	c.Assert(err, jc.ErrorIsNil)
	preventPeerUnitsDestroyRemove(c, pr)
	err = pr.ru0.EnterScope(nil)
	c.Assert(err, jc.ErrorIsNil)
	s.assertDoesNotNeedCleanup(c)

	// Force machine destruction, check cleanup queued.
	err = machine.ForceDestroy()
	c.Assert(err, jc.ErrorIsNil)
	s.assertNeedsCleanup(c)

	// Clean up, and check that the unit has been removed...
	s.assertCleanupCount(c, 2)
	assertRemoved(c, pr.u0)

	// ...and the unit has departed relation scope...
	assertNotJoined(c, pr.ru0)

	// ...but that the machine remains, and is Dead, ready for removal by the
	// provisioner.
	assertLife(c, machine, state.Dead)
}

func (s *CleanupSuite) TestCleanupForceDestroyedControllerMachine(c *gc.C) {
	machine, err := s.State.AddMachine("quantal", state.JobManageModel)
	c.Assert(err, jc.ErrorIsNil)
	err = machine.SetHasVote(true)
	c.Assert(err, jc.ErrorIsNil)
	changes, err := s.State.EnableHA(3, constraints.Value{}, "quantal", nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(changes.Added, gc.HasLen, 2)
	c.Check(changes.Removed, gc.HasLen, 0)
	c.Check(changes.Maintained, gc.HasLen, 1)
	c.Check(changes.Promoted, gc.HasLen, 0)
	c.Check(changes.Demoted, gc.HasLen, 0)
	c.Check(changes.Converted, gc.HasLen, 0)
	for _, mid := range changes.Added {
		m, err := s.State.Machine(mid)
		c.Assert(err, jc.ErrorIsNil)
		m.SetHasVote(true)
	}
	s.assertDoesNotNeedCleanup(c)
	err = machine.ForceDestroy()
	c.Assert(err, jc.ErrorIsNil)
	// The machine should no longer want the vote, should be forced to not have the vote, and forced to not be a
	// controller member anymore
	c.Assert(machine.Refresh(), jc.ErrorIsNil)
	c.Check(machine.Life(), gc.Equals, state.Dying)
	c.Check(machine.WantsVote(), jc.IsFalse)
	c.Check(machine.HasVote(), jc.IsTrue)
	c.Check(machine.Jobs(), jc.DeepEquals, []state.MachineJob{state.JobManageModel})
	controllerInfo, err := s.State.ControllerInfo()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(controllerInfo.MachineIds, gc.DeepEquals, append([]string{machine.Id()}, changes.Added...))
	// ForceDestroy still won't kill the controller if it is flagged as having a vote
	// We don't see the error because it is logged, but not returned.
	s.assertCleanupRuns(c)
	c.Assert(machine.SetHasVote(false), jc.ErrorIsNil)
	// However, if we remove the vote, it can be cleaned up.
	// ForceDestroy sets up a cleanupForceDestroyedMachine, which calls EnsureDead which sets up a cleanupDyingMachine
	// so it takes 2 cleanup runs to run clear
	s.assertCleanupCount(c, 2)
	// After we've run the cleanup for the controller machine, the machine should be dead, and it should not be
	// present in the other documents.
	assertLife(c, machine, state.Dead)
	controllerInfo, err = s.State.ControllerInfo()
	c.Assert(err, jc.ErrorIsNil)
	sort.Strings(controllerInfo.MachineIds)
	sort.Strings(changes.Added)
	// Only the machines that were added should still be part of the controller
	c.Check(controllerInfo.MachineIds, gc.DeepEquals, changes.Added)
}

func (s *CleanupSuite) TestCleanupForceDestroyMachineCleansStorageAttachments(c *gc.C) {
	machine, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	s.assertDoesNotNeedCleanup(c)

	ch := s.AddTestingCharm(c, "storage-block")
	storage := map[string]state.StorageConstraints{
		"data": makeStorageCons("loop", 1024, 1),
	}
	application := s.AddTestingApplicationWithStorage(c, "storage-block", ch, storage)
	u, err := application.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	err = u.AssignToMachine(machine)
	c.Assert(err, jc.ErrorIsNil)

	// check no cleanups
	s.assertDoesNotNeedCleanup(c)

	// this tag matches the storage instance created for the unit above.
	storageTag := names.NewStorageTag("data/0")

	sa, err := s.storageBackend.StorageAttachment(storageTag, u.UnitTag())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(sa.Life(), gc.Equals, state.Alive)

	// destroy machine and run cleanups
	err = machine.ForceDestroy()
	c.Assert(err, jc.ErrorIsNil)
	s.assertCleanupCount(c, 2)

	// After running the cleanups, the storage attachment should
	// have been removed; the storage instance should be floating,
	// and will be removed along with the machine.
	_, err = s.storageBackend.StorageAttachment(storageTag, u.UnitTag())
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
	si, err := s.storageBackend.StorageInstance(storageTag)
	c.Assert(err, jc.ErrorIsNil)
	_, hasOwner := si.Owner()
	c.Assert(hasOwner, jc.IsFalse)

	// Check that the unit has been removed.
	assertRemoved(c, u)

	// check no cleanups
	s.assertDoesNotNeedCleanup(c)
}

func (s *CleanupSuite) TestCleanupForceDestroyedMachineWithContainer(c *gc.C) {
	// Create a machine with a container.
	machine, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	container, err := s.State.AddMachineInsideMachine(state.MachineTemplate{
		Series: "quantal",
		Jobs:   []state.MachineJob{state.JobHostUnits},
	}, machine.Id(), instance.LXD)
	c.Assert(err, jc.ErrorIsNil)

	// Create active units (in relation scope, with subordinates).
	prr := newProReqRelation(c, &s.ConnSuite, charm.ScopeContainer)
	err = prr.pru0.EnterScope(nil)
	c.Assert(err, jc.ErrorIsNil)
	err = prr.pru1.EnterScope(nil)
	c.Assert(err, jc.ErrorIsNil)
	err = prr.rru0.EnterScope(nil)
	c.Assert(err, jc.ErrorIsNil)
	err = prr.rru1.EnterScope(nil)
	c.Assert(err, jc.ErrorIsNil)

	// Assign the various units to machines.
	err = prr.pu0.AssignToMachine(machine)
	c.Assert(err, jc.ErrorIsNil)
	err = prr.pu1.AssignToMachine(container)
	c.Assert(err, jc.ErrorIsNil)
	preventProReqUnitsDestroyRemove(c, prr)
	s.assertDoesNotNeedCleanup(c)

	// Force removal of the top-level machine.
	err = machine.ForceDestroy()
	c.Assert(err, jc.ErrorIsNil)
	s.assertNeedsCleanup(c)

	// And do it again, just to check that the second cleanup doc for the same
	// machine doesn't cause problems down the line.
	err = machine.ForceDestroy()
	c.Assert(err, jc.ErrorIsNil)
	s.assertNeedsCleanup(c)

	// Clean up, and check that the container has been removed...
	s.assertCleanupCount(c, 2)
	err = container.Refresh()
	c.Assert(err, jc.Satisfies, errors.IsNotFound)

	// ...and so have all the units...
	assertRemoved(c, prr.pu0)
	assertRemoved(c, prr.pu1)
	assertRemoved(c, prr.ru0)
	assertRemoved(c, prr.ru1)

	// ...and none of the units have left relation scopes occupied...
	assertNotInScope(c, prr.pru0)
	assertNotInScope(c, prr.pru1)
	assertNotInScope(c, prr.rru0)
	assertNotInScope(c, prr.rru1)

	// ...but that the machine remains, and is Dead, ready for removal by the
	// provisioner.
	assertLife(c, machine, state.Dead)
}

func (s *CleanupSuite) TestCleanupDyingUnit(c *gc.C) {
	// Create active unit, in a relation.
	prr := newProReqRelation(c, &s.ConnSuite, charm.ScopeGlobal)
	preventProReqUnitsDestroyRemove(c, prr)
	err := prr.pru0.EnterScope(nil)
	c.Assert(err, jc.ErrorIsNil)

	// Destroy provider unit 0; check it's Dying, and a cleanup has been scheduled.
	err = prr.pu0.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	err = prr.pu0.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	assertLife(c, prr.pu0, state.Dying)
	s.assertNeedsCleanup(c)

	// Check it's reported in scope until cleaned up.
	assertJoined(c, prr.pru0)
	s.assertCleanupCount(c, 1)
	assertInScope(c, prr.pru0)
	assertNotJoined(c, prr.pru0)

	// Destroy the relation, and check it sticks around...
	err = prr.rel.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	assertLife(c, prr.rel, state.Dying)

	// ...until the unit is removed, and really leaves scope.
	err = prr.pu0.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
	err = prr.pu0.Remove()
	c.Assert(err, jc.ErrorIsNil)
	assertNotInScope(c, prr.pru0)
	assertRemoved(c, prr.rel)
}

func (s *CleanupSuite) TestCleanupDyingUnitAlreadyRemoved(c *gc.C) {
	// Create active unit, in a relation.
	prr := newProReqRelation(c, &s.ConnSuite, charm.ScopeGlobal)
	preventProReqUnitsDestroyRemove(c, prr)
	err := prr.pru0.EnterScope(nil)
	c.Assert(err, jc.ErrorIsNil)

	// Destroy provider unit 0; check it's Dying, and a cleanup has been scheduled.
	err = prr.pu0.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	err = prr.pu0.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	assertLife(c, prr.pu0, state.Dying)
	s.assertNeedsCleanup(c)

	// Remove the unit, and the relation.
	err = prr.pu0.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
	err = prr.pu0.Remove()
	c.Assert(err, jc.ErrorIsNil)
	err = prr.rel.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	assertRemoved(c, prr.rel)

	// Check the cleanup still runs happily.
	s.assertCleanupCount(c, 1)
	s.assertCleanupRuns(c)
}

func (s *CleanupSuite) TestCleanupActions(c *gc.C) {
	// Create a application with a unit.
	dummy := s.AddTestingApplication(c, "dummy", s.AddTestingCharm(c, "dummy"))
	unit, err := dummy.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)

	// check no cleanups
	s.assertDoesNotNeedCleanup(c)

	// Add a couple actions to the unit
	_, err = unit.AddAction("snapshot", nil)
	c.Assert(err, jc.ErrorIsNil)
	_, err = unit.AddAction("snapshot", nil)
	c.Assert(err, jc.ErrorIsNil)

	// make sure unit still has actions
	actions, err := unit.PendingActions()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(actions), gc.Equals, 2)

	// destroy unit and run cleanups
	err = dummy.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	s.assertCleanupRuns(c)

	// make sure unit still has actions, after first cleanup pass
	actions, err = unit.PendingActions()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(actions), gc.Equals, 2)

	// second cleanup pass
	s.assertCleanupRuns(c)

	// make sure unit has no actions, after second cleanup pass
	actions, err = unit.PendingActions()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(actions), gc.Equals, 0)

	// check no cleanups
	s.assertDoesNotNeedCleanup(c)
}

func (s *CleanupSuite) TestCleanupWithCompletedActions(c *gc.C) {
	// Create a application with a unit.
	dummy := s.AddTestingApplication(c, "dummy", s.AddTestingCharm(c, "dummy"))
	unit, err := dummy.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	s.assertDoesNotNeedCleanup(c)

	// Add a completed action to the unit.
	action, err := unit.AddAction("snapshot", nil)
	c.Assert(err, jc.ErrorIsNil)
	action, err = action.Finish(state.ActionResults{
		Status:  state.ActionCompleted,
		Message: "done",
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(action.Status(), gc.Equals, state.ActionCompleted)

	// Destroy application and run cleanups.
	err = dummy.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	// First cleanup marks all units of the application as dying.
	s.assertCleanupRuns(c)
	// Second cleanup clear pending actions.
	s.assertCleanupRuns(c)
	// Check no cleanups.
	s.assertDoesNotNeedCleanup(c)
}

func (s *CleanupSuite) TestCleanupStorageAttachments(c *gc.C) {
	s.assertDoesNotNeedCleanup(c)

	ch := s.AddTestingCharm(c, "storage-block")
	storage := map[string]state.StorageConstraints{
		"data": makeStorageCons("loop", 1024, 1),
	}
	application := s.AddTestingApplicationWithStorage(c, "storage-block", ch, storage)
	u, err := application.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)

	// check no cleanups
	s.assertDoesNotNeedCleanup(c)

	// this tag matches the storage instance created for the unit above.
	storageTag := names.NewStorageTag("data/0")

	sa, err := s.storageBackend.StorageAttachment(storageTag, u.UnitTag())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(sa.Life(), gc.Equals, state.Alive)

	// destroy unit and run cleanups; the storage should be detached
	err = u.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	s.assertCleanupRuns(c)

	// After running the cleanup, the attachment should be removed
	// (short-circuited, because volume was never attached).
	_, err = s.storageBackend.StorageAttachment(storageTag, u.UnitTag())
	c.Assert(err, jc.Satisfies, errors.IsNotFound)

	// check no cleanups
	s.assertDoesNotNeedCleanup(c)
}

func (s *CleanupSuite) TestCleanupStorageInstances(c *gc.C) {
	ch := s.AddTestingCharm(c, "storage-block")
	storage := map[string]state.StorageConstraints{
		"allecto": makeStorageCons("modelscoped-block", 1024, 1),
	}
	application := s.AddTestingApplicationWithStorage(c, "storage-block", ch, storage)
	u, err := application.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)

	// check no cleanups
	s.assertDoesNotNeedCleanup(c)

	// this tag matches the storage instance created for the unit above.
	storageTag := names.NewStorageTag("allecto/0")

	si, err := s.storageBackend.StorageInstance(storageTag)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(si.Life(), gc.Equals, state.Alive)

	// destroy storage instance and run cleanups
	err = s.storageBackend.DestroyStorageInstance(storageTag, true)
	c.Assert(err, jc.ErrorIsNil)
	si, err = s.storageBackend.StorageInstance(storageTag)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(si.Life(), gc.Equals, state.Dying)
	sa, err := s.storageBackend.StorageAttachment(storageTag, u.UnitTag())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(sa.Life(), gc.Equals, state.Alive)
	s.assertCleanupRuns(c)

	// After running the cleanup, the attachment should be removed
	// (short-circuited, because volume was never attached).
	_, err = s.storageBackend.StorageAttachment(storageTag, u.UnitTag())
	c.Assert(err, jc.Satisfies, errors.IsNotFound)

	// check no cleanups
	s.assertDoesNotNeedCleanup(c)
}

func (s *CleanupSuite) TestCleanupMachineStorage(c *gc.C) {
	ch := s.AddTestingCharm(c, "storage-block")
	storage := map[string]state.StorageConstraints{
		"data": makeStorageCons("modelscoped", 1024, 1),
	}
	application := s.AddTestingApplicationWithStorage(c, "storage-block", ch, storage)
	unit, err := application.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	err = s.State.AssignUnit(unit, state.AssignCleanEmpty)
	c.Assert(err, jc.ErrorIsNil)
	machineId, err := unit.AssignedMachineId()
	c.Assert(err, jc.ErrorIsNil)
	machine, err := s.State.Machine(machineId)
	c.Assert(err, jc.ErrorIsNil)

	// Destroy the application, so we can destroy the machine.
	err = unit.Destroy()
	s.assertCleanupRuns(c)
	err = application.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	s.assertCleanupRuns(c)

	// check no cleanups
	s.assertDoesNotNeedCleanup(c)

	// destroy machine and run cleanups; the volume attachment
	// should be marked dying.
	err = machine.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	s.assertCleanupRuns(c)

	vas, err := s.storageBackend.MachineVolumeAttachments(machine.MachineTag())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(vas, gc.HasLen, 1)
	c.Assert(vas[0].Life(), gc.Equals, state.Dying)

	// check no cleanups
	s.assertDoesNotNeedCleanup(c)
}

func (s *CleanupSuite) TestCleanupCAASApplicationWithStorage(c *gc.C) {
	s.assertCleanupCAASEntityWithStorage(c, func(st *state.State, app *state.Application) error {
		op := app.DestroyOperation()
		op.DestroyStorage = true
		return st.ApplyOperation(op)
	})
}

func (s *CleanupSuite) TestCleanupCAASUnitWithStorage(c *gc.C) {
	s.assertCleanupCAASEntityWithStorage(c, func(st *state.State, app *state.Application) error {
		units, err := app.AllUnits()
		if err != nil {
			return err
		}
		op := units[0].DestroyOperation()
		op.DestroyStorage = true
		return st.ApplyOperation(op)
	})
}

func (s *CleanupSuite) assertCleanupCAASEntityWithStorage(c *gc.C, deleteOp func(*state.State, *state.Application) error) {
	st := s.Factory.MakeCAASModel(c, nil)
	defer st.Close()
	sb, err := state.NewStorageBackend(st)
	c.Assert(err, jc.ErrorIsNil)
	broker, err := stateenvirons.GetNewCAASBrokerFunc(caas.New)(st)
	c.Assert(err, jc.ErrorIsNil)
	registry := stateenvirons.NewStorageProviderRegistry(broker)
	s.policy = testing.MockPolicy{
		GetStorageProviderRegistry: func() (corestorage.ProviderRegistry, error) {
			return registry, nil
		},
	}

	ch := state.AddTestingCharmForSeries(c, st, "kubernetes", "storage-filesystem")
	storCons := map[string]state.StorageConstraints{
		"data": makeStorageCons("", 1024, 1),
	}
	application := state.AddTestingApplicationWithStorage(c, st, "storage-filesystem", ch, storCons)
	unit, err := application.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	fs, err := sb.AllFilesystems()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(fs, gc.HasLen, 1)
	fas, err := sb.UnitFilesystemAttachments(unit.UnitTag())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(fas, gc.HasLen, 1)

	err = deleteOp(st, application)
	c.Assert(err, jc.ErrorIsNil)

	err = application.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	for i := 0; i < 4; i++ {
		err = st.Cleanup()
		c.Assert(err, jc.ErrorIsNil)
	}

	// check no cleanups
	state.AssertNoCleanups(c, st)

	fas, err = sb.UnitFilesystemAttachments(unit.UnitTag())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(fas, gc.HasLen, 0)
	fs, err = sb.AllFilesystems()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(fs, gc.HasLen, 0)
}

func (s *CleanupSuite) TestCleanupVolumeAttachments(c *gc.C) {
	_, err := s.State.AddOneMachine(state.MachineTemplate{
		Series: "quantal",
		Jobs:   []state.MachineJob{state.JobHostUnits},
		Volumes: []state.HostVolumeParams{{
			Volume: state.VolumeParams{Pool: "loop", Size: 1024},
		}},
	})
	c.Assert(err, jc.ErrorIsNil)
	s.assertDoesNotNeedCleanup(c)

	err = s.storageBackend.DestroyVolume(names.NewVolumeTag("0/0"))
	c.Assert(err, jc.ErrorIsNil)
	s.assertCleanupRuns(c)

	attachment, err := s.storageBackend.VolumeAttachment(names.NewMachineTag("0"), names.NewVolumeTag("0/0"))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(attachment.Life(), gc.Equals, state.Dying)
}

func (s *CleanupSuite) TestCleanupFilesystemAttachments(c *gc.C) {
	_, err := s.State.AddOneMachine(state.MachineTemplate{
		Series: "quantal",
		Jobs:   []state.MachineJob{state.JobHostUnits},
		Filesystems: []state.HostFilesystemParams{{
			Filesystem: state.FilesystemParams{Pool: "rootfs", Size: 1024},
		}},
	})
	c.Assert(err, jc.ErrorIsNil)
	s.assertDoesNotNeedCleanup(c)

	err = s.storageBackend.DestroyFilesystem(names.NewFilesystemTag("0/0"))
	c.Assert(err, jc.ErrorIsNil)
	s.assertCleanupRuns(c)

	attachment, err := s.storageBackend.FilesystemAttachment(names.NewMachineTag("0"), names.NewFilesystemTag("0/0"))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(attachment.Life(), gc.Equals, state.Dying)
}

func (s *CleanupSuite) TestCleanupResourceBlob(c *gc.C) {
	app := s.AddTestingApplication(c, "wp", s.AddTestingCharm(c, "wordpress"))
	data := "ancient-debris"
	res := resourcetesting.NewResource(c, nil, "mug", "wp", data).Resource
	resources, err := s.State.Resources()
	c.Assert(err, jc.ErrorIsNil)
	_, err = resources.SetResource("wp", res.Username, res.Resource, bytes.NewBufferString(data))
	c.Assert(err, jc.ErrorIsNil)

	err = app.Destroy()
	c.Assert(err, jc.ErrorIsNil)

	path := "application-wp/resources/mug"
	stateStorage := storage.NewStorage(s.State.ModelUUID(), s.State.MongoSession())
	closer, _, err := stateStorage.Get(path)
	c.Assert(err, jc.ErrorIsNil)
	err = closer.Close()
	c.Assert(err, jc.ErrorIsNil)

	s.assertCleanupRuns(c)

	_, _, err = stateStorage.Get(path)
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *CleanupSuite) TestCleanupResourceBlobHandlesMissing(c *gc.C) {
	app := s.AddTestingApplication(c, "wp", s.AddTestingCharm(c, "wordpress"))
	data := "ancient-debris"
	res := resourcetesting.NewResource(c, nil, "mug", "wp", data).Resource
	resources, err := s.State.Resources()
	c.Assert(err, jc.ErrorIsNil)
	_, err = resources.SetResource("wp", res.Username, res.Resource, bytes.NewBufferString(data))
	c.Assert(err, jc.ErrorIsNil)

	err = app.Destroy()
	c.Assert(err, jc.ErrorIsNil)

	path := "application-wp/resources/mug"
	stateStorage := storage.NewStorage(s.State.ModelUUID(), s.State.MongoSession())
	err = stateStorage.Remove(path)
	c.Assert(err, jc.ErrorIsNil)

	s.assertCleanupRuns(c)
	// Make sure the cleanup completed successfully.
	s.assertDoesNotNeedCleanup(c)
}

func (s *CleanupSuite) TestNothingToCleanup(c *gc.C) {
	s.assertDoesNotNeedCleanup(c)
	s.assertCleanupRuns(c)
	s.assertDoesNotNeedCleanup(c)
}

func (s *CleanupSuite) TestCleanupIDSanity(c *gc.C) {
	// Cleanup IDs shouldn't be ObjectIdHex("blah")
	app := s.AddTestingApplication(c, "wp", s.AddTestingCharm(c, "wordpress"))
	err := app.Destroy()
	c.Assert(err, jc.ErrorIsNil)

	coll := s.Session.DB("juju").C("cleanups")
	var ids []struct {
		ID string `bson:"_id"`
	}
	err = coll.Find(nil).All(&ids)
	c.Assert(err, jc.ErrorIsNil)
	for _, item := range ids {
		c.Assert(item.ID, gc.Not(gc.Matches), `.*ObjectIdHex\(.*`)
	}
	s.assertCleanupRuns(c)
}

func (s *CleanupSuite) assertCleanupRuns(c *gc.C) {
	err := s.State.Cleanup()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *CleanupSuite) assertNeedsCleanup(c *gc.C) {
	actual, err := s.State.NeedsCleanup()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(actual, jc.IsTrue)
}

func (s *CleanupSuite) assertDoesNotNeedCleanup(c *gc.C) {
	state.AssertNoCleanups(c, s.State)
}

// assertCleanupCount is useful because certain cleanups cause other cleanups
// to be queued; it makes more sense to just run cleanup again than to unpick
// object destruction so that we run the cleanups inline while running cleanups.
func (s *CleanupSuite) assertCleanupCount(c *gc.C, count int) {
	for i := 0; i < count; i++ {
		c.Logf("checking cleanups %d", i)
		s.assertNeedsCleanup(c)
		s.assertCleanupRuns(c)
	}
	s.assertDoesNotNeedCleanup(c)
}
