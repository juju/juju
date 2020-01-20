// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.
package cache_test

import (
	"time"

	"github.com/juju/clock/testclock"
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/worker.v1/workertest"

	"github.com/juju/juju/core/cache"
	"github.com/juju/juju/core/life"
)

type ControllerSuite struct {
	cache.BaseSuite
}

var _ = gc.Suite(&ControllerSuite{})

func (s *ControllerSuite) TestConfigValid(c *gc.C) {
	err := s.Config.Validate()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *ControllerSuite) TestConfigMissingChanges(c *gc.C) {
	s.Config.Changes = nil
	err := s.Config.Validate()
	c.Check(err, gc.ErrorMatches, "nil Changes not valid")
	c.Check(err, jc.Satisfies, errors.IsNotValid)
}

func (s *ControllerSuite) TestController(c *gc.C) {
	controller, err := s.NewController()
	c.Assert(err, jc.ErrorIsNil)

	c.Check(controller.ModelUUIDs(), gc.HasLen, 0)
	c.Check(controller.Report(), gc.HasLen, 0)

	workertest.CleanKill(c, controller)
}

func (s *ControllerSuite) TestAddModel(c *gc.C) {
	controller, events := s.new(c)
	s.processChange(c, modelChange, events)

	c.Check(controller.ModelUUIDs(), jc.SameContents, []string{modelChange.ModelUUID})
	c.Check(controller.Report(), gc.DeepEquals, map[string]interface{}{
		"model-uuid": map[string]interface{}{
			"name":              "model-owner/test-model",
			"life":              life.Value("alive"),
			"application-count": 0,
			"charm-count":       0,
			"machine-count":     0,
			"unit-count":        0,
			"relation-count":    0,
			"branch-count":      0,
		}})

	// The model has the first ID and is registered.
	mod, err := controller.Model(modelChange.ModelUUID)
	c.Assert(err, jc.ErrorIsNil)
	s.AssertResident(c, mod.CacheId(), true)
}

func (s *ControllerSuite) TestRemoveModel(c *gc.C) {
	controller, events := s.new(c)
	s.processChange(c, modelChange, events)

	mod, err := controller.Model(modelChange.ModelUUID)
	c.Assert(err, jc.ErrorIsNil)

	remove := cache.RemoveModel{ModelUUID: modelChange.ModelUUID}
	s.processChange(c, remove, events)

	c.Check(controller.ModelUUIDs(), gc.HasLen, 0)
	c.Check(controller.Report(), gc.HasLen, 0)
	s.AssertResident(c, mod.CacheId(), false)
}

func (s *ControllerSuite) TestWaitForModelExists(c *gc.C) {
	controller, events := s.new(c)
	clock := testclock.NewClock(time.Now())
	done := make(chan struct{})
	// Process the change event before waiting on the model. This
	// way we know the model exists in the cache before we ask.
	s.processChange(c, modelChange, events)

	go func() {
		defer close(done)
		model, err := controller.WaitForModel(modelChange.ModelUUID, clock)
		c.Check(err, jc.ErrorIsNil)
		c.Check(model.UUID(), gc.Equals, modelChange.ModelUUID)
	}()

	select {
	case <-done:
	case <-time.After(testing.LongWait):
		c.Errorf("WaitForModel did not return after %s", testing.LongWait)
	}
}

func (s *ControllerSuite) TestWaitForModelArrives(c *gc.C) {
	//loggo.GetLogger("juju.core.cache").SetLogLevel(loggo.TRACE)
	controller, events := s.new(c)
	clock := testclock.NewClock(time.Now())
	done := make(chan struct{})
	go func() {
		defer close(done)
		model, err := controller.WaitForModel(modelChange.ModelUUID, clock)
		c.Check(err, jc.ErrorIsNil)
		c.Check(model.UUID(), gc.Equals, modelChange.ModelUUID)
	}()

	// Don't process the change until we know the model is selecting
	// on the clock.
	c.Assert(clock.WaitAdvance(time.Second, testing.LongWait, 1), jc.ErrorIsNil)
	s.processChange(c, modelChange, events)
	select {
	case <-done:
	case <-time.After(testing.LongWait):
		c.Errorf("WaitForModel did not return after %s", testing.LongWait)
	}
}

