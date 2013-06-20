// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrader_test

import (
	. "launchpad.net/gocheck"
	//"launchpad.net/juju-core/errors"
	"launchpad.net/juju-core/juju/testing"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api"
	//"launchpad.net/juju-core/state/api/upgrader"
	//"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/state/apiserver"
	coretesting "launchpad.net/juju-core/testing"
	stdtesting "testing"
)

func TestAll(t *stdtesting.T) {
	coretesting.MgoTestPackage(t)
}

type upgraderSuite struct {
	testing.JujuConnSuite

	server *apiserver.Server
	stateAPI *api.State

	machine  *state.Machine
}

var _ = Suite(&upgraderSuite{})

func defaultPassword(stm *state.Machine) string {
	return stm.Tag() + " password"
}

// Dial options with no timeouts and no retries
var fastDialOpts = api.DialOpts{}

func (s *upgraderSuite) SetUpTest(c *C) {
	s.JujuConnSuite.SetUpTest(c)

	// Create a machine to work with
	var err error
	s.machine, err = s.State.AddMachine("series", state.JobHostUnits)
	c.Assert(err, IsNil)
	err = s.machine.SetPassword(defaultPassword(s.machine))

	// Start the testing API server.
	s.server, err = apiserver.NewServer(
		s.State,
		"localhost:12345",
		[]byte(coretesting.ServerCert),
		[]byte(coretesting.ServerKey),
	)
	c.Assert(err, IsNil)

	// Login as the machine agent of the created machine.
	_, info, err := s.APIConn.Environ.StateInfo()
	c.Assert(err, IsNil)
	info.Tag = s.machine.Tag()
	info.Password = defaultPassword(s.machine)
	c.Logf("opening state; entity %q, password %q", info.Tag, info.Password)
	s.stateAPI, err = api.Open(info, fastDialOpts)
	c.Assert(err, IsNil)
	c.Assert(s.stateAPI, NotNil)

	// Create the upgrader facade.
	//s.machiner, err = s.stateAPI.Machiner()
	//c.Assert(err, IsNil)
	//c.Assert(s.machiner, NotNil)
}

func (s *upgraderSuite) TearDownTest(c *C) {
	if s.stateAPI != nil {
		err := s.stateAPI.Close()
		c.Assert(err, IsNil)
	}
	if s.server != nil {
		err := s.server.Stop()
		c.Assert(err, IsNil)
	}
	s.JujuConnSuite.TearDownTest(c)
}

