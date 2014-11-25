// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/uniter"
	"github.com/juju/juju/state"
)

type environSuite struct {
	uniterSuite
	apiEnviron   *uniter.Environment
	stateEnviron *state.Environment
}

var _ = gc.Suite(&environSuite{})

func (s *environSuite) SetUpTest(c *gc.C) {
	s.uniterSuite.SetUpTest(c)
	var err error
	s.apiEnviron, err = s.uniter.Environment()
	c.Assert(err, jc.ErrorIsNil)
	s.stateEnviron, err = s.State.Environment()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *environSuite) TearDownTest(c *gc.C) {
	s.uniterSuite.TearDownTest(c)
}

func (s *environSuite) TestUUID(c *gc.C) {
	c.Assert(s.apiEnviron.UUID(), gc.Equals, s.stateEnviron.UUID())
}

func (s *environSuite) TestName(c *gc.C) {
	c.Assert(s.apiEnviron.Name(), gc.Equals, s.stateEnviron.Name())
}
