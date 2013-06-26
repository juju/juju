// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrader_test

import (
	. "launchpad.net/gocheck"
	jujutesting "launchpad.net/juju-core/juju/testing"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api/params"
	apitesting "launchpad.net/juju-core/state/apiserver/testing"
	"launchpad.net/juju-core/state/apiserver/upgrader"
)

type upgraderSuite struct {
	jujutesting.JujuConnSuite

	// These are raw State objects. Use them for setup and assertions, but
	// should never be touched by the API calls themselves
	rawMachine *state.Machine
	upgrader   *upgrader.UpgraderAPI
	resources  apitesting.FakeResourceRegistry
}

var _ = Suite(&upgraderSuite{})

func (s *upgraderSuite) SetUpTest(c *C) {
	s.JujuConnSuite.SetUpTest(c)

	// Create a machine to work with
	var err error
	s.rawMachine, err = s.State.AddMachine("series", state.JobHostUnits)
	c.Assert(err, IsNil)
	err = s.rawMachine.SetPassword("test-password")
	c.Assert(err, IsNil)

	s.upgrader, err = upgrader.NewUpgraderAPI(s.State)
	c.Assert(err, IsNil)
}

func (s *upgraderSuite) TestWatchNothing(c *C) {
	// Not an error to watch nothing
	results, err := s.upgrader.Watch(params.Agents{})
	c.Assert(err, IsNil)
	c.Check(results.Results, HasLen, 0)
}

func (s *upgraderSuite) TestWatch(c *C) {
	args := params.Agents{
		Tags: []string{s.rawMachine.Tag()},
	}
	results, err := s.upgrader.Watch(args)
	c.Assert(err, IsNil)
	c.Check(results.Results, HasLen, 1)
	// Not Implemented Yet
	//c.Check(results.Results[0].UpgraderWatchId, Not(Equals), "")
}
