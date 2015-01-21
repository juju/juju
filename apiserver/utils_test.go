// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/state/testing"
)

type utilsSuite struct {
	testing.StateSuite
}

var _ = gc.Suite(&utilsSuite{})

func (s *utilsSuite) TestValidateEmpty(c *gc.C) {
	st, needsClosing, err := validateEnvironUUID(
		validateArgs{
			st: s.State,
		})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(needsClosing, jc.IsFalse)
	c.Assert(st.EnvironUUID(), gc.Equals, s.State.EnvironUUID())
}

func (s *utilsSuite) TestValidateEmptyStrict(c *gc.C) {
	_, _, err := validateEnvironUUID(
		validateArgs{
			st:     s.State,
			strict: true,
		})
	c.Assert(err, gc.ErrorMatches, `unknown environment: ""`)
}

func (s *utilsSuite) TestValidateStateServer(c *gc.C) {
	st, needsClosing, err := validateEnvironUUID(
		validateArgs{
			st:      s.State,
			envUUID: s.State.EnvironUUID(),
		})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(needsClosing, jc.IsFalse)
	c.Assert(st.EnvironUUID(), gc.Equals, s.State.EnvironUUID())
}

func (s *utilsSuite) TestValidateStateServerStrict(c *gc.C) {
	st, needsClosing, err := validateEnvironUUID(
		validateArgs{
			st:      s.State,
			envUUID: s.State.EnvironUUID(),
			strict:  true,
		})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(needsClosing, jc.IsFalse)
	c.Assert(st.EnvironUUID(), gc.Equals, s.State.EnvironUUID())
}

func (s *utilsSuite) TestValidateBadEnvUUID(c *gc.C) {
	_, _, err := validateEnvironUUID(
		validateArgs{
			st:      s.State,
			envUUID: "bad",
		})
	c.Assert(err, gc.ErrorMatches, `unknown environment: "bad"`)
}

func (s *utilsSuite) TestValidateOtherEnvironment(c *gc.C) {
	envState := s.Factory.MakeEnvironment(c, nil)
	defer envState.Close()

	st, needsClosing, err := validateEnvironUUID(
		validateArgs{
			st:      s.State,
			envUUID: envState.EnvironUUID(),
		})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(needsClosing, jc.IsTrue)
	c.Assert(st.EnvironUUID(), gc.Equals, envState.EnvironUUID())
	st.Close()
}

func (s *utilsSuite) TestValidateOtherEnvironmentStateServerOnly(c *gc.C) {
	envState := s.Factory.MakeEnvironment(c, nil)
	defer envState.Close()

	_, _, err := validateEnvironUUID(
		validateArgs{
			st:                 s.State,
			envUUID:            envState.EnvironUUID(),
			stateServerEnvOnly: true,
		})
	c.Assert(err, gc.ErrorMatches, `unknown environment: ".*"`)
}
