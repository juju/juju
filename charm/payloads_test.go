// Copyright 2015 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package charm_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/charm"
)

var _ = gc.Suite(&payloadClassSuite{})

type payloadClassSuite struct{}

func (s *payloadClassSuite) TestParsePayloadClassOkay(c *gc.C) {
	name := "my-payload"
	data := map[string]interface{}{
		"type": "docker",
	}
	payloadClass := charm.ParsePayloadClass(name, data)

	c.Check(payloadClass, jc.DeepEquals, charm.PayloadClass{
		Name: "my-payload",
		Type: "docker",
	})
}

func (s *payloadClassSuite) TestParsePayloadClassMissingName(c *gc.C) {
	name := ""
	data := map[string]interface{}{
		"type": "docker",
	}
	payloadClass := charm.ParsePayloadClass(name, data)

	c.Check(payloadClass, jc.DeepEquals, charm.PayloadClass{
		Name: "",
		Type: "docker",
	})
}

func (s *payloadClassSuite) TestParsePayloadClassEmpty(c *gc.C) {
	name := "my-payload"
	var data map[string]interface{}
	payloadClass := charm.ParsePayloadClass(name, data)

	c.Check(payloadClass, jc.DeepEquals, charm.PayloadClass{
		Name: "my-payload",
	})
}

func (s *payloadClassSuite) TestValidateFull(c *gc.C) {
	payloadClass := charm.PayloadClass{
		Name: "my-payload",
		Type: "docker",
	}
	err := payloadClass.Validate()

	c.Check(err, jc.ErrorIsNil)
}

func (s *payloadClassSuite) TestValidateZeroValue(c *gc.C) {
	var payloadClass charm.PayloadClass
	err := payloadClass.Validate()

	c.Check(err, gc.NotNil)
}

func (s *payloadClassSuite) TestValidateMissingName(c *gc.C) {
	payloadClass := charm.PayloadClass{
		Type: "docker",
	}
	err := payloadClass.Validate()

	c.Check(err, gc.ErrorMatches, `payload class missing name`)
}

func (s *payloadClassSuite) TestValidateBadName(c *gc.C) {
	payloadClass := charm.PayloadClass{
		Name: "my-###-payload",
		Type: "docker",
	}
	err := payloadClass.Validate()

	c.Check(err, gc.ErrorMatches, `invalid payload class "my-###-payload"`)
}

func (s *payloadClassSuite) TestValidateMissingType(c *gc.C) {
	payloadClass := charm.PayloadClass{
		Name: "my-payload",
	}
	err := payloadClass.Validate()

	c.Check(err, gc.ErrorMatches, `payload class missing type`)
}
