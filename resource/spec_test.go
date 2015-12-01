// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resource_test

import (
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	charmresource "gopkg.in/juju/charm.v6-unstable/resource"

	"github.com/juju/juju/resource"
)

var _ = gc.Suite(&specSuite{})

type specSuite struct {
	testing.IsolationSuite
}

func (specSuite) TestNewSpecUpload(c *gc.C) {
	info := charmresource.Info{
		Name:    "spam",
		Type:    "file",
		Path:    "spam.tgz",
		Comment: "you need it",
	}
	origin := resource.OriginUpload
	revision := resource.NoRevision

	spec, err := resource.NewSpec(info, origin, revision)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(spec.Definition(), jc.DeepEquals, info)
	c.Check(spec.Origin(), gc.Equals, origin)
	c.Check(spec.Revision(), gc.Equals, revision)
}

func (specSuite) TestNewSpecEmptyInfo(c *gc.C) {
	var info charmresource.Info
	origin := resource.OriginUpload
	revision := resource.NoRevision

	spec, err := resource.NewSpec(info, origin, revision)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(spec.Definition(), jc.DeepEquals, info)
	c.Check(spec.Origin(), gc.Equals, origin)
	c.Check(spec.Revision(), gc.Equals, revision)
}

func (specSuite) TestNewSpecEmptyOrigin(c *gc.C) {
	info := charmresource.Info{
		Name:    "spam",
		Type:    "file",
		Path:    "spam.tgz",
		Comment: "you need it",
	}
	revision := resource.NoRevision

	_, err := resource.NewSpec(info, "", revision)

	c.Check(err, jc.Satisfies, errors.IsNotSupported)
	c.Check(err, gc.ErrorMatches, `.*origin.*`)
}

func (specSuite) TestNewSpecUnknownOrigin(c *gc.C) {
	info := charmresource.Info{
		Name:    "spam",
		Type:    "file",
		Path:    "spam.tgz",
		Comment: "you need it",
	}
	revision := resource.NoRevision

	_, err := resource.NewSpec(info, "<bogus>", revision)

	c.Check(err, jc.Satisfies, errors.IsNotSupported)
	c.Check(err, gc.ErrorMatches, `.*origin.*`)
}
