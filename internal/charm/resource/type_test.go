// Copyright 2015 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package resource_test

import (
	"testing"

	"github.com/juju/errors"
	"github.com/juju/tc"

	"github.com/juju/juju/internal/charm/resource"
)

func TestTypeSuite(t *testing.T) {
	tc.Run(t, &TypeSuite{})
}

type TypeSuite struct{}

func (s *TypeSuite) TestParseTypeOkay(c *tc.C) {
	for resourceType, expected := range map[string]resource.Type{
		"file":      resource.TypeFile,
		"oci-image": resource.TypeContainerImage,
	} {
		rt, err := resource.ParseType(resourceType)
		c.Assert(err, tc.ErrorIsNil)

		c.Check(rt, tc.Equals, expected)
	}
}

func (s *TypeSuite) TestParseTypeEmpty(c *tc.C) {
	rt, err := resource.ParseType("")

	c.Check(err, tc.ErrorMatches, `unsupported resource type ""`)
	var unknown resource.Type
	c.Check(rt, tc.Equals, unknown)
}

func (s *TypeSuite) TestParseTypeUnsupported(c *tc.C) {
	rt, err := resource.ParseType("spam")

	c.Check(err, tc.ErrorMatches, `unsupported resource type "spam"`)
	var unknown resource.Type
	c.Check(rt, tc.Equals, unknown)
}

func (s *TypeSuite) TestTypeStringSupported(c *tc.C) {
	supported := map[resource.Type]string{
		resource.TypeFile:           "file",
		resource.TypeContainerImage: "oci-image",
	}
	for rt, expected := range supported {
		str := rt.String()

		c.Check(str, tc.Equals, expected)
	}
}

func (s *TypeSuite) TestTypeStringUnknown(c *tc.C) {
	var unknown resource.Type
	str := unknown.String()

	c.Check(str, tc.Equals, "")
}

func (s *TypeSuite) TestTypeValidateSupported(c *tc.C) {
	supported := []resource.Type{
		resource.TypeFile,
		resource.TypeContainerImage,
	}
	for _, rt := range supported {
		err := rt.Validate()

		c.Check(err, tc.ErrorIsNil)
	}
}

func (s *TypeSuite) TestTypeValidateUnknown(c *tc.C) {
	var unknown resource.Type
	err := unknown.Validate()

	c.Check(err, tc.ErrorIs, errors.NotValid)
	c.Check(err, tc.ErrorMatches, `unknown resource type`)
}
