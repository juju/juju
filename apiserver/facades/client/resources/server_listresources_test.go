// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resources_test

import (
	charmresource "github.com/juju/charm/v12/resource"
	"github.com/juju/errors"
	"github.com/juju/names/v4"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	coreresources "github.com/juju/juju/core/resources"
	"github.com/juju/juju/rpc/params"
)

var _ = gc.Suite(&ListResourcesSuite{})

type ListResourcesSuite struct {
	BaseSuite
}

func (s *ListResourcesSuite) TestOkay(c *gc.C) {
	defer s.setUpTest(c).Finish()
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

	appTag := names.NewApplicationTag("a-application")
	s.backend.EXPECT().ListResources(appTag.Id()).Return(coreresources.ApplicationResources{
		Resources: []coreresources.Resource{
			res1,
			res2,
		},
		UnitResources: []coreresources.UnitResources{
			{
				Tag: tag0,
				Resources: []coreresources.Resource{
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
	}, nil)

	results, err := s.newFacade(c).ListResources(params.ListResourcesArgs{
		Entities: []params.Entity{{
			Tag: appTag.String(),
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
						Tag: tag0.String(),
					},
					Resources: []params.Resource{
						apiRes1,
						apiRes2,
					},
				},
				{
					// we should have a listing for every unit, even if they
					// have no
					Entity: params.Entity{
						Tag: tag1.String(),
					},
				},
			},
			CharmStoreResources: []params.CharmResource{
				apiChRes1,
				apiChRes2,
			},
		}},
	})
}

func (s *ListResourcesSuite) TestEmpty(c *gc.C) {
	defer s.setUpTest(c).Finish()
	tag := names.NewApplicationTag("a-application")
	s.backend.EXPECT().ListResources(tag.Id()).Return(coreresources.ApplicationResources{}, nil)

	results, err := s.newFacade(c).ListResources(params.ListResourcesArgs{
		Entities: []params.Entity{{
			Tag: tag.String(),
		}},
	})
	c.Assert(err, jc.ErrorIsNil)

	c.Check(results, jc.DeepEquals, params.ResourcesResults{
		Results: []params.ResourcesResult{{}},
	})
}

func (s *ListResourcesSuite) TestError(c *gc.C) {
	defer s.setUpTest(c).Finish()
	failure := errors.New("<failure>")
	tag := names.NewApplicationTag("a-application")
	s.backend.EXPECT().ListResources(tag.Id()).Return(coreresources.ApplicationResources{}, failure)

	results, err := s.newFacade(c).ListResources(params.ListResourcesArgs{
		Entities: []params.Entity{{
			Tag: tag.String(),
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
}
