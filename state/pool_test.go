// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/state"
	statetesting "github.com/juju/juju/state/testing"
)

type statePoolSuite struct {
	statetesting.StateSuite
	State1, State2              *state.State
	EnvUUID, EnvUUID1, EnvUUID2 string
}

var _ = gc.Suite(&statePoolSuite{})

func (s *statePoolSuite) SetUpTest(c *gc.C) {
	s.StateSuite.SetUpTest(c)
	s.EnvUUID = s.State.EnvironUUID()

	s.State1 = s.Factory.MakeEnvironment(c, nil)
	s.AddCleanup(func(*gc.C) { s.State1.Close() })
	s.EnvUUID1 = s.State1.EnvironUUID()

	s.State2 = s.Factory.MakeEnvironment(c, nil)
	s.AddCleanup(func(*gc.C) { s.State2.Close() })
	s.EnvUUID2 = s.State2.EnvironUUID()
}

func (s *statePoolSuite) TestGet(c *gc.C) {
	p := state.NewStatePool(s.State)
	defer p.Close()

	st1, err := p.Get(s.EnvUUID1)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(st1.EnvironUUID(), gc.Equals, s.EnvUUID1)

	st2, err := p.Get(s.EnvUUID2)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(st2.EnvironUUID(), gc.Equals, s.EnvUUID2)

	// Check that the same instances are returned
	// when a State for the same env is re-requested.
	st1_, err := p.Get(s.EnvUUID1)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(st1_, gc.Equals, st1)

	st2_, err := p.Get(s.EnvUUID2)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(st2_, gc.Equals, st2)
}

func (s *statePoolSuite) TestGetWithStateServerEnv(c *gc.C) {
	p := state.NewStatePool(s.State)
	defer p.Close()

	// When a State for the state server env is requested, the same
	// State that was original passed in should be returned.
	st0, err := p.Get(s.EnvUUID)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(st0, gc.Equals, s.State)
}

func (s *statePoolSuite) TestSystemState(c *gc.C) {
	p := state.NewStatePool(s.State)
	defer p.Close()

	st0 := p.SystemState()
	c.Assert(st0, gc.Equals, s.State)
}

func (s *statePoolSuite) TestClose(c *gc.C) {
	p := state.NewStatePool(s.State)
	defer p.Close()

	// Get some State instances.
	st1, err := p.Get(s.EnvUUID1)
	c.Assert(err, jc.ErrorIsNil)

	st2, err := p.Get(s.EnvUUID1)
	c.Assert(err, jc.ErrorIsNil)

	// Now close them.
	err = p.Close()
	c.Assert(err, jc.ErrorIsNil)

	// Confirm that state server State isn't closed.
	_, err = s.State.Environment()
	c.Assert(err, jc.ErrorIsNil)

	// Ensure that new ones are returned if further States are
	// requested.
	st1_, err := p.Get(s.EnvUUID1)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(st1_, gc.Not(gc.Equals), st1)

	st2_, err := p.Get(s.EnvUUID2)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(st2_, gc.Not(gc.Equals), st2)
}
