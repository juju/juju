// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuclient_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/model"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/juju/testing"
)

type ModelValidationSuite struct {
	testing.BaseSuite
	model jujuclient.ModelDetails
}

func (s *ModelValidationSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.model = jujuclient.ModelDetails{
		ModelUUID: "test.uuid",
		ModelType: model.IAAS,
	}
}

var _ = gc.Suite(&ModelValidationSuite{})

func (s *ModelValidationSuite) TestValidateModelName(c *gc.C) {
	c.Assert(jujuclient.ValidateModelName("foo@bar/baz"), jc.ErrorIsNil)
	c.Assert(jujuclient.ValidateModelName("foo"), gc.ErrorMatches, `validating model name "foo": unqualified model name "foo" not valid`)
	c.Assert(jujuclient.ValidateModelName(""), gc.ErrorMatches, `validating model name "": unqualified model name "" not valid`)
	c.Assert(jujuclient.ValidateModelName("!"), gc.ErrorMatches, `validating model name "!": unqualified model name "!" not valid`)
	c.Assert(jujuclient.ValidateModelName("!/foo"), gc.ErrorMatches, `validating model name "!/foo": user name "!" not valid`)
}

func (s *ModelValidationSuite) TestValidateModelDetailsNoModelUUID(c *gc.C) {
	s.model.ModelUUID = ""
	s.assertValidateModelDetailsFails(c, "missing uuid, model details not valid")
}

func (s *ModelValidationSuite) TestValidateModelDetailsNoModelType(c *gc.C) {
	s.model.ModelType = ""
	s.assertValidateModelDetailsFails(c, "missing type, model details not valid")
}

func (s *ModelValidationSuite) assertValidateModelDetailsFails(c *gc.C, failureMessage string) {
	err := jujuclient.ValidateModelDetails(s.model)
	c.Assert(err, gc.ErrorMatches, failureMessage)
}
