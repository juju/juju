// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resource_test

import (
	"time"

	"github.com/juju/tc"
	"github.com/juju/testing"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/resource"
	charmresource "github.com/juju/juju/internal/charm/resource"
)

type ResourceSuite struct {
	testing.IsolationSuite
}

var _ = tc.Suite(&ResourceSuite{})

func (ResourceSuite) TestValidateUploadUsed(c *tc.C) {
	res := resource.Resource{
		Resource:        newFullCharmResource(c, "spam"),
		UUID:            "a-application/spam",
		ApplicationName: "a-application",
		RetrievedBy:     "a-user",
		Timestamp:       time.Now(),
	}

	err := res.Validate()

	c.Check(err, tc.ErrorIsNil)
}

func (ResourceSuite) TestValidateUploadNotUsed(c *tc.C) {
	res := resource.Resource{
		Resource:        newFullCharmResource(c, "spam"),
		UUID:            "a-application/spam",
		ApplicationName: "a-application",
	}

	err := res.Validate()

	c.Check(err, tc.ErrorIsNil)
}

func (ResourceSuite) TestValidateZeroValue(c *tc.C) {
	var res resource.Resource

	err := res.Validate()

	c.Check(err, tc.ErrorIs, coreerrors.NotValid)
	c.Check(err, tc.ErrorMatches, `.*bad info.*`)
}

func (ResourceSuite) TestValidateBadInfo(c *tc.C) {
	var charmRes charmresource.Resource
	c.Assert(charmRes.Validate(), tc.NotNil)

	res := resource.Resource{
		Resource: charmRes,
	}

	err := res.Validate()

	c.Check(err, tc.ErrorIs, coreerrors.NotValid)
	c.Check(err, tc.ErrorMatches, `.*bad info.*`)
}

func (ResourceSuite) TestValidateMissingID(c *tc.C) {
	res := resource.Resource{
		Resource:        newFullCharmResource(c, "spam"),
		ApplicationName: "a-application",
		RetrievedBy:     "a-user",
		Timestamp:       time.Now(),
	}

	err := res.Validate()

	c.Check(err, tc.ErrorIsNil)
}

func (ResourceSuite) TestValidateMissingApplicationName(c *tc.C) {
	res := resource.Resource{
		Resource:    newFullCharmResource(c, "spam"),
		UUID:        "a-application/spam",
		RetrievedBy: "a-user",
		Timestamp:   time.Now(),
	}

	err := res.Validate()

	c.Check(err, tc.ErrorIs, coreerrors.NotValid)
	c.Check(err, tc.ErrorMatches, `.*missing application name.*`)
}

func (ResourceSuite) TestValidateMissingUsername(c *tc.C) {
	res := resource.Resource{
		Resource:        newFullCharmResource(c, "spam"),
		UUID:            "a-application/spam",
		ApplicationName: "a-application",
		RetrievedBy:     "",
		Timestamp:       time.Now(),
	}

	err := res.Validate()

	c.Check(err, tc.ErrorIsNil)
}

func (ResourceSuite) TestValidateMissingTimestamp(c *tc.C) {
	res := resource.Resource{
		Resource:        newFullCharmResource(c, "spam"),
		UUID:            "a-application/spam",
		ApplicationName: "a-application",
		RetrievedBy:     "a-user",
		Timestamp:       time.Time{},
	}

	err := res.Validate()

	c.Check(err, tc.ErrorIs, coreerrors.NotValid)
	c.Check(err, tc.ErrorMatches, `.*missing timestamp.*`)
}

func (ResourceSuite) TestRevisionStringNone(c *tc.C) {
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
	c.Check(err, tc.ErrorIsNil)

	c.Check(res.RevisionString(), tc.Equals, "-")
}

func (ResourceSuite) TestRevisionStringTime(c *tc.C) {
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
	c.Check(err, tc.ErrorIsNil)

	c.Check(res.RevisionString(), tc.Equals, "2012-07-08 15:59:05 +0000 UTC")
}

func (ResourceSuite) TestRevisionStringNumber(c *tc.C) {
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
	c.Check(err, tc.ErrorIsNil)

	c.Check(res.RevisionString(), tc.Equals, "7")
}

func (s *ResourceSuite) TestAsMap(c *tc.C) {
	spam := newStoreResource(c, "spam", "a-application", 2)
	eggs := newStoreResource(c, "eggs", "a-application", 3)
	res := []resource.Resource{
		spam,
		eggs,
	}

	resMap := resource.AsMap(res)

	c.Check(resMap, tc.DeepEquals, map[string]resource.Resource{
		"spam": spam,
		"eggs": eggs,
	})
}
