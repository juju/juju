// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	. "launchpad.net/gocheck"

	"launchpad.net/juju-core/constraints"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/errors"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/testing"
	"launchpad.net/juju-core/testing/checkers"
)

type InitializeSuite struct {
	testing.MgoSuite
	testing.LoggingSuite
	State *state.State
}

var _ = Suite(&InitializeSuite{})

func (s *InitializeSuite) SetUpSuite(c *C) {
	s.LoggingSuite.SetUpSuite(c)
	s.MgoSuite.SetUpSuite(c)
}

func (s *InitializeSuite) TearDownSuite(c *C) {
	s.MgoSuite.TearDownSuite(c)
	s.LoggingSuite.TearDownSuite(c)
}

func (s *InitializeSuite) SetUpTest(c *C) {
	s.LoggingSuite.SetUpTest(c)
	s.MgoSuite.SetUpTest(c)
	var err error
	s.State, err = state.Open(state.TestingStateInfo(), state.TestingDialOpts())
	c.Assert(err, IsNil)
}

func (s *InitializeSuite) TearDownTest(c *C) {
	s.State.Close()
	s.MgoSuite.TearDownTest(c)
	s.LoggingSuite.TearDownTest(c)
}

func (s *InitializeSuite) TestInitialize(c *C) {
	_, err := s.State.EnvironConfig()
	c.Assert(err, checkers.Satisfies, errors.IsNotFoundError)
	_, err = s.State.Annotator("environment-foo")
	c.Assert(err, checkers.Satisfies, errors.IsNotFoundError)
	_, err = s.State.EnvironConstraints()
	c.Assert(err, checkers.Satisfies, errors.IsNotFoundError)

	cfg := testing.EnvironConfig(c)
	initial := cfg.AllAttrs()
	st, err := state.Initialize(state.TestingStateInfo(), cfg, state.TestingDialOpts())
	c.Assert(err, IsNil)
	c.Assert(st, NotNil)
	err = st.Close()
	c.Assert(err, IsNil)

	cfg, err = s.State.EnvironConfig()
	c.Assert(err, IsNil)
	c.Assert(cfg.AllAttrs(), DeepEquals, initial)
	env, err := s.State.Annotator("environment-" + cfg.Name())
	c.Assert(err, IsNil)
	annotations, err := env.Annotations()
	c.Assert(err, IsNil)
	c.Assert(annotations, HasLen, 0)
	cons, err := s.State.EnvironConstraints()
	c.Assert(err, IsNil)
	c.Assert(cons, DeepEquals, constraints.Value{})
}

func (s *InitializeSuite) TestDoubleInitializeConfig(c *C) {
	cfg := testing.EnvironConfig(c)
	initial := cfg.AllAttrs()
	st := state.TestingInitialize(c, cfg)
	st.Close()

	// A second initialize returns an open *State, but ignores its params.
	// TODO(fwereade) I think this is crazy, but it's what we were testing
	// for originally...
	cfg, err := cfg.Apply(map[string]interface{}{"authorized-keys": "something-else"})
	c.Assert(err, IsNil)
	st, err = state.Initialize(state.TestingStateInfo(), cfg, state.TestingDialOpts())
	c.Assert(err, IsNil)
	c.Assert(st, NotNil)
	st.Close()

	cfg, err = s.State.EnvironConfig()
	c.Assert(err, IsNil)
	c.Assert(cfg.AllAttrs(), DeepEquals, initial)
}

func (s *InitializeSuite) TestEnvironConfigWithAdminSecret(c *C) {
	// admin-secret blocks Initialize.
	good := testing.EnvironConfig(c)
	bad, err := good.Apply(map[string]interface{}{"admin-secret": "foo"})

	_, err = state.Initialize(state.TestingStateInfo(), bad, state.TestingDialOpts())
	c.Assert(err, ErrorMatches, "admin-secret should never be written to the state")

	// admin-secret blocks SetEnvironConfig.
	st := state.TestingInitialize(c, good)
	st.Close()
	err = s.State.SetEnvironConfig(bad)
	c.Assert(err, ErrorMatches, "admin-secret should never be written to the state")

	// EnvironConfig remains inviolate.
	cfg, err := s.State.EnvironConfig()
	c.Assert(err, IsNil)
	c.Assert(cfg.AllAttrs(), DeepEquals, good.AllAttrs())
}

func (s *InitializeSuite) TestEnvironConfigWithoutAgentVersion(c *C) {
	// admin-secret blocks Initialize.
	good := testing.EnvironConfig(c)
	attrs := good.AllAttrs()
	delete(attrs, "agent-version")
	bad, err := config.New(attrs)
	c.Assert(err, IsNil)

	_, err = state.Initialize(state.TestingStateInfo(), bad, state.TestingDialOpts())
	c.Assert(err, ErrorMatches, "agent-version must always be set in state")

	// Bad agent-version blocks SetEnvironConfig.
	st := state.TestingInitialize(c, good)
	st.Close()
	err = s.State.SetEnvironConfig(bad)
	c.Assert(err, ErrorMatches, "agent-version must always be set in state")

	// EnvironConfig remains inviolate.
	cfg, err := s.State.EnvironConfig()
	c.Assert(err, IsNil)
	c.Assert(cfg.AllAttrs(), DeepEquals, good.AllAttrs())
}
