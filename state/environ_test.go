// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/state"
)

type EnvironSuite struct {
	ConnSuite
	env *state.Environment
}

var _ = gc.Suite(&EnvironSuite{})

func (s *EnvironSuite) SetUpTest(c *gc.C) {
	s.ConnSuite.SetUpTest(c)
	env, err := s.State.Environment()
	c.Assert(err, gc.IsNil)
	s.env = env
}

func (s *EnvironSuite) TestTag(c *gc.C) {
	expected := "environment-" + s.env.UUID()
	c.Assert(s.env.Tag(), gc.Equals, expected)
}

func (s *EnvironSuite) TestName(c *gc.C) {
	c.Assert(s.env.Name(), gc.Equals, "testenv")
}

func (s *EnvironSuite) TestUUID(c *gc.C) {
	uuidA := s.env.UUID()
	c.Assert(uuidA, gc.HasLen, 36)

	// Check that two environments have different UUIDs.
	s.State.Close()
	s.MgoSuite.TearDownTest(c)
	s.MgoSuite.SetUpTest(c)
	s.State = state.TestingInitialize(c, nil, state.Policy(nil))
	env, err := s.State.Environment()
	c.Assert(err, gc.IsNil)
	uuidB := env.UUID()
	c.Assert(uuidA, gc.Not(gc.Equals), uuidB)
}

func (s *EnvironSuite) TestAnnotatorForEnvironment(c *gc.C) {
	testAnnotator(c, func() (state.Annotator, error) {
		return s.State.Environment()
	})
}
