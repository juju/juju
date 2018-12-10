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
	testing.IsolationSuite

	gauges *cache.ControllerGauges

	changes chan interface{}
	config  cache.ControllerConfig
}

var _ = gc.Suite(&ControllerSuite{})

func (s *ControllerSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
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

func (s *ControllerSuite) new(c *gc.C) (*cache.Controller, <-chan interface{}) {
	events := s.captureModelEvents(c)
	controller, err := cache.NewController(s.config)
	c.Assert(err, jc.ErrorIsNil)
	s.AddCleanup(func(c *gc.C) { workertest.CleanKill(c, controller) })
	return controller, events
}

func (s *ControllerSuite) TestAddModel(c *gc.C) {
	controller, events := s.new(c)
	s.processChange(c, modelChange, events)

	c.Check(controller.ModelUUIDs(), jc.SameContents, []string{"model-uuid"})
	c.Check(controller.Report(), gc.DeepEquals, map[string]interface{}{
		"model-uuid": map[string]interface{}{
			"name": "model-owner/test-model",
			"life": life.Value("alive"),
		}})
}

func (s *ControllerSuite) TestRemoveModel(c *gc.C) {
	controller, events := s.new(c)
	s.processChange(c, modelChange, events)

	remove := cache.RemoveModel{ModelUUID: "model-uuid"}
	s.processChange(c, remove, events)

	c.Check(controller.ModelUUIDs(), gc.HasLen, 0)
	c.Check(controller.Report(), gc.HasLen, 0)
}

func (s *ControllerSuite) processChange(c *gc.C, change interface{}, notify <-chan interface{}) {
	select {
	case s.changes <- change:
	case <-time.After(testing.LongWait):
		c.Fatalf("contoller did not read change")
	}
	select {
	case obtained := <-notify:
		c.Check(obtained, jc.DeepEquals, change)
	case <-time.After(testing.LongWait):
		c.Fatalf("contoller did not handle change")
	}
}

func (s *ControllerSuite) captureModelEvents(c *gc.C) <-chan interface{} {
	events := make(chan interface{})
	s.config.Notify = func(change interface{}) {
		send := false
		switch change.(type) {
		case cache.ModelChange:
			send = true
		case cache.RemoveModel:
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

func (s *ControllerSuite) nextChange(c *gc.C, changes <-chan interface{}) interface{} {
	var obtained interface{}
	select {
	case obtained = <-changes:
	case <-time.After(testing.LongWait):
		c.Fatalf("no change")
	}
	return obtained
}
