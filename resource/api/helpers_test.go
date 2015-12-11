// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package api_test

import (
	"time"

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

func (helpersSuite) TestResource2API(c *gc.C) {
	now := time.Now()
	res := resource.Resource{
		Info: resource.Info{
			Resource: charmresource.Resource{
				Meta: charmresource.Meta{
					Name:    "spam",
					Type:    charmresource.TypeFile,
					Path:    "spam.tgz",
					Comment: "you need it",
				},
				Revision:    1,
				Fingerprint: "chdec737riyg2kqja3yh",
			},
			Origin: resource.OriginKindUpload,
		},
		Username:  "a-user",
		Timestamp: now,
	}
	err := res.Validate()
	c.Assert(err, jc.ErrorIsNil)
	apiRes := api.Resource2API(res)

	c.Check(apiRes, jc.DeepEquals, api.Resource{
		ResourceInfo: api.ResourceInfo{
			Name:        "spam",
			Type:        "file",
			Path:        "spam.tgz",
			Comment:     "you need it",
			Revision:    1,
			Fingerprint: "chdec737riyg2kqja3yh",
			Origin:      "upload",
		},
		Username:  "a-user",
		Timestamp: now,
	})
}

func (helpersSuite) TestAPI2Resource(c *gc.C) {
	now := time.Now()
	res, err := api.API2Resource(api.Resource{
		ResourceInfo: api.ResourceInfo{
			Name:        "spam",
			Type:        "file",
			Path:        "spam.tgz",
			Comment:     "you need it",
			Revision:    1,
			Fingerprint: "chdec737riyg2kqja3yh",
			Origin:      "upload",
		},
		Username:  "a-user",
		Timestamp: now,
	})
	c.Assert(err, jc.ErrorIsNil)

	expected := resource.Resource{
		Info: resource.Info{
			Resource: charmresource.Resource{
				Meta: charmresource.Meta{
					Name:    "spam",
					Type:    charmresource.TypeFile,
					Path:    "spam.tgz",
					Comment: "you need it",
				},
				Revision:    1,
				Fingerprint: "chdec737riyg2kqja3yh",
			},
			Origin: resource.OriginKindUpload,
		},
		Username:  "a-user",
		Timestamp: now,
	}
	err = expected.Validate()
	c.Assert(err, jc.ErrorIsNil)

	c.Check(res, jc.DeepEquals, expected)
}

func (helpersSuite) TestResourceInfo2API(c *gc.C) {
	info := resource.Info{
		Resource: charmresource.Resource{
			Meta: charmresource.Meta{
				Name:    "spam",
				Type:    charmresource.TypeFile,
				Path:    "spam.tgz",
				Comment: "you need it",
			},
			Revision:    1,
			Fingerprint: "chdec737riyg2kqja3yh",
		},
		Origin: resource.OriginKindUpload,
	}
	err := info.Validate()
	c.Assert(err, jc.ErrorIsNil)
	apiInfo := api.ResourceInfo2API(info)

	c.Check(apiInfo, jc.DeepEquals, api.ResourceInfo{
		Name:        "spam",
		Type:        "file",
		Path:        "spam.tgz",
		Comment:     "you need it",
		Revision:    1,
		Fingerprint: "chdec737riyg2kqja3yh",
		Origin:      "upload",
	})
}

func (helpersSuite) TestAPI2ResourceInfo(c *gc.C) {
	info, err := api.API2ResourceInfo(api.ResourceInfo{
		Name:        "spam",
		Type:        "file",
		Path:        "spam.tgz",
		Comment:     "you need it",
		Revision:    1,
		Fingerprint: "chdec737riyg2kqja3yh",
		Origin:      "upload",
	})
	c.Assert(err, jc.ErrorIsNil)

	expected := resource.Info{
		Resource: charmresource.Resource{
			Meta: charmresource.Meta{
				Name:    "spam",
				Type:    charmresource.TypeFile,
				Path:    "spam.tgz",
				Comment: "you need it",
			},
			Revision:    1,
			Fingerprint: "chdec737riyg2kqja3yh",
		},
		Origin: resource.OriginKindUpload,
	}
	err = expected.Validate()
	c.Assert(err, jc.ErrorIsNil)

	c.Check(info, jc.DeepEquals, expected)
}
