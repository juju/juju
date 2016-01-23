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
	uuid, err := validateModelUUID(
		validateArgs{
			statePool: s.pool,
		})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(uuid, gc.Equals, s.State.EnvironUUID())
}

func (s *utilsSuite) TestValidateEmptyStrict(c *gc.C) {
	_, err := validateModelUUID(
		validateArgs{
			statePool: s.pool,
			strict:    true,
		})
	c.Assert(err, gc.ErrorMatches, `unknown model: ""`)
}

func (s *utilsSuite) TestValidateController(c *gc.C) {
	uuid, err := validateModelUUID(
		validateArgs{
			statePool: s.pool,
			modelUUID: s.State.EnvironUUID(),
		})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(uuid, gc.Equals, s.State.EnvironUUID())
}

func (s *utilsSuite) TestValidateControllerStrict(c *gc.C) {
	uuid, err := validateModelUUID(
		validateArgs{
			statePool: s.pool,
			modelUUID: s.State.EnvironUUID(),
			strict:    true,
		})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(uuid, gc.Equals, s.State.EnvironUUID())
}

func (s *utilsSuite) TestValidateBadModelUUID(c *gc.C) {
	_, err := validateModelUUID(
		validateArgs{
			statePool: s.pool,
			modelUUID: "bad",
		})
	c.Assert(err, gc.ErrorMatches, `unknown model: "bad"`)
}

func (s *utilsSuite) TestValidateOtherModel(c *gc.C) {
	envState := s.Factory.MakeEnvironment(c, nil)
	defer envState.Close()

	uuid, err := validateModelUUID(
		validateArgs{
			statePool: s.pool,
			modelUUID: envState.EnvironUUID(),
		})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(uuid, gc.Equals, envState.EnvironUUID())
}

func (s *utilsSuite) TestValidateOtherModelControllerOnly(c *gc.C) {
	envState := s.Factory.MakeEnvironment(c, nil)
	defer envState.Close()

	_, err := validateModelUUID(
		validateArgs{
			statePool:           s.pool,
			modelUUID:           envState.EnvironUUID(),
			controllerModelOnly: true,
		})
	c.Assert(err, gc.ErrorMatches, `requested model ".*" is not the controller model`)
}
