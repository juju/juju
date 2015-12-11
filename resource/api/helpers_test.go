// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package api_test

import (
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	charmresource "gopkg.in/juju/charm.v6-unstable/resource"

	"github.com/juju/juju/resource"
	"github.com/juju/juju/resource/api"
)

type helpersSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&helpersSuite{})

func (helpersSuite) TestResourceInfo2API(c *gc.C) {
	info := resource.Info{
		Info: charmresource.Info{
			Name:    "spam",
			Type:    charmresource.TypeFile,
			Path:    "spam.tgz",
			Comment: "you need it",
		},
		Origin:   resource.OriginKindUpload,
		Revision: 1,
	}
	err := info.Validate()
	c.Assert(err, jc.ErrorIsNil)
	apiInfo := api.ResourceInfo2API(info)

	c.Check(apiInfo, jc.DeepEquals, api.ResourceInfo{
		Name:     "spam",
		Type:     "file",
		Path:     "spam.tgz",
		Comment:  "you need it",
		Origin:   "upload",
		Revision: 1,
	})
}

func (helpersSuite) TestAPI2ResourceInfo(c *gc.C) {
	info, err := api.API2ResourceInfo(api.ResourceInfo{
		Name:     "spam",
		Type:     "file",
		Path:     "spam.tgz",
		Comment:  "you need it",
		Origin:   "upload",
		Revision: 1,
	})
	c.Assert(err, jc.ErrorIsNil)

	expected := resource.Info{
		Info: charmresource.Info{
			Name:    "spam",
			Type:    charmresource.TypeFile,
			Path:    "spam.tgz",
			Comment: "you need it",
		},
		Origin:   resource.OriginKindUpload,
		Revision: 1,
	}
	err = expected.Validate()
	c.Assert(err, jc.ErrorIsNil)

	c.Check(info, jc.DeepEquals, expected)
}
