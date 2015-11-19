// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"

	"github.com/juju/juju/testcharms"
)

// compatSuite contains backwards compatibility tests,
// for ensuring state operations behave correctly across
// schema changes.
type compatSuite struct {
	internalStateSuite
	env *Environment
}

var _ = gc.Suite(&compatSuite{})

func (s *compatSuite) SetUpTest(c *gc.C) {
	s.internalStateSuite.SetUpTest(c)

	env, err := s.state.Environment()
	c.Assert(err, jc.ErrorIsNil)
	s.env = env
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
	c.Assert(err, jc.ErrorIsNil)

	// Now check the assertAliveOp and Destroy work as if
	// the environment is Alive.
	err = s.state.runTransaction([]txn.Op{s.env.assertAliveOp()})
	c.Assert(err, jc.ErrorIsNil)
	err = s.env.Destroy()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *compatSuite) TestGetServiceWithoutEndpointBindingsReturnsNotFound(c *gc.C) {
	charm := addCharm(c, s.state, "quantal", testcharms.Repo.CharmDir("mysql"))
	owner := s.env.Owner()
	service, err := s.state.AddService(AddServiceArgs{Name: "mysql", Owner: owner.String(), Charm: charm})
	c.Assert(err, jc.ErrorIsNil)
	// In 1.26+ all services have associated document in the endpoint bindings
	// collection. We remove it here to test backwards compatibility.
	ops := []txn.Op{removeEndpointBindingsOp(s.state, service.globalKey())}
	err = s.state.runTransaction(ops)
	c.Assert(err, jc.ErrorIsNil)

	bindings, err := service.EndpointBindings()
	c.Assert(err, gc.ErrorMatches, `endpoint bindings for "s#mysql" not found`)
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
	c.Assert(bindings, gc.IsNil)
}

func (s *compatSuite) TestGetServiceWithoutNetworksIsOK(c *gc.C) {
	charm := addCharm(c, s.state, "quantal", testcharms.Repo.CharmDir("mysql"))
	owner := s.env.Owner()
	service, err := s.state.AddService(AddServiceArgs{Name: "mysql", Owner: owner.String(), Charm: charm})
	c.Assert(err, jc.ErrorIsNil)
	// In 1.17.7+ all services have associated document in the
	// requested networks collection. We remove it here to test
	// backwards compatibility.
	ops := []txn.Op{removeRequestedNetworksOp(s.state, service.globalKey())}
	err = s.state.runTransaction(ops)
	c.Assert(err, jc.ErrorIsNil)

	// Now check the trying to fetch service's networks is OK.
	networks, err := service.Networks()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(networks, gc.HasLen, 0)
}

func (s *compatSuite) TestGetMachineWithoutRequestedNetworksIsOK(c *gc.C) {
	machine, err := s.state.AddMachine("quantal", JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	// In 1.17.7+ all machines have associated document in the
	// requested networks collection. We remove it here to test
	// backwards compatibility.
	ops := []txn.Op{removeRequestedNetworksOp(s.state, machine.globalKey())}
	err = s.state.runTransaction(ops)
	c.Assert(err, jc.ErrorIsNil)

	// Now check the trying to fetch machine's networks is OK.
	networks, err := machine.RequestedNetworks()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(networks, gc.HasLen, 0)
}
