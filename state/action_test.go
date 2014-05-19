package state_test

import (
	gc "launchpad.net/gocheck"
)

type ActionSuite struct {
	ConnSuite
}

var _ = gc.Suite(&ActionSuite{})

func (s *ActionSuite) TokenTest(c *gc.C) {
	action, err := s.ConnSuite.State.AddAction("fakeunit/0", "fakeaction", nil)
	c.Assert(err, gc.IsNil)

	action2, err := s.ConnSuite.State.Action(action.Id())
	c.Assert(err, gc.IsNil)
	c.Assert(action.Name(), gc.Equals, action2.Name())
}
