// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrader_test

import (
	"time"

	. "launchpad.net/gocheck"

	jujutesting "launchpad.net/juju-core/juju/testing"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api/params"
	apitesting "launchpad.net/juju-core/state/apiserver/testing"
	"launchpad.net/juju-core/state/apiserver/upgrader"
	"launchpad.net/juju-core/testing/checkers"
)

type upgraderSuite struct {
	jujutesting.JujuConnSuite

	// These are raw State objects. Use them for setup and assertions, but
	// should never be touched by the API calls themselves
	rawMachine *state.Machine
	upgrader   *upgrader.UpgraderAPI
	resources  apitesting.FakeResourceRegistry
	authorizer apitesting.FakeAuthorizer
}

var _ = Suite(&upgraderSuite{})

func (s *upgraderSuite) SetUpTest(c *C) {
	s.JujuConnSuite.SetUpTest(c)
	s.resources = make(apitesting.FakeResourceRegistry)

	// Create a machine to work with
	var err error
	s.rawMachine, err = s.State.AddMachine("series", state.JobHostUnits)
	c.Assert(err, IsNil)
	err = s.rawMachine.SetPassword("test-password")
	c.Assert(err, IsNil)

	// The default auth is as the machine agent
	s.authorizer = apitesting.FakeAuthorizer{
		Tag:          s.rawMachine.Tag(),
		LoggedIn:     true,
		Manager:      false,
		MachineAgent: true,
		Client:       false,
	}
	s.upgrader, err = upgrader.NewUpgraderAPI(s.State, s.resources, s.authorizer)
	c.Assert(err, IsNil)
}

func (s *upgraderSuite) TearDownTest(c *C) {
	if s.resources != nil {
		for _, resource := range s.resources {
			resource.Stop()
		}
	}
	s.JujuConnSuite.TearDownTest(c)
}

func (s *upgraderSuite) TestWatchAPIVersionNothing(c *C) {
	// Not an error to watch nothing
	results, err := s.upgrader.WatchAPIVersion(params.Agents{})
	c.Assert(err, IsNil)
	c.Check(results.Results, HasLen, 0)
}

func (s *upgraderSuite) TestWatchAPIVersion(c *C) {
	args := params.Agents{
		Tags: []string{s.rawMachine.Tag()},
	}
	results, err := s.upgrader.WatchAPIVersion(args)
	c.Assert(err, IsNil)
	c.Check(results.Results, HasLen, 1)
	c.Check(results.Results[0].EntityWatcherId, Not(Equals), "")
	resource, ok := s.resources[results.Results[0].EntityWatcherId]
	c.Check(ok, checkers.IsTrue)
	defer func() {
		err := resource.Stop()
		c.Assert(err, IsNil)
	}()
	// Check that the watcher returns an initial event
	channel := resource.(*state.EnvironConfigWatcher).Changes()
	// Should use helpers from state/watcher_test.go when generalised
	select {
	case _, ok := <-channel:
		c.Assert(ok, Equals, true)
	case <-time.After(50 * time.Millisecond):
		c.Fatal("timeout waiting for entity watcher")
	}
}

func (s *upgraderSuite) TestWatchAPIVersionRefusesNonAgent(c *C) {
	// We aren't even a machine agent
	anAuthorizer := s.authorizer
	anAuthorizer.MachineAgent = false
	anUpgrader, err := upgrader.NewUpgraderAPI(s.State, s.resources, anAuthorizer)
	c.Check(err, NotNil)
	c.Check(anUpgrader, IsNil)
	c.Assert(err, ErrorMatches, "permission denied")
}

func (s *upgraderSuite) TestWatchAPIVersionRefusesWrongAgent(c *C) {
	// We are a machine agent, but not the one we are trying to track
	anAuthorizer := s.authorizer
	anAuthorizer.Tag = "machine-12354"
	anUpgrader, err := upgrader.NewUpgraderAPI(s.State, s.resources, anAuthorizer)
	c.Check(err, IsNil)
	args := params.Agents{
		Tags: []string{s.rawMachine.Tag()},
	}
	results, err := anUpgrader.WatchAPIVersion(args)
	// It is not an error to make the request, but the specific item is rejected
	c.Assert(err, IsNil)
	c.Check(results.Results, HasLen, 1)
	c.Check(results.Results[0].EntityWatcherId, Equals, "")
	c.Assert(results.Results[0].Error, NotNil)
	err = *results.Results[0].Error
	c.Check(err, ErrorMatches, "permission denied")
}
