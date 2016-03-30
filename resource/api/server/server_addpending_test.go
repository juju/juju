// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package server_test

import (
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	charmresource "gopkg.in/juju/charm.v6-unstable/resource"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/resource/api"
	"github.com/juju/juju/resource/api/server"
)

var _ = gc.Suite(&AddPendingResourcesSuite{})

type AddPendingResourcesSuite struct {
	BaseSuite
}

func (s *AddPendingResourcesSuite) TestNoURL(c *gc.C) {
	res1, apiRes1 := newResource(c, "spam", "a-user", "spamspamspam")
	id1 := "some-unique-ID"
	s.data.ReturnAddPendingResource = id1
	facade, err := server.NewFacade(s.data, s.newCSClient)
	c.Assert(err, jc.ErrorIsNil)

	result, err := facade.AddPendingResources(api.AddPendingResourcesArgs{
		Entity: params.Entity{
			Tag: "service-a-service",
		},
		Resources: []api.CharmResource{
			apiRes1.CharmResource,
		},
	})
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckCallNames(c, "AddPendingResource")
	s.stub.CheckCall(c, 0, "AddPendingResource", "a-service", "", res1.Resource, nil)
	c.Check(result, jc.DeepEquals, api.AddPendingResourcesResult{
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
	facade, err := server.NewFacade(s.data, s.newCSClient)
	c.Assert(err, jc.ErrorIsNil)

	result, err := facade.AddPendingResources(api.AddPendingResourcesArgs{
		Entity: params.Entity{
			Tag: "service-a-service",
		},
		AddCharmWithAuthorization: params.AddCharmWithAuthorization{
			URL: "cs:~a-user/trusty/spam-5",
		},
		Resources: []api.CharmResource{
			apiRes1.CharmResource,
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Error, gc.IsNil)

	s.stub.CheckCallNames(c, "newCSClient", "ListResources", "AddPendingResource")
	s.stub.CheckCall(c, 2, "AddPendingResource", "a-service", "", res1.Resource, nil)
	c.Check(result, jc.DeepEquals, api.AddPendingResourcesResult{
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
	facade, err := server.NewFacade(s.data, s.newCSClient)
	c.Assert(err, jc.ErrorIsNil)

	result, err := facade.AddPendingResources(api.AddPendingResourcesArgs{
		Entity: params.Entity{
			Tag: "service-a-service",
		},
		AddCharmWithAuthorization: params.AddCharmWithAuthorization{
			URL: "cs:~a-user/trusty/spam-5",
		},
		Resources: []api.CharmResource{
			apiRes1.CharmResource,
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Error, gc.IsNil)

	s.stub.CheckCallNames(c, "newCSClient", "ListResources", "AddPendingResource")
	s.stub.CheckCall(c, 2, "AddPendingResource", "a-service", "", res1.Resource, nil)
	c.Check(result, jc.DeepEquals, api.AddPendingResourcesResult{
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
	s.csClient.ReturnGetResource = &expected
	facade, err := server.NewFacade(s.data, s.newCSClient)
	c.Assert(err, jc.ErrorIsNil)

	result, err := facade.AddPendingResources(api.AddPendingResourcesArgs{
		Entity: params.Entity{
			Tag: "service-a-service",
		},
		AddCharmWithAuthorization: params.AddCharmWithAuthorization{
			URL: "cs:~a-user/trusty/spam-5",
		},
		Resources: []api.CharmResource{
			apiRes1.CharmResource,
		},
	})
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckCallNames(c, "newCSClient", "ListResources", "GetResource", "AddPendingResource")
	s.stub.CheckCall(c, 3, "AddPendingResource", "a-service", "", expected, nil)
	c.Check(result, jc.DeepEquals, api.AddPendingResourcesResult{
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
	facade, err := server.NewFacade(s.data, s.newCSClient)
	c.Assert(err, jc.ErrorIsNil)

	result, err := facade.AddPendingResources(api.AddPendingResourcesArgs{
		Entity: params.Entity{
			Tag: "service-a-service",
		},
		AddCharmWithAuthorization: params.AddCharmWithAuthorization{
			URL: "cs:~a-user/trusty/spam-5",
		},
		Resources: []api.CharmResource{
			apiRes1.CharmResource,
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Error, gc.IsNil)

	s.stub.CheckCallNames(c, "newCSClient", "ListResources", "AddPendingResource")
	s.stub.CheckCall(c, 2, "AddPendingResource", "a-service", "", res1.Resource, nil)
	c.Check(result, jc.DeepEquals, api.AddPendingResourcesResult{
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
	facade, err := server.NewFacade(s.data, s.newCSClient)
	c.Assert(err, jc.ErrorIsNil)

	result, err := facade.AddPendingResources(api.AddPendingResourcesArgs{
		Entity: params.Entity{
			Tag: "service-a-service",
		},
		AddCharmWithAuthorization: params.AddCharmWithAuthorization{
			URL: "local:trusty/spam",
		},
		Resources: []api.CharmResource{
			apiRes1.CharmResource,
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Error, gc.IsNil)

	s.stub.CheckCallNames(c, "AddPendingResource")
	s.stub.CheckCall(c, 0, "AddPendingResource", "a-service", "", expected, nil)
	c.Check(result, jc.DeepEquals, api.AddPendingResourcesResult{
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
	facade, err := server.NewFacade(s.data, s.newCSClient)
	c.Assert(err, jc.ErrorIsNil)

	result, err := facade.AddPendingResources(api.AddPendingResourcesArgs{
		Entity: params.Entity{
			Tag: "service-a-service",
		},
		AddCharmWithAuthorization: params.AddCharmWithAuthorization{
			URL: "cs:~a-user/trusty/spam-5",
		},
		Resources: []api.CharmResource{
			apiRes1.CharmResource,
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Error, gc.IsNil)

	s.stub.CheckCallNames(c, "newCSClient", "ListResources", "AddPendingResource")
	s.stub.CheckCall(c, 2, "AddPendingResource", "a-service", "", res1.Resource, nil)
	c.Check(result, jc.DeepEquals, api.AddPendingResourcesResult{
		PendingIDs: []string{
			id1,
		},
	})
}

// TODO(ericsnow) Once the CS API has ListResources() implemented:
//func (s *AddPendingResourcesSuite) TestUnknownResource(c *gc.C) {
//	_, apiRes1 := newResource(c, "spam", "a-user", "spamspamspam")
//	apiRes1.Origin = charmresource.OriginStore.String()
//	facade, err := server.NewFacade(s.data, s.newCSClient)
//	c.Assert(err, jc.ErrorIsNil)
//
//	result, err := facade.AddPendingResources(api.AddPendingResourcesArgs{
//		Entity: params.Entity{
//			Tag: "service-a-service",
//		},
//		AddCharmWithAuthorization: params.AddCharmWithAuthorization{
//			URL: "cs:~a-user/trusty/spam-5",
//		},
//		Resources: []api.CharmResource{
//			apiRes1.CharmResource,
//		},
//	})
//	c.Assert(err, jc.ErrorIsNil)
//
//	s.stub.CheckCallNames(c, "newCSClient", "ListResources")
//	c.Check(result, jc.DeepEquals, api.AddPendingResourcesResult{
//		ErrorResult: params.ErrorResult{Error: &params.Error{
//			Message: `charm store resource "spam" not found`,
//			Code:    params.CodeNotFound,
//		}},
//	})
//}

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
	facade, err := server.NewFacade(s.data, s.newCSClient)
	c.Assert(err, jc.ErrorIsNil)

	result, err := facade.AddPendingResources(api.AddPendingResourcesArgs{
		Entity: params.Entity{
			Tag: "service-a-service",
		},
		AddCharmWithAuthorization: params.AddCharmWithAuthorization{
			URL: "cs:~a-user/trusty/spam-5",
		},
		Resources: []api.CharmResource{
			apiRes1.CharmResource,
		},
	})
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckCallNames(c, "newCSClient", "ListResources", "AddPendingResource")
	s.stub.CheckCall(c, 2, "AddPendingResource", "a-service", "", res1.Resource, nil)
	c.Check(result, jc.DeepEquals, api.AddPendingResourcesResult{
		PendingIDs: []string{
			id1,
		},
	})
}

func (s *AddPendingResourcesSuite) TestDataStoreError(c *gc.C) {
	_, apiRes1 := newResource(c, "spam", "a-user", "spamspamspam")
	failure := errors.New("<failure>")
	s.stub.SetErrors(failure)
	facade, err := server.NewFacade(s.data, s.newCSClient)
	c.Assert(err, jc.ErrorIsNil)

	result, err := facade.AddPendingResources(api.AddPendingResourcesArgs{
		Entity: params.Entity{
			Tag: "service-a-service",
		},
		Resources: []api.CharmResource{
			apiRes1.CharmResource,
		},
	})
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckCallNames(c, "AddPendingResource")
	c.Check(result, jc.DeepEquals, api.AddPendingResourcesResult{
		ErrorResult: params.ErrorResult{Error: &params.Error{
			Message: `while adding pending resource info for "spam": <failure>`,
		}},
	})
}
