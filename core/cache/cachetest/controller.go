// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cachetest

import (
	"time"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/cache"
	"github.com/juju/juju/state"
	"github.com/juju/juju/testing"
)

// TestController wraps a cache controller for testing.
// It allows synchronisation of state objects with the cache
// without the need for a multi-watcher and cache worker.
// This is useful when testing with StateSuite;
// JujuConnSuite sets up a cache worker and multiwatcher to keep the model
// cache in sync, so direct population using this technique is not necessary.
type TestController struct {
	*cache.Controller

	matchers []func(interface{}) bool
	changes  chan interface{}
	events   chan interface{}
}

// NewTestController returns creates and returns a new test controller
// with an initial set of matchers for receiving cache event notifications.
// The controller can be instantiated like this in suite/test setups in order
// to retain a common set of matchers, but `Init` should be called in each
// test (see below).
func NewTestController(matchers ...func(interface{}) bool) *TestController {
	return &TestController{
		matchers: matchers,
	}
}

// Init instantiates the inner cache controller and sets up event
// synchronisation based on the input matchers.
// Changes sent to the cache can be waited on by using the `NextChange` method.
//
// NOTE: It is recommended to perform this initialisation in the actual test
// method rather than `SetupSuite` or `SetupTest` as different gc.C references
// are supplied to each of those methods.
func (tc *TestController) Init(c *gc.C, matchers ...func(interface{}) bool) {
	tc.events = make(chan interface{})
	matchers = append(tc.matchers, matchers...)

	notify := func(change interface{}) {
		send := false
		for _, m := range matchers {
			if m(change) {
				send = true
				break
			}
		}

		if send {
			c.Logf("sending %#v", change)
			select {
			case tc.events <- change:
			case <-time.After(testing.LongWait):
				c.Fatalf("change not processed by test")
			}
		}
	}

	tc.changes = make(chan interface{})
	cc, err := cache.NewController(cache.ControllerConfig{
		Changes: tc.changes,
		Notify:  notify,
	})
	c.Assert(err, jc.ErrorIsNil)
	tc.Controller = cc
}

// UpdateModel updates the current model for the input state in the cache.
func (tc *TestController) UpdateModel(c *gc.C, m *state.Model) {
	tc.SendChange(ModelChange(c, m))
}

// UpdateCharm updates the input state charm in the cache.
func (tc *TestController) UpdateCharm(modelUUID string, ch *state.Charm) {
	tc.SendChange(CharmChange(modelUUID, ch))
}

// UpdateApplication updates the input state application in the cache.
func (tc *TestController) UpdateApplication(c *gc.C, modelUUID string, app *state.Application) {
	tc.SendChange(ApplicationChange(c, modelUUID, app))
}

// UpdateMachine updates the input state machine in the cache.
func (tc *TestController) UpdateMachine(c *gc.C, modelUUID string, machine *state.Machine) {
	tc.SendChange(MachineChange(c, modelUUID, machine))
}

// UpdateUnit updates the input state unit in the cache.
func (tc *TestController) UpdateUnit(c *gc.C, modelUUID string, unit *state.Unit) {
	tc.SendChange(UnitChange(c, modelUUID, unit))
}

func (tc *TestController) SendChange(change interface{}) {
	tc.changes <- change
}

// NextChange returns the next change processed by the cache that satisfies a
// matcher, or fails the test with a time-out.
func (tc *TestController) NextChange(c *gc.C) interface{} {
	var obtained interface{}
	select {
	case obtained = <-tc.events:
	case <-time.After(testing.LongWait):
		c.Fatalf("change not processed by test")
	}
	return obtained
}
