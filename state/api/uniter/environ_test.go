// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter_test

import (
	gc "launchpad.net/gocheck"
)

type environSuite struct {
	uniterSuite
}

var _ = gc.Suite(&environSuite{})

func (s *environSuite) SetUpTest(c *gc.C) {
	s.uniterSuite.SetUpTest(c)
}

func (s *environSuite) TearDownTest(c *gc.C) {
	s.uniterSuite.TearDownTest(c)
}

func (s *environSuite) TestUUID(c *gc.C) {
	apiEnviron, err := s.uniter.Environment()
	c.Assert(err, gc.IsNil)
	stateEnviron, err := s.State.Environment()
	c.Assert(err, gc.IsNil)
	uuid, err := apiEnviron.UUID()
	c.Assert(err, gc.IsNil)
	c.Assert(uuid, gc.Equals, stateEnviron.UUID())
}
