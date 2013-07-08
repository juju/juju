// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package watcher_test

import (
	stdtesting "testing"
	"time"

	. "launchpad.net/gocheck"

	"launchpad.net/juju-core/juju/testing"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api"
	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/state/api/watcher"
	"launchpad.net/juju-core/state/apiserver"
	statetesting "launchpad.net/juju-core/state/testing"
	coretesting "launchpad.net/juju-core/testing"
)

func TestAll(t *stdtesting.T) {
	coretesting.MgoTestPackage(t)
}

// shortWait is a reasonable amount of time to be sure a call is in a blocking
// state (won't return without other prompting)
const shortWait = 50 * time.Millisecond

type watcherSuite struct {
	testing.JujuConnSuite

	server   *apiserver.Server
	stateAPI *api.State

	// These are raw State objects. Use them for setup and assertions, but
	// should never be touched by the API calls themselves
	rawMachine *state.Machine
	rawCharm   *state.Charm
	rawService *state.Service
	rawUnit    *state.Unit
}

var _ = Suite(&watcherSuite{})

func (s *watcherSuite) SetUpTest(c *C) {
	s.JujuConnSuite.SetUpTest(c)

	// Create a machine to work with
	var err error
	s.rawMachine, err = s.State.AddMachine("series", state.JobHostUnits)
	c.Assert(err, IsNil)
	err = s.rawMachine.SetPassword("test-password")
	c.Assert(err, IsNil)

	// Start the testing API server.
	s.server, err = apiserver.NewServer(
		s.State,
		"localhost:12345",
		[]byte(coretesting.ServerCert),
		[]byte(coretesting.ServerKey),
	)
	c.Assert(err, IsNil)

	// Login as the machine agent of the created machine.
	s.stateAPI = s.OpenAPIAs(c, s.rawMachine.Tag(), "test-password")
	c.Assert(s.stateAPI, NotNil)
}

func (s *watcherSuite) TearDownTest(c *C) {
	if s.stateAPI != nil {
		err := s.stateAPI.Close()
		c.Check(err, IsNil)
	}
	if s.server != nil {
		err := s.server.Stop()
		c.Check(err, IsNil)
	}
	s.JujuConnSuite.TearDownTest(c)
}

func (s *watcherSuite) TestWatchInitialEventConsumed(c *C) {
	// Machiner.Watch should send the initial event as part of the Watch
	// call (for NotifyWatchers there is no state to be transmitted). So a
	// call to Next() should not have anything to return.
	var results params.NotifyWatchResults
	args := params.Machines{Ids: []string{s.rawMachine.Id()}}
	err := s.stateAPI.Call("Machiner", "", "Watch", args, &results)
	c.Assert(err, IsNil)
	c.Assert(results.Results, HasLen, 1)
	result := results.Results[0]
	c.Assert(result.Error, IsNil)

	// We expect the Call() to "Next" to block, so run it in a goroutine.
	done := make(chan error)
	go func() {
		ignored := struct{}{}
		done <- s.stateAPI.Call("NotifyWatcher", result.NotifyWatcherId, "Next", nil, &ignored)
	}()

	select {
	case err := <-done:
		c.Errorf("Call(Next) did not block immediately after Watch(): err %v", err)
	case <-time.After(shortWait):
	}
}

func (s *watcherSuite) TestWatchMachine(c *C) {
	var results params.NotifyWatchResults
	args := params.Machines{Ids: []string{s.rawMachine.Id()}}
	err := s.stateAPI.Call("Machiner", "", "Watch", args, &results)
	c.Assert(err, IsNil)
	c.Assert(results.Results, HasLen, 1)
	result := results.Results[0]
	c.Assert(result.Error, IsNil)

	// params.NotifyWatcher conforms to the state.NotifyWatcher interface
	w := watcher.NewNotifyWatcher(s.stateAPI, result)
	wc := statetesting.NewNotifyWatcherC(c, s.State, w)
	wc.AssertOneChange()
	statetesting.AssertStop(c, w)
	wc.AssertClosed()
}
