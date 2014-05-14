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
	action := newAction(s.SettingsSuite.state, actionDoc{})
	err := action.setRunning()
	c.Assert(err, gc.IsNil)
}