func (s *ControllerSuite) TestWaitForModelTimeout(c *gc.C) {
	controller, events := s.new(c)
	clock := testclock.NewClock(time.Now())
	done := make(chan struct{})
	go func() {
		defer close(done)
		model, err := controller.WaitForModel(modelChange.ModelUUID, clock)
		c.Check(err, jc.Satisfies, errors.IsTimeout)
		c.Check(model, gc.IsNil)
	}()

	c.Assert(clock.WaitAdvance(10*time.Second, testing.LongWait, 1), jc.ErrorIsNil)

	s.processChange(c, modelChange, events)
	select {
	case <-done:
	case <-time.After(testing.LongWait):
		c.Errorf("WaitForModel did not return after %s", testing.LongWait)
	}
}

func (s *ControllerSuite) TestAddApplication(c *gc.C) {
	controller, events := s.new(c)
	s.processChange(c, appChange, events)

	mod, err := controller.Model(modelChange.ModelUUID)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(mod.Report()["application-count"], gc.Equals, 1)

	app, err := mod.Application(appChange.Name)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(app, gc.NotNil)

	s.AssertResident(c, app.CacheId(), true)
}

func (s *ControllerSuite) TestRemoveApplication(c *gc.C) {
	controller, events := s.new(c)
	s.processChange(c, appChange, events)

	mod, err := controller.Model(modelChange.ModelUUID)
	c.Assert(err, jc.ErrorIsNil)
	app, err := mod.Application(appChange.Name)
	c.Assert(err, jc.ErrorIsNil)

	remove := cache.RemoveApplication{
		ModelUUID: modelChange.ModelUUID,
		Name:      appChange.Name,
	}
	s.processChange(c, remove, events)

	c.Assert(err, jc.ErrorIsNil)
	c.Check(mod.Report()["application-count"], gc.Equals, 0)
	s.AssertResident(c, app.CacheId(), false)
}

func (s *ControllerSuite) TestAddCharm(c *gc.C) {
	controller, events := s.new(c)
	s.processChange(c, charmChange, events)

	mod, err := controller.Model(modelChange.ModelUUID)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(mod.Report()["charm-count"], gc.Equals, 1)

	ch, err := mod.Charm(charmChange.CharmURL)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(ch, gc.NotNil)
	s.AssertResident(c, ch.CacheId(), true)
}

func (s *ControllerSuite) TestRemoveCharm(c *gc.C) {
	controller, events := s.new(c)
	s.processChange(c, charmChange, events)

	mod, err := controller.Model(modelChange.ModelUUID)
	c.Assert(err, jc.ErrorIsNil)
	ch, err := mod.Charm(charmChange.CharmURL)
	c.Assert(err, jc.ErrorIsNil)

	remove := cache.RemoveCharm{
		ModelUUID: modelChange.ModelUUID,
		CharmURL:  charmChange.CharmURL,
	}
	s.processChange(c, remove, events)

	c.Check(mod.Report()["charm-count"], gc.Equals, 0)
	s.AssertResident(c, ch.CacheId(), false)
}

func (s *ControllerSuite) TestAddMachine(c *gc.C) {
	controller, events := s.new(c)
	s.processChange(c, machineChange, events)

	mod, err := controller.Model(machineChange.ModelUUID)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(mod.Report()["machine-count"], gc.Equals, 1)

	machine, err := mod.Machine(machineChange.Id)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(machine, gc.NotNil)
	s.AssertResident(c, machine.CacheId(), true)
}

