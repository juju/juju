// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resource_test

import (
	"time"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/resource"
	charmresource "github.com/juju/juju/internal/charm/resource"
)

type ResourceSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&ResourceSuite{})

func (ResourceSuite) TestValidateUploadUsed(c *gc.C) {
	res := resource.Resource{
		Resource:        newFullCharmResource(c, "spam"),
		UUID:            "a-application/spam",
		ApplicationName: "a-application",
		RetrievedBy:     "a-user",
		Timestamp:       time.Now(),
	}

	err := res.Validate()

	c.Check(err, jc.ErrorIsNil)
}

func (ResourceSuite) TestValidateUploadNotUsed(c *gc.C) {
	res := resource.Resource{
		Resource:        newFullCharmResource(c, "spam"),
		UUID:            "a-application/spam",
		ApplicationName: "a-application",
	}

	err := res.Validate()

	c.Check(err, jc.ErrorIsNil)
}

func (ResourceSuite) TestValidateZeroValue(c *gc.C) {
	var res resource.Resource

	err := res.Validate()

	c.Check(err, jc.ErrorIs, coreerrors.NotValid)
	c.Check(err, gc.ErrorMatches, `.*bad info.*`)
}

func (ResourceSuite) TestValidateBadInfo(c *gc.C) {
	var charmRes charmresource.Resource
	c.Assert(charmRes.Validate(), gc.NotNil)

	res := resource.Resource{
		Resource: charmRes,
	}

	err := res.Validate()

	c.Check(err, jc.ErrorIs, coreerrors.NotValid)
	c.Check(err, gc.ErrorMatches, `.*bad info.*`)
}

func (ResourceSuite) TestValidateMissingID(c *gc.C) {
	res := resource.Resource{
		Resource:        newFullCharmResource(c, "spam"),
		ApplicationName: "a-application",
		RetrievedBy:     "a-user",
		Timestamp:       time.Now(),
	}

	err := res.Validate()

	c.Check(err, jc.ErrorIsNil)
}

func (ResourceSuite) TestValidateMissingApplicationName(c *gc.C) {
	res := resource.Resource{
		Resource:    newFullCharmResource(c, "spam"),
		UUID:        "a-application/spam",
		RetrievedBy: "a-user",
		Timestamp:   time.Now(),
	}

	err := res.Validate()

	c.Check(err, jc.ErrorIs, coreerrors.NotValid)
	c.Check(err, gc.ErrorMatches, `.*missing application name.*`)
}

func (ResourceSuite) TestValidateMissingUsername(c *gc.C) {
	res := resource.Resource{
		Resource:        newFullCharmResource(c, "spam"),
		UUID:            "a-application/spam",
		ApplicationName: "a-application",
		RetrievedBy:     "",
		Timestamp:       time.Now(),
	}

	err := res.Validate()

	c.Check(err, jc.ErrorIsNil)
}

func (ResourceSuite) TestValidateMissingTimestamp(c *gc.C) {
	res := resource.Resource{
		Resource:        newFullCharmResource(c, "spam"),
		UUID:            "a-application/spam",
		ApplicationName: "a-application",
		RetrievedBy:     "a-user",
		Timestamp:       time.Time{},
	}

	err := res.Validate()

	c.Check(err, jc.ErrorIs, coreerrors.NotValid)
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
		ApplicationName: "svc",
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
		ApplicationName: "svc",
		RetrievedBy:     "a-user",
		Timestamp:       time.Date(2012, 7, 8, 15, 59, 5, 5, time.UTC),
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
		ApplicationName: "svc",
		RetrievedBy:     "a-user",
		Timestamp:       time.Date(2012, 7, 8, 15, 59, 5, 5, time.UTC),
	}

	err := res.Validate()
	c.Check(err, jc.ErrorIsNil)

	c.Check(res.RevisionString(), gc.Equals, "7")
}

func (s *ResourceSuite) TestAsMap(c *gc.C) {
	spam := newStoreResource(c, "spam", "a-application", 2)
	eggs := newStoreResource(c, "eggs", "a-application", 3)
	res := []resource.Resource{
		spam,
		eggs,
	}

	resMap := resource.AsMap(res)

	c.Check(resMap, jc.DeepEquals, map[string]resource.Resource{
		"spam": spam,
		"eggs": eggs,
	})
}
