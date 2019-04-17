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
	cache.BaseSuite

	changes chan interface{}
	config  cache.ControllerConfig
}

var _ = gc.Suite(&ControllerSuite{})

func (s *ControllerSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
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
