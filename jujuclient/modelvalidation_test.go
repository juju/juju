// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuclient_test

import (
	"github.com/juju/tc"

	"github.com/juju/juju/core/model"
	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/jujuclient"
)

type ModelValidationSuite struct {
	testing.BaseSuite
	model jujuclient.ModelDetails
}

func (s *ModelValidationSuite) SetUpTest(c *tc.C) {
	s.BaseSuite.SetUpTest(c)
	s.model = jujuclient.ModelDetails{
		ModelUUID: "test.uuid",
		ModelType: model.IAAS,
	}
}

var _ = tc.Suite(&ModelValidationSuite{})

func (s *ModelValidationSuite) TestValidateModelName(c *tc.C) {
	c.Assert(jujuclient.ValidateModelName("foo@bar/baz"), tc.ErrorIsNil)
	c.Assert(jujuclient.ValidateModelName("foo"), tc.ErrorMatches, `validating model name "foo": unqualified model name "foo" not valid`)
	c.Assert(jujuclient.ValidateModelName(""), tc.ErrorMatches, `validating model name "": unqualified model name "" not valid`)
	c.Assert(jujuclient.ValidateModelName("!"), tc.ErrorMatches, `validating model name "!": unqualified model name "!" not valid`)
	c.Assert(jujuclient.ValidateModelName("!/foo"), tc.ErrorMatches, `validating model name "!/foo": user name "!" not valid`)
}

func (s *ModelValidationSuite) TestValidateModelDetailsNoModelUUID(c *tc.C) {
	s.model.ModelUUID = ""
	s.assertValidateModelDetailsFails(c, "missing uuid, model details not valid")
}

func (s *ModelValidationSuite) TestValidateModelDetailsNoModelType(c *tc.C) {
	s.model.ModelType = ""
	s.assertValidateModelDetailsFails(c, "missing type, model details not valid")
}

func (s *ModelValidationSuite) assertValidateModelDetailsFails(c *tc.C, failureMessage string) {
	err := jujuclient.ValidateModelDetails(s.model)
	c.Assert(err, tc.ErrorMatches, failureMessage)
}
