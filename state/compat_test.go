// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"labix.org/v2/mgo/txn"
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/testing"
	"launchpad.net/juju-core/testing/testbase"
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
	s.state = TestingInitialize(c, nil)
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
		Update: D{{"$unset", D{{"life", nil}}}},
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
