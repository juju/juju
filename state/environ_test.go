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
	uuidA := s.env.UUID()
	c.Assert(uuidA, FitsTypeOf, trivial.UUID{})

	// Check that two environments have different UUIDs.
	s.State.Close()
	s.MgoSuite.TearDownTest(c)
	s.MgoSuite.SetUpTest(c)
	s.State = state.TestingInitialize(c, nil)
	env, err := s.State.Environment()
	c.Assert(err, IsNil)
	uuidB := env.UUID()
	c.Assert(uuidA, Not(Equals), uuidB)
}

func (s *EnvironSuite) TestAnnotatorForEnvironment(c *C) {
	testAnnotator(c, func() (state.Annotator, error) {
		return s.State.Environment()
	})
}
