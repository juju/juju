// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package api_test

import (
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v6-unstable"

	"github.com/juju/juju/resource"
	"github.com/juju/juju/resource/api"
)

var _ = gc.Suite(&helpersSuite{})

type helpersSuite struct {
	testing.IsolationSuite
}

func (helpersSuite) TestResourceSpec2API(c *gc.C) {
	spec, err := resource.NewResourceSpec(
		charm.ResourceInfo{
			Name:    "spam",
			Type:    "file",
			Path:    "spam.tgz",
			Comment: "you need it",
		},
		resource.OriginUpload,
		resource.NoRevision,
	)
	c.Assert(err, jc.ErrorIsNil)
	apiSpec := api.ResourceSpec2API(spec)

	c.Check(apiSpec, jc.DeepEquals, api.ResourceSpec{
		Name:     "spam",
		Type:     "file",
		Path:     "spam.tgz",
		Comment:  "you need it",
		Origin:   "upload",
		Revision: "",
	})
}

func (helpersSuite) TestAPI2ResourceSpec(c *gc.C) {
	spec, err := api.API2ResourceSpec(api.ResourceSpec{
		Name:     "spam",
		Type:     "file",
		Path:     "spam.tgz",
		Comment:  "you need it",
		Origin:   "upload",
		Revision: "",
	})
	c.Assert(err, jc.ErrorIsNil)

	expected, err := resource.NewResourceSpec(
		charm.ResourceInfo{
			Name:    "spam",
			Type:    "file",
			Path:    "spam.tgz",
			Comment: "you need it",
		},
		resource.OriginUpload,
		resource.NoRevision,
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(spec, jc.DeepEquals, expected)
}
