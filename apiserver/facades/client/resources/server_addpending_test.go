// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resources_test

import (
	charmresource "github.com/juju/charm/v7/resource"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/facades/client/resources"
	"github.com/juju/juju/apiserver/params"
)

var _ = gc.Suite(&AddPendingResourcesSuite{})

type AddPendingResourcesSuite struct {
	BaseSuite
}

func (s *AddPendingResourcesSuite) TestNoURL(c *gc.C) {
	res1, apiRes1 := newResource(c, "spam", "a-user", "spamspamspam")
	id1 := "some-unique-ID"
	s.data.ReturnAddPendingResource = id1
	facade, err := resources.NewFacade(s.data, s.newCSClient)
	c.Assert(err, jc.ErrorIsNil)

	result, err := facade.AddPendingResources(params.AddPendingResourcesArgs{
		Entity: params.Entity{
			Tag: "application-a-application",
		},
		Resources: []params.CharmResource{
			apiRes1.CharmResource,
		},
	})
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckCallNames(c, "AddPendingResource")
	s.stub.CheckCall(c, 0, "AddPendingResource", "a-application", "", res1.Resource)
	c.Check(result, jc.DeepEquals, params.AddPendingResourcesResult{
		PendingIDs: []string{
			id1,
		},
	})
}

