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

	// s.charm = s.AddTestingCharm(c, "wordpress")
	// var err error
	// s.service = s.AddTestingService(c, "wordpress", s.charm)
	// c.Assert(err, gc.IsNil)
	// testAction := s.State.AddAction("wordpress", "snapshot", "outfile: foo.tar.gz")
	// c.Assert(s.State.Action("%v", testAction.doc.Id), gc.DeepEquals, testAction)
	// //c.Assert(s.unit.ActionList()[0], gc.Equals, s.Action(testAction.doc.Id))
}
