// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"
	"time"

	"github.com/juju/clock/testclock"
	"github.com/juju/errors"
	"github.com/juju/mgo/v3/bson"
	mgotesting "github.com/juju/mgo/v3/testing"
	"github.com/juju/mgo/v3/txn"
	"github.com/juju/names/v5"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/v3"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/mongo"
	"github.com/juju/juju/storage"
	"github.com/juju/juju/storage/provider"
	"github.com/juju/juju/storage/provider/dummy"
	"github.com/juju/juju/testing"
)

var _ = gc.Suite(&internalStateSuite{})

// internalStateSuite manages a *State instance for tests in the state
// package (i.e. internal tests) that need it. It is similar to
// state.testing.StateSuite but is duplicated to avoid cyclic imports.
type internalStateSuite struct {
	mgotesting.MgoSuite
	testing.BaseSuite
	controller *Controller
	pool       *StatePool
	state      *State
	owner      names.UserTag
	modelCount int
	clock      *testclock.Clock
}

func (s *internalStateSuite) SetUpSuite(c *gc.C) {
	s.MgoSuite.SetUpSuite(c)
	s.BaseSuite.SetUpSuite(c)
}

func (s *internalStateSuite) TearDownSuite(c *gc.C) {
	s.BaseSuite.TearDownSuite(c)
	s.MgoSuite.TearDownSuite(c)
}

