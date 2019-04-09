// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.
package cache_test

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/worker.v1/workertest"

	"github.com/juju/juju/core/cache"
	"github.com/juju/juju/core/life"
)

type ControllerSuite struct {
	baseSuite

	changes chan interface{}
	config  cache.ControllerConfig
}

var _ = gc.Suite(&ControllerSuite{})

func (s *ControllerSuite) SetUpTest(c *gc.C) {
	s.baseSuite.SetUpTest(c)
	s.changes = make(chan interface{})
	s.config = cache.ControllerConfig{
		Changes: s.changes,
	}
}

func (s *ControllerSuite) TestConfigValid(c *gc.C) {
	err := s.config.Validate()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *ControllerSuite) TestConfigMissingChanges(c *gc.C) {
	s.config.Changes = nil
	err := s.config.Validate()
	c.Check(err, gc.ErrorMatches, "nil Changes not valid")
	c.Check(err, jc.Satisfies, errors.IsNotValid)
}

func (s *ControllerSuite) TestController(c *gc.C) {
	controller, err := cache.NewController(s.config)
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
		}})
}

func (s *ControllerSuite) TestRemoveModel(c *gc.C) {
	controller, events := s.new(c)
	s.processChange(c, modelChange, events)

	remove := cache.RemoveModel{ModelUUID: modelChange.ModelUUID}
	s.processChange(c, remove, events)

	c.Check(controller.ModelUUIDs(), gc.HasLen, 0)
	c.Check(controller.Report(), gc.HasLen, 0)
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
}

func (s *ControllerSuite) TestRemoveApplication(c *gc.C) {
	controller, events := s.new(c)
	s.processChange(c, appChange, events)

	remove := cache.RemoveApplication{
		ModelUUID: modelChange.ModelUUID,
		Name:      appChange.Name,
	}
	s.processChange(c, remove, events)

	mod, err := controller.Model(modelChange.ModelUUID)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(mod.Report()["application-count"], gc.Equals, 0)
}

func (s *ControllerSuite) TestAddCharm(c *gc.C) {
	controller, events := s.new(c)
	s.processChange(c, charmChange, events)

	mod, err := controller.Model(modelChange.ModelUUID)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(mod.Report()["charm-count"], gc.Equals, 1)

	app, err := mod.Charm(charmChange.CharmURL)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(app, gc.NotNil)
}

func (s *ControllerSuite) TestRemoveCharm(c *gc.C) {
	controller, events := s.new(c)
	s.processChange(c, charmChange, events)

	remove := cache.RemoveCharm{
		ModelUUID: modelChange.ModelUUID,
		CharmURL:  charmChange.CharmURL,
	}
	s.processChange(c, remove, events)

	mod, err := controller.Model(modelChange.ModelUUID)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(mod.Report()["charm-count"], gc.Equals, 0)
}

func (s *ControllerSuite) TestAddMachine(c *gc.C) {
	controller, events := s.new(c)
	s.processChange(c, machineChange, events)

	mod, err := controller.Model(machineChange.ModelUUID)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(mod.Report()["machine-count"], gc.Equals, 1)

	app, err := mod.Machine(machineChange.Id)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(app, gc.NotNil)
}

func (s *ControllerSuite) TestRemoveMachine(c *gc.C) {
	controller, events := s.new(c)
	s.processChange(c, machineChange, events)

	remove := cache.RemoveMachine{
		ModelUUID: machineChange.ModelUUID,
		Id:        machineChange.Id,
	}
	s.processChange(c, remove, events)

	mod, err := controller.Model(machineChange.ModelUUID)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(mod.Report()["machine-count"], gc.Equals, 0)
}

func (s *ControllerSuite) TestAddUnit(c *gc.C) {
	controller, events := s.new(c)
	s.processChange(c, unitChange, events)

	mod, err := controller.Model(modelChange.ModelUUID)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(mod.Report()["unit-count"], gc.Equals, 1)

	app, err := mod.Unit(unitChange.Name)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(app, gc.NotNil)
}

func (s *ControllerSuite) TestRemoveUnit(c *gc.C) {
	controller, events := s.new(c)
	s.processChange(c, unitChange, events)

	remove := cache.RemoveUnit{
		ModelUUID: modelChange.ModelUUID,
		Name:      unitChange.Name,
	}
	s.processChange(c, remove, events)

	mod, err := controller.Model(modelChange.ModelUUID)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(mod.Report()["unit-count"], gc.Equals, 0)
}

func (s *ControllerSuite) TestMarkWithNoModels(c *gc.C) {
	controller, err := cache.NewController(s.config)
	c.Assert(err, jc.ErrorIsNil)

	result := controller.Mark()
	c.Check(result, gc.HasLen, 0)

	workertest.CleanKill(c, controller)
}

