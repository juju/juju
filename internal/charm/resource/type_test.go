// Copyright 2015 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package resource_test

import (
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/internal/charm/resource"
)

var _ = gc.Suite(&TypeSuite{})

type TypeSuite struct{}

func (s *TypeSuite) TestParseTypeOkay(c *gc.C) {
	for resourceType, expected := range map[string]resource.Type{
		"file":      resource.TypeFile,
		"oci-image": resource.TypeContainerImage,
	} {
		rt, err := resource.ParseType(resourceType)
		c.Assert(err, jc.ErrorIsNil)

		c.Check(rt, gc.Equals, expected)
	}
}

func (s *TypeSuite) TestParseTypeEmpty(c *gc.C) {
	rt, err := resource.ParseType("")

	c.Check(err, gc.ErrorMatches, `unsupported resource type ""`)
	var unknown resource.Type
	c.Check(rt, gc.Equals, unknown)
}

func (s *TypeSuite) TestParseTypeUnsupported(c *gc.C) {
	rt, err := resource.ParseType("spam")

	c.Check(err, gc.ErrorMatches, `unsupported resource type "spam"`)
	var unknown resource.Type
	c.Check(rt, gc.Equals, unknown)
}

func (s *TypeSuite) TestTypeStringSupported(c *gc.C) {
	supported := map[resource.Type]string{
		resource.TypeFile:           "file",
		resource.TypeContainerImage: "oci-image",
	}
	for rt, expected := range supported {
		str := rt.String()

		c.Check(str, gc.Equals, expected)
	}
}

func (s *TypeSuite) TestTypeStringUnknown(c *gc.C) {
	var unknown resource.Type
	str := unknown.String()

	c.Check(str, gc.Equals, "")
}

func (s *TypeSuite) TestTypeValidateSupported(c *gc.C) {
	supported := []resource.Type{
		resource.TypeFile,
		resource.TypeContainerImage,
	}
	for _, rt := range supported {
		err := rt.Validate()

		c.Check(err, jc.ErrorIsNil)
	}
}

func (s *TypeSuite) TestTypeValidateUnknown(c *gc.C) {
	var unknown resource.Type
	err := unknown.Validate()

	c.Check(err, jc.ErrorIs, errors.NotValid)
	c.Check(err, gc.ErrorMatches, `unknown resource type`)
}
