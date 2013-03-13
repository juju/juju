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
	env, err := s.State.Environment()
	c.Assert(err, IsNil)
	s.env = env
}

func (s *EnvironSuite) TestEntityName(c *C) {
	c.Assert(s.env.EntityName(), Equals, "environment-test")
}

func (s *ServiceSuite) TestAnnotatorForEnvironment(c *C) {
	testAnnotator(c, func() (annotator, error) {
		env, err := s.State.Environment()
		c.Assert(err, IsNil)
		return env, nil
	})
}
