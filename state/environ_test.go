package state_test

import (
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/trivial"
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

func (s *EnvironSuite) TestTag(c *C) {
	cfg, err := s.State.EnvironConfig()
	c.Assert(err, IsNil)
	expected := "environment-" + cfg.Name()
	c.Assert(s.env.Tag(), Equals, expected)
}

func (s *EnvironSuite) TestUUID(c *C) {
	uuid := s.env.UUID()
	c.Assert(uuid, FitsTypeOf, trivial.UUID{})
}

func (s *EnvironSuite) TestAnnotatorForEnvironment(c *C) {
	testAnnotator(c, func() (state.Annotator, error) {
		return s.State.Environment()
	})
}