func (s *ControllerSuite) TestRemoveMachine(c *gc.C) {
	controller, events := s.new(c)
	s.processChange(c, machineChange, events)

	mod, err := controller.Model(machineChange.ModelUUID)
	c.Assert(err, jc.ErrorIsNil)
	machine, err := mod.Machine(machineChange.Id)
	c.Assert(err, jc.ErrorIsNil)

	remove := cache.RemoveMachine{
		ModelUUID: machineChange.ModelUUID,
		Id:        machineChange.Id,
	}
	s.processChange(c, remove, events)

	c.Check(mod.Report()["machine-count"], gc.Equals, 0)
	s.AssertResident(c, machine.CacheId(), false)
}

func (s *ControllerSuite) TestAddUnit(c *gc.C) {
	controller, events := s.new(c)
	s.processChange(c, unitChange, events)

	mod, err := controller.Model(modelChange.ModelUUID)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(mod.Report()["unit-count"], gc.Equals, 1)

	unit, err := mod.Unit(unitChange.Name)
	c.Assert(err, jc.ErrorIsNil)
	s.AssertResident(c, unit.CacheId(), true)
}

func (s *ControllerSuite) TestRemoveUnit(c *gc.C) {
	controller, events := s.new(c)
	s.processChange(c, unitChange, events)

	mod, err := controller.Model(modelChange.ModelUUID)
	c.Assert(err, jc.ErrorIsNil)
	unit, err := mod.Unit(unitChange.Name)
	c.Assert(err, jc.ErrorIsNil)

	remove := cache.RemoveUnit{
		ModelUUID: modelChange.ModelUUID,
		Name:      unitChange.Name,
	}
	s.processChange(c, remove, events)

	c.Check(mod.Report()["unit-count"], gc.Equals, 0)
	s.AssertResident(c, unit.CacheId(), false)
}

func (s *ControllerSuite) TestAddRelation(c *gc.C) {
	controller, events := s.new(c)
	s.processChange(c, relationChange, events)

	mod, err := controller.Model(relationChange.ModelUUID)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(mod.Report()["relation-count"], gc.Equals, 1)

	relation, err := mod.Relation(relationChange.Key)
	c.Assert(err, jc.ErrorIsNil)
	s.AssertResident(c, relation.CacheId(), true)
}

func (s *ControllerSuite) TestRemoveRelation(c *gc.C) {
	controller, events := s.new(c)
	s.processChange(c, relationChange, events)

	mod, err := controller.Model(relationChange.ModelUUID)
	c.Assert(err, jc.ErrorIsNil)
	relation, err := mod.Relation(relationChange.Key)
	c.Assert(err, jc.ErrorIsNil)

	remove := cache.RemoveRelation{
		ModelUUID: modelChange.ModelUUID,
		Key:       relationChange.Key,
	}
	s.processChange(c, remove, events)

	c.Check(mod.Report()["relation-count"], gc.Equals, 0)
	s.AssertResident(c, relation.CacheId(), false)
}

func (s *ControllerSuite) TestAddBranch(c *gc.C) {
	controller, events := s.new(c)
	s.processChange(c, branchChange, events)

	mod, err := controller.Model(modelChange.ModelUUID)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(mod.Report()["branch-count"], gc.Equals, 1)

	branch, err := mod.Branch(branchChange.Name)
	c.Assert(err, jc.ErrorIsNil)
	s.AssertResident(c, branch.CacheId(), true)
}

func (s *ControllerSuite) TestRemoveBranch(c *gc.C) {
	controller, events := s.new(c)
	s.processChange(c, branchChange, events)

	mod, err := controller.Model(modelChange.ModelUUID)
	c.Assert(err, jc.ErrorIsNil)
	branch, err := mod.Branch(branchChange.Name)
	c.Assert(err, jc.ErrorIsNil)

	remove := cache.RemoveBranch{
		ModelUUID: modelChange.ModelUUID,
		Id:        branchChange.Id,
	}
	s.processChange(c, remove, events)

	c.Check(mod.Report()["unit-count"], gc.Equals, 0)
	s.AssertResident(c, branch.CacheId(), false)
}

