// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver_test

import (
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/juju/testing"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api"
	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/state/apiserver"
	coretesting "launchpad.net/juju-core/testing"
)

type machinerSuite struct {
	testing.JujuConnSuite
	server *apiserver.Server
	root   *apiserver.SrvRoot

	machine0 *state.Machine
	machine1 *state.Machine
}

var _ = Suite(&machinerSuite{})

func (s *machinerSuite) SetUpTest(c *C) {
	s.JujuConnSuite.SetUpTest(c)

	// Create a machine so that we can login as its agent
	var err error
	s.machine0, err = s.State.AddMachine("series", state.JobManageEnviron)
	c.Assert(err, IsNil)
	setDefaultPassword(c, s.machine0)
	// Add another normal machine
	s.machine1, err = s.State.AddMachine("series", state.JobHostUnits)
	c.Assert(err, IsNil)
	setDefaultPassword(c, s.machine0)

	// Start the testing API server.
	s.server, err = apiserver.NewServer(
		s.State,
		"localhost:12345",
		[]byte(coretesting.ServerCert),
		[]byte(coretesting.ServerKey),
	)
	c.Assert(err, IsNil)

	// Login as the machine agent of the created machine and get the root.
	s.root, err = apiserver.ServerLoginAndGetRoot(
		s.server,
		s.machine0.Tag(),
		defaultPassword(s.machine0))
	c.Assert(err, IsNil)
}

func (s *machinerSuite) TearDownTest(c *C) {
	if s.server != nil {
		err := s.server.Stop()
		c.Assert(err, IsNil)
	}
	s.JujuConnSuite.TearDownTest(c)
}

func (s *machinerSuite) assertError(c *C, err *params.Error, code, messageRegexp string) {
	c.Assert(err, NotNil)
	c.Assert(api.ErrCode(err), Equals, code)
	c.Assert(err, ErrorMatches, messageRegexp)
}

func (s *machinerSuite) TestMachinerFailsWithNotEmptyId(c *C) {
	_, err := s.root.Machiner("blah")
	c.Assert(err, ErrorMatches, api.CodeBadVersion)
}

func (s *machinerSuite) TestMachines(c *C) {
}

func (s *machinerSuite) TestSetStatus(c *C) {
}

func (s *machinerSuite) TestLife(c *C) {
}

func (s *machinerSuite) TestEnsureDead(c *C) {
}
