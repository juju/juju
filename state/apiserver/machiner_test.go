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
	machiner, err := s.root.Machiner("")
	c.Assert(err, IsNil)

	// Request the machines in specific order to make sure the
	// response respects it.
	args := params.Machines{
		Ids: []string{"1", "0", "42"},
	}
	result, err := machiner.Machines(args)
	c.Assert(err, IsNil)
	c.Assert(result.Machines[0].Error, IsNil)
	c.Assert(result.Machines[0].Machine.Id, Equals, "1")
	c.Assert(result.Machines[1].Error, IsNil)
	c.Assert(result.Machines[1].Machine.Id, Equals, "0")
	s.assertError(c, result.Machines[2].Error, api.CodeNotFound, "machine 42 not found")
}

func (s *machinerSuite) TestSetStatus(c *C) {
	machiner, err := s.root.Machiner("")
	c.Assert(err, IsNil)

	err = s.machine0.SetStatus(params.StatusStarted, "blah")
	c.Assert(err, IsNil)
	err = s.machine1.SetStatus(params.StatusStopped, "foo")
	c.Assert(err, IsNil)

	args := params.MachinesSetStatus{
		Machines: []params.MachineSetStatus{
			{Id: "1", Status: params.StatusError, Info: "not really"},
			{Id: "0", Status: params.StatusStopped, Info: "foobar"},
			{Id: "42", Status: params.StatusStarted, Info: "blah"},
		}}
	result, err := machiner.SetStatus(args)
	c.Assert(err, IsNil)
	c.Assert(result.Errors[0], IsNil)
	c.Assert(result.Errors[1], IsNil)
	s.assertError(c, result.Errors[2], api.CodeNotFound, "machine 42 not found")

	// Verify
	status, info, err := s.machine0.Status()
	c.Assert(err, IsNil)
	c.Assert(status, Equals, params.StatusStopped)
	c.Assert(info, Equals, "foobar")
	status, info, err = s.machine1.Status()
	c.Assert(err, IsNil)
	c.Assert(status, Equals, params.StatusError)
	c.Assert(info, Equals, "not really")
}

func (s *machinerSuite) TestLife(c *C) {
	machiner, err := s.root.Machiner("")
	c.Assert(err, IsNil)

	err = s.machine1.EnsureDead()
	c.Assert(err, IsNil)
	err = s.machine1.Refresh()
	c.Assert(err, IsNil)
	c.Assert(s.machine1.Life(), Equals, state.Dead)

	args := params.Machines{
		Ids: []string{"1", "0", "42"},
	}
	result, err := machiner.Life(args)
	c.Assert(err, IsNil)
	c.Assert(result.Machines[0].Error, IsNil)
	c.Assert(string(result.Machines[0].Life), Equals, "dead")
	c.Assert(result.Machines[1].Error, IsNil)
	c.Assert(string(result.Machines[1].Life), Equals, "alive")
	s.assertError(c, result.Machines[2].Error, api.CodeNotFound, "machine 42 not found")
}

func (s *machinerSuite) TestEnsureDead(c *C) {
	machiner, err := s.root.Machiner("")
	c.Assert(err, IsNil)

	c.Assert(s.machine0.Life(), Equals, state.Alive)
	c.Assert(s.machine1.Life(), Equals, state.Alive)

	args := params.Machines{
		Ids: []string{"1", "0", "42"},
	}
	result, err := machiner.EnsureDead(args)
	c.Assert(err, IsNil)
	c.Assert(result.Errors[0], IsNil)
	s.assertError(c, result.Errors[1], "", "machine 0 is required by the environment")
	s.assertError(c, result.Errors[2], api.CodeNotFound, "machine 42 not found")

	err = s.machine0.Refresh()
	c.Assert(err, IsNil)
	c.Assert(s.machine0.Life(), Equals, state.Alive)
	err = s.machine1.Refresh()
	c.Assert(err, IsNil)
	c.Assert(s.machine1.Life(), Equals, state.Dead)
}
