// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/constraints"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/errors"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/testing"
	jc "launchpad.net/juju-core/testing/checkers"
	"launchpad.net/juju-core/testing/testbase"
)

type InitializeSuite struct {
	testing.MgoSuite
	testbase.LoggingSuite
	State *state.State
}

var _ = gc.Suite(&InitializeSuite{})

func (s *InitializeSuite) SetUpSuite(c *gc.C) {
	s.LoggingSuite.SetUpSuite(c)
	s.MgoSuite.SetUpSuite(c)
}

func (s *InitializeSuite) TearDownSuite(c *gc.C) {
	s.MgoSuite.TearDownSuite(c)
	s.LoggingSuite.TearDownSuite(c)
}

func (s *InitializeSuite) SetUpTest(c *gc.C) {
	s.LoggingSuite.SetUpTest(c)
	s.MgoSuite.SetUpTest(c)
	var err error
	s.State, err = state.Open(state.TestingStateInfo(), state.TestingDialOpts())
	c.Assert(err, gc.IsNil)
}

func (s *InitializeSuite) TearDownTest(c *gc.C) {
	s.State.Close()
	s.MgoSuite.TearDownTest(c)
	s.LoggingSuite.TearDownTest(c)
}

func (s *InitializeSuite) TestInitialize(c *gc.C) {
	_, err := s.State.EnvironConfig()
	c.Assert(err, jc.Satisfies, errors.IsNotFoundError)
	_, err = s.State.FindEntity("environment-foo")
	// TODO(axw) 2013-12-04 #1257587
	// remove backwards compatibility for environment-tag; see state.go
	c.Assert(err, jc.Satisfies, errors.IsNotFoundError)
	_, err = s.State.EnvironConstraints()
	c.Assert(err, jc.Satisfies, errors.IsNotFoundError)

	cfg := testing.EnvironConfig(c)
	initial := cfg.AllAttrs()
	st, err := state.Initialize(state.TestingStateInfo(), cfg, state.TestingDialOpts())
	c.Assert(err, gc.IsNil)
	c.Assert(st, gc.NotNil)
	err = st.Close()
	c.Assert(err, gc.IsNil)

	cfg, err = s.State.EnvironConfig()
	c.Assert(err, gc.IsNil)
	c.Assert(cfg.AllAttrs(), gc.DeepEquals, initial)

	env, err := s.State.Environment()
	c.Assert(err, gc.IsNil)
	entity, err := s.State.FindEntity("environment-" + env.UUID())
	c.Assert(err, gc.IsNil)
	annotator := entity.(state.Annotator)
	annotations, err := annotator.Annotations()
	c.Assert(err, gc.IsNil)
	c.Assert(annotations, gc.HasLen, 0)
	cons, err := s.State.EnvironConstraints()
	c.Assert(err, gc.IsNil)
	c.Assert(&cons, jc.Satisfies, constraints.IsEmpty)
}

func (s *InitializeSuite) TestDoubleInitializeConfig(c *gc.C) {
	cfg := testing.EnvironConfig(c)
	initial := cfg.AllAttrs()
	st := state.TestingInitialize(c, cfg)
	st.Close()

	// A second initialize returns an open *State, but ignores its params.
	// TODO(fwereade) I think this is crazy, but it's what we were testing
	// for originally...
	cfg, err := cfg.Apply(map[string]interface{}{"authorized-keys": "something-else"})
	c.Assert(err, gc.IsNil)
	st, err = state.Initialize(state.TestingStateInfo(), cfg, state.TestingDialOpts())
	c.Assert(err, gc.IsNil)
	c.Assert(st, gc.NotNil)
	st.Close()

	cfg, err = s.State.EnvironConfig()
	c.Assert(err, gc.IsNil)
	c.Assert(cfg.AllAttrs(), gc.DeepEquals, initial)
}

func (s *InitializeSuite) TestEnvironConfigWithAdminSecret(c *gc.C) {
	// admin-secret blocks Initialize.
	good := testing.EnvironConfig(c)
	bad, err := good.Apply(map[string]interface{}{"admin-secret": "foo"})

	_, err = state.Initialize(state.TestingStateInfo(), bad, state.TestingDialOpts())
	c.Assert(err, gc.ErrorMatches, "admin-secret should never be written to the state")

	// admin-secret blocks SetEnvironConfig.
	st := state.TestingInitialize(c, good)
	st.Close()
	err = s.State.SetEnvironConfig(bad, good)
	c.Assert(err, gc.ErrorMatches, "admin-secret should never be written to the state")

	// EnvironConfig remains inviolate.
	cfg, err := s.State.EnvironConfig()
	c.Assert(err, gc.IsNil)
	c.Assert(cfg.AllAttrs(), gc.DeepEquals, good.AllAttrs())
}

func (s *InitializeSuite) TestEnvironConfigWithoutAgentVersion(c *gc.C) {
	// admin-secret blocks Initialize.
	good := testing.EnvironConfig(c)
	attrs := good.AllAttrs()
	delete(attrs, "agent-version")
	bad, err := config.New(config.NoDefaults, attrs)
	c.Assert(err, gc.IsNil)

	_, err = state.Initialize(state.TestingStateInfo(), bad, state.TestingDialOpts())
	c.Assert(err, gc.ErrorMatches, "agent-version must always be set in state")

	// Bad agent-version blocks SetEnvironConfig.
	st := state.TestingInitialize(c, good)
	st.Close()
	err = s.State.SetEnvironConfig(bad, good)
	c.Assert(err, gc.ErrorMatches, "agent-version must always be set in state")

	// EnvironConfig remains inviolate.
	cfg, err := s.State.EnvironConfig()
	c.Assert(err, gc.IsNil)
	c.Assert(cfg.AllAttrs(), gc.DeepEquals, good.AllAttrs())
}
