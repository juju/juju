// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/names"
	gitjujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"

	"github.com/juju/juju/testcharms"
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
	owner := names.NewLocalUserTag("test-admin")
	st, err := Initialize(owner, TestingMongoInfo(), testing.EnvironConfig(c), TestingDialOpts(), nil)
	c.Assert(err, jc.ErrorIsNil)
	s.state = st
	env, err := s.state.Environment()
	c.Assert(err, jc.ErrorIsNil)
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
	c.Assert(err, jc.ErrorIsNil)

	// Now check the assertAliveOp and Destroy work as if
	// the environment is Alive.
	err = s.state.runTransaction([]txn.Op{s.env.assertAliveOp()})
	c.Assert(err, jc.ErrorIsNil)
	err = s.env.Destroy()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *compatSuite) TestGetServiceWithoutNetworksIsOK(c *gc.C) {
	charm := addCharm(c, s.state, "quantal", testcharms.Repo.CharmDir("mysql"))
	owner := s.env.Owner()
	service, err := s.state.AddService("mysql", owner.String(), charm, nil)
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
