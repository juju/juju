// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resources

import (
	"context"

	"github.com/juju/names/v6"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	apiresources "github.com/juju/juju/api/client/resources"
	coreapplication "github.com/juju/juju/core/application"
	"github.com/juju/juju/core/resource"
	resourcetesting "github.com/juju/juju/core/resource/testing"
	coreunit "github.com/juju/juju/core/unit"
	charmresource "github.com/juju/juju/internal/charm/resource"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/rpc/params"
)

var _ = gc.Suite(&ListResourcesSuite{})

type ListResourcesSuite struct {
	BaseSuite
}

func (s *ListResourcesSuite) TestListResourcesOkay(c *gc.C) {
	defer s.setupMocks(c).Finish()
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
	s.applicationService.EXPECT().GetApplicationIDByName(gomock.Any(),
		appTag.Id()).Return("a-application-id", nil)
	s.resourceService.EXPECT().ListResources(gomock.Any(), coreapplication.ID("a-application-id")).Return(
		resource.ApplicationResources{
			Resources: []resource.Resource{
				res1,
				res2,
			},
			UnitResources: []resource.UnitResources{
				{
					Name: coreunit.Name(tag0.Id()),
					Resources: []resource.Resource{
						res1,
						res2,
					},
				},
				{
					Name: coreunit.Name(tag1.Id()),
				},
			},
			RepositoryResources: []charmresource.Resource{
				chres1,
				chres2,
			},
		}, nil)

	results, err := s.newFacade(c).ListResources(context.Background(), params.ListResourcesArgs{
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

func (s *ListResourcesSuite) TestListResourcesEmpty(c *gc.C) {
	defer s.setupMocks(c).Finish()
	tag := names.NewApplicationTag("a-application")
	s.applicationService.EXPECT().GetApplicationIDByName(gomock.Any(), "a-application").Return("a-application-id", nil)
	s.resourceService.EXPECT().ListResources(gomock.Any(), coreapplication.ID("a-application-id")).Return(resource.
		ApplicationResources{}, nil)

	results, err := s.newFacade(c).ListResources(context.Background(), params.ListResourcesArgs{
		Entities: []params.Entity{{
			Tag: tag.String(),
		}},
	})
	c.Assert(err, jc.ErrorIsNil)

	c.Check(results, jc.DeepEquals, params.ResourcesResults{
		Results: []params.ResourcesResult{{}},
	})
}

func (s *ListResourcesSuite) TestListResourcesErrorGetAppID(c *gc.C) {
	defer s.setupMocks(c).Finish()
	failure := errors.New("<failure>")
	tag := names.NewApplicationTag("a-application")
	s.applicationService.EXPECT().GetApplicationIDByName(gomock.Any(), "a-application").Return("", failure)

	results, err := s.newFacade(c).ListResources(context.Background(), params.ListResourcesArgs{
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

func (s *ListResourcesSuite) TestListResourcesError(c *gc.C) {
	defer s.setupMocks(c).Finish()
	failure := errors.New("<failure>")
	tag := names.NewApplicationTag("a-application")
	s.applicationService.EXPECT().GetApplicationIDByName(gomock.Any(), "a-application").Return("a-application-id", nil)
	s.resourceService.EXPECT().ListResources(gomock.Any(), coreapplication.ID("a-application-id")).Return(resource.
		ApplicationResources{},
		failure)

	results, err := s.newFacade(c).ListResources(context.Background(), params.ListResourcesArgs{
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

func (s *ListResourcesSuite) TestServiceResources2API(c *gc.C) {
	res1 := resourcetesting.NewResource(c, nil, "res1", "a-application", "data").Resource
	res2 := resourcetesting.NewResource(c, nil, "res2", "a-application", "data2").Resource

	tag0 := names.NewUnitTag("a-application/0")
	tag1 := names.NewUnitTag("a-application/1")

	chres1 := res1.Resource
	chres2 := res2.Resource
	chres1.Revision++
	chres2.Revision++

	svcRes := resource.ApplicationResources{
		Resources: []resource.Resource{
			res1,
			res2,
		},
		UnitResources: []resource.UnitResources{
			{
				Name: coreunit.Name(tag0.Id()),
				Resources: []resource.Resource{
					res1,
					res2,
				},
			},
			{
				Name: coreunit.Name(tag1.Id()),
			},
		},
		RepositoryResources: []charmresource.Resource{
			chres1,
			chres2,
		},
	}

	result := applicationResources2APIResult(svcRes)

	apiRes1 := apiresources.Resource2API(res1)
	apiRes2 := apiresources.Resource2API(res2)

	apiChRes1 := apiresources.CharmResource2API(chres1)
	apiChRes2 := apiresources.CharmResource2API(chres2)

	c.Check(result, jc.DeepEquals, params.ResourcesResult{
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
	})
}
