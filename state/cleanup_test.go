// Copyright 2014-2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"bytes"
	"sort"
	"time"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v6"
	"gopkg.in/juju/names.v3"

	"github.com/juju/juju/caas"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/status"
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

func (s *CleanupSuite) TestCleanupDyingApplicationNoUnits(c *gc.C) {
	mysql := s.AddTestingApplication(c, "mysql", s.AddTestingCharm(c, "mysql"))
	c.Assert(mysql.Destroy(), jc.ErrorIsNil)
	c.Assert(mysql.Refresh(), jc.Satisfies, errors.IsNotFound)
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

	wordpress := s.AddTestingApplication(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	eps, err := s.State.InferEndpoints("wordpress", "mysql")
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.State.AddRelation(eps[0], eps[1])
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(remoteApp.Refresh(), jc.ErrorIsNil)
	c.Assert(wordpress.Refresh(), jc.ErrorIsNil)

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
	s.assertCleanupCount(c, 4)
	assertRemoved(c, pr.u0)

	// ...and the unit has departed relation scope...
	assertNotJoined(c, pr.ru0)

	// ...and the machine has been removed (since model destroy does a
	// force-destroy on the machine).
	c.Assert(machine.Refresh(), jc.Satisfies, errors.IsNotFound)
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

	// The first cleanup removes the units, which schedules
	// the application to be removed. This removes the application
	// queing up a change for actions and charms.
	s.assertCleanupCount(c, 3)
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

func (s *CleanupSuite) TestCleanupModelBranches(c *gc.C) {
	s.assertDoesNotNeedCleanup(c)

	// Create a branch.
	c.Assert(s.Model.AddBranch(newBranchName, newBranchCreator), jc.ErrorIsNil)
	branches, err := s.State.Branches()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(branches, gc.HasLen, 1)
	s.assertDoesNotNeedCleanup(c)

	// Destroy the model and check the branches unaffected, but a cleanup for
	// the branches has been scheduled.
	model, err := s.State.Model()
	c.Assert(err, jc.ErrorIsNil)
	err = model.Destroy(state.DestroyModelParams{})
	c.Assert(err, jc.ErrorIsNil)
	s.assertNeedsCleanup(c)
	s.assertCleanupCount(c, 1)

	s.assertCleanupRuns(c)
	err = model.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	s.assertCleanupCount(c, 0)

	_, err = s.Model.Branch(newBranchName)
	c.Assert(err, jc.Satisfies, errors.IsNotFound)

	branches, err = s.State.Branches()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(branches, gc.HasLen, 0)

	// Now we should have all the cleanups done
	s.assertDoesNotNeedCleanup(c)
}

func (s *CleanupSuite) TestDestroyControllerMachineErrors(c *gc.C) {
	manager, err := s.State.AddMachine("quantal", state.JobManageModel)
	c.Assert(err, jc.ErrorIsNil)
	node, err := s.State.ControllerNode(manager.Id())
	c.Assert(err, jc.ErrorIsNil)
	node.SetHasVote(true)
	c.Assert(err, jc.ErrorIsNil)
	s.assertDoesNotNeedCleanup(c)
	err = manager.Destroy()
	c.Assert(err, gc.ErrorMatches, "controller 0 is the only controller")
	s.assertDoesNotNeedCleanup(c)
	assertLife(c, manager, state.Alive)
}

const dontWait = time.Duration(0)

func (s *CleanupSuite) TestCleanupForceDestroyedMachineUnit(c *gc.C) {
	// Create a machine.
	machine, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	err = machine.SetProvisioned("inst-id", "", "fake_nonce", nil)
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
	err = machine.ForceDestroy(time.Minute)
	c.Assert(err, jc.ErrorIsNil)
	s.assertNeedsCleanup(c)

	// Clean up, and check that the unit has been removed...
	s.assertCleanupCountDirty(c, 2)
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
	node, err := s.State.ControllerNode(machine.Id())
	c.Assert(err, jc.ErrorIsNil)
	err = node.SetHasVote(true)
	c.Assert(err, jc.ErrorIsNil)
	changes, err := s.State.EnableHA(3, constraints.Value{}, "quantal", nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(changes.Added, gc.HasLen, 2)
	c.Check(changes.Removed, gc.HasLen, 0)
	c.Check(changes.Maintained, gc.HasLen, 1)
	c.Check(changes.Converted, gc.HasLen, 0)
	for _, mid := range changes.Added {
		m, err := s.State.Machine(mid)
		c.Assert(err, jc.ErrorIsNil)
		node, err := s.State.ControllerNode(m.Id())
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(node.SetHasVote(true), jc.ErrorIsNil)
	}
	s.assertDoesNotNeedCleanup(c)
	err = machine.ForceDestroy(time.Minute)
	c.Assert(err, jc.ErrorIsNil)
	// The machine should no longer want the vote, should be forced to not have the vote, and forced to not be a
	// controller member anymore
	c.Assert(machine.Refresh(), jc.ErrorIsNil)
	c.Check(machine.Life(), gc.Equals, state.Dying)
	node, err = s.State.ControllerNode(machine.Id())
	c.Assert(err, jc.ErrorIsNil)
	c.Check(node.WantsVote(), jc.IsFalse)
	c.Check(node.HasVote(), jc.IsTrue)
	c.Check(machine.Jobs(), jc.DeepEquals, []state.MachineJob{state.JobManageModel})
	controllerIds, err := s.State.ControllerIds()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(controllerIds, gc.DeepEquals, append([]string{machine.Id()}, changes.Added...))
	// ForceDestroy still won't kill the controller if it is flagged as having a vote
	// We don't see the error because it is logged, but not returned.
	s.assertCleanupRuns(c)
	c.Assert(node.SetHasVote(false), jc.ErrorIsNil)
	// However, if we remove the vote, it can be cleaned up.
	// ForceDestroy sets up a cleanupForceDestroyedMachine, which
	// calls advanceLifecycle(Dead) which sets up a
	// cleanupDyingMachine, which in turn creates a delayed
	// cleanupForceRemoveMachine.
	// Run the first two.
	s.assertCleanupCountDirty(c, 2)
	// After we've run the cleanup for the controller machine, the machine should be dead, and it should not be
	// present in the other documents.
	assertLife(c, machine, state.Dead)
	controllerIds, err = s.State.ControllerIds()
	c.Assert(err, jc.ErrorIsNil)
	sort.Strings(controllerIds)
	sort.Strings(changes.Added)
	// Only the machines that were added should still be part of the controller
	c.Check(controllerIds, gc.DeepEquals, changes.Added)
}

func (s *CleanupSuite) TestCleanupForceDestroyMachineCleansStorageAttachments(c *gc.C) {
	machine, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	s.assertDoesNotNeedCleanup(c)

	err = machine.SetProvisioned("inst-id", "", "fake_nonce", nil)
	c.Assert(err, jc.ErrorIsNil)

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
	err = machine.ForceDestroy(time.Minute)
	c.Assert(err, jc.ErrorIsNil)

	// Run cleanups to remove the unit and make the machine dead.
	s.assertCleanupCountDirty(c, 2)

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

	s.Clock.Advance(time.Minute)
	// Check that the last cleanup to remove the machine runs.
	s.assertCleanupCount(c, 1)
}

func (s *CleanupSuite) TestCleanupForceDestroyedMachineWithContainer(c *gc.C) {
	// Create a machine with a container.
	machine, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	err = machine.SetProvisioned("inst-id", "", "fake_nonce", nil)
	c.Assert(err, jc.ErrorIsNil)
	container, err := s.State.AddMachineInsideMachine(state.MachineTemplate{
		Series: "quantal",
		Jobs:   []state.MachineJob{state.JobHostUnits},
	}, machine.Id(), instance.LXD)
	c.Assert(err, jc.ErrorIsNil)
	err = container.SetProvisioned("inst-id", "", "fake_nonce", nil)
	c.Assert(err, jc.ErrorIsNil)

	// Create active units (in relation scope, with subordinates).
	prr := newProReqRelation(c, &s.ConnSuite, charm.ScopeContainer, machine, container)
	prr.allEnterScope(c)

	preventProReqUnitsDestroyRemove(c, prr)
	s.assertDoesNotNeedCleanup(c)

	// Force removal of the top-level machine.
	err = machine.ForceDestroy(time.Minute)
	c.Assert(err, jc.ErrorIsNil)
	s.assertNeedsCleanup(c)

	// And do it again, just to check that the second cleanup doc for the same
	// machine doesn't cause problems down the line.
	err = machine.ForceDestroy(time.Minute)
	c.Assert(err, jc.ErrorIsNil)
	s.assertNeedsCleanup(c)

	// Clean up, and check that the container has been removed...
	s.assertCleanupCountDirty(c, 2)
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

func (s *CleanupSuite) TestForceDestroyMachineSchedulesRemove(c *gc.C) {
	machine, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	err = machine.SetProvisioned("inst-id", "", "fake_nonce", nil)
	c.Assert(err, jc.ErrorIsNil)

	s.assertDoesNotNeedCleanup(c)

	err = machine.ForceDestroy(time.Minute)
	c.Assert(err, jc.ErrorIsNil)
	s.assertNeedsCleanup(c)

	s.assertCleanupRuns(c)

	assertLifeIs(c, machine, state.Dead)

	// Running a cleanup pass succeeds but doesn't get rid of cleanups
	// because there's a scheduled one.
	s.assertCleanupRuns(c)
	assertLifeIs(c, machine, state.Dead)
	s.assertNeedsCleanup(c)

	s.Clock.Advance(time.Minute)
	s.assertCleanupCount(c, 1)
	err = machine.Refresh()
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
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

	operationID, err := s.Model.EnqueueOperation("a test")
	c.Assert(err, jc.ErrorIsNil)
	// Add a couple actions to the unit
	_, err = unit.AddAction(operationID, "snapshot", nil)
	c.Assert(err, jc.ErrorIsNil)
	_, err = unit.AddAction(operationID, "snapshot", nil)
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

	// Application has been cleaned up, but now we cleanup the charm
	c.Assert(dummy.Refresh(), jc.Satisfies, errors.IsNotFound)
	s.assertCleanupRuns(c)

	// check no cleanups
	s.assertDoesNotNeedCleanup(c)
}

func (s *CleanupSuite) TestCleanupWithCompletedActions(c *gc.C) {
	for _, status := range []state.ActionStatus{
		state.ActionCompleted,
		state.ActionCancelled,
		state.ActionAborted,
		state.ActionFailed,
	} {
		// Create a application with a unit.
		dummy := s.AddTestingApplication(c, "dummy", s.AddTestingCharm(c, "dummy"))
		unit, err := dummy.AddUnit(state.AddUnitParams{})
		c.Assert(err, jc.ErrorIsNil)
		s.assertDoesNotNeedCleanup(c)

		// Add a completed action to the unit.
		operationID, err := s.Model.EnqueueOperation("a test")
		c.Assert(err, jc.ErrorIsNil)
		action, err := unit.AddAction(operationID, "snapshot", nil)
		c.Assert(err, jc.ErrorIsNil)
		action, err = action.Finish(state.ActionResults{
			Status:  status,
			Message: "done",
		})
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(action.Status(), gc.Equals, status)

		// Destroy application and run cleanups.
		err = dummy.Destroy()
		c.Assert(err, jc.ErrorIsNil)
		// First cleanup marks all units of the application as dying.
		// Second cleanup clear pending actions.
		s.assertCleanupCount(c, 3)
	}
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
	err = s.storageBackend.DestroyStorageInstance(storageTag, true, false, dontWait)
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
	c.Assert(err, jc.ErrorIsNil)
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
	model, err := st.Model()
	c.Assert(err, jc.ErrorIsNil)
	broker, err := stateenvirons.GetNewCAASBrokerFunc(caas.New)(model)
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

func (s *CleanupSuite) TestDyingUnitWithForceSchedulesForceFallback(c *gc.C) {
	ch := s.AddTestingCharm(c, "mysql")
	application := s.AddTestingApplication(c, "mysql", ch)
	unit, err := application.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	err = s.State.AssignUnit(unit, state.AssignCleanEmpty)
	c.Assert(err, jc.ErrorIsNil)

	err = unit.SetAgentStatus(status.StatusInfo{
		Status: status.Idle,
	})
	c.Assert(err, jc.ErrorIsNil)

	opErrs, err := unit.DestroyWithForce(true, time.Minute)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(opErrs, gc.IsNil)

	// The unit should be dying, and there's a deferred cleanup to
	// endeaden it.
	assertLifeIs(c, unit, state.Dying)

	s.assertNeedsCleanup(c)
	// dyingUnit
	s.assertCleanupRuns(c)

	s.Clock.Advance(time.Minute)
	// forceDestroyedUnit
	s.assertCleanupRuns(c)

	assertLifeIs(c, unit, state.Dead)

	s.Clock.Advance(time.Minute)
	// forceRemoveUnit
	s.assertCleanupRuns(c)

	assertUnitRemoved(c, unit)
	// After this there are three cleanups remaining: removedUnit,
	// dyingMachine, forceRemoveMachine (but the last is delayed a
	// minute).
	s.assertCleanupCountDirty(c, 2)

	s.Clock.Advance(time.Minute)
	s.assertCleanupCount(c, 1)
}

func (s *CleanupSuite) TestForceDestroyUnitDestroysSubordinates(c *gc.C) {
	prr := newProReqRelation(c, &s.ConnSuite, charm.ScopeContainer)
	prr.allEnterScope(c)
	for _, principal := range []*state.Unit{prr.pu0, prr.pu1} {
		err := s.State.AssignUnit(principal, state.AssignCleanEmpty)
		c.Assert(err, jc.ErrorIsNil)
	}
	for _, unit := range []*state.Unit{prr.pu0, prr.pu1, prr.ru0, prr.ru1} {
		err := unit.SetAgentStatus(status.StatusInfo{
			Status: status.Idle,
		})
		c.Assert(err, jc.ErrorIsNil)
	}

	unit := prr.pu0
	subordinate := prr.ru0

	opErrs, err := unit.DestroyWithForce(true, time.Duration(0))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(opErrs, gc.IsNil)

	assertLifeIs(c, unit, state.Dying)
	assertLifeIs(c, subordinate, state.Alive)

	s.assertNeedsCleanup(c)
	// dyingUnit(mysql/0)
	s.assertNextCleanup(c, "dyingUnit(mysql/0)")

	// forceDestroyUnit(mysql/0) triggers destruction of the subordinate that
	// needs to run - it fails because the subordinates haven't yet
	// been removed. It will be pending until near the end of this test when it
	// finally succeeds.
	s.assertNextCleanup(c, "forceDestroyUnit(mysql/0)")

	assertLifeIs(c, subordinate, state.Dying)
	assertLifeIs(c, unit, state.Dying)

	// dyingUnit(logging/0) runs and schedules the force cleanup.
	s.assertNextCleanup(c, "dyingUnit(logging/0)")
	// forceDestroyUnit(logging/0) sets it to dead.
	s.assertNextCleanup(c, "forceDestroyUnit(logging/0)")

	assertLifeIs(c, subordinate, state.Dead)
	assertLifeIs(c, unit, state.Dying)

	// forceRemoveUnit(logging/0) runs
	s.assertNextCleanup(c, "forceRemoveUnit(logging/0)")
	assertUnitRemoved(c, subordinate)

	// Now forceDestroyUnit(mysql/0) can run successfully and make the unit dead
	s.assertNextCleanup(c, "forceRemoveUnit(mysql/0)")
	assertLifeIs(c, unit, state.Dead)

	// forceRemoveUnit
	s.assertNextCleanup(c, "forceRemoveUnit")

	assertUnitRemoved(c, unit)
	// After this there are three cleanups remaining: removedUnit,
	// dyingMachine, forceRemoveMachine.
	s.assertCleanupCount(c, 3)
}

func (s *CleanupSuite) TestForceDestroyUnitLeavesRelations(c *gc.C) {
	prr := newProReqRelation(c, &s.ConnSuite, charm.ScopeGlobal)
	prr.allEnterScope(c)
	for _, unit := range []*state.Unit{prr.pu0, prr.pu1, prr.ru0, prr.ru1} {
		err := s.State.AssignUnit(unit, state.AssignCleanEmpty)
		c.Assert(err, jc.ErrorIsNil)
		err = unit.SetAgentStatus(status.StatusInfo{
			Status: status.Idle,
		})
		c.Assert(err, jc.ErrorIsNil)
	}

	unit := prr.pu0
	opErrs, err := unit.DestroyWithForce(true, dontWait)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(opErrs, gc.IsNil)

	assertLifeIs(c, unit, state.Dying)
	assertUnitInScope(c, unit, prr.rel, true)

	// dyingUnit schedules forceDestroyedUnit
	s.assertCleanupRuns(c)
	// ...which leaves the scope for its relations.
	s.assertCleanupRuns(c)

	assertLifeIs(c, unit, state.Dead)
	assertUnitInScope(c, unit, prr.rel, false)

	// forceRemoveUnit
	s.assertCleanupRuns(c)

	assertUnitRemoved(c, unit)
	// After this there are three cleanups remaining: removedUnit,
	// dyingMachine, forceRemoveMachine.
	s.assertCleanupCount(c, 3)
}

func (s *CleanupSuite) TestForceDestroyUnitRemovesStorageAttachments(c *gc.C) {
	s.assertDoesNotNeedCleanup(c)

	ch := s.AddTestingCharm(c, "storage-block")
	storage := map[string]state.StorageConstraints{
		"data": makeStorageCons("loop", 1024, 1),
	}
	application := s.AddTestingApplicationWithStorage(c, "storage-block", ch, storage)
	u, err := application.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)

	machine, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	err = u.AssignToMachine(machine)
	c.Assert(err, jc.ErrorIsNil)
	err = u.SetAgentStatus(status.StatusInfo{
		Status: status.Idle,
	})
	c.Assert(err, jc.ErrorIsNil)

	// this tag matches the storage instance created for the unit above.
	storageTag := names.NewStorageTag("data/0")

	sa, err := s.storageBackend.StorageAttachment(storageTag, u.UnitTag())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(sa.Life(), gc.Equals, state.Alive)

	// Ensure there's a volume on the machine hosting the unit so the
	// attachment removal can't be short-circuited.
	err = machine.SetProvisioned("inst-id", "", "fake_nonce", nil)
	c.Assert(err, jc.ErrorIsNil)
	volume, err := s.storageBackend.StorageInstanceVolume(storageTag)
	c.Assert(err, jc.ErrorIsNil)
	err = s.storageBackend.SetVolumeInfo(
		volume.VolumeTag(), state.VolumeInfo{VolumeId: "vol-123"})
	c.Assert(err, jc.ErrorIsNil)
	err = s.storageBackend.SetVolumeAttachmentInfo(
		machine.MachineTag(),
		volume.VolumeTag(),
		state.VolumeAttachmentInfo{DeviceName: "sdc"},
	)
	c.Assert(err, jc.ErrorIsNil)

	// destroy unit and run cleanups
	opErrs, err := u.DestroyWithForce(true, dontWait)
	c.Assert(opErrs, gc.IsNil)
	c.Assert(err, jc.ErrorIsNil)
	s.assertCleanupRuns(c)

	// After running the cleanup, the attachment should still be
	// around because volume was attached.
	_, err = s.storageBackend.StorageAttachment(storageTag, u.UnitTag())
	c.Assert(err, jc.ErrorIsNil)

	// So now run the forceDestroyedUnit cleanup...
	s.assertCleanupRuns(c)

	// ...and the storage instance should be gone.
	// After running the cleanup, the attachment should still be
	// around because volume was attached.
	_, err = s.storageBackend.StorageAttachment(storageTag, u.UnitTag())
	c.Assert(err, jc.Satisfies, errors.IsNotFound)

	// forceRemoveUnit
	s.assertCleanupRuns(c)

	assertUnitRemoved(c, u)
	// After this there are two cleanups remaining: removedUnit,
	// dyingMachine, forceRemoveMachine.
	s.assertCleanupCount(c, 3)
}

func (s *CleanupSuite) TestForceDestroyApplicationRemovesUnitsThatAreAlreadyDying(c *gc.C) {
	// If you remove an application when it has a unit in error, and
	// then you try to force-remove it, it should get cleaned up
	// correctly.
	mysql := s.AddTestingApplication(c, "mysql", s.AddTestingCharm(c, "mysql"))
	unit, err := mysql.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	preventUnitDestroyRemove(c, unit)
	s.assertDoesNotNeedCleanup(c)

	err = mysql.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	err = mysql.Refresh()
	c.Assert(err, jc.ErrorIsNil)

	// The application is dying and there's a cleanup to make the unit
	// dying but it hasn't run yet.
	s.assertNeedsCleanup(c)
	assertLife(c, mysql, state.Dying)
	assertLife(c, unit, state.Alive)

	// cleanupUnitsForDyingApplication
	s.assertCleanupRuns(c)
	assertLife(c, unit, state.Dying)
	// dyingUnit
	s.assertCleanupRuns(c)
	assertLife(c, unit, state.Dying)

	// Simulate the unit being in error by never coming back and
	// reporting the unit dead. The user eventually gets tired of
	// waiting and force-removes the application.
	op := mysql.DestroyOperation()
	op.Force = true
	err = s.State.ApplyOperation(op)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(op.Errors, gc.HasLen, 0)

	// cleanupUnitsForDyingApplication
	s.assertNeedsCleanup(c)
	s.assertCleanupRuns(c)
	assertLifeIs(c, unit, state.Dying)

	// Even though the unit was already dying, because we're forcing
	// we rerun the destroy operation so that fallback-scheduling can
	// happen.

	// dyingUnit
	s.assertNeedsCleanup(c)
	s.assertCleanupRuns(c)
	assertLifeIs(c, unit, state.Dying)

	// forceDestroyedUnit
	s.assertNeedsCleanup(c)
	s.assertCleanupRuns(c)
	assertLifeIs(c, unit, state.Dead)

	// forceRemoveUnit
	s.assertNeedsCleanup(c)
	s.assertCleanupRuns(c)
	assertUnitRemoved(c, unit)

	// application
	s.assertNeedsCleanup(c)
	s.assertCleanupRuns(c)
	assertRemoved(c, mysql)
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

// assertCleanupCountDirty is the same as assertCleanupCount, but it
// checks that there are still cleanups to run.
func (s *CleanupSuite) assertCleanupCountDirty(c *gc.C, count int) {
	for i := 0; i < count; i++ {
		c.Logf("checking cleanups %d", i)
		s.assertNeedsCleanup(c)
		s.assertCleanupRuns(c)
	}
	s.assertNeedsCleanup(c)
}

// assertNextCleanup tracks that the next cleanup runs, and logs what cleanup we are expecting.
func (s *CleanupSuite) assertNextCleanup(c *gc.C, message string) {
	c.Logf("expect cleanup: %s", message)
	s.assertNeedsCleanup(c)
	s.assertCleanupRuns(c)
}

type lifeChecker interface {
	Refresh() error
	Life() state.Life
}

func assertLifeIs(c *gc.C, thing lifeChecker, expected state.Life) {
	err := thing.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(thing.Life(), gc.Equals, expected)
}

func assertUnitRemoved(c *gc.C, thing lifeChecker) {
	err := thing.Refresh()
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func assertUnitInScope(c *gc.C, unit *state.Unit, rel *state.Relation, expected bool) {
	ru, err := rel.Unit(unit)
	c.Assert(err, jc.ErrorIsNil)
	inscope, err := ru.InScope()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(inscope, gc.Equals, expected)
}
