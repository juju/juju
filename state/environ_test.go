package state_test

import (
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/state"
)

type EnvironSuite struct {
	ConnSuite
	env *state.Environment
}

var _ = Suite(&EnvironSuite{})

func (s *EnvironSuite) SetUpTest(c *C) {
	s.ConnSuite.SetUpTest(c)
	s.env = s.State.Environment()
}

func (s *EnvironSuite) TestEntityName(c *C) {
	c.Assert(s.env.EntityName(), Equals, "environment")
}

func (s *EnvironSuite) TestSetPassword(c *C) {
	c.Assert(s.env.SetPassword("passwd"), ErrorMatches, "cannot set password of environment")
}

func (s *EnvironSuite) TestPasswordValid(c *C) {
	c.Assert(s.env.PasswordValid("passwd"), Equals, false)
}

func (s *EnvironSuite) TestRefresh(c *C) {
	c.Assert(s.env.Refresh(), ErrorMatches, "cannot refresh the environment")
}

func (s *ServiceSuite) TestAnnotatorForEnvironment(c *C) {
	testAnnotator(c, func() (annotator, error) {
		return s.State.Environment(), nil
	})
}