func (s *ControllerSuite) TestMarkAndSweep(c *gc.C) {
	controller, events := s.new(c)

	// Note that the model change is processed last.
	s.processChange(c, charmChange, events)
	s.processChange(c, appChange, events)
	s.processChange(c, machineChange, events)
	s.processChange(c, unitChange, events)
	s.processChange(c, modelChange, events)

	controller.Mark()

	done := make(chan struct{})
	go func() {
		// Removals are congruent with LIFO.
		// Model is last because models are added if they do not exist,
		// when we first get a delta for one of their entities.
		c.Check(s.nextChange(c, events), gc.FitsTypeOf, cache.RemoveUnit{})
		c.Check(s.nextChange(c, events), gc.FitsTypeOf, cache.RemoveMachine{})
		c.Check(s.nextChange(c, events), gc.FitsTypeOf, cache.RemoveApplication{})
		c.Check(s.nextChange(c, events), gc.FitsTypeOf, cache.RemoveCharm{})
		c.Check(s.nextChange(c, events), gc.FitsTypeOf, cache.RemoveModel{})
		close(done)
	}()

	controller.Sweep()
	select {
	case <-done:
	case <-time.After(testing.LongWait):
		c.Fatal("timeout waiting for sweep removal messages")
	}

	s.AssertNoResidents(c)
}

func (s *ControllerSuite) new(c *gc.C) (*cache.Controller, <-chan interface{}) {
	events := s.captureEvents(c)
	controller, err := s.NewController()
	c.Assert(err, jc.ErrorIsNil)
	s.AddCleanup(func(c *gc.C) { workertest.CleanKill(c, controller) })
	return controller, events
}

func (s *ControllerSuite) captureEvents(c *gc.C) <-chan interface{} {
	events := make(chan interface{})
	s.Config.Notify = func(change interface{}) {
		send := false
		switch change.(type) {
		case cache.ModelChange:
			send = true
		case cache.RemoveModel:
			send = true
		case cache.ApplicationChange:
			send = true
		case cache.RemoveApplication:
			send = true
		case cache.CharmChange:
			send = true
		case cache.RemoveCharm:
			send = true
		case cache.MachineChange:
			send = true
		case cache.RemoveMachine:
			send = true
		case cache.UnitChange:
			send = true
		case cache.RemoveUnit:
			send = true
		case cache.RelationChange:
			send = true
		case cache.RemoveRelation:
			send = true
		case cache.BranchChange:
			send = true
		case cache.RemoveBranch:
			send = true
		default:
			// no-op
		}
		if send {
			c.Logf("sending %#v", change)
			select {
			case events <- change:
			case <-time.After(testing.LongWait):
				c.Fatalf("change not processed by test")
			}
		}
	}
	return events
}

func (s *ControllerSuite) processChange(c *gc.C, change interface{}, notify <-chan interface{}) {
	select {
	case s.Changes <- change:
	case <-time.After(testing.LongWait):
		c.Fatalf("controller did not read change")
	}
	select {
	case obtained := <-notify:
		c.Check(obtained, jc.DeepEquals, change)
	case <-time.After(testing.LongWait):
		c.Fatalf("controller did not handle change")
	}
}

func (s *ControllerSuite) nextChange(c *gc.C, changes <-chan interface{}) interface{} {
	var obtained interface{}
	select {
	case obtained = <-changes:
	case <-time.After(testing.LongWait):
		c.Fatalf("no change")
	}
	return obtained
}

func (s *ControllerSuite) TestWatchMachineStops(c *gc.C) {
	controller, _ := s.newWithMachine(c)
	m, err := controller.Model(modelChange.ModelUUID)
	c.Assert(err, jc.ErrorIsNil)

	w, err := m.WatchMachines()
	c.Assert(err, jc.ErrorIsNil)
	wc := cache.NewStringsWatcherC(c, w)
	// Sends initial event.
	wc.AssertOneChange([]string{machineChange.Id})

	// The worker is the first and only resource (1).
	resourceId := uint64(1)
	s.AssertWorkerResource(c, m.Resident, resourceId, true)
	wc.AssertStops()
	s.AssertWorkerResource(c, m.Resident, resourceId, false)
}

