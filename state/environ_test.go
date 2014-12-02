// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"github.com/juju/errors"
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/state"
	"github.com/juju/juju/testing"
)

type EnvironSuite struct {
	ConnSuite
}

var _ = gc.Suite(&EnvironSuite{})

func (s *EnvironSuite) TestEnvironment(c *gc.C) {
	env, err := s.State.Environment()
	c.Assert(err, jc.ErrorIsNil)

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
	cfg, uuid := s.createTestEnvConfig(c)
	owner := names.NewUserTag("test@remote")

	env, st, err := s.State.NewEnvironment(cfg, owner)
	c.Assert(err, jc.ErrorIsNil)
	defer st.Close()

	envTag := names.NewEnvironTag(uuid)
	assertEnvMatches := func(env *state.Environment) {
		c.Assert(env.UUID(), gc.Equals, envTag.Id())
		c.Assert(env.Tag(), gc.Equals, envTag)
		c.Assert(env.ServerTag(), gc.Equals, s.envTag)
		c.Assert(env.Owner(), gc.Equals, owner)
		c.Assert(env.Name(), gc.Equals, "testing")
		c.Assert(env.Life(), gc.Equals, state.Alive)
	}
	assertEnvMatches(env)

	// Since the environ tag for the State connection is different,
	// asking for this environment through FindEntity returns a not found error.
	env, err = s.State.GetEnvironment(envTag)
	c.Assert(err, jc.ErrorIsNil)
	assertEnvMatches(env)

	env, err = st.Environment()
	c.Assert(err, jc.ErrorIsNil)
	assertEnvMatches(env)

	_, err = s.State.FindEntity(envTag)
	c.Assert(err, jc.Satisfies, errors.IsNotFound)

	entity, err := st.FindEntity(envTag)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(entity.Tag(), gc.Equals, envTag)

	// Ensure the environment is functional by adding a machine
	_, err = st.AddMachine("quantal", state.JobManageEnviron)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *EnvironSuite) TestStateServerEnvironment(c *gc.C) {
	env, err := s.State.StateServerEnvironment()
	c.Assert(err, jc.ErrorIsNil)

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
	cfg, _ := s.createTestEnvConfig(c)
	_, st, err := s.State.NewEnvironment(cfg, names.NewUserTag("test@remote"))
	defer st.Close()

	env, err := st.StateServerEnvironment()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(env.Tag(), gc.Equals, s.envTag)
	c.Assert(env.Name(), gc.Equals, "testenv")
	c.Assert(env.Owner(), gc.Equals, s.owner)
	c.Assert(env.Life(), gc.Equals, state.Alive)

	testAnnotator(c, func() (state.Annotator, error) {
		return env, nil
	})
}

// createTestEnvConfig returns a new environment config and its UUID for testing.
func (s *EnvironSuite) createTestEnvConfig(c *gc.C) (*config.Config, string) {
	uuid, err := utils.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	return testing.CustomEnvironConfig(c, testing.Attrs{
		"name": "testing",
		"uuid": uuid.String(),
	}), uuid.String()
}
