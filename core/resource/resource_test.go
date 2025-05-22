// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resource_test

import (
	"testing"
	"time"

	"github.com/juju/tc"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/resource"
	charmresource "github.com/juju/juju/internal/charm/resource"
	"github.com/juju/juju/internal/testhelpers"
)

type ResourceSuite struct {
	testhelpers.IsolationSuite
}

func TestResourceSuite(t *testing.T) {
	tc.Run(t, &ResourceSuite{})
}

func (s *ResourceSuite) TestValidateUploadUsed(c *tc.C) {
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

func (s *ResourceSuite) TestValidateUploadNotUsed(c *tc.C) {
	res := resource.Resource{
		Resource:        newFullCharmResource(c, "spam"),
		UUID:            "a-application/spam",
		ApplicationName: "a-application",
	}

	err := res.Validate()

	c.Check(err, tc.ErrorIsNil)
}

func (s *ResourceSuite) TestValidateZeroValue(c *tc.C) {
	var res resource.Resource

	err := res.Validate()

	c.Check(err, tc.ErrorIs, coreerrors.NotValid)
	c.Check(err, tc.ErrorMatches, `.*bad info.*`)
}

func (s *ResourceSuite) TestValidateBadInfo(c *tc.C) {
	var charmRes charmresource.Resource
	c.Assert(charmRes.Validate(), tc.NotNil)

	res := resource.Resource{
		Resource: charmRes,
	}

	err := res.Validate()

	c.Check(err, tc.ErrorIs, coreerrors.NotValid)
	c.Check(err, tc.ErrorMatches, `.*bad info.*`)
}

func (s *ResourceSuite) TestValidateMissingID(c *tc.C) {
	res := resource.Resource{
		Resource:        newFullCharmResource(c, "spam"),
		ApplicationName: "a-application",
		RetrievedBy:     "a-user",
		Timestamp:       time.Now(),
	}

	err := res.Validate()

	c.Check(err, tc.ErrorIsNil)
}

func (s *ResourceSuite) TestValidateMissingApplicationName(c *tc.C) {
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

func (s *ResourceSuite) TestValidateMissingUsername(c *tc.C) {
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

func (s *ResourceSuite) TestValidateMissingTimestamp(c *tc.C) {
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

func (s *ResourceSuite) TestRevisionStringNone(c *tc.C) {
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

func (s *ResourceSuite) TestRevisionStringTime(c *tc.C) {
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

func (s *ResourceSuite) TestRevisionStringNumber(c *tc.C) {
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