func (s *ControllerSuite) TestWatchMachineAddMachine(c *gc.C) {
	w, events := s.setupWithWatchMachine(c)
	defer workertest.CleanKill(c, w)
	wc := cache.NewStringsWatcherC(c, w)
	// Sends initial event.
	wc.AssertOneChange([]string{machineChange.Id})

	change := cache.MachineChange{
		ModelUUID: modelChange.ModelUUID,
		Id:        "2",
	}
	s.processChange(c, change, events)
	wc.AssertOneChange([]string{change.Id})
}

func (s *ControllerSuite) TestWatchMachineAddContainerNoChange(c *gc.C) {
	w, events := s.setupWithWatchMachine(c)
	defer workertest.CleanKill(c, w)
	wc := cache.NewStringsWatcherC(c, w)
	// Sends initial event.
	wc.AssertOneChange([]string{machineChange.Id})

	change := cache.MachineChange{
		ModelUUID: modelChange.ModelUUID,
		Id:        "2/lxd/0",
	}
	s.processChange(c, change, events)
	change2 := change
	change2.Id = "3"
	s.processChange(c, change2, events)
	wc.AssertOneChange([]string{change2.Id})
}

func (s *ControllerSuite) TestWatchMachineRemoveMachine(c *gc.C) {
	w, events := s.setupWithWatchMachine(c)
	defer workertest.CleanKill(c, w)
	wc := cache.NewStringsWatcherC(c, w)
	// Sends initial event.
	wc.AssertOneChange([]string{machineChange.Id})

	change := cache.RemoveMachine{
		ModelUUID: modelChange.ModelUUID,
		Id:        machineChange.Id,
	}
	s.processChange(c, change, events)
	wc.AssertOneChange([]string{change.Id})
}

func (s *ControllerSuite) TestWatchMachineChangeMachine(c *gc.C) {
	w, events := s.setupWithWatchMachine(c)
	defer workertest.CleanKill(c, w)
	wc := cache.NewStringsWatcherC(c, w)
	// Sends initial event.
	wc.AssertOneChange([]string{machineChange.Id})

	change := cache.MachineChange{
		ModelUUID: modelChange.ModelUUID,
		Id:        "0",
	}
	s.processChange(c, change, events)
	wc.AssertNoChange()
}

func (s *ControllerSuite) TestWatchMachineGatherMachines(c *gc.C) {
	w, events := s.setupWithWatchMachine(c)
	defer workertest.CleanKill(c, w)
	wc := cache.NewStringsWatcherC(c, w)
	// Sends initial event.
	wc.AssertOneChange([]string{machineChange.Id})

	change := cache.MachineChange{
		ModelUUID: modelChange.ModelUUID,
		Id:        "2",
	}
	s.processChange(c, change, events)
	change2 := change
	change2.Id = "3"
	s.processChange(c, change2, events)
	wc.AssertMaybeCombinedChanges([]string{change.Id, change2.Id})
}

func (s *ControllerSuite) newWithMachine(c *gc.C) (*cache.Controller, <-chan interface{}) {
	events := s.captureEvents(c)
	controller, err := s.NewController()
	c.Assert(err, jc.ErrorIsNil)
	s.AddCleanup(func(c *gc.C) { workertest.CleanKill(c, controller) })
	s.processChange(c, modelChange, events)
	s.processChange(c, machineChange, events)
	return controller, events
}

func (s *ControllerSuite) setupWithWatchMachine(c *gc.C) (*cache.PredicateStringsWatcher, <-chan interface{}) {
	controller, events := s.newWithMachine(c)
	m, err := controller.Model(modelChange.ModelUUID)
	c.Assert(err, jc.ErrorIsNil)

	containerChange := cache.MachineChange{
		ModelUUID: modelChange.ModelUUID,
		Id:        "2/lxd/0",
	}
	s.processChange(c, containerChange, events)

	w, err := m.WatchMachines()
	c.Assert(err, jc.ErrorIsNil)
	return w, events
}