func (s *internalStateSuite) SetUpTest(c *gc.C) {
	s.MgoSuite.SetUpTest(c)
	s.BaseSuite.SetUpTest(c)

	s.owner = names.NewLocalUserTag("test-admin")
	modelCfg := testing.ModelConfig(c)
	controllerCfg := testing.FakeControllerConfig()
	s.clock = testclock.NewClock(testing.NonZeroTime())
	ctlr, err := Initialize(InitializeParams{
		Clock:            s.clock,
		ControllerConfig: controllerCfg,
		ControllerModelArgs: ModelArgs{
			Type:        ModelTypeIAAS,
			CloudName:   "dummy",
			CloudRegion: "dummy-region",
			Owner:       s.owner,
			Config:      modelCfg,
			StorageProviderRegistry: storage.ChainedProviderRegistry{
				dummy.StorageProviders(),
				provider.CommonStorageProviders(),
			},
		},
		Cloud: cloud.Cloud{
			Name:      "dummy",
			Type:      "dummy",
			AuthTypes: []cloud.AuthType{cloud.EmptyAuthType},
			Regions: []cloud.Region{
				{
					Name: "dummy-region",
				},
			},
		},
		MongoSession:        s.Session,
		WatcherPollInterval: 10 * time.Millisecond,
		AdminPassword:       "dummy-secret",
		NewPolicy: func(*State) Policy {
			return internalStatePolicy{}
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	s.controller = ctlr
	s.pool = ctlr.StatePool()
	s.state, err = ctlr.SystemState()
	c.Assert(err, jc.ErrorIsNil)
	s.AddCleanup(func(*gc.C) {
		// Controller closes pool, pool closes all states.
		s.controller.Close()
	})
}

func (s *internalStateSuite) TearDownTest(c *gc.C) {
	s.BaseSuite.TearDownTest(c)
	s.MgoSuite.TearDownTest(c)
}

func (s *internalStateSuite) newState(c *gc.C) *State {
	s.modelCount++
	cfg := testing.CustomModelConfig(c, testing.Attrs{
		"name": fmt.Sprintf("testmodel%d", s.modelCount),
		"uuid": utils.MustNewUUID().String(),
	})
	_, st, err := s.controller.NewModel(ModelArgs{
		Type:        ModelTypeIAAS,
		CloudName:   "dummy",
		CloudRegion: "dummy-region",
		Config:      cfg,
		Owner:       s.owner,
		StorageProviderRegistry: storage.ChainedProviderRegistry{
			dummy.StorageProviders(),
			provider.CommonStorageProviders(),
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	s.AddCleanup(func(*gc.C) { st.Close() })
	return st
}

func (s *internalStateSuite) newCAASState(c *gc.C) *State {
	s.modelCount++
	cfg := testing.CustomModelConfig(c, testing.Attrs{
		"name": fmt.Sprintf("testmodel%d", s.modelCount),
		"uuid": utils.MustNewUUID().String(),
	})
	_, st, err := s.controller.NewModel(ModelArgs{
		Type:        ModelTypeCAAS,
		CloudName:   "dummy",
		CloudRegion: "dummy-region",
		Config:      cfg,
		Owner:       s.owner,
		StorageProviderRegistry: storage.ChainedProviderRegistry{
			dummy.StorageProviders(),
			provider.CommonStorageProviders(),
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	s.AddCleanup(func(*gc.C) { st.Close() })
	return st
}

type cleanupInternalSuite struct {
	internalStateSuite
}

var _ = gc.Suite(&cleanupInternalSuite{})

type failCollectionDatabase struct {
	Database
	collection string
	failAt     int
	calls      int
	err        error
}

func (db *failCollectionDatabase) GetCollection(name string) (mongo.Collection, SessionCloser, error) {
	if name == db.collection {
		db.calls++
		if db.failAt > 0 && db.calls == db.failAt {
			return nil, nil, db.err
		}
	}
	return db.Database.GetCollection(name)
}

type failRunTransactionDatabase struct {
	Database
	err    error
	failed bool
}

func (db *failRunTransactionDatabase) RunTransaction(ops []txn.Op) error {
	if !db.failed {
		db.failed = true
		return db.err
	}
	return db.Database.RunTransaction(ops)
}

func pendingForceCleanupsForUnit(c *gc.C, st *State, unitName string) []cleanupDoc {
	cleanups, closer, err := st.db().GetCollection(cleanupsC)
	c.Assert(err, jc.ErrorIsNil)
	defer closer()

	var docs []cleanupDoc
	err = cleanups.Find(bson.D{
		{"prefix", unitName},
		{"kind", bson.D{{"$in", []cleanupKind{
			cleanupDyingUnit,
			cleanupForceDestroyedUnit,
		}}}},
	}).All(&docs)
	c.Assert(err, jc.ErrorIsNil)
	return docs

}

func assertPendingForceCleanupCount(c *gc.C, st *State, unitName string, expected int) {
	c.Assert(pendingForceCleanupsForUnit(c, st, unitName), gc.HasLen, expected)
}

func assertForceDestroyedUnitCount(c *gc.C, st *State, unitName string, expected int) {
	cleanups, closer, err := st.db().GetCollection(cleanupsC)
	c.Assert(err, jc.ErrorIsNil)
	defer closer()
	var docs []cleanupDoc
	err = cleanups.Find(bson.D{
		{"prefix", unitName},
		{"kind", cleanupForceDestroyedUnit},
	}).All(&docs)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(docs, gc.HasLen, expected)
}

func addUnitToMachine(c *gc.C, application *Application, machine *Machine) *Unit {
	unit, err := application.AddUnit(AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(unit.AssignToMachine(machine), jc.ErrorIsNil)
	return unit
}

func setUnitLife(c *gc.C, st *State, unit *Unit, life Life) {
	c.Assert(st.db().RunTransaction([]txn.Op{{
		C:      unitsC,
		Id:     unit.doc.DocID,
		Assert: txn.DocExists,
		Update: bson.D{{"$set", bson.D{{"life", life}}}},
	}}), jc.ErrorIsNil)
	c.Assert(unit.Refresh(), jc.ErrorIsNil)
}

func removeUnitAgentStatus(c *gc.C, st *State, unit *Unit) {
	statuses, closer, err := st.db().GetCollection(statusesC)
	c.Assert(err, jc.ErrorIsNil)
	defer closer()
	c.Assert(statuses.Writeable().RemoveId(st.docID(unit.globalAgentKey())), jc.ErrorIsNil)
	_, err = getStatus(st.db(), unit.globalAgentKey(), "agent")
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *cleanupInternalSuite) TestCleanupForceDestroyedMachineHandlesUnitLifecycle(c *gc.C) {
	st := s.newState(c)
	machine, err := st.AddMachine(UbuntuBase("12.10"), JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	application := AddTestingApplication(c, st, "dummy", AddTestingCharm(c, st, "dummy"))
	unit, err := application.AddUnit(AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(unit.AssignToMachine(machine), jc.ErrorIsNil)
	now := testing.NonZeroTime()
	c.Assert(unit.SetAgentStatus(status.StatusInfo{
		Status: status.Idle,
		Since:  &now,
	}), jc.ErrorIsNil)
	filter := status.StatusHistoryFilter{Size: 10}
	history, err := unit.AgentHistory().StatusHistory(filter)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(history), jc.GreaterThan, 0)

	const maxWait = time.Minute
	// machine.ForceDestroy schedules cleanupEvacuateMachine.
	// This destroys Alive units to Dying and schedules a future
	// cleanupForceDestroyedMachine (forceDying=true) for Dying units.
	c.Assert(st.db().RunTransaction([]txn.Op{
		newCleanupOp(cleanupEvacuateMachine, machine.Id(), true, maxWait),
	}), jc.ErrorIsNil)

	c.Assert(st.Cleanup(nil), jc.ErrorIsNil)
	c.Assert(unit.Refresh(), jc.ErrorIsNil)
	c.Check(unit.Life(), gc.Equals, Dying)
	history, err = unit.AgentHistory().StatusHistory(filter)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(history, gc.HasLen, 0)
	AssertCleanupsWithKind(c, st, cleanupEvacuateMachine)

	c.Assert(st.Cleanup(nil), jc.ErrorIsNil)
	AssertCleanupCountWithKind(c, st, cleanupEvacuateMachine, 0)
	AssertCleanupCountWithKind(c, st, cleanupDyingUnit, 0)
	AssertCleanupCountWithKind(c, st, cleanupForceDestroyedUnit, 1)
	AssertCleanupMaxWait(c, st, cleanupForceDestroyedUnit, unit.Name(), maxWait)
	AssertCleanupCountWithKind(c, st, cleanupForceDestroyedMachine, 1)

	c.Assert(st.Cleanup(nil), jc.ErrorIsNil)
	AssertCleanupCountWithKind(c, st, cleanupEvacuateMachine, 0)
	AssertCleanupCountWithKind(c, st, cleanupDyingUnit, 0)
	AssertCleanupCountWithKind(c, st, cleanupForceDestroyedUnit, 1)

	// Advance the clock so the future cleanupForceDestroyedMachine is due,
	// then simulate the unit reaching Dead during its grace period.
	s.clock.Advance(maxWait)
	c.Assert(unit.EnsureDead(), jc.ErrorIsNil)
	c.Assert(st.Cleanup(nil), jc.ErrorIsNil)
	c.Assert(unit.Refresh(), jc.Satisfies, errors.IsNotFound)
	AssertCleanupsWithKind(c, st, cleanupForceDestroyedMachine)
}

func (s *cleanupInternalSuite) TestCleanupEvacuateMachineRemovesDeadUnitAndHistoryWithoutForce(c *gc.C) {
	st := s.newState(c)
	machine, err := st.AddMachine(UbuntuBase("12.10"), JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	application := AddTestingApplication(c, st, "dummy", AddTestingCharm(c, st, "dummy"))
	unit, err := application.AddUnit(AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(unit.AssignToMachine(machine), jc.ErrorIsNil)

	now := testing.NonZeroTime()
	for i := 0; i < 3; i++ {
		c.Assert(unit.SetAgentStatus(status.StatusInfo{
			Status:  status.Executing,
			Message: fmt.Sprintf("agent status %d", i),
			Since:   &now,
		}), jc.ErrorIsNil)
		c.Assert(unit.SetStatus(status.StatusInfo{
			Status:  status.Active,
			Message: fmt.Sprintf("workload status %d", i),
			Since:   &now,
		}), jc.ErrorIsNil)
		c.Assert(unit.SetWorkloadVersion(fmt.Sprintf("v.%d", i)), jc.ErrorIsNil)
	}
	filter := status.StatusHistoryFilter{Size: 100}
	histories := []func(status.StatusHistoryFilter) ([]status.StatusInfo, error){
		unit.AgentHistory().StatusHistory,
		unit.StatusHistory,
		unit.WorkloadVersionHistory().StatusHistory,
	}
	for _, history := range histories {
		info, err := history(filter)
		c.Assert(err, jc.ErrorIsNil)
		c.Check(len(info), jc.GreaterThan, 0)
	}

	c.Assert(unit.EnsureDead(), jc.ErrorIsNil)
	err = st.cleanupEvacuateMachineInternal(machine.Id(), false, false, time.Minute)
	c.Assert(err, gc.ErrorMatches, "waiting for units to be removed from "+machine.Id())
	c.Assert(unit.Refresh(), jc.Satisfies, errors.IsNotFound)
	for _, history := range histories {
		info, err := history(filter)
		c.Assert(err, jc.ErrorIsNil)
		c.Check(info, gc.HasLen, 0)
	}
}

func (s *cleanupInternalSuite) TestCleanupEvacuateMachineEscalatesDyingUnitOnce(c *gc.C) {
	st := s.newState(c)
	machine, err := st.AddMachine(UbuntuBase("12.10"), JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	application := AddTestingApplication(c, st, "dummy", AddTestingCharm(c, st, "dummy"))
	unit, err := application.AddUnit(AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(unit.AssignToMachine(machine), jc.ErrorIsNil)
	c.Assert(unit.SetAgentStatus(status.StatusInfo{Status: status.Idle}), jc.ErrorIsNil)
	c.Assert(unit.Destroy(), jc.ErrorIsNil)
	c.Assert(st.Cleanup(nil), jc.ErrorIsNil)
	AssertCleanupCountWithKind(c, st, cleanupDyingUnit, 0)

	const maxWait = time.Minute
	c.Assert(st.db().RunTransaction([]txn.Op{
		newCleanupOp(cleanupEvacuateMachine, machine.Id(), true, maxWait),
	}), jc.ErrorIsNil)
	c.Assert(st.Cleanup(nil), jc.ErrorIsNil)
	AssertCleanupCountWithKind(c, st, cleanupDyingUnit, 0)
	assertForceDestroyedUnitCount(c, st, unit.Name(), 0)
	AssertCleanupCountWithKind(c, st, cleanupForceDestroyedMachine, 1)

	c.Assert(st.Cleanup(nil), jc.ErrorIsNil)
	AssertCleanupCountWithKind(c, st, cleanupDyingUnit, 0)
	assertForceDestroyedUnitCount(c, st, unit.Name(), 0)
	AssertCleanupCountWithKind(c, st, cleanupForceDestroyedMachine, 1)
}

func (s *cleanupInternalSuite) TestCleanupEvacuateMachineContainerSchedulesSingleContinuation(c *gc.C) {
	st := s.newState(c)
	parent, err := st.AddMachine(UbuntuBase("12.10"), JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	container, err := st.AddMachineInsideMachine(MachineTemplate{
		Base: UbuntuBase("12.10"),
		Jobs: []MachineJob{JobHostUnits},
	}, parent.Id(), instance.LXD)
	c.Assert(err, jc.ErrorIsNil)
	application := AddTestingApplication(c, st, "dummy", AddTestingCharm(c, st, "dummy"))
	unit := addUnitToMachine(c, application, container)
	setUnitLife(c, st, unit, Dying)

	const maxWait = time.Minute
	c.Assert(parent.ForceDestroy(maxWait), jc.ErrorIsNil)
	c.Assert(st.Cleanup(nil), jc.ErrorIsNil)
	c.Assert(st.Cleanup(nil), jc.ErrorIsNil)

	AssertCleanupCountWithKind(c, st, cleanupEvacuateMachine, 0)
	AssertCleanupCountWithKind(c, st, cleanupForceDestroyedMachine, 1)
	AssertCleanupMaxWait(c, st, cleanupForceDestroyedMachine, parent.Id(), maxWait)
	c.Assert(unit.Refresh(), jc.ErrorIsNil)
	c.Check(unit.Life(), gc.Equals, Dying)
}

func (s *cleanupInternalSuite) TestCleanupEvacuateMachineMissingAgentStatusConverges(c *gc.C) {
	for _, startingLife := range []Life{Alive, Dying} {
		c.Logf("starting life %s", startingLife)
		st := s.newState(c)
		machine, err := st.AddMachine(UbuntuBase("12.10"), JobHostUnits)
		c.Assert(err, jc.ErrorIsNil)
		application := AddTestingApplication(c, st, "dummy", AddTestingCharm(c, st, "dummy"))
		unit := addUnitToMachine(c, application, machine)
		c.Assert(unit.SetAgentStatus(status.StatusInfo{Status: status.Idle}), jc.ErrorIsNil)
		removeUnitAgentStatus(c, st, unit)
		if startingLife == Dying {
			setUnitLife(c, st, unit, Dying)
		}

		const maxWait = time.Duration(0)
		c.Assert(st.db().RunTransaction([]txn.Op{
			newCleanupOp(cleanupEvacuateMachine, machine.Id(), true, maxWait),
		}), jc.ErrorIsNil)

		c.Assert(st.Cleanup(nil), jc.ErrorIsNil)
		if startingLife == Alive {
			// Alive unit is destroyed to Dying; the cleanup op remains pending.
			AssertCleanupCountWithKind(c, st, cleanupEvacuateMachine, 1)
		} else {
			// Dying unit is skipped; the handler schedules a future cleanup and completes.
			AssertCleanupCountWithKind(c, st, cleanupEvacuateMachine, 0)
		}
		c.Assert(unit.Refresh(), jc.ErrorIsNil)
		c.Check(unit.Life(), gc.Equals, Dying)

		// For the Alive case, the second pass sees the now-Dying unit,
		// skips it, and schedules a future cleanupForceDestroyedMachine.
		// For the Dying case, the first pass already scheduled it.
		c.Assert(st.Cleanup(nil), jc.ErrorIsNil)
		AssertCleanupCountWithKind(c, st, cleanupEvacuateMachine, 0)
		assertForceDestroyedUnitCount(c, st, unit.Name(), 0)

		// Run cleanups to converge: the future-dated
		// cleanupForceDestroyedMachine fires (forceDying=true), forces the
		// Dying unit to Dead, and removes it and the machine.
		c.Assert(st.Cleanup(nil), jc.ErrorIsNil)
		c.Assert(st.Cleanup(nil), jc.ErrorIsNil)
		c.Assert(st.Cleanup(nil), jc.ErrorIsNil)
		c.Assert(st.Cleanup(nil), jc.ErrorIsNil)
		needsCleanup, err := st.NeedsCleanup()
		c.Assert(err, jc.ErrorIsNil)
		c.Check(needsCleanup, jc.IsFalse)
		c.Assert(unit.Refresh(), jc.Satisfies, errors.IsNotFound)
		assertForceDestroyedUnitCount(c, st, unit.Name(), 0)
	}
}

func (s *cleanupInternalSuite) TestCleanupEvacuateMachineIgnoresMalformedPendingDyingCleanup(c *gc.C) {
	st := s.newState(c)
	machine, err := st.AddMachine(UbuntuBase("12.10"), JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	application := AddTestingApplication(c, st, "dummy", AddTestingCharm(c, st, "dummy"))
	unit := addUnitToMachine(c, application, machine)
	c.Assert(unit.SetAgentStatus(status.StatusInfo{Status: status.Idle}), jc.ErrorIsNil)
	setUnitLife(c, st, unit, Dying)

	const maxWait = time.Minute
	c.Assert(st.db().RunTransaction([]txn.Op{
		newCleanupOp(cleanupDyingUnit, unit.Name(), "not-a-bool", true, maxWait),
	}), jc.ErrorIsNil)

	for i := 0; i < 3; i++ {
		err := st.cleanupEvacuateMachineInternal(machine.Id(), true, false, maxWait)
		c.Assert(err, jc.ErrorIs, errForceCleanupRequired)
		AssertCleanupCountWithKind(c, st, cleanupDyingUnit, 1)
		assertForceDestroyedUnitCount(c, st, unit.Name(), 0)
	}
}

func (s *cleanupInternalSuite) TestCleanupEvacuateMachineContinuesAfterUnitOperationError(c *gc.C) {
	st := s.newState(c)
	machine, err := st.AddMachine(UbuntuBase("12.10"), JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	application := AddTestingApplication(c, st, "dummy", AddTestingCharm(c, st, "dummy"))
	var units []*Unit
	for i := 0; i < 3; i++ {
		unit := addUnitToMachine(c, application, machine)
		c.Assert(unit.SetAgentStatus(status.StatusInfo{Status: status.Idle}), jc.ErrorIsNil)
		units = append(units, unit)
	}

	boom := errors.New("min units lookup failed")
	faultState := *st
	database := &failCollectionDatabase{
		Database:   st.database,
		collection: minUnitsC,
		failAt:     2,
		err:        boom,
	}
	faultState.database = database
	const maxWait = time.Minute
	err = faultState.cleanupEvacuateMachineInternal(machine.Id(), true, false, maxWait)
	c.Assert(err, gc.ErrorMatches, "waiting for units to be removed from "+machine.Id())
	c.Check(database.calls, gc.Equals, 3)

	alive, dying := 0, 0
	for _, unit := range units {
		c.Assert(unit.Refresh(), jc.ErrorIsNil)
		switch unit.Life() {
		case Alive:
			alive++
			assertForceDestroyedUnitCount(c, st, unit.Name(), 0)
		case Dying:
			dying++
			assertForceDestroyedUnitCount(c, st, unit.Name(), 0)
		default:
			c.Fatalf("unit %q has unexpected life %s", unit.Name(), unit.Life())
		}
	}
	c.Check(alive, gc.Equals, 1)
	c.Check(dying, gc.Equals, 2)

	err = st.cleanupEvacuateMachineInternal(machine.Id(), true, false, maxWait)
	c.Assert(err, jc.ErrorIs, errForceCleanupRequired)
	for _, unit := range units {
		c.Assert(unit.Refresh(), jc.ErrorIsNil)
		c.Check(unit.Life(), gc.Equals, Dying)
	}
}

func (s *cleanupInternalSuite) TestCleanupForceDestroyedMachineLegacyArgsEvacuatesUnit(c *gc.C) {
	st := s.newState(c)
	machine, err := st.AddMachine(UbuntuBase("12.10"), JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	application := AddTestingApplication(c, st, "dummy", AddTestingCharm(c, st, "dummy"))
	unit := addUnitToMachine(c, application, machine)
	c.Assert(unit.SetAgentStatus(status.StatusInfo{Status: status.Idle}), jc.ErrorIsNil)
	c.Assert(st.db().RunTransaction([]txn.Op{
		newCleanupOp(cleanupForceDestroyedMachine, machine.Id()),
	}), jc.ErrorIsNil)

	c.Assert(st.Cleanup(nil), jc.ErrorIsNil)
	c.Assert(unit.Refresh(), jc.ErrorIsNil)
	c.Check(unit.Life(), gc.Equals, Dying)
	AssertCleanupCountWithKind(c, st, cleanupDyingUnit, 1)
	assertForceDestroyedUnitCount(c, st, unit.Name(), 0)
	AssertCleanupCountWithKind(c, st, cleanupForceDestroyedMachine, 1)

	c.Assert(st.Cleanup(nil), jc.ErrorIsNil)
	AssertCleanupCountWithKind(c, st, cleanupForceDestroyedUnit, 1)
	AssertCleanupMaxWait(c, st, cleanupForceDestroyedUnit, unit.Name(), 0)

	// cleanupForceDestroyedMachine fires (forceDying=true):
	// unit Dying -> forceDestroyUnit -> Dead.
	// cleanupForceDestroyedUnit fires:
	// forceDestroyUnit -> schedules cleanupForceRemoveUnit.
	c.Assert(st.Cleanup(nil), jc.ErrorIsNil)

	// cleanupForceDestroyedMachine re-fires:
	// unit Dead -> RemoveWithForce -> removed.
	// machine.Units() empty -> cleanupDestroyedMachineInternal ->
	// machine Dead.
	// cleanupForceRemoveUnit fires: unit already removed.
	c.Assert(st.Cleanup(nil), jc.ErrorIsNil)

	// cleanupForceRemoveMachine fires: machine Dead -> Remove.
	c.Assert(st.Cleanup(nil), jc.ErrorIsNil)

	needsCleanup, err := st.NeedsCleanup()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(needsCleanup, jc.IsFalse)
	c.Assert(unit.Refresh(), jc.Satisfies, errors.IsNotFound)
	c.Assert(machine.Refresh(), jc.Satisfies, errors.IsNotFound)
}

func (s *cleanupInternalSuite) TestCleanupEvacuateMachineEscalatesPendingNonForceCleanup(c *gc.C) {
	st := s.newState(c)
	machine, err := st.AddMachine(UbuntuBase("12.10"), JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	application := AddTestingApplication(c, st, "dummy", AddTestingCharm(c, st, "dummy"))
	unit := addUnitToMachine(c, application, machine)
	c.Assert(unit.SetAgentStatus(status.StatusInfo{Status: status.Idle}), jc.ErrorIsNil)
	c.Assert(unit.Destroy(), jc.ErrorIsNil)
	c.Assert(unit.Refresh(), jc.ErrorIsNil)
	c.Check(unit.Life(), gc.Equals, Dying)
	assertPendingForceCleanupCount(c, st, unit.Name(), 1)
	assertForceDestroyedUnitCount(c, st, unit.Name(), 0)

	for i := 0; i < 3; i++ {
		err := st.cleanupEvacuateMachineInternal(machine.Id(), true, false, 0)
		c.Assert(err, jc.ErrorIs, errForceCleanupRequired)
		assertPendingForceCleanupCount(c, st, unit.Name(), 1)
		assertForceDestroyedUnitCount(c, st, unit.Name(), 0)
	}
}

func (s *cleanupInternalSuite) TestCleanupEvacuateMachineIgnoresForceRemoveForDyingUnit(c *gc.C) {
	st := s.newState(c)
	machine, err := st.AddMachine(UbuntuBase("12.10"), JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	application := AddTestingApplication(c, st, "dummy", AddTestingCharm(c, st, "dummy"))
	unit, err := application.AddUnit(AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(unit.AssignToMachine(machine), jc.ErrorIsNil)
	c.Assert(unit.SetAgentStatus(status.StatusInfo{Status: status.Idle}), jc.ErrorIsNil)
	c.Assert(unit.Destroy(), jc.ErrorIsNil)
	c.Assert(st.Cleanup(nil), jc.ErrorIsNil)

	const maxWait = time.Minute
	c.Assert(st.db().RunTransaction([]txn.Op{
		newCleanupAtOp(st.stateClock.Now().Add(maxWait), cleanupForceRemoveUnit, unit.Name(), maxWait),
	}), jc.ErrorIsNil)

	err = st.cleanupEvacuateMachineInternal(machine.Id(), true, false, maxWait)
	c.Assert(err, jc.ErrorIs, errForceCleanupRequired)
	AssertCleanupCountWithKind(c, st, cleanupForceRemoveUnit, 1)
	AssertCleanupCountWithKind(c, st, cleanupDyingUnit, 0)
	assertForceDestroyedUnitCount(c, st, unit.Name(), 0)
}

func (s *cleanupInternalSuite) TestCleanupForceDestroyedUnitEscalatesSubordinateOnce(c *gc.C) {
	st := s.newState(c)
	principalApplication := AddTestingApplication(c, st, "mysql", AddTestingCharm(c, st, "mysql"))
	subordinateApplication := AddTestingApplication(c, st, "logging", AddTestingCharm(c, st, "logging"))
	endpoints, err := st.InferEndpoints(principalApplication.Name(), subordinateApplication.Name())
	c.Assert(err, jc.ErrorIsNil)
	relation, err := st.AddRelation(endpoints...)
	c.Assert(err, jc.ErrorIsNil)
	principal, err := principalApplication.AddUnit(AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	principalRelationUnit, err := relation.Unit(principal)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(principalRelationUnit.EnterScope(nil), jc.ErrorIsNil)
	c.Assert(principalRelationUnit.LeaveScope(), jc.ErrorIsNil)
	c.Assert(principal.Refresh(), jc.ErrorIsNil)
	c.Assert(principal.SubordinateNames(), gc.HasLen, 1)
	subordinate, err := st.Unit(principal.SubordinateNames()[0])
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(subordinate.SetAgentStatus(status.StatusInfo{Status: status.Idle}), jc.ErrorIsNil)

	const maxWait = time.Minute
	c.Assert(st.db().RunTransaction([]txn.Op{
		newCleanupOp(cleanupForceDestroyedUnit, principal.Name(), maxWait),
	}), jc.ErrorIsNil)

	c.Assert(st.Cleanup(nil), jc.ErrorIsNil)
	docs := pendingForceCleanupsForUnit(c, st, subordinate.Name())
	c.Assert(docs, gc.HasLen, 1)
	c.Check(docs[0].Kind, gc.Equals, cleanupDyingUnit)

	c.Assert(st.Cleanup(nil), jc.ErrorIsNil)
	docs = pendingForceCleanupsForUnit(c, st, subordinate.Name())
	c.Assert(docs, gc.HasLen, 1)
	c.Check(docs[0].Kind, gc.Equals, cleanupForceDestroyedUnit)
	stableDocID := docs[0].DocID
	stableWhen := docs[0].When

	for i := 0; i < 3; i++ {
		c.Assert(st.Cleanup(nil), jc.ErrorIsNil)
		docs = pendingForceCleanupsForUnit(c, st, subordinate.Name())
		c.Assert(docs, gc.HasLen, 1)
		c.Check(docs[0].Kind, gc.Equals, cleanupForceDestroyedUnit)
		c.Check(docs[0].DocID, gc.Equals, stableDocID)
		c.Check(docs[0].When, gc.Equals, stableWhen)
	}
}

func (s *cleanupInternalSuite) TestCleanupEvacuateMissingMachineRemovesUpgradeSeriesLock(c *gc.C) {
	st := s.newState(c)
	machine, err := st.AddMachine(UbuntuBase("12.10"), JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(machine.CreateUpgradeSeriesLock(nil, UbuntuBase("16.04")), jc.ErrorIsNil)

	machines, closer, err := st.db().GetCollection(machinesC)
	c.Assert(err, jc.ErrorIsNil)
	defer closer()
	c.Assert(machines.Writeable().RemoveId(machine.Id()), jc.ErrorIsNil)

	c.Assert(st.cleanupEvacuateMachineInternal(machine.Id(), true, false, time.Minute), jc.ErrorIsNil)
	_, err = st.getUpgradeSeriesLock(machine.Id())
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *cleanupInternalSuite) TestCleanupForceDestroyedUnitRetriesSubordinateLookupError(c *gc.C) {
	st := s.newState(c)
	application := AddTestingApplication(c, st, "dummy", AddTestingCharm(c, st, "dummy"))
	unit, err := application.AddUnit(AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(st.db().RunTransaction([]txn.Op{{
		C:      unitsC,
		Id:     unit.doc.DocID,
		Assert: txn.DocExists,
		Update: bson.D{{"$set", bson.D{{"subordinates", []string{"logging/0"}}}}},
	}}), jc.ErrorIsNil)
	c.Assert(st.db().RunTransaction([]txn.Op{
		newCleanupOp(cleanupForceDestroyedUnit, unit.Name(), time.Minute),
	}), jc.ErrorIsNil)

	boom := errors.New("subordinate lookup failed")
	faultState := *st
	database := &failCollectionDatabase{
		Database:   st.database,
		collection: unitsC,
		failAt:     2,
		err:        boom,
	}
	faultState.database = database
	c.Assert(faultState.Cleanup(nil), jc.ErrorIsNil)
	c.Check(database.calls, gc.Equals, 2)

	AssertCleanupCountWithKind(c, st, cleanupForceDestroyedUnit, 1)
	AssertCleanupCountWithKind(c, st, cleanupForceRemoveUnit, 0)
	c.Assert(unit.Refresh(), jc.ErrorIsNil)

	c.Assert(st.db().RunTransaction([]txn.Op{{
		C:      unitsC,
		Id:     unit.doc.DocID,
		Assert: txn.DocExists,
		Update: bson.D{{"$set", bson.D{{"subordinates", []string{}}}}},
	}}), jc.ErrorIsNil)
	c.Assert(unit.Refresh(), jc.ErrorIsNil)
	c.Assert(st.Cleanup(nil), jc.ErrorIsNil)
	c.Assert(unit.Refresh(), jc.ErrorIsNil)
	c.Check(unit.Life(), gc.Equals, Dead)
	AssertCleanupCountWithKind(c, st, cleanupForceDestroyedUnit, 0)
	AssertCleanupCountWithKind(c, st, cleanupForceRemoveUnit, 1)
}

func (s *cleanupInternalSuite) TestCleanupForceDestroyedUnitRetriesEnsureDeadError(c *gc.C) {
	st := s.newState(c)
	application := AddTestingApplication(c, st, "dummy", AddTestingCharm(c, st, "dummy"))
	unit, err := application.AddUnit(AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(st.db().RunTransaction([]txn.Op{{
		C:      unitsC,
		Id:     unit.doc.DocID,
		Assert: txn.DocExists,
		Update: bson.D{{"$set", bson.D{{"life", Dying}}}},
	}}), jc.ErrorIsNil)
	c.Assert(unit.Refresh(), jc.ErrorIsNil)
	c.Assert(st.db().RunTransaction([]txn.Op{
		newCleanupOp(cleanupForceDestroyedUnit, unit.Name(), time.Minute),
	}), jc.ErrorIsNil)

	boom := errors.New("ensure dead failed")
	faultState := *st
	database := &failRunTransactionDatabase{
		Database: st.database,
		err:      boom,
	}
	faultState.database = database
	c.Assert(faultState.Cleanup(nil), jc.ErrorIsNil)
	c.Check(database.failed, jc.IsTrue)

	AssertCleanupCountWithKind(c, st, cleanupForceDestroyedUnit, 1)
	AssertCleanupCountWithKind(c, st, cleanupForceRemoveUnit, 0)
	c.Assert(unit.Refresh(), jc.ErrorIsNil)
	c.Check(unit.Life(), gc.Equals, Dying)

	c.Assert(st.Cleanup(nil), jc.ErrorIsNil)
	c.Assert(unit.Refresh(), jc.ErrorIsNil)
	c.Check(unit.Life(), gc.Equals, Dead)
	AssertCleanupCountWithKind(c, st, cleanupForceDestroyedUnit, 0)
	AssertCleanupCountWithKind(c, st, cleanupForceRemoveUnit, 1)
}

func (s *cleanupInternalSuite) TestDestroyWithForceApplicationLookupErrorPreservesUnitData(c *gc.C) {
	st := s.newState(c)
	application := AddTestingApplication(c, st, "dummy", AddTestingCharm(c, st, "dummy"))
	unit, err := application.AddUnit(AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(unit.AssignToNewMachine(), jc.ErrorIsNil)

	now := testing.NonZeroTime()
	for i := 0; i < 3; i++ {
		c.Assert(unit.SetAgentStatus(status.StatusInfo{
			Status:  status.Executing,
			Message: fmt.Sprintf("agent status %d", i),
			Since:   &now,
		}), jc.ErrorIsNil)
		c.Assert(unit.SetStatus(status.StatusInfo{
			Status:  status.Active,
			Message: fmt.Sprintf("workload status %d", i),
			Since:   &now,
		}), jc.ErrorIsNil)
		c.Assert(unit.SetWorkloadVersion(fmt.Sprintf("v.%d", i)), jc.ErrorIsNil)
	}
	c.Assert(unit.UnassignFromMachine(), jc.ErrorIsNil)
	filter := status.StatusHistoryFilter{Size: 100}
	histories := []func(status.StatusHistoryFilter) ([]status.StatusInfo, error){
		unit.AgentHistory().StatusHistory,
		unit.StatusHistory,
		unit.WorkloadVersionHistory().StatusHistory,
	}

	boom := errors.New("application lookup failed")
	faultState := *st
	database := &failCollectionDatabase{
		Database:   st.database,
		collection: applicationsC,
		failAt:     1,
		err:        boom,
	}
	faultState.database = database
	faultUnit := *unit
	faultUnit.st = &faultState

	opErrs, err := faultUnit.DestroyWithForce(true, 0)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(database.calls, gc.Equals, 1)
	foundInjectedError := false
	for _, opErr := range opErrs {
		if errors.Cause(opErr) == boom {
			foundInjectedError = true
			break
		}
	}
	c.Check(foundInjectedError, jc.IsTrue)
	c.Check(faultUnit.Life(), gc.Equals, Alive)
	c.Assert(unit.Refresh(), jc.ErrorIsNil)
	c.Check(unit.Life(), gc.Equals, Alive)
	for _, history := range histories {
		info, err := history(filter)
		c.Assert(err, jc.ErrorIsNil)
		c.Check(len(info), jc.GreaterThan, 0)
	}
}

func (s *cleanupInternalSuite) TestCleanupEvacuateDyingMachineWithoutForceIsNoOp(c *gc.C) {
	st := s.newState(c)
	machine, err := st.AddMachine(UbuntuBase("12.10"), JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(machine.Destroy(), jc.ErrorIsNil)

	err = st.cleanupEvacuateMachineInternal(machine.Id(), false, false, 0)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(machine.Refresh(), jc.ErrorIsNil)
	c.Check(machine.Life(), gc.Equals, Dying)
}

func (s *cleanupInternalSuite) TestCleanupContainersWaitsForDyingContainerWithoutForce(c *gc.C) {
	st := s.newState(c)
	parent, err := st.AddMachine(UbuntuBase("12.10"), JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	child, err := st.AddMachineInsideMachine(MachineTemplate{
		Base: UbuntuBase("12.10"),
		Jobs: []MachineJob{JobHostUnits},
	}, parent.Id(), instance.LXD)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(child.Destroy(), jc.ErrorIsNil)
	c.Assert(parent.DestroyWithParams(false, true, 0), jc.ErrorIsNil)

	err = st.cleanupEvacuateMachineInternal(parent.Id(), false, false, 0)
	c.Assert(err, gc.ErrorMatches, fmt.Sprintf(
		"waiting for container %s to be removed from %s",
		child.Id(), parent.Id(),
	))
	c.Assert(st.Cleanup(nil), jc.ErrorIsNil)
	AssertEvacuateMachineCleanupParams(c, st, parent.Id(), false, 0)
	c.Assert(parent.Refresh(), jc.ErrorIsNil)
	c.Check(parent.Life(), gc.Equals, Alive)
	c.Assert(child.Refresh(), jc.ErrorIsNil)
	c.Check(child.Life(), gc.Equals, Dying)

	c.Assert(child.EnsureDead(), jc.ErrorIsNil)
	c.Assert(st.Cleanup(nil), jc.ErrorIsNil)
	c.Assert(child.Refresh(), jc.Satisfies, errors.IsNotFound)
	c.Assert(parent.Refresh(), jc.ErrorIsNil)
	c.Check(parent.Life(), gc.Equals, Dead)
	needsCleanup, err := st.NeedsCleanup()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(needsCleanup, jc.IsFalse)
}

func (s *cleanupInternalSuite) TestCleanupContainersContinuesAfterMissingContainerWithoutForce(c *gc.C) {
	st := s.newState(c)
	parent, err := st.AddMachine(UbuntuBase("12.10"), JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	missingChild, err := st.AddMachineInsideMachine(MachineTemplate{
		Base: UbuntuBase("12.10"),
		Jobs: []MachineJob{JobHostUnits},
	}, parent.Id(), instance.LXD)
	c.Assert(err, jc.ErrorIsNil)
	dyingChild, err := st.AddMachineInsideMachine(MachineTemplate{
		Base: UbuntuBase("12.10"),
		Jobs: []MachineJob{JobHostUnits},
	}, parent.Id(), instance.LXD)
	c.Assert(err, jc.ErrorIsNil)
	machines, closer, err := st.db().GetCollection(machinesC)
	c.Assert(err, jc.ErrorIsNil)
	defer closer()
	c.Assert(machines.Writeable().RemoveId(missingChild.Id()), jc.ErrorIsNil)
	c.Assert(dyingChild.Destroy(), jc.ErrorIsNil)
	c.Assert(parent.DestroyWithParams(false, true, 0), jc.ErrorIsNil)

	err = st.cleanupEvacuateMachineInternal(parent.Id(), false, false, 0)
	c.Assert(err, gc.ErrorMatches, fmt.Sprintf(
		"waiting for container %s to be removed from %s",
		dyingChild.Id(), parent.Id(),
	))
	c.Assert(missingChild.Refresh(), jc.Satisfies, errors.IsNotFound)
	c.Assert(dyingChild.Refresh(), jc.ErrorIsNil)
	c.Check(dyingChild.Life(), gc.Equals, Dying)
	c.Assert(parent.Refresh(), jc.ErrorIsNil)
	c.Check(parent.Life(), gc.Equals, Alive)
}

type internalStatePolicy struct{}

func (internalStatePolicy) Prechecker() (environs.InstancePrechecker, error) {
	return nil, errors.NotImplementedf("Prechecker")
}

func (internalStatePolicy) ConfigValidator() (config.Validator, error) {
	return nil, errors.NotImplementedf("ConfigValidator")
}

func (internalStatePolicy) ConstraintsValidator(context.ProviderCallContext) (constraints.Validator, error) {
	return nil, errors.NotImplementedf("ConstraintsValidator")
}

func (internalStatePolicy) InstanceDistributor() (context.Distributor, error) {
	return nil, errors.NotImplementedf("InstanceDistributor")
}

func (internalStatePolicy) StorageProviderRegistry() (storage.ProviderRegistry, error) {
	return storage.ChainedProviderRegistry{
		dummy.StorageProviders(),
		provider.CommonStorageProviders(),
	}, nil
}

func (internalStatePolicy) ProviderConfigSchemaSource(cloudName string) (config.ConfigSchemaSource, error) {
	return nil, errors.NotImplementedf("ConfigSchemaSource")
}
