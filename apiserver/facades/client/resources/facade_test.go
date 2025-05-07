// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resources

import (
	"context"

	"github.com/juju/names/v6"
	"github.com/juju/tc"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"

	apiresources "github.com/juju/juju/api/client/resources"
	"github.com/juju/juju/apiserver/internal/charms"
	coreapplication "github.com/juju/juju/core/application"
	"github.com/juju/juju/core/application/testing"
	"github.com/juju/juju/core/resource"
	resourcetesting "github.com/juju/juju/core/resource/testing"
	coreunit "github.com/juju/juju/core/unit"
	applicationcharm "github.com/juju/juju/domain/application/charm"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	domainresource "github.com/juju/juju/domain/resource"
	"github.com/juju/juju/internal/charm"
	charmresource "github.com/juju/juju/internal/charm/resource"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/rpc/params"
)

var _ = tc.Suite(&resourcesSuite{})

type resourcesSuite struct {
	BaseSuite
}

func (s *resourcesSuite) TestListResourcesOkay(c *tc.C) {
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

func (s *resourcesSuite) TestListResourcesEmpty(c *tc.C) {
	defer s.setupMocks(c).Finish()
	tag := names.NewApplicationTag("a-application")
	s.applicationService.EXPECT().GetApplicationIDByName(gomock.Any(), "a-application").Return("a-application-id", nil)
	s.resourceService.EXPECT().ListResources(gomock.Any(), coreapplication.ID("a-application-id")).Return(resource.ApplicationResources{}, nil)

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

func (s *resourcesSuite) TestListResourcesErrorGetAppID(c *tc.C) {
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

func (s *resourcesSuite) TestListResourcesError(c *tc.C) {
	defer s.setupMocks(c).Finish()
	failure := errors.New("<failure>")
	tag := names.NewApplicationTag("a-application")
	s.applicationService.EXPECT().GetApplicationIDByName(gomock.Any(), "a-application").Return("a-application-id", nil)
	s.resourceService.EXPECT().ListResources(gomock.Any(), coreapplication.ID("a-application-id")).Return(resource.ApplicationResources{}, failure)

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

func (s *resourcesSuite) TestServiceResources2API(c *tc.C) {
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

var _ = tc.Suite(&addPendingResourceSuite{})

type addPendingResourceSuite struct {
	BaseSuite

	appTag               names.ApplicationTag
	appUUID              coreapplication.ID
	curl                 *charm.URL
	charmLoc             applicationcharm.CharmLocator
	pendingResourceIDOne resource.UUID
	pendingResourceIDTwo resource.UUID
	resourceNameOne      string
	resourceNameTwo      string
}

func (s *addPendingResourceSuite) SetUpTest(c *tc.C) {
	s.appTag = names.NewApplicationTag("testapp")
	s.appUUID = testing.GenApplicationUUID(c)
	s.curl = charm.MustParseURL("testcharm")
	var err error
	s.charmLoc, err = charms.CharmLocatorFromURL(s.curl.String())
	c.Assert(err, jc.ErrorIsNil)
	s.pendingResourceIDOne = resourcetesting.GenResourceUUID(c)
	s.pendingResourceIDTwo = resourcetesting.GenResourceUUID(c)
	s.resourceNameOne = "foo"
	s.resourceNameTwo = "bar"
}

// TestAddPendingResourcesBeforeApplication test the happy path of
// AddPendingResources were the code leads to calling
// AddResourcesBeforeApplication.
func (s *addPendingResourceSuite) TestAddPendingResourcesBeforeApplication(c *tc.C) {
	defer s.setupMocks(c).Finish()

	resourceRevision := 42
	s.expectGetApplicationIDByName(applicationerrors.ApplicationNotFound)
	s.expectResolveResourceForBeforeApplication(resourceRevision)
	s.expectAddResourcesBeforeApplication(resourceRevision)

	args := params.AddPendingResourcesArgsV2{
		Entity: params.Entity{Tag: s.appTag.String()},
		URL:    s.curl.String(),
		Resources: []params.CharmResource{
			{
				Name:   s.resourceNameOne,
				Type:   charmresource.TypeFile.String(),
				Origin: charmresource.OriginUpload.String(),
				Path:   "test",
			}, {
				Name:     s.resourceNameTwo,
				Type:     charmresource.TypeContainerImage.String(),
				Origin:   charmresource.OriginStore.String(),
				Revision: resourceRevision,
				Path:     "test",
			},
		},
	}
	results, err := s.newFacade(c).AddPendingResources(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Error, tc.IsNil)
	c.Assert(results.ErrorResult.Error, tc.IsNil)
	c.Assert(results.PendingIDs, tc.DeepEquals, []string{
		s.pendingResourceIDOne.String(),
		s.pendingResourceIDTwo.String(),
	})
}

// TestAddPendingResourcesUpdateStoreResource test the happy path of
// AddPendingResources for a store resource where the code leads to
// calling UpdateResourceRevision.
func (s *addPendingResourceSuite) TestAddPendingResourcesUpdateStoreResource(c *tc.C) {
	defer s.setupMocks(c).Finish()

	resourceRevision := 42
	s.expectGetApplicationIDByName(nil)
	s.expectResolveResourcesStoreContainer(s.resourceNameTwo, resourceRevision)
	s.expectGetApplicationResourceIDTwo()
	newUUIDTwo := s.expectUpdateResourceRevisionTwo(c, resourceRevision)

	args := params.AddPendingResourcesArgsV2{
		Entity: params.Entity{Tag: s.appTag.String()},
		URL:    s.curl.String(),
		Resources: []params.CharmResource{
			{
				Name:     s.resourceNameTwo,
				Type:     charmresource.TypeContainerImage.String(),
				Origin:   charmresource.OriginStore.String(),
				Revision: resourceRevision,
				Path:     "test",
			},
		},
	}
	expectedResults := params.AddPendingResourcesResult{
		ErrorResult: params.ErrorResult{},
		PendingIDs:  []string{newUUIDTwo.String()},
	}
	results, err := s.newFacade(c).AddPendingResources(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, tc.DeepEquals, expectedResults)
}

// TestAddPendingResourcesUpdateUploadResource test the happy path of
// AddPendingResources for an upload resource where the code leads to
// calling UpdateUploadResource.
func (s *addPendingResourceSuite) TestAddPendingResourcesUpdateUploadResource(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectGetApplicationIDByName(nil)
	s.expectResolveResourcesUploadContainer(c)
	s.expectGetApplicationResourceIDTwo()
	newUUIDTwo := s.expectUpdateUploadResourceTwo(c)

	args := params.AddPendingResourcesArgsV2{
		Entity: params.Entity{Tag: s.appTag.String()},
		URL:    s.curl.String(),
		Resources: []params.CharmResource{
			{
				Name:   s.resourceNameTwo,
				Type:   charmresource.TypeContainerImage.String(),
				Origin: charmresource.OriginUpload.String(),
				Path:   "test",
			},
		},
	}
	expectedResults := params.AddPendingResourcesResult{
		ErrorResult: params.ErrorResult{},
		PendingIDs:  []string{newUUIDTwo.String()},
	}
	results, err := s.newFacade(c).AddPendingResources(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, tc.DeepEquals, expectedResults)
}

func (s *addPendingResourceSuite) expectResolveResourcesUploadContainer(c *tc.C) {
	resolveArgs := []charmresource.Resource{
		{
			Meta:   charmresource.Meta{Name: s.resourceNameTwo, Type: charmresource.TypeContainerImage, Path: "test"},
			Origin: charmresource.OriginUpload,
		},
	}
	s.repository.EXPECT().ResolveResources(gomock.Any(), resolveArgs, gomock.Any()).Return(resolveArgs, nil)
}

func (s *addPendingResourceSuite) expectResolveResourcesStoreContainer(resName string, revision int) {
	resolveArgs := []charmresource.Resource{
		{
			Meta:     charmresource.Meta{Name: resName, Type: charmresource.TypeContainerImage, Path: "test"},
			Origin:   charmresource.OriginStore,
			Revision: revision,
		},
	}
	s.repository.EXPECT().ResolveResources(gomock.Any(), resolveArgs, gomock.Any()).Return(resolveArgs, nil)
}

func (s *addPendingResourceSuite) expectGetApplicationIDByName(err error) {
	var id coreapplication.ID
	if err == nil {
		id = s.appUUID
	}
	s.applicationService.EXPECT().GetApplicationIDByName(gomock.Any(), s.appTag.Name).Return(id, err)
}

func (s *addPendingResourceSuite) expectGetApplicationResourceIDTwo() {
	getResIDArgs := domainresource.GetApplicationResourceIDArgs{
		ApplicationID: s.appUUID,
		Name:          s.resourceNameTwo,
	}
	s.resourceService.EXPECT().GetApplicationResourceID(gomock.Any(), getResIDArgs).Return(s.pendingResourceIDTwo, nil)
}

func (s *addPendingResourceSuite) expectUpdateResourceRevisionTwo(c *tc.C, resourceRevision int) resource.UUID {
	updateResourceArgs := domainresource.UpdateResourceRevisionArgs{
		ResourceUUID: s.pendingResourceIDTwo,
		Revision:     resourceRevision,
	}
	newUUID := resourcetesting.GenResourceUUID(c)
	s.resourceService.EXPECT().UpdateResourceRevision(gomock.Any(), updateResourceArgs).Return(newUUID, nil)
	return newUUID
}

func (s *addPendingResourceSuite) expectUpdateUploadResourceTwo(c *tc.C) resource.UUID {
	newUUID := resourcetesting.GenResourceUUID(c)
	s.resourceService.EXPECT().UpdateUploadResource(gomock.Any(), s.pendingResourceIDTwo).Return(newUUID, nil)
	return newUUID
}

func (s *addPendingResourceSuite) expectResolveResourceForBeforeApplication(resourceRevision int) {
	resolveArgs := []charmresource.Resource{
		{
			Meta:   charmresource.Meta{Name: s.resourceNameOne, Type: charmresource.TypeFile, Path: "test"},
			Origin: charmresource.OriginUpload,
		}, {
			Meta:     charmresource.Meta{Name: s.resourceNameTwo, Type: charmresource.TypeContainerImage, Path: "test"},
			Origin:   charmresource.OriginStore,
			Revision: resourceRevision,
		},
	}
	s.repository.EXPECT().ResolveResources(gomock.Any(), resolveArgs, gomock.Any()).Return(resolveArgs, nil)
}

func (s *addPendingResourceSuite) expectAddResourcesBeforeApplication(resourceRevision int) {
	addResourceArgs := domainresource.AddResourcesBeforeApplicationArgs{
		ApplicationName: s.appTag.Name,
		CharmLocator:    s.charmLoc,
		ResourceDetails: []domainresource.AddResourceDetails{
			{
				Name:   s.resourceNameOne,
				Origin: charmresource.OriginUpload,
			},
			{
				Name:     s.resourceNameTwo,
				Origin:   charmresource.OriginStore,
				Revision: ptr(resourceRevision),
			},
		},
	}
	addResourceRetVal := []resource.UUID{s.pendingResourceIDOne, s.pendingResourceIDTwo}
	s.resourceService.EXPECT().AddResourcesBeforeApplication(gomock.Any(), addResourceArgs).Return(addResourceRetVal, nil)
}

func ptr[T any](v T) *T {
	return &v
}
