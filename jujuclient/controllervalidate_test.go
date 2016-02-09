// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuclient_test

import (
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/jujuclient"
)

type ControllerValidateSuite struct {
	baseControllersSuite
}

var _ = gc.Suite(&ControllerValidateSuite{})

func (s *ControllerValidateSuite) TestUpdateControllerNoName(c *gc.C) {
	s.controllerName = ""
	s.assertValidateFails(c, "missing name, controller info not valid")
}

func (s *ControllerValidateSuite) TestUpdateControllerNoControllerUUID(c *gc.C) {
	s.controller.ControllerUUID = ""
	s.assertValidateFails(c, "missing uuid, controller info not valid")
}

func (s *ControllerValidateSuite) TestUpdateControllerNoCACert(c *gc.C) {
	s.controller.CACert = ""
	s.assertValidateFails(c, "missing ca-cert, controller info not valid")
}

func (s *ControllerValidateSuite) assertValidateFails(c *gc.C, failureMessage string) {
	err := jujuclient.ValidateControllerDetails(s.controllerName, s.controller)
	c.Assert(err, gc.ErrorMatches, failureMessage)
}
