// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package server_test

import (
	"github.com/juju/errors"
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	charmresource "gopkg.in/juju/charm.v6-unstable/resource"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/resource"
	"github.com/juju/juju/resource/api"
	"github.com/juju/juju/resource/api/server"
)

var _ = gc.Suite(&ListResourcesSuite{})

type ListResourcesSuite struct {
	BaseSuite
}

func (s *ListResourcesSuite) TestOkay(c *gc.C) {
	res1, apiRes1 := newResource(c, "spam", "a-user", "spamspamspam")
	res2, apiRes2 := newResource(c, "eggs", "a-user", "...")

	tag0 := names.NewUnitTag("a-service/0")
	tag1 := names.NewUnitTag("a-service/1")

	chres1 := res1.Resource
	chres2 := res2.Resource
	chres1.Revision++
	chres2.Revision++

	apiChRes1 := apiRes1.CharmResource
	apiChRes2 := apiRes2.CharmResource
	apiChRes1.Revision++
	apiChRes2.Revision++

	s.data.ReturnListResources = resource.ServiceResources{
		Resources: []resource.Resource{
			res1,
			res2,
		},
		UnitResources: []resource.UnitResources{
			{
				Tag: tag0,
				Resources: []resource.Resource{
					res1,
					res2,
				},
			},
			// note: nothing for tag1
		},
		CharmStoreResources: []charmresource.Resource{
			chres1,
			chres2,
		},
	}

	s.data.ReturnUnits = []names.UnitTag{
		tag0,
		tag1,
	}

	facade := server.NewFacade(s.data)

	results, err := facade.ListResources(api.ListResourcesArgs{
		Entities: []params.Entity{{
			Tag: "service-a-service",
		}},
	})
	c.Assert(err, jc.ErrorIsNil)

	c.Check(results, jc.DeepEquals, api.ResourcesResults{
		Results: []api.ResourcesResult{{
			Resources: []api.Resource{
				apiRes1,
				apiRes2,
			},
			UnitResources: []api.UnitResources{
				{
					Entity: params.Entity{
						Tag: "unit-a-service-0",
					},
					Resources: []api.Resource{
						apiRes1,
						apiRes2,
					},
				},
				{
					// we should have a listing for every unit, even if they
					// have no resources.
					Entity: params.Entity{
						Tag: "unit-a-service-1",
					},
				},
			},
			CharmStoreResources: []api.CharmResource{
				apiChRes1,
				apiChRes2,
			},
		}},
	})
	s.stub.CheckCallNames(c, "ListResources", "Units")
	s.stub.CheckCall(c, 0, "ListResources", "a-service")
}

func (s *ListResourcesSuite) TestEmpty(c *gc.C) {
	facade := server.NewFacade(s.data)

	results, err := facade.ListResources(api.ListResourcesArgs{
		Entities: []params.Entity{{
			Tag: "service-a-service",
		}},
	})
	c.Assert(err, jc.ErrorIsNil)

	c.Check(results, jc.DeepEquals, api.ResourcesResults{
		Results: []api.ResourcesResult{{}},
	})
	s.stub.CheckCallNames(c, "ListResources", "Units")
}

func (s *ListResourcesSuite) TestError(c *gc.C) {
	failure := errors.New("<failure>")
	s.stub.SetErrors(failure)
	facade := server.NewFacade(s.data)

	results, err := facade.ListResources(api.ListResourcesArgs{
		Entities: []params.Entity{{
			Tag: "service-a-service",
		}},
	})
	c.Assert(err, jc.ErrorIsNil)

	c.Check(results, jc.DeepEquals, api.ResourcesResults{
		Results: []api.ResourcesResult{{
			ErrorResult: params.ErrorResult{Error: &params.Error{
				Message: "<failure>",
			}},
		}},
	})
	s.stub.CheckCallNames(c, "ListResources")
}
