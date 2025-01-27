// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resourceshookcontext

import (
	"context"
	"errors"

	jujuerrors "github.com/juju/errors"
	"github.com/juju/names/v6"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	coreapplication "github.com/juju/juju/core/application"
	coreresource "github.com/juju/juju/core/resource"
	coreunit "github.com/juju/juju/core/unit"
	charmresource "github.com/juju/juju/internal/charm/resource"
	"github.com/juju/juju/rpc/params"
)

type unitFacadeSuite struct {
	resourceService    *MockResourceService
	applicationService *MockApplicationService
}

var _ = gc.Suite(&unitFacadeSuite{})

func (s *unitFacadeSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.resourceService = NewMockResourceService(ctrl)
	s.applicationService = NewMockApplicationService(ctrl)

	return ctrl
}

// TestNewUnitFacadeApplicationTag tests the creation of a UnitFacade using an
// application tag and verifies that underlying method to get the appID
// rely on the right call to application service.
func (s *unitFacadeSuite) TestNewUnitFacadeApplicationTag(c *gc.C) {
	// Arrange
	defer s.setupMocks(c).Finish()
	tag := names.NewApplicationTag("a-application")
	s.applicationService.EXPECT().GetApplicationIDByName(gomock.Any(), tag.Id()).Return("expected-application-id", nil)

	// Act
	facade, err := NewUnitFacade(tag,
		s.applicationService,
		s.resourceService)
	c.Assert(err, jc.ErrorIsNil, gc.Commentf("(Act) unexpected error: %v", err))
	c.Assert(facade, gc.NotNil, gc.Commentf("(Act) facade is nil"))
	appID, err := facade.getApplicationID(context.Background())

	// Assert
	c.Assert(err, jc.ErrorIsNil, gc.Commentf("(Assert) unexpected error: %v", err))
	c.Check(appID, gc.Equals, coreapplication.ID("expected-application-id"),
		gc.Commentf("(Assert) application ID doesn't match: %v", appID))
}

// TestNewUnitFacadeApplicationTagError verifies error handling during
// UnitFacade creation with an application tag when application service fails.
func (s *unitFacadeSuite) TestNewUnitFacadeApplicationTagError(c *gc.C) {
	// Arrange
	defer s.setupMocks(c).Finish()
	tag := names.NewApplicationTag("a-application")
	expectedError := errors.New("expected error")
	s.applicationService.EXPECT().GetApplicationIDByName(gomock.Any(), gomock.Any()).Return("", expectedError)

	// Act
	facade, err := NewUnitFacade(tag,
		s.applicationService,
		s.resourceService)
	c.Assert(err, jc.ErrorIsNil, gc.Commentf("(Act) unexpected error: %v", err))
	c.Assert(facade, gc.NotNil, gc.Commentf("(Act) facade is nil"))
	_, err = facade.getApplicationID(context.Background())

	// Assert
	c.Assert(err, jc.ErrorIs, expectedError, gc.Commentf("(Assert) unexpected error: %v", err))
}

// TestNewUnitFacadeUnitTag verifies the creation of a UnitFacade using a unit
// tag and verifies that underlying method to get the appID  rely on the right
// call to application service.
func (s *unitFacadeSuite) TestNewUnitFacadeUnitTag(c *gc.C) {
	// Arrange
	defer s.setupMocks(c).Finish()
	tag := names.NewUnitTag("a-application/0")
	s.applicationService.EXPECT().GetApplicationIDByUnitName(gomock.Any(),
		coreunit.Name(tag.Id())).Return("expected-application-id", nil)

	// Act
	facade, err := NewUnitFacade(tag,
		s.applicationService,
		s.resourceService)
	c.Assert(err, jc.ErrorIsNil, gc.Commentf("(Act) unexpected error: %v", err))
	c.Assert(facade, gc.NotNil, gc.Commentf("(Act) facade is nil"))
	appID, err := facade.getApplicationID(context.Background())

	// Assert
	c.Assert(err, jc.ErrorIsNil, gc.Commentf("(Assert) unexpected error: %v", err))
	c.Check(appID, gc.Equals, coreapplication.ID("expected-application-id"),
		gc.Commentf("(Assert) application ID doesn't match: %v", appID))
}

