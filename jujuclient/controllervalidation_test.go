// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuclient_test

import (
	"github.com/juju/tc"

	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/jujuclient"
)

type ControllerValidationSuite struct {
	testing.BaseSuite
	controller jujuclient.ControllerDetails
}

func (s *ControllerValidationSuite) SetUpTest(c *tc.C) {
	s.BaseSuite.SetUpTest(c)
	s.controller = jujuclient.ControllerDetails{
		ControllerUUID: "test.uuid",
		APIEndpoints:   []string{"test.api.endpoint"},
		CACert:         "test.ca.cert",
		Cloud:          "aws",
		CloudRegion:    "southeastasia",
	}
}

var _ = tc.Suite(&ControllerValidationSuite{})

func (s *ControllerValidationSuite) TestValidateControllerName(c *tc.C) {
	c.Assert(jujuclient.ValidateControllerName(""), tc.ErrorMatches, "empty controller name not valid")
}

func (s *ControllerValidationSuite) TestValidateControllerDetailsNoControllerUUID(c *tc.C) {
	s.controller.ControllerUUID = ""
	s.assertValidateControllerDetailsFails(c, "missing uuid, controller details not valid")
}

func (s *ControllerValidationSuite) assertValidateControllerDetailsFails(c *tc.C, failureMessage string) {
	err := jujuclient.ValidateControllerDetails(s.controller)
	c.Assert(err, tc.ErrorMatches, failureMessage)
}
