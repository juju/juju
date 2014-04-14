// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"labix.org/v2/mgo/bson"
	"labix.org/v2/mgo/txn"
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/testing"
	"launchpad.net/juju-core/testing/testbase"
	"launchpad.net/juju-core/utils"
)

// compatSuite contains backwards compatibility tests,
// for ensuring state operations behave correctly across
// schema changes.
type compatSuite struct {
	testbase.LoggingSuite
	testing.MgoSuite
	state *State
	env   *Environment
}

var _ = gc.Suite(&compatSuite{})

func (s *compatSuite) SetUpSuite(c *gc.C) {
	s.LoggingSuite.SetUpSuite(c)
	s.MgoSuite.SetUpSuite(c)
}

func (s *compatSuite) TearDownSuite(c *gc.C) {
	s.MgoSuite.TearDownSuite(c)
	s.LoggingSuite.TearDownSuite(c)
}

func (s *compatSuite) SetUpTest(c *gc.C) {
	s.LoggingSuite.SetUpTest(c)
	s.MgoSuite.SetUpTest(c)
	s.state = TestingInitialize(c, nil, Policy(nil))
	env, err := s.state.Environment()
	c.Assert(err, gc.IsNil)
	s.env = env
}

func (s *compatSuite) TearDownTest(c *gc.C) {
	s.state.Close()
	s.MgoSuite.TearDownTest(c)
	s.LoggingSuite.TearDownTest(c)
}

func (s *compatSuite) TestEnvironAssertAlive(c *gc.C) {
	// 1.17+ has a "Life" field in environment documents.
	// We remove it here, to test 1.16 compatibility.
	ops := []txn.Op{{
		C:      s.state.environments.Name,
		Id:     s.env.doc.UUID,
		Update: bson.D{{"$unset", bson.D{{"life", nil}}}},
	}}
	err := s.state.runTransaction(ops)
	c.Assert(err, gc.IsNil)

	// Now check the assertAliveOp and Destroy work as if
	// the environment is Alive.
	err = s.state.runTransaction([]txn.Op{s.env.assertAliveOp()})
	c.Assert(err, gc.IsNil)
	err = s.env.Destroy()
	c.Assert(err, gc.IsNil)
}

func (s *compatSuite) TestGetServiceWithoutNetworksIsOK(c *gc.C) {
	_, err := s.state.AddUser(AdminUser, "pass")
	c.Assert(err, gc.IsNil)
	charm := addCharm(c, s.state, "quantal", testing.Charms.Dir("mysql"))
	service, err := s.state.AddService("mysql", "user-admin", charm, nil, nil)
	c.Assert(err, gc.IsNil)
	// In 1.17.7+ all services have associated document in the
	// requested networks collection. We remove it here to test
	// backwards compatibility.
	ops := []txn.Op{removeRequestedNetworksOp(s.state, service.globalKey())}
	err = s.state.runTransaction(ops)
	c.Assert(err, gc.IsNil)

	// Now check the trying to fetch service's networks is OK.
	include, exclude, err := service.Networks()
	c.Assert(err, gc.IsNil)
	c.Assert(include, gc.HasLen, 0)
	c.Assert(exclude, gc.HasLen, 0)
}

func (s *compatSuite) TestGetMachineWithoutRequestedNetworksIsOK(c *gc.C) {
	machine, err := s.state.AddMachine("quantal", JobHostUnits)
	c.Assert(err, gc.IsNil)
	// In 1.17.7+ all machines have associated document in the
	// requested networks collection. We remove it here to test
	// backwards compatibility.
	ops := []txn.Op{removeRequestedNetworksOp(s.state, machine.globalKey())}
	err = s.state.runTransaction(ops)
	c.Assert(err, gc.IsNil)

	// Now check the trying to fetch machine's networks is OK.
	include, exclude, err := machine.RequestedNetworks()
	c.Assert(err, gc.IsNil)
	c.Assert(include, gc.HasLen, 0)
	c.Assert(exclude, gc.HasLen, 0)
}

func (s *compatSuite) TestOpenStateWithoutAdmin(c *gc.C) {
	// https://launchpad.net/bugs/1306902
	// In 1.18, machine-0 did not have access to the "admin" database. In
	// newer versions we need access in order to do replicaSet mutations.
	// However, we have not added the ability during upgrade to add
	// machine-0 to the admin db, so we should still continue even when it
	// doesn't have rights.
	machine, err := s.state.AddMachine("quantal", JobManageEnviron)
	c.Assert(err, gc.IsNil)
	machinePassword, err := utils.RandomPassword()
	c.Assert(err, gc.IsNil)
	err = machine.SetPassword(machinePassword)
	c.Assert(err, gc.IsNil)
	err = machine.SetMongoPassword(machinePassword)
	c.Assert(err, gc.IsNil)
	// (jam) The only way I've found to actually ensure "machine-0" is
	// removed from the Admin database is to actually login *as* the
	// machine agent we just gave admin rights to, and then remove it.
	adminDB := s.state.db.Session.DB("admin")
	err = adminDB.Login(machine.Tag(), machinePassword)
	c.Assert(err, gc.IsNil)
	err = adminDB.RemoveUser(machine.Tag())
	c.Assert(err, gc.IsNil)
	info := TestingStateInfo()
	info.Tag = machine.Tag()
	info.Password = machinePassword
	machineState, err := Open(info, TestingDialOpts(), Policy(nil))
	c.Assert(err, gc.IsNil)
	machineState.Close()
}
