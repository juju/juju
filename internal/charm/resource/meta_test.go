// Copyright 2015 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package resource_test

import (
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/internal/charm/resource"
)

var _ = gc.Suite(&MetaSuite{})

type MetaSuite struct{}

func (s *MetaSuite) TestValidateFull(c *gc.C) {
	res := resource.Meta{
		Name:        "my-resource",
		Type:        resource.TypeFile,
		Path:        "filename.tgz",
		Description: "One line that is useful when operators need to push it.",
	}
	err := res.Validate()

	c.Check(err, jc.ErrorIsNil)
}

func (s *MetaSuite) TestValidateZeroValue(c *gc.C) {
	var res resource.Meta
	err := res.Validate()

	c.Check(err, jc.ErrorIs, errors.NotValid)
}

func (s *MetaSuite) TestValidateMissingName(c *gc.C) {
	res := resource.Meta{
		Type:        resource.TypeFile,
		Path:        "filename.tgz",
		Description: "One line that is useful when operators need to push it.",
	}
	err := res.Validate()

	c.Check(err, jc.ErrorIs, errors.NotValid)
	c.Check(err, gc.ErrorMatches, `resource missing name`)
}

func (s *MetaSuite) TestValidateMissingType(c *gc.C) {
	res := resource.Meta{
		Name:        "my-resource",
		Path:        "filename.tgz",
		Description: "One line that is useful when operators need to push it.",
	}
	err := res.Validate()

	c.Check(err, jc.ErrorIs, errors.NotValid)
	c.Check(err, gc.ErrorMatches, `resource missing type`)
}

func (s *MetaSuite) TestValidateMissingPath(c *gc.C) {
	res := resource.Meta{
		Name:        "my-resource",
		Type:        resource.TypeFile,
		Description: "One line that is useful when operators need to push it.",
	}
	err := res.Validate()

	c.Check(err, jc.ErrorIs, errors.NotValid)
	c.Check(err, gc.ErrorMatches, `resource missing filename`)
}

func (s *MetaSuite) TestValidateNestedPath(c *gc.C) {
	res := resource.Meta{
		Name: "my-resource",
		Type: resource.TypeFile,
		Path: "spam/eggs",
	}
	err := res.Validate()

	c.Check(err, jc.ErrorIs, errors.NotValid)
	c.Check(err, gc.ErrorMatches, `.*filename cannot contain "/" .*`)
}

func (s *MetaSuite) TestValidateAbsolutePath(c *gc.C) {
	res := resource.Meta{
		Name: "my-resource",
		Type: resource.TypeFile,
		Path: "/spam/eggs",
	}
	err := res.Validate()

	c.Check(err, jc.ErrorIs, errors.NotValid)
	c.Check(err, gc.ErrorMatches, `.*filename cannot contain "/" .*`)
}

func (s *MetaSuite) TestValidateSuspectPath(c *gc.C) {
	res := resource.Meta{
		Name: "my-resource",
		Type: resource.TypeFile,
		Path: "git@github.com:juju/juju.git",
	}
	err := res.Validate()

	c.Check(err, jc.ErrorIs, errors.NotValid)
	c.Check(err, gc.ErrorMatches, `.*filename cannot contain "/" .*`)
}

func (s *MetaSuite) TestValidateMissingComment(c *gc.C) {
	res := resource.Meta{
		Name: "my-resource",
		Type: resource.TypeFile,
		Path: "filename.tgz",
	}
	err := res.Validate()

	c.Check(err, jc.ErrorIsNil)
}
