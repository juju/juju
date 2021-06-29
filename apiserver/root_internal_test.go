// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"time"

	"github.com/juju/clock/testclock"
	"github.com/juju/errors"
	"github.com/juju/loggo/v2"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/cache"
	statetesting "github.com/juju/juju/state/testing"
)

type facadeContextSuite struct {
	statetesting.StateSuite

	changes    chan interface{}
	handled    chan interface{}
	controller *cache.Controller
	clock      *testclock.Clock
}

var _ = gc.Suite(&facadeContextSuite{})

func (s *facadeContextSuite) SetUpTest(c *gc.C) {
	s.StateSuite.SetUpTest(c)

	s.changes = make(chan interface{})
	s.handled = make(chan interface{})

	controller, err := cache.NewController(cache.ControllerConfig{
		Changes: s.changes,
		Notify: func(e interface{}) {
			s.handled <- e
		}})
	c.Assert(err, jc.ErrorIsNil)
	s.controller = controller
	s.clock = testclock.NewClock(time.Now())
}

func (s *facadeContextSuite) newContext() *facadeContext {
	// This is a bare minimum facade context for these tests.
	return &facadeContext{
		r: &apiRoot{
			clock: s.clock,
			shared: &sharedServerContext{
				controller: s.controller,
				logger:     loggo.GetLogger("test"),
			},
			state: s.State,
		},
	}
}

func (s *facadeContextSuite) processChange(c *gc.C, change interface{}) {
	select {
	case s.changes <- change:
	case <-time.After(testing.LongWait):
		c.Fatalf("controller did not read change")
	}
	select {
	case obtained := <-s.handled:
		c.Check(obtained, jc.DeepEquals, change)
	case <-time.After(testing.LongWait):
		c.Fatalf("controller did not handle change")
	}
}

func (s *facadeContextSuite) TestCachedModelValid(c *gc.C) {
	// Populate the cache with the model we are looking for.
	s.processChange(c, cache.ModelChange{
		ModelUUID: "some-uuid",
	})
	// We don't need to advance the clock to get the model
	// as it is already in the cache.
	ctx := s.newContext()
	model, err := ctx.CachedModel("some-uuid")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(model.UUID(), gc.Equals, "some-uuid")
}

func (s *facadeContextSuite) TestCachedModelMissing(c *gc.C) {
	ctx := s.newContext()
	done := make(chan interface{})
	go func() {
		defer close(done)
		model, err := ctx.CachedModel("some-uuid")
		c.Check(err, jc.Satisfies, errors.IsNotFound)
		c.Check(model, gc.IsNil)
	}()

	s.clock.WaitAdvance(10*time.Second, testing.LongWait, 1)
	select {
	case <-done:
	case <-time.After(testing.LongWait):
		c.Error("CachedModel didn't return")
	}
}

func (s *facadeContextSuite) TestCachedModelTimeout(c *gc.C) {
	// Make a model in the DB, but don't tell the cache about it.
	state := s.Factory.MakeModel(c, nil)
	defer state.Close()

	ctx := s.newContext()
	done := make(chan interface{})
	go func() {
		defer close(done)
		model, err := ctx.CachedModel(state.ModelUUID())
		c.Check(err, jc.Satisfies, errors.IsTimeout)
		c.Check(model, gc.IsNil)
	}()

	s.clock.WaitAdvance(10*time.Second, testing.LongWait, 1)
	select {
	case <-done:
	case <-time.After(testing.LongWait):
		c.Error("CachedModel didn't return")
	}
}
