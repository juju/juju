package state

import (
	gc "launchpad.net/gocheck"
)

type ActionSuite struct {
	SettingsSuite
}

var _ = gc.Suite(&ActionSuite{})

func (s *ActionSuite) SetUpSuite(c *gc.C) {
	s.SettingsSuite.SetUpSuite(c)
}

func (s *ActionSuite) TearDownSuite(c *gc.C) {
	s.SettingsSuite.TearDownSuite(c)
}

func (s *ActionSuite) SetUpTest(c *gc.C) {
	s.SettingsSuite.SetUpTest(c)
}

func (s *ActionSuite) TearDownTest(c *gc.C) {
	s.SettingsSuite.TearDownTest(c)
}

func (s *ActionSuite) TokenTest(c *gc.C) {
	action, err := s.SettingsSuite.state.AddAction("fakeunit/0", "fakeaction", nil)
	c.Assert(err, gc.IsNil)

	err = action.setRunning()
	c.Assert(err, gc.IsNil)

	action2, err := s.SettingsSuite.state.Action(action.Id())
	c.Assert(err, gc.IsNil)
	c.Assert(action2.Status(), gc.Equals, action.Status())
}
