// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"github.com/juju/errors"
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/state"
)

type EnvironSuite struct {
	ConnSuite
}

var _ = gc.Suite(&EnvironSuite{})

func (s *EnvironSuite) TestEnvironment(c *gc.C) {
	env, err := s.State.Environment()
	c.Assert(err, gc.IsNil)

	expectedTag := names.NewEnvironTag(env.UUID())
	c.Assert(env.Tag(), gc.Equals, expectedTag)
	c.Assert(env.ServerTag(), gc.Equals, expectedTag)
	c.Assert(env.Name(), gc.Equals, "testenv")
	c.Assert(env.Owner(), gc.Equals, s.owner)
	c.Assert(env.Life(), gc.Equals, state.Alive)

	testAnnotator(c, func() (state.Annotator, error) {
		return env, nil
	})
}

func (s *EnvironSuite) TestNewEnvironment(c *gc.C) {
	owner := names.NewUserTag("test@remote")
	uuid, err := utils.NewUUID()
	c.Assert(err, gc.IsNil)
	envTag := names.NewEnvironTag(uuid.String())
	env, err := s.State.NewEnvironment(envTag, s.envTag, owner, "testing")
	c.Assert(err, gc.IsNil)

	assertMatches := func(env *state.Environment) {
		c.Assert(env.UUID(), gc.Equals, envTag.Id())
		c.Assert(env.Tag(), gc.Equals, envTag)
		c.Assert(env.ServerTag(), gc.Equals, s.envTag)
		c.Assert(env.Owner(), gc.Equals, owner)
		c.Assert(env.Name(), gc.Equals, "testing")
		c.Assert(env.Life(), gc.Equals, state.Alive)
	}
	assertMatches(env)

	// Since the environ tag for the State connection is different,
	// asking for this environment through FindEntity returns a not found error.
	env, err = s.State.GetEnvironment(envTag)
	c.Assert(err, gc.IsNil)
	assertMatches(env)

	_, err = s.State.FindEntity(envTag)
	c.Assert(err, jc.Satisfies, errors.IsNotFound)

	st, err := s.State.ForEnviron(envTag)
	c.Assert(err, gc.IsNil)
	defer st.Close()
	entity, err := st.FindEntity(envTag)
	c.Assert(err, gc.IsNil)
	c.Assert(entity.Tag(), gc.Equals, envTag)

	env, err = st.Environment()
	c.Assert(err, gc.IsNil)
	assertMatches(env)
}

func (s *EnvironSuite) TestStateServerEnvironment(c *gc.C) {
	env, err := s.State.StateServerEnvironment()
	c.Assert(err, gc.IsNil)

	expectedTag := names.NewEnvironTag(env.UUID())
	c.Assert(env.Tag(), gc.Equals, expectedTag)
	c.Assert(env.ServerTag(), gc.Equals, expectedTag)
	c.Assert(env.Name(), gc.Equals, "testenv")
	c.Assert(env.Owner(), gc.Equals, s.owner)
	c.Assert(env.Life(), gc.Equals, state.Alive)

	testAnnotator(c, func() (state.Annotator, error) {
		return env, nil
	})
}

func (s *EnvironSuite) TestStateServerEnvironmentAccessibleFromOtherEnvironments(c *gc.C) {
	owner := names.NewUserTag("test@remote")
	uuid, err := utils.NewUUID()
	c.Assert(err, gc.IsNil)
	envTag := names.NewEnvironTag(uuid.String())
	_, err = s.State.NewEnvironment(envTag, s.envTag, owner, "testing")
	c.Assert(err, gc.IsNil)

	st, err := s.State.ForEnviron(envTag)
	c.Assert(err, gc.IsNil)
	defer st.Close()

	env, err := s.State.StateServerEnvironment()
	c.Assert(err, gc.IsNil)

	expectedTag := names.NewEnvironTag(env.UUID())
	c.Assert(env.Tag(), gc.Equals, expectedTag)
	c.Assert(env.Name(), gc.Equals, "testenv")
	c.Assert(env.Owner(), gc.Equals, s.owner)
	c.Assert(env.Life(), gc.Equals, state.Alive)

	testAnnotator(c, func() (state.Annotator, error) {
		return env, nil
	})
}
