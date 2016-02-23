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
	"github.com/juju/juju/resource/resourcetesting"
)

type ResourceSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&ResourceSuite{})

func (ResourceSuite) TestValidateUploadUsed(c *gc.C) {
	res := resource.Resource{
		Resource:  newFullCharmResource(c, "spam"),
		ID:        "a-service/spam",
		ServiceID: "a-service",
		Username:  "a-user",
		Timestamp: time.Now(),
	}

	err := res.Validate()

	c.Check(err, jc.ErrorIsNil)
}

func (ResourceSuite) TestValidateUploadNotUsed(c *gc.C) {
	res := resource.Resource{
		Resource:  newFullCharmResource(c, "spam"),
		ID:        "a-service/spam",
		ServiceID: "a-service",
	}

	err := res.Validate()

	c.Check(err, jc.ErrorIsNil)
}

func (ResourceSuite) TestValidateUploadPending(c *gc.C) {
	res := resource.Resource{
		Resource:  newFullCharmResource(c, "spam"),
		ID:        "a-service/spam",
		PendingID: "some-unique-ID",
		ServiceID: "a-service",
		Username:  "a-user",
		Timestamp: time.Now(),
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
		Resource:  newFullCharmResource(c, "spam"),
		ServiceID: "a-service",
		Username:  "a-user",
		Timestamp: time.Now(),
	}

	err := res.Validate()

	c.Check(err, jc.ErrorIsNil)
}

func (ResourceSuite) TestValidateMissingServiceID(c *gc.C) {
	res := resource.Resource{
		Resource:  newFullCharmResource(c, "spam"),
		ID:        "a-service/spam",
		Username:  "a-user",
		Timestamp: time.Now(),
	}

	err := res.Validate()

	c.Check(errors.Cause(err), jc.Satisfies, errors.IsNotValid)
	c.Check(err, gc.ErrorMatches, `.*missing service ID.*`)
}

func (ResourceSuite) TestValidateMissingUsername(c *gc.C) {
	res := resource.Resource{
		Resource:  newFullCharmResource(c, "spam"),
		ID:        "a-service/spam",
		ServiceID: "a-service",
		Username:  "",
		Timestamp: time.Now(),
	}

	err := res.Validate()

	c.Check(err, jc.ErrorIsNil)
}

func (ResourceSuite) TestValidateMissingTimestamp(c *gc.C) {
	res := resource.Resource{
		Resource:  newFullCharmResource(c, "spam"),
		ID:        "a-service/spam",
		ServiceID: "a-service",
		Username:  "a-user",
		Timestamp: time.Time{},
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
		ServiceID: "svc",
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
		ServiceID: "svc",
		Username:  "a-user",
		Timestamp: time.Date(2012, 7, 8, 15, 59, 5, 5, time.UTC),
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
		ServiceID: "svc",
		Username:  "a-user",
		Timestamp: time.Date(2012, 7, 8, 15, 59, 5, 5, time.UTC),
	}

	err := res.Validate()
	c.Check(err, jc.ErrorIsNil)

	c.Check(res.RevisionString(), gc.Equals, "7")
}

func (s *ResourceSuite) TestUpdatesUploaded(c *gc.C) {
	csRes := newStoreResource(c, "spam", "a-service", 2)
	res := csRes // a copy
	res.Origin = charmresource.OriginUpload
	sr := resource.ServiceResources{
		Resources: []resource.Resource{
			res,
		},
		CharmStoreResources: []charmresource.Resource{
			csRes.Resource,
		},
	}

	updates, err := sr.Updates()
	c.Assert(err, jc.ErrorIsNil)

	c.Check(updates, gc.HasLen, 0)
}

func (s *ResourceSuite) TestUpdatesDifferent(c *gc.C) {
	spam := newStoreResource(c, "spam", "a-service", 2)
	eggs := newStoreResource(c, "eggs", "a-service", 3)
	expected := eggs.Resource
	expected.Revision += 1
	sr := resource.ServiceResources{
		Resources: []resource.Resource{
			spam,
			eggs,
		},
		CharmStoreResources: []charmresource.Resource{
			spam.Resource,
			expected,
		},
	}

	updates, err := sr.Updates()
	c.Assert(err, jc.ErrorIsNil)

	c.Check(updates, jc.DeepEquals, []charmresource.Resource{expected})
}

func (s *ResourceSuite) TestUpdatesBadOrdering(c *gc.C) {
	spam := newStoreResource(c, "spam", "a-service", 2)
	eggs := newStoreResource(c, "eggs", "a-service", 3)
	expected := eggs.Resource
	expected.Revision += 1
	sr := resource.ServiceResources{
		Resources: []resource.Resource{
			spam,
			eggs,
		},
		CharmStoreResources: []charmresource.Resource{
			expected,
			spam.Resource,
		},
	}

	updates, err := sr.Updates()
	c.Assert(err, jc.ErrorIsNil)

	c.Check(updates, jc.DeepEquals, []charmresource.Resource{expected})
}

func (s *ResourceSuite) TestUpdatesNone(c *gc.C) {
	spam := newStoreResource(c, "spam", "a-service", 2)
	eggs := newStoreResource(c, "eggs", "a-service", 3)
	sr := resource.ServiceResources{
		Resources: []resource.Resource{
			spam,
			eggs,
		},
		CharmStoreResources: []charmresource.Resource{
			spam.Resource,
			eggs.Resource,
		},
	}

	updates, err := sr.Updates()
	c.Assert(err, jc.ErrorIsNil)

	c.Check(updates, gc.HasLen, 0)
}

func newStoreResource(c *gc.C, name, serviceID string, revision int) resource.Resource {
	content := name
	opened := resourcetesting.NewResource(c, nil, name, serviceID, content)
	res := opened.Resource
	res.Origin = charmresource.OriginStore
	res.Revision = revision
	err := res.Validate()
	c.Assert(err, jc.ErrorIsNil)
	return res
}
