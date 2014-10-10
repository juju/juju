// Copyright 2014 package action_test

package action_test

import gc "gopkg.in/check.v1"
import jc "github.com/juju/testing/checkers"

type UndefinedActionCommandSuite struct {
	BaseActionSuite
}

var _ = gc.Suite(&UndefinedActionCommandSuite{})

func (s *UndefinedActionCommandSuite) SetUpTest(c *gc.C) {
	s.BaseActionSuite.SetUpTest(c)
}

func (s *UndefinedActionCommandSuite) TestInit(c *gc.C) {
	c.Assert(true, jc.IsTrue)
}

func (s *UndefinedActionCommandSuite) TestRun(c *gc.C) {
	c.Assert(true, jc.IsTrue)
}

func (s *UndefinedActionCommandSuite) TestHelp(c *gc.C) {
	c.Assert(true, jc.IsTrue)
}