// TestNewUnitFacadeUnitTagError verifies error handling during UnitFacade
// creation using a unit tag when the application service fails.
func (s *unitFacadeSuite) TestNewUnitFacadeUnitTagError(c *gc.C) {
	// Arrange
	defer s.setupMocks(c).Finish()
	tag := names.NewUnitTag("a-application/0")
	expectedError := errors.New("expected error")
	s.applicationService.EXPECT().GetApplicationIDByUnitName(gomock.Any(), gomock.Any()).Return("", expectedError)

	// Act
	facade, err := NewUnitFacade(tag,
		s.applicationService,
		s.resourceService)
	c.Assert(err, jc.ErrorIsNil, gc.Commentf("(Act) unexpected error: %v", err))
	c.Assert(facade, gc.NotNil, gc.Commentf("(Act) facade is nil"))
	_, err = facade.getApplicationID(context.Background())

	// Assert
	c.Assert(err, jc.ErrorIs, expectedError, gc.Commentf("(Assert) unexpected error: %v", err))
}

// TestNewUnitUnexpectedTag verifies that creating a UnitFacade with an invalid
// tag returns the expected error.
func (s *unitFacadeSuite) TestNewUnitUnexpectedTag(c *gc.C) {
	// Arrange
	tag := names.NewActionTag("42")

	// Act
	_, err := NewUnitFacade(tag,
		s.applicationService,
		s.resourceService)

	// Assert
	c.Assert(err, gc.ErrorMatches, "expected names.UnitTag or names.ApplicationTag.*",
		gc.Commentf("(Assert) error doesn't match or no error: %v", err))
}

// TestGetResourceInfoGetApplicationIDError verifies the behavior of
// GetResourceInfo when getApplicationID returns an error.
func (s *unitFacadeSuite) TestGetResourceInfoGetApplicationIDError(c *gc.C) {
	// Arrange
	defer s.setupMocks(c).Finish()
	expectedError := errors.New("expected error")
	facade := UnitFacade{
		getApplicationIDFromAPI: func(ctx context.Context) (coreapplication.ID, error) { return "", expectedError },
	}

	// Act
	result, err := facade.GetResourceInfo(nil, params.ListUnitResourcesArgs{ResourceNames: []string{"a-resource"}})

	// Assert
	c.Assert(err, jc.ErrorIsNil, gc.Commentf("(Assert) unexpected error: %v", err))
	c.Check(result.Error, gc.ErrorMatches, ".*expected error.*", gc.Commentf("(Assert) unexpected error result: %v",
		result.Error))
}

// TestGetApplicationIDCache verifies that the application ID is correctly retrieved and cached to avoid redundant API calls.
func (s *unitFacadeSuite) TestGetApplicationIDCache(c *gc.C) {
	// Arrange
	defer s.setupMocks(c).Finish()
	facade := UnitFacade{
		getApplicationIDFromAPI: func(ctx context.Context) (coreapplication.ID, error) { return "cached-id", nil },
	}

	// Act & Assert: first retrieval (non cached)
	id, err := facade.getApplicationID(context.Background())
	c.Assert(err, jc.ErrorIsNil, gc.Commentf("(Act) unexpected error: %v", err))
	c.Check(id, gc.Equals, coreapplication.ID("cached-id"), gc.Commentf("(Assert) unexpected application ID: %v", id))
	c.Check(facade.applicationID, gc.Equals, coreapplication.ID("cached-id"),
		gc.Commentf("(Assert)application ID should be cached: %v", id))

	// Act & Assert: first retrieval (cached)
	id, err = facade.getApplicationID(context.Background())
	c.Assert(err, jc.ErrorIsNil, gc.Commentf("(Act) unexpected error: %v", err))
	c.Check(id, gc.Equals, coreapplication.ID("cached-id"), gc.Commentf("(Assert) unexpected application ID: %v", id))

}

// TestGetResourceInfoEmpty verifies that GetResourceInfo returns an empty list
// of resources when no resources are specified.
func (s *unitFacadeSuite) TestGetResourceInfoEmpty(c *gc.C) {
	// Arrange
	tag := names.NewApplicationTag("a-application")
	facade, err := NewUnitFacade(tag,
		s.applicationService,
		s.resourceService)
	c.Assert(err, jc.ErrorIsNil, gc.Commentf("(Arrange) unexpected error: %v", err))

	// Act
	result, err := facade.GetResourceInfo(nil, params.ListUnitResourcesArgs{})

	// Assert
	c.Assert(err, jc.ErrorIsNil, gc.Commentf("(Assert) unexpected error: %v", err))
	c.Check(result, gc.DeepEquals, params.UnitResourcesResult{
		Resources: []params.UnitResourceResult{},
	}, gc.Commentf("(Assert) should be empty: %v", result))
}

