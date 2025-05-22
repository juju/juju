// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resource_test

import (
	stdtesting "testing"

	"github.com/juju/tc"

	"github.com/juju/juju/core/resource"
	resourcetesting "github.com/juju/juju/core/resource/testing"
	charmresource "github.com/juju/juju/internal/charm/resource"
	"github.com/juju/juju/internal/testhelpers"
)

type ServiceResourcesSuite struct {
	testhelpers.IsolationSuite
}

func TestServiceResourcesSuite(t *stdtesting.T) {
	tc.Run(t, &ServiceResourcesSuite{})
}

func (s *ServiceResourcesSuite) TestUpdatesUploaded(c *tc.C) {
	csRes := newStoreResource(c, "spam", "a-application", 2)
	res := csRes // a copy
	res.Origin = charmresource.OriginUpload
	sr := resource.ApplicationResources{
		Resources: []resource.Resource{
			res,
		},
		RepositoryResources: []charmresource.Resource{
			csRes.Resource,
		},
	}

	updates, err := sr.Updates()
	c.Assert(err, tc.ErrorIsNil)

	c.Check(updates, tc.HasLen, 0)
}

func (s *ServiceResourcesSuite) TestUpdatesDifferent(c *tc.C) {
	spam := newStoreResource(c, "spam", "a-application", 2)
	eggs := newStoreResource(c, "eggs", "a-application", 3)
	expected := eggs.Resource
	expected.Revision += 1
	sr := resource.ApplicationResources{
		Resources: []resource.Resource{
			spam,
			eggs,
		},
		RepositoryResources: []charmresource.Resource{
			spam.Resource,
			expected,
		},
	}

	updates, err := sr.Updates()
	c.Assert(err, tc.ErrorIsNil)

	c.Check(updates, tc.DeepEquals, []charmresource.Resource{expected})
}

func (s *ServiceResourcesSuite) TestUpdatesBadOrdering(c *tc.C) {
	spam := newStoreResource(c, "spam", "a-application", 2)
	eggs := newStoreResource(c, "eggs", "a-application", 3)
	expected := eggs.Resource
	expected.Revision += 1
	sr := resource.ApplicationResources{
		Resources: []resource.Resource{
			spam,
			eggs,
		},
		RepositoryResources: []charmresource.Resource{
			expected,
			spam.Resource,
		},
	}

	updates, err := sr.Updates()
	c.Assert(err, tc.ErrorIsNil)

	c.Check(updates, tc.DeepEquals, []charmresource.Resource{expected})
}

func (s *ServiceResourcesSuite) TestUpdatesNone(c *tc.C) {
	spam := newStoreResource(c, "spam", "a-application", 2)
	eggs := newStoreResource(c, "eggs", "a-application", 3)
	birds := newStoreResource(c, "birds", "a-application", 3)
	sr := resource.ApplicationResources{
		Resources: []resource.Resource{
			spam,
			eggs,
			birds,
		},
		RepositoryResources: []charmresource.Resource{
			spam.Resource,
			eggs.Resource,
		},
	}

	updates, err := sr.Updates()
	c.Assert(err, tc.ErrorIsNil)

	c.Check(updates, tc.HasLen, 0)
}

func newStoreResource(c *tc.C, name, applicationName string, revision int) resource.Resource {
	content := name
	opened := resourcetesting.NewResource(c, nil, name, applicationName, content)
	res := opened.Resource
	res.Origin = charmresource.OriginStore
	res.Revision = revision
	err := res.Validate()
	c.Assert(err, tc.ErrorIsNil)
	return res
}
