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

func (s *utilsSuite) SetUpTest(c *gc.C) {
	s.StateSuite.SetUpTest(c)
}

func (s *utilsSuite) TestValidateEmpty(c *gc.C) {
	uuid, err := validateModelUUID(
		validateArgs{
			statePool: s.StatePool,
		})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(uuid, gc.Equals, s.State.ModelUUID())
}

func (s *utilsSuite) TestValidateEmptyStrict(c *gc.C) {
	_, err := validateModelUUID(
		validateArgs{
			statePool: s.StatePool,
			strict:    true,
		})
	c.Assert(err, gc.ErrorMatches, `unknown model: ""`)
}

func (s *utilsSuite) TestValidateController(c *gc.C) {
	uuid, err := validateModelUUID(
		validateArgs{
			statePool: s.StatePool,
			modelUUID: s.State.ModelUUID(),
		})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(uuid, gc.Equals, s.State.ModelUUID())
}

func (s *utilsSuite) TestValidateControllerStrict(c *gc.C) {
	uuid, err := validateModelUUID(
		validateArgs{
			statePool: s.StatePool,
			modelUUID: s.State.ModelUUID(),
			strict:    true,
		})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(uuid, gc.Equals, s.State.ModelUUID())
}

func (s *utilsSuite) TestValidateBadModelUUID(c *gc.C) {
	_, err := validateModelUUID(
		validateArgs{
			statePool: s.StatePool,
			modelUUID: "bad",
		})
	c.Assert(err, gc.ErrorMatches, `unknown model: "bad"`)
}

func (s *utilsSuite) TestValidateOtherModel(c *gc.C) {
	envState := s.Factory.MakeModel(c, nil)
	defer envState.Close()

	uuid, err := validateModelUUID(
		validateArgs{
			statePool: s.StatePool,
			modelUUID: envState.ModelUUID(),
		})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(uuid, gc.Equals, envState.ModelUUID())
}

func (s *utilsSuite) TestValidateOtherModelControllerOnly(c *gc.C) {
	envState := s.Factory.MakeModel(c, nil)
	defer envState.Close()

	_, err := validateModelUUID(
		validateArgs{
			statePool:           s.StatePool,
			modelUUID:           envState.ModelUUID(),
			controllerModelOnly: true,
		})
	c.Assert(err, gc.ErrorMatches, `requested model ".*" is not the controller model`)
}
