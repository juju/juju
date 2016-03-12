// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resource_test

import (
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	charmresource "gopkg.in/juju/charm.v6-unstable/resource"

	"github.com/juju/juju/resource"
	"github.com/juju/juju/resource/resourcetesting"
)

type ServiceResourcesSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&ServiceResourcesSuite{})

func (s *ServiceResourcesSuite) TestUpdatesUploaded(c *gc.C) {
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

func (s *ServiceResourcesSuite) TestUpdatesDifferent(c *gc.C) {
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

func (s *ServiceResourcesSuite) TestUpdatesBadOrdering(c *gc.C) {
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

func (s *ServiceResourcesSuite) TestUpdatesNone(c *gc.C) {
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
