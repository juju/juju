// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package watcher_test

import (
	stdtesting "testing"
	"time"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/juju/testing"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api"
	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/state/api/watcher"
	statetesting "launchpad.net/juju-core/state/testing"
	coretesting "launchpad.net/juju-core/testing"
)

func TestAll(t *stdtesting.T) {
	coretesting.MgoTestPackage(t)
}

type watcherSuite struct {
	testing.JujuConnSuite

	stateAPI *api.State

	// These are raw State objects. Use them for setup and assertions, but
	// should never be touched by the API calls themselves
	rawMachine *state.Machine
}

var _ = gc.Suite(&watcherSuite{})

func (s *watcherSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)

	// Create a machine to work with
	var err error
	s.rawMachine, err = s.State.AddMachine("series", state.JobHostUnits)
	c.Assert(err, gc.IsNil)
	err = s.rawMachine.SetPassword("test-password")
	c.Assert(err, gc.IsNil)

	// Login as the machine agent of the created machine.
	s.stateAPI = s.OpenAPIAs(c, s.rawMachine.Tag(), "test-password")
	c.Assert(s.stateAPI, gc.NotNil)
}

func (s *watcherSuite) TearDownTest(c *gc.C) {
	if s.stateAPI != nil {
		err := s.stateAPI.Close()
		c.Check(err, gc.IsNil)
	}
	s.JujuConnSuite.TearDownTest(c)
}

