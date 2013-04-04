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
	env, err := s.State.Environment()
	c.Assert(err, IsNil)
	uuid := env.UUID()
	c.Assert(uuid, FitsTypeOf, trivial.UUID{})
	c.Assert(uuid.String(), Matches, "[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[8,9,a,b][0-9a-f]{3}-[0-9a-f]{12}")
}

func (s *EnvironSuite) TestAnnotatorForEnvironment(c *C) {
	testAnnotator(c, func() (state.Annotator, error) {
		return s.State.Environment()
	})
}
