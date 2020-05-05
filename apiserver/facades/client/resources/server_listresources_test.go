// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resources_test

import (
	charmresource "github.com/juju/charm/v7/resource"
	"github.com/juju/errors"
	"github.com/juju/names/v4"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/facades/client/resources"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/resource"
)

var _ = gc.Suite(&ListResourcesSuite{})

type ListResourcesSuite struct {
	BaseSuite
}

func (s *ListResourcesSuite) TestOkay(c *gc.C) {
	res1, apiRes1 := newResource(c, "spam", "a-user", "spamspamspam")
	res2, apiRes2 := newResource(c, "eggs", "a-user", "...")

	tag0 := names.NewUnitTag("a-application/0")
	tag1 := names.NewUnitTag("a-application/1")

	chres1 := res1.Resource
	chres2 := res2.Resource
	chres1.Revision++
	chres2.Revision++

	apiChRes1 := apiRes1.CharmResource
	apiChRes2 := apiRes2.CharmResource
	apiChRes1.Revision++
	apiChRes2.Revision++

	s.data.ReturnListResources = resource.ApplicationResources{
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
			{
				Tag: tag1,
			},
		},
		CharmStoreResources: []charmresource.Resource{
			chres1,
			chres2,
		},
	}

	facade, err := resources.NewFacade(s.data, s.newCSClient)
	c.Assert(err, jc.ErrorIsNil)

	results, err := facade.ListResources(params.ListResourcesArgs{
		Entities: []params.Entity{{
			Tag: "application-a-application",
		}},
	})
	c.Assert(err, jc.ErrorIsNil)

	c.Check(results, jc.DeepEquals, params.ResourcesResults{
		Results: []params.ResourcesResult{{
			Resources: []params.Resource{
				apiRes1,
				apiRes2,
			},
			UnitResources: []params.UnitResources{
				{
					Entity: params.Entity{
						Tag: "unit-a-application-0",
					},
					Resources: []params.Resource{
						apiRes1,
						apiRes2,
					},
				},
				{
					// we should have a listing for every unit, even if they
					// have no resources.
					Entity: params.Entity{
						Tag: "unit-a-application-1",
					},
				},
			},
			CharmStoreResources: []params.CharmResource{
				apiChRes1,
				apiChRes2,
			},
		}},
	})
	s.stub.CheckCallNames(c, "ListResources")
	s.stub.CheckCall(c, 0, "ListResources", "a-application")
}

func (s *ListResourcesSuite) TestEmpty(c *gc.C) {
	facade, err := resources.NewFacade(s.data, s.newCSClient)
	c.Assert(err, jc.ErrorIsNil)

	results, err := facade.ListResources(params.ListResourcesArgs{
		Entities: []params.Entity{{
			Tag: "application-a-application",
		}},
	})
	c.Assert(err, jc.ErrorIsNil)

	c.Check(results, jc.DeepEquals, params.ResourcesResults{
		Results: []params.ResourcesResult{{}},
	})
	s.stub.CheckCallNames(c, "ListResources")
}

func (s *ListResourcesSuite) TestError(c *gc.C) {
	failure := errors.New("<failure>")
	s.stub.SetErrors(failure)
	facade, err := resources.NewFacade(s.data, s.newCSClient)
	c.Assert(err, jc.ErrorIsNil)

	results, err := facade.ListResources(params.ListResourcesArgs{
		Entities: []params.Entity{{
			Tag: "application-a-application",
		}},
	})
	c.Assert(err, jc.ErrorIsNil)

	c.Check(results, jc.DeepEquals, params.ResourcesResults{
		Results: []params.ResourcesResult{{
			ErrorResult: params.ErrorResult{Error: &params.Error{
				Message: "<failure>",
			}},
		}},
	})
	s.stub.CheckCallNames(c, "ListResources")
}