func (s *watcherSuite) TestWatchInitialEventConsumed(c *gc.C) {
	// Machiner.Watch should send the initial event as part of the Watch
	// call (for NotifyWatchers there is no state to be transmitted). So a
	// call to Next() should not have anything to return.
	var results params.NotifyWatchResults
	args := params.Entities{Entities: []params.Entity{{Tag: s.rawMachine.Tag()}}}
	err := s.stateAPI.Call("Machiner", "", "Watch", args, &results)
	c.Assert(err, gc.IsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	result := results.Results[0]
	c.Assert(result.Error, gc.IsNil)

	// We expect the Call() to "Next" to block, so run it in a goroutine.
	done := make(chan error)
	go func() {
		ignored := struct{}{}
		done <- s.stateAPI.Call("NotifyWatcher", result.NotifyWatcherId, "Next", nil, &ignored)
	}()

	select {
	case err := <-done:
		c.Errorf("Call(Next) did not block immediately after Watch(): err %v", err)
	case <-time.After(coretesting.ShortWait):
	}
}

func (s *watcherSuite) TestWatchMachine(c *gc.C) {
	var results params.NotifyWatchResults
	args := params.Entities{Entities: []params.Entity{{Tag: s.rawMachine.Tag()}}}
	err := s.stateAPI.Call("Machiner", "", "Watch", args, &results)
	c.Assert(err, gc.IsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	result := results.Results[0]
	c.Assert(result.Error, gc.IsNil)

	// params.NotifyWatcher conforms to the state.NotifyWatcher interface
	w := watcher.NewNotifyWatcher(s.stateAPI, result)
	wc := statetesting.NewNotifyWatcherC(c, s.State, w)
	wc.AssertOneChange()
	statetesting.AssertStop(c, w)
	wc.AssertClosed()
}

func (s *watcherSuite) TestNotifyWatcherStopsWithPendingSend(c *gc.C) {
	var results params.NotifyWatchResults
	args := params.Entities{Entities: []params.Entity{{Tag: s.rawMachine.Tag()}}}
	err := s.stateAPI.Call("Machiner", "", "Watch", args, &results)
	c.Assert(err, gc.IsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	result := results.Results[0]
	c.Assert(result.Error, gc.IsNil)

	// params.NotifyWatcher conforms to the state.NotifyWatcher interface
	w := watcher.NewNotifyWatcher(s.stateAPI, result)
	wc := statetesting.NewNotifyWatcherC(c, s.State, w)

	// Now, without reading any changes try stopping the watcher.
	statetesting.AssertCanStopWhenSending(c, w)
	wc.AssertClosed()
}

func (s *watcherSuite) TestWatchUnitsKeepsEvents(c *gc.C) {
	// Create two services, relate them, and add one unit to each - a
	// principal and a subordinate.
	mysql, err := s.State.AddService("mysql", s.AddTestingCharm(c, "mysql"))
	c.Assert(err, gc.IsNil)
	logging, err := s.State.AddService("logging", s.AddTestingCharm(c, "logging"))
	c.Assert(err, gc.IsNil)
	eps, err := s.State.InferEndpoints([]string{"mysql", "logging"})
	c.Assert(err, gc.IsNil)
	rel, err := s.State.AddRelation(eps...)
	c.Assert(err, gc.IsNil)
	principal, err := mysql.AddUnit()
	c.Assert(err, gc.IsNil)
	err = principal.AssignToMachine(s.rawMachine)
	c.Assert(err, gc.IsNil)
	relUnit, err := rel.Unit(principal)
	c.Assert(err, gc.IsNil)
	err = relUnit.EnterScope(nil)
	c.Assert(err, gc.IsNil)
	subordinate, err := logging.Unit("logging/0")
	c.Assert(err, gc.IsNil)

	// Call the Deployer facade's WatchUnits for machine-0.
	var results params.StringsWatchResults
	args := params.Entities{Entities: []params.Entity{{Tag: s.rawMachine.Tag()}}}
	err = s.stateAPI.Call("Deployer", "", "WatchUnits", args, &results)
	c.Assert(err, gc.IsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	result := results.Results[0]
	c.Assert(result.Error, gc.IsNil)

	// Start a StringsWatcher and check the initial event.
	w := watcher.NewStringsWatcher(s.stateAPI, result)
	wc := statetesting.NewStringsWatcherC(c, s.State, w)
	wc.AssertChange("mysql/0", "logging/0")
	wc.AssertNoChange()

	// Now, without reading any changes advance the lifecycle of both
	// units, inducing an update server-side after each two changes to
	// ensure they're reported as separate events over the API.
	err = subordinate.EnsureDead()
	c.Assert(err, gc.IsNil)
	s.BackingState.Sync()
	err = subordinate.Remove()
	c.Assert(err, gc.IsNil)
	err = principal.EnsureDead()
	c.Assert(err, gc.IsNil)
	s.BackingState.Sync()

	// Expect these changes as 2 separate events, so that
	// nothing gets lost.
	wc.AssertChange("logging/0")
	wc.AssertChange("mysql/0")
	wc.AssertNoChange()

	statetesting.AssertStop(c, w)
	wc.AssertClosed()
}

func (s *watcherSuite) TestStringsWatcherStopsWithPendingSend(c *gc.C) {
	// Call the Deployer facade's WatchUnits for machine-0.
	var results params.StringsWatchResults
	args := params.Entities{Entities: []params.Entity{{Tag: s.rawMachine.Tag()}}}
	err := s.stateAPI.Call("Deployer", "", "WatchUnits", args, &results)
	c.Assert(err, gc.IsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	result := results.Results[0]
	c.Assert(result.Error, gc.IsNil)

	// Start a StringsWatcher and check the initial event.
	w := watcher.NewStringsWatcher(s.stateAPI, result)
	wc := statetesting.NewStringsWatcherC(c, s.State, w)

	// Create a service, deploy a unit of it on the machine.
	mysql, err := s.State.AddService("mysql", s.AddTestingCharm(c, "mysql"))
	c.Assert(err, gc.IsNil)
	principal, err := mysql.AddUnit()
	c.Assert(err, gc.IsNil)
	err = principal.AssignToMachine(s.rawMachine)
	c.Assert(err, gc.IsNil)

	// Ensure the initial event is delivered. Then test the watcher
	// can be stopped cleanly without reading the pending change.
	s.BackingState.Sync()
	statetesting.AssertCanStopWhenSending(c, w)
	wc.AssertClosed()
}
