// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machiner_test

import (
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/errors"
	"launchpad.net/juju-core/juju/testing"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api"
	"launchpad.net/juju-core/state/api/machiner"
	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/state/apiserver"
	coretesting "launchpad.net/juju-core/testing"
	stdtesting "testing"
)

func TestAll(t *stdtesting.T) {
	coretesting.MgoTestPackage(t)
}

type machinerSuite struct {
	testing.JujuConnSuite

	server *apiserver.Server
	st     *api.State

	machine *state.Machine
	mstate  *machiner.State
}

var _ = Suite(&machinerSuite{})

func defaultPassword(stm *state.Machine) string {
	return stm.Tag() + " password"
}

func (s *machinerSuite) SetUpTest(c *C) {
	s.JujuConnSuite.SetUpTest(c)

	// Create a machine so we can login as its agent.
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
	s.st, err = api.Open(info, api.DialOpts{})
	c.Assert(err, IsNil)
	c.Assert(s.st, NotNil)

	// Create the machiner facade.
	s.mstate = s.st.Machiner()
	c.Assert(s.mstate, NotNil)
}

func (s *machinerSuite) TearDownTest(c *C) {
	var err error
	if s.st != nil {
		err = s.st.Close()
		c.Assert(err, IsNil)
	}
	if s.server != nil {
		err = s.server.Stop()
		c.Assert(err, IsNil)
	}
	s.JujuConnSuite.TearDownTest(c)
}

func (s *machinerSuite) TestMachineAndMachineId(c *C) {
	machine, err := s.mstate.Machine("42")
	c.Assert(err, ErrorMatches, "machine 42 not found")
	c.Assert(api.ErrCode(err), Equals, api.CodeNotFound)
	c.Assert(machine, IsNil)

	machine, err = s.mstate.Machine("0")
	c.Assert(err, IsNil)
	c.Assert(machine.Id(), Equals, "0")
}

func (s *machinerSuite) TestSetStatus(c *C) {
	machine, err := s.mstate.Machine("0")
	c.Assert(err, IsNil)

	status, info, err := s.machine.Status()
	c.Assert(err, IsNil)
	c.Assert(status, Equals, params.StatusPending)
	c.Assert(info, Equals, "")

	err = machine.SetStatus(params.StatusStarted, "blah")
	c.Assert(err, IsNil)

	status, info, err = s.machine.Status()
	c.Assert(err, IsNil)
	c.Assert(status, Equals, params.StatusStarted)
	c.Assert(info, Equals, "blah")
}

func (s *machinerSuite) TestEnsureDead(c *C) {
	c.Assert(s.machine.Life(), Equals, state.Alive)

	machine, err := s.mstate.Machine("0")
	c.Assert(err, IsNil)

	err = machine.EnsureDead()
	c.Assert(err, IsNil)

	err = s.machine.Refresh()
	c.Assert(err, IsNil)
	c.Assert(s.machine.Life(), Equals, state.Dead)

	err = machine.EnsureDead()
	c.Assert(err, IsNil)
	err = s.machine.Refresh()
	c.Assert(err, IsNil)
	c.Assert(s.machine.Life(), Equals, state.Dead)

	err = s.machine.Remove()
	c.Assert(err, IsNil)
	err = s.machine.Refresh()
	c.Assert(errors.IsNotFoundError(err), Equals, true)

	err = machine.EnsureDead()
	c.Assert(err, ErrorMatches, "machine 0 not found")
	c.Assert(api.ErrCode(err), Equals, api.CodeNotFound)
}

func (s *machinerSuite) TestRefresh(c *C) {
	machine, err := s.mstate.Machine("0")
	c.Assert(err, IsNil)
	c.Assert(machine.Life(), Equals, params.Life("alive"))

	err = machine.EnsureDead()
	c.Assert(err, IsNil)
	c.Assert(machine.Life(), Equals, params.Life("alive"))

	err = machine.Refresh()
	c.Assert(err, IsNil)
	c.Assert(machine.Life(), Equals, params.Life("dead"))
}
