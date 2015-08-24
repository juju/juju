// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/state"
	"github.com/juju/juju/state/testing"
)

type utilsSuite struct {
	testing.StateSuite
	pool *state.StatePool
}

var _ = gc.Suite(&utilsSuite{})

func (s *utilsSuite) SetUpTest(c *gc.C) {
	s.StateSuite.SetUpTest(c)
	s.pool = state.NewStatePool(s.State)
	s.AddCleanup(func(*gc.C) { s.pool.Close() })
}

func (s *utilsSuite) TestValidateEmpty(c *gc.C) {
	st, err := validateEnvironUUID(
		validateArgs{
			statePool: s.pool,
		})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(st.EnvironUUID(), gc.Equals, s.State.EnvironUUID())
}

func (s *utilsSuite) TestValidateEmptyStrict(c *gc.C) {
	_, err := validateEnvironUUID(
		validateArgs{
			statePool: s.pool,
			strict:    true,
		})
	c.Assert(err, gc.ErrorMatches, `unknown environment: ""`)
}

func (s *utilsSuite) TestValidateStateServer(c *gc.C) {
	st, err := validateEnvironUUID(
		validateArgs{
			statePool: s.pool,
			envUUID:   s.State.EnvironUUID(),
		})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(st.EnvironUUID(), gc.Equals, s.State.EnvironUUID())
}

func (s *utilsSuite) TestValidateStateServerStrict(c *gc.C) {
	st, err := validateEnvironUUID(
		validateArgs{
			statePool: s.pool,
			envUUID:   s.State.EnvironUUID(),
			strict:    true,
		})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(st.EnvironUUID(), gc.Equals, s.State.EnvironUUID())
}

func (s *utilsSuite) TestValidateBadEnvUUID(c *gc.C) {
	_, err := validateEnvironUUID(
		validateArgs{
			statePool: s.pool,
			envUUID:   "bad",
		})
	c.Assert(err, gc.ErrorMatches, `unknown environment: "bad"`)
}

func (s *utilsSuite) TestValidateOtherEnvironment(c *gc.C) {
	envState := s.Factory.MakeEnvironment(c, nil)
	defer envState.Close()

	st, err := validateEnvironUUID(
		validateArgs{
			statePool: s.pool,
			envUUID:   envState.EnvironUUID(),
		})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(st.EnvironUUID(), gc.Equals, envState.EnvironUUID())
	st.Close()
}

func (s *utilsSuite) TestValidateOtherEnvironmentStateServerOnly(c *gc.C) {
	envState := s.Factory.MakeEnvironment(c, nil)
	defer envState.Close()

	_, err := validateEnvironUUID(
		validateArgs{
			statePool:          s.pool,
			envUUID:            envState.EnvironUUID(),
			stateServerEnvOnly: true,
		})
	c.Assert(err, gc.ErrorMatches, `requested environment ".*" is not the state server environment`)
}