// TestGetResourceInfoListResourceError tests error handling when ListResources
// fails in GetResourceInfo method.
func (s *unitFacadeSuite) TestGetResourceInfoListResourceError(c *gc.C) {
	// Arrange
	defer s.setupMocks(c).Finish()
	expectedError := errors.New("expected error")
	tag := names.NewApplicationTag("a-application")
	s.applicationService.EXPECT().GetApplicationIDByName(gomock.Any(), gomock.Any()).Return("expected-application-id", nil)
	s.resourceService.EXPECT().GetResourcesByApplicationID(gomock.Any(), gomock.Any()).Return(nil, expectedError)
	facade, err := NewUnitFacade(tag,
		s.applicationService,
		s.resourceService)
	c.Assert(err, jc.ErrorIsNil, gc.Commentf("(Arrang) unexpected error: %v", err))

	// Act
	result, err := facade.GetResourceInfo(nil, params.ListUnitResourcesArgs{ResourceNames: []string{"a-resource"}})

	// Assert
	c.Assert(err, jc.ErrorIsNil, gc.Commentf("(Assert) unexpected error: %v", err))
	c.Check(result.Error, gc.ErrorMatches, ".*expected error.*", gc.Commentf("(Assert) unexpected error result: %v",
		result.Error))
}

// TestGetResourceInfo validates the retrieval of resource information based
// on specified resource names using mock services. It verifies that:
// - Only requested resources are retrieved
// - Requesting unexisting resources return a not found error
// - All requested resources (found and not found), yield a line in the results.
func (s *unitFacadeSuite) TestGetResourceInfo(c *gc.C) {
	// helpers
	minimalResourceInfo := func(name string) coreresource.Resource {
		return coreresource.Resource{
			Resource: charmresource.Resource{
				Meta: charmresource.Meta{
					Name: name,
				},
			},
		}
	}

	// Arrange
	defer s.setupMocks(c).Finish()
	tag := names.NewApplicationTag("a-application")
	s.applicationService.EXPECT().GetApplicationIDByName(gomock.Any(), gomock.Any()).Return("expected-application-id", nil)
	s.resourceService.EXPECT().GetResourcesByApplicationID(gomock.Any(), gomock.Any()).Return([]coreresource.Resource{
		minimalResourceInfo("fetched-resource-1"),
		minimalResourceInfo("not-fetched-resource"),
		minimalResourceInfo("fetched-resource-2"),
	}, nil)
	facade, err := NewUnitFacade(tag,
		s.applicationService,
		s.resourceService)
	c.Assert(err, jc.ErrorIsNil, gc.Commentf("(Arrang) unexpected error: %v", err))

	// Act
	result, err := facade.GetResourceInfo(nil,
		params.ListUnitResourcesArgs{ResourceNames: []string{
			"not-found-resource",
			"fetched-resource-2",
			"fetched-resource-1"}})

	// Assert
	c.Assert(err, jc.ErrorIsNil, gc.Commentf("(Assert) unexpected error: %v", err))
	c.Check(result, gc.DeepEquals, params.UnitResourcesResult{
		Resources: []params.UnitResourceResult{
			{
				ErrorResult: params.ErrorResult{
					Error: apiservererrors.ServerError(jujuerrors.NotFoundf("resource %q", "not-found-resource")),
				},
			},
			{
				Resource: params.Resource{
					CharmResource: params.CharmResource{
						Name: "fetched-resource-2",
						// fingerprint is not nil in result when empty,
						// so we have to set it to empty list in the assert
						Fingerprint: []uint8{},
					},
				},
			},
			{
				Resource: params.Resource{
					CharmResource: params.CharmResource{
						Name: "fetched-resource-1",
						// fingerprint is not nil in result when empty,
						// so we have to set it to empty list in the assert
						Fingerprint: []uint8{},
					},
				},
			},
		},
	}, gc.Commentf("(Assert) unexpected resources result: %+v", result))
}
