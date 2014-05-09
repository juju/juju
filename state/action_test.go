package state_test

import (
	gc "launchpad.net/gocheck"
	"launchpad.net/juju-core/state"
)

type ActionSuite struct {
	ConnSuite
	charm   *state.Charm
	service *state.Service
	unit    *state.Unit
	action  *state.Action
}

var _ = gc.Suite(&ActionSuite{})

func (s *ActionSuite) SetUpTest(c *gc.C) {
	s.ConnSuite.SetUpTest(c)
}
