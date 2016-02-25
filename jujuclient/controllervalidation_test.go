// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuclient_test

import (
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/jujuclient"
	"github.com/juju/juju/testing"
)

type ControllerValidationSuite struct {
	testing.BaseSuite
	controller jujuclient.ControllerDetails
}

func (s *ControllerValidationSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.controller = jujuclient.ControllerDetails{
		[]string{"test.server.hostname"},
		"test.uuid",
		[]string{"test.api.endpoint"},
		"test.ca.cert",
	}
}

var _ = gc.Suite(&ControllerValidationSuite{})

func (s *ControllerValidationSuite) TestValidateControllerName(c *gc.C) {
	c.Assert(jujuclient.ValidateControllerName(""), gc.ErrorMatches, "empty controller name not valid")
}

func (s *ControllerValidationSuite) TestValidateControllerDetailsNoControllerUUID(c *gc.C) {
	s.controller.ControllerUUID = ""
	s.assertValidateControllerDetailsFails(c, "missing uuid, controller details not valid")
}

func (s *ControllerValidationSuite) TestValidateControllerDetailsNoCACert(c *gc.C) {
	s.controller.CACert = ""
	s.assertValidateControllerDetailsFails(c, "missing ca-cert, controller details not valid")
}

func (s *ControllerValidationSuite) assertValidateControllerDetailsFails(c *gc.C, failureMessage string) {
	err := jujuclient.ValidateControllerDetails(s.controller)
	c.Assert(err, gc.ErrorMatches, failureMessage)
}
