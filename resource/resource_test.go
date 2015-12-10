// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resource_test

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	charmresource "gopkg.in/juju/charm.v6-unstable/resource"

	"github.com/juju/juju/resource"
)

type ResourceSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&ResourceSuite{})

func (ResourceSuite) TestValidateUploadOkay(c *gc.C) {
	res := resource.Resource{
		Info: resource.Info{
			Info: charmresource.Info{
				Name:    "spam",
				Type:    charmresource.TypeFile,
				Path:    "spam.tgz",
				Comment: "you need it",
			},
			Origin:   resource.OriginKindUpload,
			Revision: 1,
		},
		Username:    "a-user",
		Timestamp:   time.Now(),
		Fingerprint: "chdec737riyg2kqja3yh",
	}

	err := res.Validate()

	c.Check(err, jc.ErrorIsNil)
}

func (ResourceSuite) TestValidateZeroValue(c *gc.C) {
	var res resource.Resource

	err := res.Validate()

	c.Check(errors.Cause(err), jc.Satisfies, errors.IsNotValid)
}

func (ResourceSuite) TestValidateBadInfo(c *gc.C) {
	c.Assert(resource.Info{}.Validate(), gc.NotNil)

	res := resource.Resource{
		Info:        resource.Info{},
		Username:    "a-user",
		Timestamp:   time.Now(),
		Fingerprint: "chdec737riyg2kqja3yh",
	}

	err := res.Validate()

	c.Check(errors.Cause(err), jc.Satisfies, errors.IsNotValid)
	c.Check(err, gc.ErrorMatches, `.*bad info.*`)
}

func (ResourceSuite) TestValidateBadUsername(c *gc.C) {
	res := resource.Resource{
		Info: resource.Info{
			Info: charmresource.Info{
				Name:    "spam",
				Type:    charmresource.TypeFile,
				Path:    "spam.tgz",
				Comment: "you need it",
			},
			Origin:   resource.OriginKindUpload,
			Revision: 1,
		},
		Username:    "",
		Timestamp:   time.Now(),
		Fingerprint: "chdec737riyg2kqja3yh",
	}

	err := res.Validate()

	c.Check(errors.Cause(err), jc.Satisfies, errors.IsNotValid)
	c.Check(err, gc.ErrorMatches, `.*missing username.*`)
}

func (ResourceSuite) TestValidateBadTimestamp(c *gc.C) {
	res := resource.Resource{
		Info: resource.Info{
			Info: charmresource.Info{
				Name:    "spam",
				Type:    charmresource.TypeFile,
				Path:    "spam.tgz",
				Comment: "you need it",
			},
			Origin:   resource.OriginKindUpload,
			Revision: 1,
		},
		Username:    "a-user",
		Timestamp:   time.Time{},
		Fingerprint: "chdec737riyg2kqja3yh",
	}

	err := res.Validate()

	c.Check(errors.Cause(err), jc.Satisfies, errors.IsNotValid)
	c.Check(err, gc.ErrorMatches, `.*missing timestamp.*`)
}

func (ResourceSuite) TestValidateBadFingerprint(c *gc.C) {
	res := resource.Resource{
		Info: resource.Info{
			Info: charmresource.Info{
				Name:    "spam",
				Type:    charmresource.TypeFile,
				Path:    "spam.tgz",
				Comment: "you need it",
			},
			Origin:   resource.OriginKindUpload,
			Revision: 1,
		},
		Username:    "a-user",
		Timestamp:   time.Now(),
		Fingerprint: "",
	}

	err := res.Validate()

	c.Check(errors.Cause(err), jc.Satisfies, errors.IsNotValid)
	c.Check(err, gc.ErrorMatches, `.*missing fingerprint.*`)
}
