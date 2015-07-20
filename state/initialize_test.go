// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"github.com/juju/names"
	gitjujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/constraints"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/state"
	statetesting "github.com/juju/juju/state/testing"
	"github.com/juju/juju/testing"
)

type InitializeSuite struct {
	gitjujutesting.MgoSuite
	testing.BaseSuite
	State *state.State
}

var _ = gc.Suite(&InitializeSuite{})

func (s *InitializeSuite) SetUpSuite(c *gc.C) {
	s.BaseSuite.SetUpSuite(c)
	s.MgoSuite.SetUpSuite(c)
}

func (s *InitializeSuite) TearDownSuite(c *gc.C) {
	s.MgoSuite.TearDownSuite(c)
	s.BaseSuite.TearDownSuite(c)
}

func (s *InitializeSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.MgoSuite.SetUpTest(c)
}

func (s *InitializeSuite) openState(c *gc.C, environTag names.EnvironTag) {
	st, err := state.Open(
		environTag,
		statetesting.NewMongoInfo(),
		statetesting.NewDialOpts(),
		state.Policy(nil),
	)
	c.Assert(err, jc.ErrorIsNil)
	s.State = st
}

func (s *InitializeSuite) TearDownTest(c *gc.C) {
	if s.State != nil {
		err := s.State.Close()
		c.Check(err, jc.ErrorIsNil)
	}
	s.MgoSuite.TearDownTest(c)
	s.BaseSuite.TearDownTest(c)
}

func (s *InitializeSuite) TestInitialize(c *gc.C) {
	cfg := testing.EnvironConfig(c)
	uuid, _ := cfg.UUID()
	initial := cfg.AllAttrs()
	owner := names.NewLocalUserTag("initialize-admin")
	st, err := state.Initialize(owner, statetesting.NewMongoInfo(), cfg, statetesting.NewDialOpts(), nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(st, gc.NotNil)
	envTag := st.EnvironTag()
	c.Assert(envTag.Id(), gc.Equals, uuid)
	err = st.Close()
	c.Assert(err, jc.ErrorIsNil)

	s.openState(c, envTag)

	cfg, err = s.State.EnvironConfig()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cfg.AllAttrs(), gc.DeepEquals, initial)
	// Check that the environment has been created.
	env, err := s.State.Environment()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(env.Tag(), gc.Equals, envTag)
	// Check that the owner has been created.
	c.Assert(env.Owner(), gc.Equals, owner)
	// Check that the owner can be retrieved by the tag.
	entity, err := s.State.FindEntity(env.Owner())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(entity.Tag(), gc.Equals, owner)
	// Check that the owner has an EnvUser created for the bootstrapped environment.
	envUser, err := s.State.EnvironmentUser(env.Owner())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(envUser.UserTag(), gc.Equals, owner)
	c.Assert(envUser.EnvironmentTag(), gc.Equals, env.Tag())

	// Check that the environment can be found through the tag.
	entity, err = s.State.FindEntity(envTag)
	c.Assert(err, jc.ErrorIsNil)
	cons, err := s.State.EnvironConstraints()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(&cons, jc.Satisfies, constraints.IsEmpty)

	addrs, err := s.State.APIHostPorts()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(addrs, gc.HasLen, 0)

	info, err := s.State.StateServerInfo()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(info, jc.DeepEquals, &state.StateServerInfo{EnvironmentTag: envTag})
}

func (s *InitializeSuite) TestDoubleInitializeConfig(c *gc.C) {
	cfg := testing.EnvironConfig(c)
	owner := names.NewLocalUserTag("initialize-admin")

	mgoInfo := statetesting.NewMongoInfo()
	dialOpts := statetesting.NewDialOpts()
	st, err := state.Initialize(owner, mgoInfo, cfg, dialOpts, state.Policy(nil))
	c.Assert(err, jc.ErrorIsNil)
	err = st.Close()
	c.Check(err, jc.ErrorIsNil)

	st, err = state.Initialize(owner, mgoInfo, cfg, dialOpts, state.Policy(nil))
	c.Check(err, gc.ErrorMatches, "already initialized")
	if !c.Check(st, gc.IsNil) {
		err = st.Close()
		c.Check(err, jc.ErrorIsNil)
	}
}

func (s *InitializeSuite) TestEnvironConfigWithAdminSecret(c *gc.C) {
	// admin-secret blocks Initialize.
	good := testing.EnvironConfig(c)
	badUpdateAttrs := map[string]interface{}{"admin-secret": "foo"}
	bad, err := good.Apply(badUpdateAttrs)
	owner := names.NewLocalUserTag("initialize-admin")

	_, err = state.Initialize(owner, statetesting.NewMongoInfo(), bad, statetesting.NewDialOpts(), state.Policy(nil))
	c.Assert(err, gc.ErrorMatches, "admin-secret should never be written to the state")

	// admin-secret blocks UpdateEnvironConfig.
	st := statetesting.Initialize(c, owner, good, nil)
	st.Close()

	s.openState(c, st.EnvironTag())
	err = s.State.UpdateEnvironConfig(badUpdateAttrs, nil, nil)
	c.Assert(err, gc.ErrorMatches, "admin-secret should never be written to the state")

	// EnvironConfig remains inviolate.
	cfg, err := s.State.EnvironConfig()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cfg.AllAttrs(), gc.DeepEquals, good.AllAttrs())
}

func (s *InitializeSuite) TestEnvironConfigWithoutAgentVersion(c *gc.C) {
	// admin-secret blocks Initialize.
	good := testing.EnvironConfig(c)
	attrs := good.AllAttrs()
	delete(attrs, "agent-version")
	bad, err := config.New(config.NoDefaults, attrs)
	c.Assert(err, jc.ErrorIsNil)
	owner := names.NewLocalUserTag("initialize-admin")

	_, err = state.Initialize(owner, statetesting.NewMongoInfo(), bad, statetesting.NewDialOpts(), state.Policy(nil))
	c.Assert(err, gc.ErrorMatches, "agent-version must always be set in state")

	st := statetesting.Initialize(c, owner, good, nil)
	// yay side effects
	st.Close()

	s.openState(c, st.EnvironTag())
	err = s.State.UpdateEnvironConfig(map[string]interface{}{}, []string{"agent-version"}, nil)
	c.Assert(err, gc.ErrorMatches, "agent-version must always be set in state")

	// EnvironConfig remains inviolate.
	cfg, err := s.State.EnvironConfig()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cfg.AllAttrs(), gc.DeepEquals, good.AllAttrs())
}
