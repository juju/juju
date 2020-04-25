// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resource_test

import (
	"time"

	charmresource "github.com/juju/charm/v7/resource"
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/resource"
)

type ResourceSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&ResourceSuite{})

func (ResourceSuite) TestValidateUploadUsed(c *gc.C) {
	res := resource.Resource{
		Resource:      newFullCharmResource(c, "spam"),
		ID:            "a-application/spam",
		ApplicationID: "a-application",
		Username:      "a-user",
		Timestamp:     time.Now(),
	}

	err := res.Validate()

	c.Check(err, jc.ErrorIsNil)
}

func (ResourceSuite) TestValidateUploadNotUsed(c *gc.C) {
	res := resource.Resource{
		Resource:      newFullCharmResource(c, "spam"),
		ID:            "a-application/spam",
		ApplicationID: "a-application",
	}

	err := res.Validate()

	c.Check(err, jc.ErrorIsNil)
}

func (ResourceSuite) TestValidateUploadPending(c *gc.C) {
	res := resource.Resource{
		Resource:      newFullCharmResource(c, "spam"),
		ID:            "a-application/spam",
		PendingID:     "some-unique-ID",
		ApplicationID: "a-application",
		Username:      "a-user",
		Timestamp:     time.Now(),
	}

	err := res.Validate()

	c.Check(err, jc.ErrorIsNil)
}

func (ResourceSuite) TestValidateZeroValue(c *gc.C) {
	var res resource.Resource

	err := res.Validate()

	c.Check(errors.Cause(err), jc.Satisfies, errors.IsNotValid)
	c.Check(err, gc.ErrorMatches, `.*bad info.*`)
}

func (ResourceSuite) TestValidateBadInfo(c *gc.C) {
	var charmRes charmresource.Resource
	c.Assert(charmRes.Validate(), gc.NotNil)

	res := resource.Resource{
		Resource: charmRes,
	}

	err := res.Validate()

	c.Check(errors.Cause(err), jc.Satisfies, errors.IsNotValid)
	c.Check(err, gc.ErrorMatches, `.*bad info.*`)
}

func (ResourceSuite) TestValidateMissingID(c *gc.C) {
	res := resource.Resource{
		Resource:      newFullCharmResource(c, "spam"),
		ApplicationID: "a-application",
		Username:      "a-user",
		Timestamp:     time.Now(),
	}

	err := res.Validate()

	c.Check(err, jc.ErrorIsNil)
}

func (ResourceSuite) TestValidateMissingApplicationID(c *gc.C) {
	res := resource.Resource{
		Resource:  newFullCharmResource(c, "spam"),
		ID:        "a-application/spam",
		Username:  "a-user",
		Timestamp: time.Now(),
	}

	err := res.Validate()

	c.Check(errors.Cause(err), jc.Satisfies, errors.IsNotValid)
	c.Check(err, gc.ErrorMatches, `.*missing application ID.*`)
}

func (ResourceSuite) TestValidateMissingUsername(c *gc.C) {
	res := resource.Resource{
		Resource:      newFullCharmResource(c, "spam"),
		ID:            "a-application/spam",
		ApplicationID: "a-application",
		Username:      "",
		Timestamp:     time.Now(),
	}

	err := res.Validate()

	c.Check(err, jc.ErrorIsNil)
}

func (ResourceSuite) TestValidateMissingTimestamp(c *gc.C) {
	res := resource.Resource{
		Resource:      newFullCharmResource(c, "spam"),
		ID:            "a-application/spam",
		ApplicationID: "a-application",
		Username:      "a-user",
		Timestamp:     time.Time{},
	}

	err := res.Validate()

	c.Check(errors.Cause(err), jc.Satisfies, errors.IsNotValid)
	c.Check(err, gc.ErrorMatches, `.*missing timestamp.*`)
}

func (ResourceSuite) TestRevisionStringNone(c *gc.C) {
	res := resource.Resource{
		Resource: charmresource.Resource{
			Meta: charmresource.Meta{
				Name:        "foo",
				Type:        charmresource.TypeFile,
				Path:        "foo.tgz",
				Description: "you need it",
			},
			Origin: charmresource.OriginUpload,
		},
		ApplicationID: "svc",
	}

	err := res.Validate()
	c.Check(err, jc.ErrorIsNil)

	c.Check(res.RevisionString(), gc.Equals, "-")
}

func (ResourceSuite) TestRevisionStringTime(c *gc.C) {
	res := resource.Resource{
		Resource: charmresource.Resource{
			Meta: charmresource.Meta{
				Name:        "foo",
				Type:        charmresource.TypeFile,
				Path:        "foo.tgz",
				Description: "you need it",
			},
			Origin: charmresource.OriginUpload,
		},
		ApplicationID: "svc",
		Username:      "a-user",
		Timestamp:     time.Date(2012, 7, 8, 15, 59, 5, 5, time.UTC),
	}

	err := res.Validate()
	c.Check(err, jc.ErrorIsNil)

	c.Check(res.RevisionString(), gc.Equals, "2012-07-08 15:59:05 +0000 UTC")
}

func (ResourceSuite) TestRevisionStringNumber(c *gc.C) {
	res := resource.Resource{
		Resource: charmresource.Resource{
			Meta: charmresource.Meta{
				Name:        "foo",
				Type:        charmresource.TypeFile,
				Path:        "foo.tgz",
				Description: "you need it",
			},
			Origin:   charmresource.OriginStore,
			Revision: 7,
		},
		ApplicationID: "svc",
		Username:      "a-user",
		Timestamp:     time.Date(2012, 7, 8, 15, 59, 5, 5, time.UTC),
	}

	err := res.Validate()
	c.Check(err, jc.ErrorIsNil)

	c.Check(res.RevisionString(), gc.Equals, "7")
}

func (s *ResourceSuite) TestAsMap(c *gc.C) {
	spam := newStoreResource(c, "spam", "a-application", 2)
	eggs := newStoreResource(c, "eggs", "a-application", 3)
	resources := []resource.Resource{
		spam,
		eggs,
	}

	resMap := resource.AsMap(resources)

	c.Check(resMap, jc.DeepEquals, map[string]resource.Resource{
		"spam": spam,
		"eggs": eggs,
	})
}
