// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	gitjujutesting "github.com/juju/testing"
	charmtesting "gopkg.in/juju/charm.v3/testing"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"
	gc "launchpad.net/gocheck"

	"github.com/juju/juju/network"
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
	st, err := Initialize(TestingMongoInfo(), testing.EnvironConfig(c), TestingDialOpts(), nil)
	c.Assert(err, gc.IsNil)
	s.state = st
	env, err := s.state.Environment()
	c.Assert(err, gc.IsNil)
	s.env = env
}

func (s *compatSuite) TearDownTest(c *gc.C) {
	if s.state != nil {
		s.state.Close()
	}
	s.MgoSuite.TearDownTest(c)
	s.BaseSuite.TearDownTest(c)
}

func (s *compatSuite) TestEnvironAssertAlive(c *gc.C) {
	// 1.17+ has a "Life" field in environment documents.
	// We remove it here, to test 1.16 compatibility.
	ops := []txn.Op{{
		C:      environmentsC,
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
	_, err := s.state.AddAdminUser("pass")
	c.Assert(err, gc.IsNil)
	charm := addCharm(c, s.state, "quantal", charmtesting.Charms.CharmDir("mysql"))
	service, err := s.state.AddService("mysql", "user-admin", charm, nil)
	c.Assert(err, gc.IsNil)
	// In 1.17.7+ all services have associated document in the
	// requested networks collection. We remove it here to test
	// backwards compatibility.
	ops := []txn.Op{removeRequestedNetworksOp(s.state, service.globalKey())}
	err = s.state.runTransaction(ops)
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
	err = s.state.runTransaction(ops)
	c.Assert(err, gc.IsNil)

	// Now check the trying to fetch machine's networks is OK.
	networks, err := machine.RequestedNetworks()
	c.Assert(err, gc.IsNil)
	c.Assert(networks, gc.HasLen, 0)
}

// Check if ports stored on the unit are displayed.
func (s *compatSuite) TestShowUnitPorts(c *gc.C) {
	_, err := s.state.AddAdminUser("pass")
	c.Assert(err, gc.IsNil)
	charm := addCharm(c, s.state, "quantal", charmtesting.Charms.CharmDir("mysql"))
	service, err := s.state.AddService("mysql", "user-admin", charm, nil)
	c.Assert(err, gc.IsNil)
	unit, err := service.AddUnit()
	c.Assert(err, gc.IsNil)
	machine, err := s.state.AddMachine("quantal", JobHostUnits)
	c.Assert(err, gc.IsNil)
	c.Assert(unit.AssignToMachine(machine), gc.IsNil)

	// Add old-style ports to unit.
	port := network.Port{Protocol: "tcp", Number: 80}
	ops := []txn.Op{{
		C:      unitsC,
		Id:     unit.doc.Name,
		Assert: notDeadDoc,
		Update: bson.D{{"$addToSet", bson.D{{"ports", port}}}},
	}}
	err = s.state.runTransaction(ops)
	c.Assert(err, gc.IsNil)
	err = unit.Refresh()
	c.Assert(err, gc.IsNil)

	ports := unit.OpenedPorts()
	c.Assert(ports, gc.DeepEquals, []network.Port{{"tcp", 80}})
}

// Check if opening ports on a unit with ports stored in the unit doc works.
func (s *compatSuite) TestMigratePortsOnOpen(c *gc.C) {
	_, err := s.state.AddAdminUser("pass")
	c.Assert(err, gc.IsNil)
	charm := addCharm(c, s.state, "quantal", charmtesting.Charms.CharmDir("mysql"))
	service, err := s.state.AddService("mysql", "user-admin", charm, nil)
	c.Assert(err, gc.IsNil)
	unit, err := service.AddUnit()
	c.Assert(err, gc.IsNil)
	machine, err := s.state.AddMachine("quantal", JobHostUnits)
	c.Assert(err, gc.IsNil)
	c.Assert(unit.AssignToMachine(machine), gc.IsNil)

	// Add old-style ports to unit.
	port := network.Port{Protocol: "tcp", Number: 80}
	ops := []txn.Op{{
		C:      unitsC,
		Id:     unit.doc.Name,
		Assert: notDeadDoc,
		Update: bson.D{{"$addToSet", bson.D{{"ports", port}}}},
	}}
	err = s.state.runTransaction(ops)
	c.Assert(err, gc.IsNil)
	err = unit.Refresh()
	c.Assert(err, gc.IsNil)

	// Check if port conflicts are detected.
	err = unit.OpenPort("tcp", 80)
	c.Assert(err, gc.ErrorMatches, "cannot open ports 80-80/tcp for unit \"mysql/0\": cannot open ports 80-80/tcp on machine 0 due to conflict")

	err = unit.OpenPort("tcp", 8080)
	c.Assert(err, gc.IsNil)

	ports := unit.OpenedPorts()
	c.Assert(ports, gc.DeepEquals, []network.Port{{"tcp", 80}, {"tcp", 8080}})
}

// Check if closing ports on a unit with ports stored in the unit doc works.
func (s *compatSuite) TestMigratePortsOnClose(c *gc.C) {
	_, err := s.state.AddAdminUser("pass")
	c.Assert(err, gc.IsNil)
	charm := addCharm(c, s.state, "quantal", charmtesting.Charms.CharmDir("mysql"))
	service, err := s.state.AddService("mysql", "user-admin", charm, nil)
	c.Assert(err, gc.IsNil)
	unit, err := service.AddUnit()
	c.Assert(err, gc.IsNil)
	machine, err := s.state.AddMachine("quantal", JobHostUnits)
	c.Assert(err, gc.IsNil)
	c.Assert(unit.AssignToMachine(machine), gc.IsNil)

	// Add old-style ports to unit.
	port := network.Port{Protocol: "tcp", Number: 80}
	ops := []txn.Op{{
		C:      unitsC,
		Id:     unit.doc.Name,
		Assert: notDeadDoc,
		Update: bson.D{{"$addToSet", bson.D{{"ports", port}}}},
	}}
	err = s.state.runTransaction(ops)
	c.Assert(err, gc.IsNil)
	err = unit.Refresh()
	c.Assert(err, gc.IsNil)

	// Check if closing an unopened port works
	err = unit.ClosePort("tcp", 8080)
	c.Assert(err, gc.IsNil)

	err = unit.ClosePort("tcp", 80)
	c.Assert(err, gc.IsNil)

	ports := unit.OpenedPorts()
	c.Assert(ports, gc.DeepEquals, []network.Port{})
}
