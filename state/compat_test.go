// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	charmtesting "github.com/juju/charm/testing"
	gitjujutesting "github.com/juju/testing"
	"labix.org/v2/mgo/bson"
	"labix.org/v2/mgo/txn"
	gc "launchpad.net/gocheck"

	"github.com/juju/juju/testing"
)

// compatSuite contains backwards compatibility tests,
// for ensuring state operations behave correctly across
// schema changes.
type compatSuite struct {
	testing.BaseSuite
	gitjujutesting.MgoSuite
	state *State
	env   *Environment
}

var _ = gc.Suite(&compatSuite{})

func (s *compatSuite) SetUpSuite(c *gc.C) {
	s.BaseSuite.SetUpSuite(c)
	s.MgoSuite.SetUpSuite(c)
}

func (s *compatSuite) TearDownSuite(c *gc.C) {
	s.MgoSuite.TearDownSuite(c)
	s.BaseSuite.TearDownSuite(c)
}

func (s *compatSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.MgoSuite.SetUpTest(c)
	s.state = TestingInitialize(c, nil, Policy(nil))
	env, err := s.state.Environment()
	c.Assert(err, gc.IsNil)
	s.env = env
}

func (s *compatSuite) TearDownTest(c *gc.C) {
	s.state.Close()
	s.MgoSuite.TearDownTest(c)
	s.BaseSuite.TearDownTest(c)
}

func (s *compatSuite) TestEnvironAssertAlive(c *gc.C) {
	// 1.17+ has a "Life" field in environment documents.
	// We remove it here, to test 1.16 compatibility.
	ops := []txn.Op{{
		C:      s.state.environments.Name,
		Id:     s.env.doc.UUID,
		Update: bson.D{{"$unset", bson.D{{"life", nil}}}},
	}}
	err := s.state.RunTransaction(ops)
	c.Assert(err, gc.IsNil)

	// Now check the assertAliveOp and Destroy work as if
	// the environment is Alive.
	err = s.state.RunTransaction([]txn.Op{s.env.assertAliveOp()})
	c.Assert(err, gc.IsNil)
	err = s.env.Destroy()
	c.Assert(err, gc.IsNil)
}

func (s *compatSuite) TestGetServiceWithoutNetworksIsOK(c *gc.C) {
	_, err := s.state.AddAdminUser("pass")
	c.Assert(err, gc.IsNil)
	charm := addCharm(c, s.state, "quantal", charmtesting.Charms.Dir("mysql"))
	service, err := s.state.AddService("mysql", "user-admin", charm, nil)
	c.Assert(err, gc.IsNil)
	// In 1.17.7+ all services have associated document in the
	// requested networks collection. We remove it here to test
	// backwards compatibility.
	ops := []txn.Op{removeRequestedNetworksOp(s.state, service.globalKey())}
	err = s.state.RunTransaction(ops)
	c.Assert(err, gc.IsNil)

	// Now check the trying to fetch service's networks is OK.
	networks, err := service.Networks()
	c.Assert(err, gc.IsNil)
	c.Assert(networks, gc.HasLen, 0)
}

func (s *compatSuite) TestGetMachineWithoutRequestedNetworksIsOK(c *gc.C) {
	machine, err := s.state.AddMachine("quantal", JobHostUnits)
	c.Assert(err, gc.IsNil)
	// In 1.17.7+ all machines have associated document in the
	// requested networks collection. We remove it here to test
	// backwards compatibility.
	ops := []txn.Op{removeRequestedNetworksOp(s.state, machine.globalKey())}
	err = s.state.RunTransaction(ops)
	c.Assert(err, gc.IsNil)

	// Now check the trying to fetch machine's networks is OK.
	networks, err := machine.RequestedNetworks()
	c.Assert(err, gc.IsNil)
	c.Assert(networks, gc.HasLen, 0)
}