func (s *ControllerSuite) TestMarkWithModel(c *gc.C) {
	controller, events := s.new(c)
	s.processChange(c, modelChange, events)

	result := controller.Mark()
	c.Check(result, gc.DeepEquals, map[string]int{
		"model-uuid": 1,
	})
}

func (s *ControllerSuite) TestMarkWithModelAndEntities(c *gc.C) {
	controller, events := s.new(c)
	s.processChange(c, modelChange, events)
	s.processChange(c, appChange, events)
	s.processChange(c, machineChange, events)
	s.processChange(c, unitChange, events)

	result := controller.Mark()
	c.Check(result, gc.DeepEquals, map[string]int{
		"model-uuid": 4,
	})
}

func (s *ControllerSuite) TestSweepWithNoModels(c *gc.C) {
	controller, err := cache.NewController(s.config)
	c.Assert(err, jc.ErrorIsNil)

	result, err := controller.Sweep()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(result, gc.HasLen, 0)

	workertest.CleanKill(c, controller)
}

func (s *ControllerSuite) TestSweepAfterMarkWithNoModels(c *gc.C) {
	controller, err := cache.NewController(s.config)
	c.Assert(err, jc.ErrorIsNil)

	markResult := controller.Mark()
	c.Check(markResult, gc.HasLen, 0)

	sweepResult, err := controller.Sweep()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(sweepResult, gc.HasLen, 0)

	workertest.CleanKill(c, controller)
}

func (s *ControllerSuite) TestSweepAfterMarkWithModel(c *gc.C) {
	controller, events := s.new(c)
	s.processChange(c, modelChange, events)
	s.processChange(c, appChange, events)
	s.processChange(c, machineChange, events)
	s.processChange(c, unitChange, events)

	markResult := controller.Mark()
	c.Check(markResult, gc.DeepEquals, map[string]int{
		"model-uuid": 4,
	})

	done := make(chan struct{})
	go func() {
		c.Check(s.nextChange(c, events), gc.DeepEquals, removeModel)
		c.Check(s.nextChange(c, events), gc.DeepEquals, removeApp)
		c.Check(s.nextChange(c, events), gc.DeepEquals, removeMachine)
		c.Check(s.nextChange(c, events), gc.DeepEquals, removeUnit)
		done <- struct{}{}
	}()

	sweepResult, err := controller.Sweep()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(sweepResult, gc.DeepEquals, map[string]cache.SweepInfo{
		"model-uuid": {
			StaleCount: 4,
			FreshCount: 0,
		},
	})

	select {
	case <-done:
	case <-time.After(testing.LongWait):
		c.Fatalf("timed out waiting %s for started", testing.LongWait)
	}
	c.Check(controller.ModelUUIDs(), gc.HasLen, 0)
	c.Check(controller.Report(), gc.HasLen, 0)
}

func (s *ControllerSuite) TestMarkWithSweepAndUpdateCalls(c *gc.C) {
	controller, events := s.new(c)
	s.processChange(c, modelChange, events)
	s.processChange(c, appChange, events)
	s.processChange(c, machineChange, events)
	s.processChange(c, unitChange, events)

	markResult := controller.Mark()
	c.Check(markResult, gc.DeepEquals, map[string]int{
		"model-uuid": 4,
	})

	s.processChange(c, modelChange, events)
	s.processChange(c, appChange, events)

	done := make(chan struct{})
	go func() {
		c.Check(s.nextChange(c, events), gc.DeepEquals, removeMachine)
		c.Check(s.nextChange(c, events), gc.DeepEquals, removeUnit)
		done <- struct{}{}
	}()

	sweepResult, err := controller.Sweep()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(sweepResult, gc.DeepEquals, map[string]cache.SweepInfo{
		"model-uuid": {
			StaleCount: 2,
			FreshCount: 2,
		},
	})

	select {
	case <-done:
	case <-time.After(testing.LongWait):
		c.Fatalf("timed out waiting %s for started", testing.LongWait)
	}

	c.Check(controller.ModelUUIDs(), gc.DeepEquals, []string{"model-uuid"})
	c.Check(controller.Report(), gc.DeepEquals, map[string]interface{}{
		"model-uuid": map[string]interface{}{
			"name":              "model-owner/test-model",
			"life":              life.Value("alive"),
			"application-count": 1,
			"charm-count":       0,
			"machine-count":     0,
			"unit-count":        0,
		},
	})
}

func (s *ControllerSuite) new(c *gc.C) (*cache.Controller, <-chan interface{}) {
	events := s.captureEvents(c)
	controller, err := cache.NewController(s.config)
	c.Assert(err, jc.ErrorIsNil)
	s.AddCleanup(func(c *gc.C) { workertest.CleanKill(c, controller) })
	return controller, events
}

func (s *ControllerSuite) captureEvents(c *gc.C) <-chan interface{} {
	events := make(chan interface{})
	s.config.Notify = func(change interface{}) {
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
	case s.changes <- change:
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
