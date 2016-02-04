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

func (s *InitializeSuite) openState(c *gc.C, modelTag names.ModelTag) {
	st, err := state.Open(
		modelTag,
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
	cfg := testing.ModelConfig(c)
	uuid, _ := cfg.UUID()
	initial := cfg.AllAttrs()
	owner := names.NewLocalUserTag("initialize-admin")
	st, err := state.Initialize(owner, statetesting.NewMongoInfo(), cfg, statetesting.NewDialOpts(), nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(st, gc.NotNil)
	modelTag := st.ModelTag()
	c.Assert(modelTag.Id(), gc.Equals, uuid)
	err = st.Close()
	c.Assert(err, jc.ErrorIsNil)

	s.openState(c, modelTag)

	cfg, err = s.State.ModelConfig()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cfg.AllAttrs(), gc.DeepEquals, initial)
	// Check that the model has been created.
	env, err := s.State.Model()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(env.Tag(), gc.Equals, modelTag)
	// Check that the owner has been created.
	c.Assert(env.Owner(), gc.Equals, owner)
	// Check that the owner can be retrieved by the tag.
	entity, err := s.State.FindEntity(env.Owner())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(entity.Tag(), gc.Equals, owner)
	// Check that the owner has an ModelUser created for the bootstrapped model.
	modelUser, err := s.State.ModelUser(env.Owner())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(modelUser.UserTag(), gc.Equals, owner)
	c.Assert(modelUser.ModelTag(), gc.Equals, env.Tag())

	// Check that the model can be found through the tag.
	entity, err = s.State.FindEntity(modelTag)
	c.Assert(err, jc.ErrorIsNil)
	cons, err := s.State.ModelConstraints()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(&cons, jc.Satisfies, constraints.IsEmpty)

	addrs, err := s.State.APIHostPorts()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(addrs, gc.HasLen, 0)

	info, err := s.State.ControllerInfo()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(info, jc.DeepEquals, &state.ControllerInfo{ModelTag: modelTag})
}

func (s *InitializeSuite) TestDoubleInitializeConfig(c *gc.C) {
	cfg := testing.ModelConfig(c)
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

func (s *InitializeSuite) TestModelConfigWithAdminSecret(c *gc.C) {
	// admin-secret blocks Initialize.
	good := testing.ModelConfig(c)
	badUpdateAttrs := map[string]interface{}{"admin-secret": "foo"}
	bad, err := good.Apply(badUpdateAttrs)
	owner := names.NewLocalUserTag("initialize-admin")

	_, err = state.Initialize(owner, statetesting.NewMongoInfo(), bad, statetesting.NewDialOpts(), state.Policy(nil))
	c.Assert(err, gc.ErrorMatches, "admin-secret should never be written to the state")

	// admin-secret blocks UpdateModelConfig.
	st := statetesting.Initialize(c, owner, good, nil)
	st.Close()

	s.openState(c, st.ModelTag())
	err = s.State.UpdateModelConfig(badUpdateAttrs, nil, nil)
	c.Assert(err, gc.ErrorMatches, "admin-secret should never be written to the state")

	// ModelConfig remains inviolate.
	cfg, err := s.State.ModelConfig()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cfg.AllAttrs(), gc.DeepEquals, good.AllAttrs())
}

func (s *InitializeSuite) TestModelConfigWithoutAgentVersion(c *gc.C) {
	// admin-secret blocks Initialize.
	good := testing.ModelConfig(c)
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

	s.openState(c, st.ModelTag())
	err = s.State.UpdateModelConfig(map[string]interface{}{}, []string{"agent-version"}, nil)
	c.Assert(err, gc.ErrorMatches, "agent-version must always be set in state")

	// ModelConfig remains inviolate.
	cfg, err := s.State.ModelConfig()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cfg.AllAttrs(), gc.DeepEquals, good.AllAttrs())
}