func (s *AddPendingResourcesSuite) TestWithURLUpToDate(c *gc.C) {
	res1, apiRes1 := newResource(c, "spam", "a-user", "spamspamspam")
	res1.Origin = charmresource.OriginStore
	res1.Revision = 3
	apiRes1.Origin = charmresource.OriginStore.String()
	apiRes1.Revision = 3
	id1 := "some-unique-ID"
	s.data.ReturnAddPendingResource = id1
	s.csClient.ReturnListResources = [][]charmresource.Resource{{
		res1.Resource,
	}}
	facade, err := resources.NewFacade(s.data, s.newCSClient)
	c.Assert(err, jc.ErrorIsNil)

	result, err := facade.AddPendingResources(params.AddPendingResourcesArgs{
		Entity: params.Entity{
			Tag: "application-a-application",
		},
		AddCharmWithAuthorization: params.AddCharmWithAuthorization{
			URL: "cs:~a-user/trusty/spam-5",
		},
		Resources: []params.CharmResource{
			apiRes1.CharmResource,
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Error, gc.IsNil)

	s.stub.CheckCallNames(c, "newCSClient", "ListResources", "AddPendingResource")
	s.stub.CheckCall(c, 2, "AddPendingResource", "a-application", "", res1.Resource)
	c.Check(result, jc.DeepEquals, params.AddPendingResourcesResult{
		PendingIDs: []string{
			id1,
		},
	})
}

func (s *AddPendingResourcesSuite) TestWithURLMismatchComplete(c *gc.C) {
	res1, apiRes1 := newResource(c, "spam", "a-user", "spamspamspam")
	res1.Origin = charmresource.OriginStore
	res1.Revision = 3
	apiRes1.Origin = charmresource.OriginStore.String()
	apiRes1.Revision = 3
	id1 := "some-unique-ID"
	s.data.ReturnAddPendingResource = id1
	csRes := res1 // a copy
	csRes.Revision = 2
	s.csClient.ReturnListResources = [][]charmresource.Resource{{
		csRes.Resource,
	}}
	facade, err := resources.NewFacade(s.data, s.newCSClient)
	c.Assert(err, jc.ErrorIsNil)

	result, err := facade.AddPendingResources(params.AddPendingResourcesArgs{
		Entity: params.Entity{
			Tag: "application-a-application",
		},
		AddCharmWithAuthorization: params.AddCharmWithAuthorization{
			URL: "cs:~a-user/trusty/spam-5",
		},
		Resources: []params.CharmResource{
			apiRes1.CharmResource,
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Error, gc.IsNil)

	s.stub.CheckCallNames(c, "newCSClient", "ListResources", "AddPendingResource")
	s.stub.CheckCall(c, 2, "AddPendingResource", "a-application", "", res1.Resource)
	c.Check(result, jc.DeepEquals, params.AddPendingResourcesResult{
		PendingIDs: []string{
			id1,
		},
	})
}

func (s *AddPendingResourcesSuite) TestWithURLMismatchIncomplete(c *gc.C) {
	res1, apiRes1 := newResource(c, "spam", "a-user", "spamspamspam")
	res1.Origin = charmresource.OriginStore
	res1.Revision = 2
	apiRes1.Origin = charmresource.OriginStore.String()
	apiRes1.Revision = 3
	apiRes1.Fingerprint = nil
	apiRes1.Size = 0
	id1 := "some-unique-ID"
	s.data.ReturnAddPendingResource = id1
	csRes := res1 // a copy
	csRes.Revision = 2
	s.csClient.ReturnListResources = [][]charmresource.Resource{{
		csRes.Resource,
	}}
	expected := charmresource.Resource{
		Meta:        csRes.Meta,
		Origin:      charmresource.OriginStore,
		Revision:    3,
		Fingerprint: res1.Fingerprint,
		Size:        res1.Size,
	}
	s.csClient.ReturnResourceInfo = &expected
	facade, err := resources.NewFacade(s.data, s.newCSClient)
	c.Assert(err, jc.ErrorIsNil)

	result, err := facade.AddPendingResources(params.AddPendingResourcesArgs{
		Entity: params.Entity{
			Tag: "application-a-application",
		},
		AddCharmWithAuthorization: params.AddCharmWithAuthorization{
			URL: "cs:~a-user/trusty/spam-5",
		},
		Resources: []params.CharmResource{
			apiRes1.CharmResource,
		},
	})
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckCallNames(c, "newCSClient", "ListResources", "ResourceInfo", "AddPendingResource")
	s.stub.CheckCall(c, 3, "AddPendingResource", "a-application", "", expected)
	c.Check(result, jc.DeepEquals, params.AddPendingResourcesResult{
		PendingIDs: []string{
			id1,
		},
	})
}

func (s *AddPendingResourcesSuite) TestWithURLNoRevision(c *gc.C) {
	res1, apiRes1 := newResource(c, "spam", "a-user", "spamspamspam")
	res1.Origin = charmresource.OriginStore
	res1.Revision = 3
	res1.Size = 10
	apiRes1.Origin = charmresource.OriginStore.String()
	apiRes1.Revision = -1
	apiRes1.Size = 0
	apiRes1.Fingerprint = nil
	id1 := "some-unique-ID"
	s.data.ReturnAddPendingResource = id1
	csRes := res1 // a copy
	csRes.Revision = 3
	csRes.Size = 10
	s.csClient.ReturnListResources = [][]charmresource.Resource{{
		csRes.Resource,
	}}
	facade, err := resources.NewFacade(s.data, s.newCSClient)
	c.Assert(err, jc.ErrorIsNil)

	result, err := facade.AddPendingResources(params.AddPendingResourcesArgs{
		Entity: params.Entity{
			Tag: "application-a-application",
		},
		AddCharmWithAuthorization: params.AddCharmWithAuthorization{
			URL: "cs:~a-user/trusty/spam-5",
		},
		Resources: []params.CharmResource{
			apiRes1.CharmResource,
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Error, gc.IsNil)

	s.stub.CheckCallNames(c, "newCSClient", "ListResources", "AddPendingResource")
	s.stub.CheckCall(c, 2, "AddPendingResource", "a-application", "", res1.Resource)
	c.Check(result, jc.DeepEquals, params.AddPendingResourcesResult{
		PendingIDs: []string{
			id1,
		},
	})
}

func (s *AddPendingResourcesSuite) TestLocalCharm(c *gc.C) {
	res1, apiRes1 := newResource(c, "spam", "a-user", "spamspamspam")
	expected := charmresource.Resource{
		Meta:   res1.Meta,
		Origin: charmresource.OriginUpload,
	}
	apiRes1.Origin = charmresource.OriginStore.String()
	apiRes1.Revision = 3
	id1 := "some-unique-ID"
	s.data.ReturnAddPendingResource = id1
	facade, err := resources.NewFacade(s.data, s.newCSClient)
	c.Assert(err, jc.ErrorIsNil)

	result, err := facade.AddPendingResources(params.AddPendingResourcesArgs{
		Entity: params.Entity{
			Tag: "application-a-application",
		},
		AddCharmWithAuthorization: params.AddCharmWithAuthorization{
			URL: "local:trusty/spam",
		},
		Resources: []params.CharmResource{
			apiRes1.CharmResource,
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Error, gc.IsNil)

	s.stub.CheckCallNames(c, "AddPendingResource")
	s.stub.CheckCall(c, 0, "AddPendingResource", "a-application", "", expected)
	c.Check(result, jc.DeepEquals, params.AddPendingResourcesResult{
		PendingIDs: []string{
			id1,
		},
	})
}

func (s *AddPendingResourcesSuite) TestWithURLUpload(c *gc.C) {
	res1, apiRes1 := newResource(c, "spam", "a-user", "spamspamspam")
	res1.Origin = charmresource.OriginUpload
	res1.Revision = 0
	apiRes1.Origin = charmresource.OriginUpload.String()
	apiRes1.Revision = 0
	id1 := "some-unique-ID"
	s.data.ReturnAddPendingResource = id1
	csRes := res1 // a copy
	csRes.Origin = charmresource.OriginStore
	csRes.Revision = 3
	s.csClient.ReturnListResources = [][]charmresource.Resource{{
		csRes.Resource,
	}}
	facade, err := resources.NewFacade(s.data, s.newCSClient)
	c.Assert(err, jc.ErrorIsNil)

	result, err := facade.AddPendingResources(params.AddPendingResourcesArgs{
		Entity: params.Entity{
			Tag: "application-a-application",
		},
		AddCharmWithAuthorization: params.AddCharmWithAuthorization{
			URL: "cs:~a-user/trusty/spam-5",
		},
		Resources: []params.CharmResource{
			apiRes1.CharmResource,
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Error, gc.IsNil)

	s.stub.CheckCallNames(c, "newCSClient", "ListResources", "AddPendingResource")
	s.stub.CheckCall(c, 2, "AddPendingResource", "a-application", "", res1.Resource)
	c.Check(result, jc.DeepEquals, params.AddPendingResourcesResult{
		PendingIDs: []string{
			id1,
		},
	})
}

func (s *AddPendingResourcesSuite) TestUnknownResource(c *gc.C) {
	res1, apiRes1 := newResource(c, "spam", "a-user", "spamspamspam")
	res1.Origin = charmresource.OriginStore
	res1.Revision = 3
	apiRes1.Origin = charmresource.OriginStore.String()
	apiRes1.Revision = 3
	id1 := "some-unique-ID"
	s.data.ReturnAddPendingResource = id1
	s.csClient.ReturnListResources = [][]charmresource.Resource{{
		res1.Resource,
	}}
	facade, err := resources.NewFacade(s.data, s.newCSClient)
	c.Assert(err, jc.ErrorIsNil)

	result, err := facade.AddPendingResources(params.AddPendingResourcesArgs{
		Entity: params.Entity{
			Tag: "application-a-application",
		},
		AddCharmWithAuthorization: params.AddCharmWithAuthorization{
			URL: "cs:~a-user/trusty/spam-5",
		},
		Resources: []params.CharmResource{
			apiRes1.CharmResource,
		},
	})
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckCallNames(c, "newCSClient", "ListResources", "AddPendingResource")
	s.stub.CheckCall(c, 2, "AddPendingResource", "a-application", "", res1.Resource)
	c.Check(result, jc.DeepEquals, params.AddPendingResourcesResult{
		PendingIDs: []string{
			id1,
		},
	})
}

func (s *AddPendingResourcesSuite) TestDataStoreError(c *gc.C) {
	_, apiRes1 := newResource(c, "spam", "a-user", "spamspamspam")
	failure := errors.New("<failure>")
	s.stub.SetErrors(failure)
	facade, err := resources.NewFacade(s.data, s.newCSClient)
	c.Assert(err, jc.ErrorIsNil)

	result, err := facade.AddPendingResources(params.AddPendingResourcesArgs{
		Entity: params.Entity{
			Tag: "application-a-application",
		},
		Resources: []params.CharmResource{
			apiRes1.CharmResource,
		},
	})
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckCallNames(c, "AddPendingResource")
	c.Check(result, jc.DeepEquals, params.AddPendingResourcesResult{
		ErrorResult: params.ErrorResult{Error: &params.Error{
			Message: `while adding pending resource info for "spam": <failure>`,
		}},
	})
}
