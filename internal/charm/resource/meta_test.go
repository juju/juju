// Copyright 2015 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package resource_test

import (
	stdtesting "testing"

	"github.com/juju/errors"
	"github.com/juju/tc"

	"github.com/juju/juju/internal/charm/resource"
)

func TestMetaSuite(t *stdtesting.T) { tc.Run(t, &MetaSuite{}) }

type MetaSuite struct{}

func (s *MetaSuite) TestValidateFull(c *tc.C) {
	res := resource.Meta{
		Name:        "my-resource",
		Type:        resource.TypeFile,
		Path:        "filename.tgz",
		Description: "One line that is useful when operators need to push it.",
	}
	err := res.Validate()

	c.Check(err, tc.ErrorIsNil)
}

func (s *MetaSuite) TestValidateZeroValue(c *tc.C) {
	var res resource.Meta
	err := res.Validate()

	c.Check(err, tc.ErrorIs, errors.NotValid)
}

func (s *MetaSuite) TestValidateMissingName(c *tc.C) {
	res := resource.Meta{
		Type:        resource.TypeFile,
		Path:        "filename.tgz",
		Description: "One line that is useful when operators need to push it.",
	}
	err := res.Validate()

	c.Check(err, tc.ErrorIs, errors.NotValid)
	c.Check(err, tc.ErrorMatches, `resource missing name`)
}

func (s *MetaSuite) TestValidateMissingType(c *tc.C) {
	res := resource.Meta{
		Name:        "my-resource",
		Path:        "filename.tgz",
		Description: "One line that is useful when operators need to push it.",
	}
	err := res.Validate()

	c.Check(err, tc.ErrorIs, errors.NotValid)
	c.Check(err, tc.ErrorMatches, `resource missing type`)
}

func (s *MetaSuite) TestValidateMissingPath(c *tc.C) {
	res := resource.Meta{
		Name:        "my-resource",
		Type:        resource.TypeFile,
		Description: "One line that is useful when operators need to push it.",
	}
	err := res.Validate()

	c.Check(err, tc.ErrorIs, errors.NotValid)
	c.Check(err, tc.ErrorMatches, `resource missing filename`)
}

func (s *MetaSuite) TestValidateNestedPath(c *tc.C) {
	res := resource.Meta{
		Name: "my-resource",
		Type: resource.TypeFile,
		Path: "spam/eggs",
	}
	err := res.Validate()

	c.Check(err, tc.ErrorIs, errors.NotValid)
	c.Check(err, tc.ErrorMatches, `.*filename cannot contain "/" .*`)
}

func (s *MetaSuite) TestValidateAbsolutePath(c *tc.C) {
	res := resource.Meta{
		Name: "my-resource",
		Type: resource.TypeFile,
		Path: "/spam/eggs",
	}
	err := res.Validate()

	c.Check(err, tc.ErrorIs, errors.NotValid)
	c.Check(err, tc.ErrorMatches, `.*filename cannot contain "/" .*`)
}

func (s *MetaSuite) TestValidateSuspectPath(c *tc.C) {
	res := resource.Meta{
		Name: "my-resource",
		Type: resource.TypeFile,
		Path: "git@github.com:juju/juju.git",
	}
	err := res.Validate()

	c.Check(err, tc.ErrorIs, errors.NotValid)
	c.Check(err, tc.ErrorMatches, `.*filename cannot contain "/" .*`)
}

func (s *MetaSuite) TestValidateMissingComment(c *tc.C) {
	res := resource.Meta{
		Name: "my-resource",
		Type: resource.TypeFile,
		Path: "filename.tgz",
	}
	err := res.Validate()

	c.Check(err, tc.ErrorIsNil)
}
