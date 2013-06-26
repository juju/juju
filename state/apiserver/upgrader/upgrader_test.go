// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrader_test

import (
	. "launchpad.net/gocheck"
	//"launchpad.net/juju-core/errors"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api"
	"launchpad.net/juju-core/state/api/upgrader"
	//"launchpad.net/juju-core/state/api/params"
	//"launchpad.net/juju-core/state/apiserver"
	jujutesting "launchpad.net/juju-core/juju/testing"
	apitesting "launchpad.net/juju-core/state/apiserver/testing"
)

type upgraderSuite struct {
	jujutesting.JujuConnSuite

	stateAPI *api.State

	// These are raw State objects. Use them for setup and assertions, but
	// should never be touched by the API calls themselves
	rawMachine *state.Machine
	upgrader   *upgrader.Upgrader
	resources  apitesting.FakeResourceRegistry
}

var _ = Suite(&upgraderSuite{})

// Dial options with no timeouts and no retries
var fastDialOpts = api.DialOpts{}

func (s *upgraderSuite) SetUpTest(c *C) {
	s.JujuConnSuite.SetUpTest(c)

	// Create a machine to work with
	var err error
	s.rawMachine, err = s.State.AddMachine("series", state.JobHostUnits)
	c.Assert(err, IsNil)
	err = s.rawMachine.SetPassword("test-password")
	c.Assert(err, IsNil)

	// Login as the machine agent of the created machine.
	s.stateAPI = s.OpenAPIAs(c, s.rawMachine.Tag(), "test-password")

	// Create the upgrader facade.
	//s.upgrader, err = s.stateAPI.Upgrader()
	//c.Assert(err, IsNil)
	//c.Assert(s.upgrader, NotNil)
}

func (s *upgraderSuite) TearDownTest(c *C) {
	if s.stateAPI != nil {
		err := s.stateAPI.Close()
		c.Assert(err, IsNil)
	}
	s.JujuConnSuite.TearDownTest(c)
}
