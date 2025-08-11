// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package applicationoffers

import (
	"context"
	"testing"

	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/apiserver/authentication"
	corecrossmodel "github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/core/model"
	modeltesting "github.com/juju/juju/core/model/testing"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/core/user"
	"github.com/juju/juju/domain/access"
	"github.com/juju/juju/domain/offer"
	offererrors "github.com/juju/juju/domain/offer/errors"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/uuid"
	"github.com/juju/juju/rpc/params"
)

type offerSuite struct {
	authorizer    *MockAuthorizer
	accessService *MockAccessService
	modelService  *MockModelService
	offerService  *MockOfferService
}

func TestOfferSuite(t *testing.T) {
	tc.Run(t, &offerSuite{})
}

// TestOffer tests a successful Offer call.
func (s *offerSuite) TestOffer(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	offerAPI := s.offerAPI(c)
	modelTag := names.NewModelTag(offerAPI.modelUUID.String())
	apiUserTag := names.NewUserTag("fred")
	s.authorizer.EXPECT().GetAuthTag().Return(apiUserTag)
	s.setupCheckAPIUserAdmin(offerAPI.controllerUUID, modelTag)

	applicationName := "test-application"
	offerName := "test-offer"
	createOfferArgs := offer.ApplicationOfferArgs{
		ApplicationName: applicationName,
		OfferName:       offerName,
		Endpoints:       map[string]string{"db": "db"},
		OwnerName:       user.NameFromTag(apiUserTag),
	}
	s.offerService.EXPECT().Offer(gomock.Any(), createOfferArgs).Return(nil)

	one := params.AddApplicationOffer{
		ModelTag:        modelTag.String(),
		OfferName:       offerName,
		ApplicationName: applicationName,
		Endpoints:       map[string]string{"db": "db"},
	}
	all := params.AddApplicationOffers{Offers: []params.AddApplicationOffer{one}}

	// Act
	results, err := offerAPI.Offer(c.Context(), all)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results, tc.DeepEquals, params.ErrorResults{Results: []params.ErrorResult{{Error: nil}}})
}

// TestOfferPermission verifies an error is returned if the caller
// does not have permissions on the calling model.
func (s *offerSuite) TestOfferPermission(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	offerAPI := s.offerAPI(c)
	apiUser := names.NewUserTag("fred")
	modelTag := names.NewModelTag(offerAPI.modelUUID.String())
	s.authorizer.EXPECT().GetAuthTag().Return(apiUser)
	s.authorizer.EXPECT().HasPermission(gomock.Any(), permission.SuperuserAccess, names.NewControllerTag(offerAPI.controllerUUID)).Return(authentication.ErrorEntityMissingPermission)
	s.authorizer.EXPECT().HasPermission(gomock.Any(), permission.AdminAccess, modelTag).Return(authentication.ErrorEntityMissingPermission)

	applicationName := "test-application"
	offerName := "test-offer"
	one := params.AddApplicationOffer{
		ModelTag:        modelTag.String(),
		OfferName:       offerName,
		ApplicationName: applicationName,
		Endpoints:       map[string]string{"db": "db"},
	}
	all := params.AddApplicationOffers{Offers: []params.AddApplicationOffer{one}}

	// Act
	result, err := offerAPI.Offer(c.Context(), all)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.Results, tc.HasLen, 1)
	c.Assert(result.Results[0].Error, tc.ErrorMatches, `checking user "user-fred" has admin permission on model ".*": permission denied`)
}

// TestOfferOwnerViaArgs tests that the offer is created with a different
// owner than the caller.
func (s *offerSuite) TestOfferOwnerViaArgs(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	offerAPI := s.offerAPI(c)
	modelTag := names.NewModelTag(offerAPI.modelUUID.String())
	userTag := names.NewUserTag("admin")
	s.authorizer.EXPECT().GetAuthTag().Return(userTag)
	s.setupCheckAPIUserAdmin(offerAPI.controllerUUID, modelTag)
	offerOwnerTag := names.NewUserTag("fred")
	s.authorizer.EXPECT().EntityHasPermission(gomock.Any(), offerOwnerTag, permission.SuperuserAccess, names.NewControllerTag(offerAPI.controllerUUID)).Return(authentication.ErrorEntityMissingPermission)
	s.authorizer.EXPECT().EntityHasPermission(gomock.Any(), offerOwnerTag, permission.AdminAccess, modelTag).Return(nil)

	applicationName := "test-application"
	offerName := "test-offer"
	createOfferArgs := offer.ApplicationOfferArgs{
		ApplicationName: applicationName,
		OfferName:       offerName,
		Endpoints:       map[string]string{"db": "db"},
		OwnerName:       user.NameFromTag(offerOwnerTag),
	}
	s.offerService.EXPECT().Offer(gomock.Any(), createOfferArgs).Return(nil)

	one := params.AddApplicationOffer{
		ModelTag:        modelTag.String(),
		OfferName:       offerName,
		ApplicationName: applicationName,
		Endpoints:       map[string]string{"db": "db"},
		OwnerTag:        offerOwnerTag.String(),
	}
	all := params.AddApplicationOffers{Offers: []params.AddApplicationOffer{one}}

	// Act
	results, err := offerAPI.Offer(c.Context(), all)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results, tc.DeepEquals, params.ErrorResults{Results: []params.ErrorResult{{Error: nil}}})
}

// TestOfferOwnerViaArgs tests that the offer is created with a different
// owner than the caller.
func (s *offerSuite) TestOfferModelViaArgs(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	offerModelTag := names.NewModelTag(uuid.MustNewUUID().String())
	offerAPI := &OffersAPI{
		controllerUUID: uuid.MustNewUUID().String(),
		modelUUID:      modeltesting.GenModelUUID(c),
		authorizer:     s.authorizer,
		accessService:  s.accessService,
		modelService:   s.modelService,
		offerServiceGetter: func(_ context.Context, modelUUID model.UUID) (OfferService, error) {
			c.Check(modelUUID.String(), tc.Equals, offerModelTag.Id())
			return s.offerService, nil
		},
	}
	userTag := names.NewUserTag("fred")
	s.authorizer.EXPECT().GetAuthTag().Return(userTag)
	s.setupCheckAPIUserAdmin(offerAPI.controllerUUID, offerModelTag)

	applicationName := "test-application"
	offerName := "test-offer"
	createOfferArgs := offer.ApplicationOfferArgs{
		ApplicationName: applicationName,
		OfferName:       offerName,
		Endpoints:       map[string]string{"db": "db"},
		OwnerName:       user.NameFromTag(userTag),
	}
	s.offerService.EXPECT().Offer(gomock.Any(), createOfferArgs).Return(nil)

	one := params.AddApplicationOffer{
		ModelTag:        offerModelTag.String(),
		ApplicationName: applicationName,
		OfferName:       offerName,
		Endpoints:       map[string]string{"db": "db"},
	}
	all := params.AddApplicationOffers{Offers: []params.AddApplicationOffer{one}}

	// Act
	results, err := offerAPI.Offer(c.Context(), all)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results, tc.DeepEquals, params.ErrorResults{Results: []params.ErrorResult{{Error: nil}}})
}

// TestOfferError tests behavior when Offer fails.
func (s *offerSuite) TestOfferError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	userTag := names.NewUserTag("fred")
	s.authorizer.EXPECT().GetAuthTag().Return(userTag)
	offerAPI := s.offerAPI(c)
	modelTag := names.NewModelTag(offerAPI.modelUUID.String())
	s.setupCheckAPIUserAdmin(offerAPI.controllerUUID, modelTag)

	applicationName := "test-application"
	offerName := "test-offer"
	createOfferArgs := offer.ApplicationOfferArgs{
		ApplicationName: applicationName,
		OfferName:       offerName,
		Endpoints:       map[string]string{"db": "db"},
		OwnerName:       user.NameFromTag(userTag),
	}
	s.offerService.EXPECT().Offer(gomock.Any(), createOfferArgs).Return(errors.Errorf("boom"))

	one := params.AddApplicationOffer{
		ModelTag:        modelTag.String(),
		OfferName:       offerName,
		ApplicationName: applicationName,
		Endpoints:       map[string]string{"db": "db"},
	}
	all := params.AddApplicationOffers{Offers: []params.AddApplicationOffer{one}}

	// Act
	results, err := offerAPI.Offer(c.Context(), all)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results, tc.DeepEquals, params.ErrorResults{Results: []params.ErrorResult{
		{Error: &params.Error{Message: "boom"}},
	}})
}

// TestOfferOnlyOne tests that called Offer with more than one AddApplicationOffer
// struct fails quickly.
func (s *offerSuite) TestOfferOnlyOne(c *tc.C) {
	// Arrange
	offerAPI := s.offerAPI(c)

	// Act
	_, err := offerAPI.Offer(c.Context(), params.AddApplicationOffers{
		Offers: []params.AddApplicationOffer{
			{}, {},
		},
	})

	// Assert
	c.Assert(err, tc.ErrorMatches, "expected exactly one offer, got 2")
}

// TestModifyOfferAccess tests a basic call to ModifyOfferAccess by
// a controller admin.
func (s *offerSuite) TestModifyOfferAccess(c *tc.C) {
	s.setupMocks(c).Finish()

	// Arrange
	s.authorizer.EXPECT().HasPermission(gomock.Any(), permission.SuperuserAccess, gomock.Any()).Return(nil)

	authUserTag := names.NewUserTag("admin")
	s.authorizer.EXPECT().GetAuthTag().Return(authUserTag)
	modelInfo := model.Model{
		UUID: modeltesting.GenModelUUID(c),
	}
	qualifier := model.QualifierFromUserTag(authUserTag)
	s.modelService.EXPECT().GetModelByNameAndQualifier(gomock.Any(), "model", qualifier).Return(modelInfo, nil)

	offerURL, _ := corecrossmodel.ParseOfferURL("model.application:db")
	offerUUID := uuid.MustNewUUID()
	s.offerService.EXPECT().GetOfferUUID(gomock.Any(), offerURL).Return(offerUUID, nil)
	userTag := names.NewUserTag("simon")
	updateArgs := access.UpdatePermissionArgs{
		AccessSpec: permission.AccessSpec{
			Target: permission.ID{
				ObjectType: permission.Offer,
				Key:        offerUUID.String(),
			},
			Access: permission.ConsumeAccess,
		},
		Change:  permission.Grant,
		Subject: user.NameFromTag(userTag),
	}
	s.accessService.EXPECT().UpdatePermission(gomock.Any(), updateArgs).Return(nil)

	args := params.ModifyOfferAccessRequest{
		Changes: []params.ModifyOfferAccess{
			{
				UserTag:  userTag.String(),
				Action:   params.GrantOfferAccess,
				Access:   params.OfferConsumeAccess,
				OfferURL: offerURL.String(),
			},
		},
	}

	// Act
	results, err := s.offerAPI(c).ModifyOfferAccess(c.Context(), args)

	// Assert
	c.Assert(err, tc.IsNil)
	c.Assert(results, tc.DeepEquals, params.ErrorResults{Results: []params.ErrorResult{{Error: nil}}})
}

// TestModifyOfferAccessOfferOwner tests a basic call to ModifyOfferAccess by
// the offer owner who is not a superuser, nor model owner.
func (s *offerSuite) TestModifyOfferAccessOfferOwner(c *tc.C) {
	s.setupMocks(c).Finish()

	// Arrange:
	s.expectNotSuperuser()
	authUserTag := s.setupAuthUser("simon")

	modelUUID := s.expectGetModelByNameAndQualifier(c, authUserTag, "model")

	// Get the offer UUID.
	offerURL, _ := corecrossmodel.ParseOfferURL("model.application:db")
	offerUUID := uuid.MustNewUUID()
	s.offerService.EXPECT().GetOfferUUID(gomock.Any(), offerURL).Return(offerUUID, nil)

	// authUser does not have model admin permissions.
	s.authorizer.EXPECT().HasPermission(gomock.Any(), permission.AdminAccess, names.NewModelTag(modelUUID)).Return(authentication.ErrorEntityMissingPermission)
	// authUser has admin permissions for offer.
	s.accessService.EXPECT().ReadUserAccessLevelForTarget(gomock.Any(), user.NameFromTag(authUserTag), gomock.Any()).Return(permission.AdminAccess, nil)

	// Grant jack consumer permissions on the offer.
	userTag := names.NewUserTag("jack")
	updateArgs := access.UpdatePermissionArgs{
		AccessSpec: permission.AccessSpec{
			Target: permission.ID{
				ObjectType: permission.Offer,
				Key:        offerUUID.String(),
			},
			Access: permission.ConsumeAccess,
		},
		Change:  permission.Grant,
		Subject: user.NameFromTag(userTag),
	}
	s.accessService.EXPECT().UpdatePermission(gomock.Any(), updateArgs).Return(nil)

	args := params.ModifyOfferAccessRequest{
		Changes: []params.ModifyOfferAccess{
			{
				UserTag:  userTag.String(),
				Action:   params.GrantOfferAccess,
				Access:   params.OfferConsumeAccess,
				OfferURL: offerURL.String(),
			},
		},
	}

	// Act
	results, err := s.offerAPI(c).ModifyOfferAccess(c.Context(), args)

	// Assert
	c.Assert(err, tc.IsNil)
	c.Assert(results, tc.DeepEquals, params.ErrorResults{Results: []params.ErrorResult{{Error: nil}}})
}

// TestModifyOfferAccessModelAdmin tests a basic call to ModifyOfferAccess by
// the model admin who is not a superuser.
func (s *offerSuite) TestModifyOfferAccessModelAdmin(c *tc.C) {
	s.setupMocks(c).Finish()

	// Arrange:
	s.expectNotSuperuser()
	authUserTag := s.setupAuthUser("simon")

	modelUUID := s.expectGetModelByNameAndQualifier(c, authUserTag, "model")

	// Get the offer UUID.
	offerURL, _ := corecrossmodel.ParseOfferURL("model.application:db")
	offerUUID := uuid.MustNewUUID()
	s.offerService.EXPECT().GetOfferUUID(gomock.Any(), offerURL).Return(offerUUID, nil)

	// authUser has model admin permissions.
	s.authorizer.EXPECT().HasPermission(gomock.Any(), permission.AdminAccess, names.NewModelTag(modelUUID)).Return(nil)

	// Grant jack consumer permissions on the offer.
	userTag := names.NewUserTag("jack")
	updateArgs := access.UpdatePermissionArgs{
		AccessSpec: permission.AccessSpec{
			Target: permission.ID{
				ObjectType: permission.Offer,
				Key:        offerUUID.String(),
			},
			Access: permission.ConsumeAccess,
		},
		Change:  permission.Grant,
		Subject: user.NameFromTag(userTag),
	}
	s.accessService.EXPECT().UpdatePermission(gomock.Any(), updateArgs).Return(nil)

	args := params.ModifyOfferAccessRequest{
		Changes: []params.ModifyOfferAccess{
			{
				UserTag:  userTag.String(),
				Action:   params.GrantOfferAccess,
				Access:   params.OfferConsumeAccess,
				OfferURL: offerURL.String(),
			},
		},
	}

	// Act
	results, err := s.offerAPI(c).ModifyOfferAccess(c.Context(), args)

	// Assert
	c.Assert(err, tc.IsNil)
	c.Assert(results, tc.DeepEquals, params.ErrorResults{Results: []params.ErrorResult{{Error: nil}}})
}

// TestModifyOfferAccessPermissionDenied tests a basic call to ModifyOfferAccess by
// a user with read access to the offer.
func (s *offerSuite) TestModifyOfferAccessPermissionDenied(c *tc.C) {
	s.setupMocks(c).Finish()

	// Arrange:
	s.expectNotSuperuser()
	authUserTag := s.setupAuthUser("simon")

	modelUUID := s.expectGetModelByNameAndQualifier(c, authUserTag, "model")

	// Get the offer UUID.
	offerURL, _ := corecrossmodel.ParseOfferURL("model.application:db")
	offerUUID := uuid.MustNewUUID()
	s.offerService.EXPECT().GetOfferUUID(gomock.Any(), offerURL).Return(offerUUID, nil)

	// authUser does not have model admin permissions.
	s.expectNoModelAdminAccessPermissions(modelUUID)
	// authUser has only read permissions for offer.
	s.accessService.EXPECT().ReadUserAccessLevelForTarget(gomock.Any(), user.NameFromTag(authUserTag), gomock.Any()).Return(permission.ReadAccess, nil)

	// Grant jack consumer permissions on the offer.
	userTag := names.NewUserTag("jack")
	updateArgs := access.UpdatePermissionArgs{
		AccessSpec: permission.AccessSpec{
			Target: permission.ID{
				ObjectType: permission.Offer,
				Key:        offerUUID.String(),
			},
			Access: permission.ConsumeAccess,
		},
		Change:  permission.Grant,
		Subject: user.NameFromTag(userTag),
	}
	s.accessService.EXPECT().UpdatePermission(gomock.Any(), updateArgs).Return(nil)

	args := params.ModifyOfferAccessRequest{
		Changes: []params.ModifyOfferAccess{
			{
				UserTag:  userTag.String(),
				Action:   params.GrantOfferAccess,
				Access:   params.OfferConsumeAccess,
				OfferURL: offerURL.String(),
			},
		},
	}

	// Act
	results, err := s.offerAPI(c).ModifyOfferAccess(c.Context(), args)

	// Assert
	c.Assert(err, tc.IsNil)
	c.Assert(results, tc.DeepEquals, params.ErrorResults{Results: []params.ErrorResult{{
		Error: &params.Error{
			Message: "permission denied", Code: "unauthorized access"},
	},
	}})
}

// TestModifyOfferAccessOfferNotFound tests a basic call to ModifyOfferAccess
// where a non permission error is found.
func (s *offerSuite) TestModifyOfferAccessOfferNotFound(c *tc.C) {
	s.setupMocks(c).Finish()

	// Arrange:
	s.expectNotSuperuser()
	authUserTag := s.setupAuthUser("simon")

	modelUUID := s.expectGetModelByNameAndQualifier(c, authUserTag, "model")

	// Get the offer UUID.
	offerURL, _ := corecrossmodel.ParseOfferURL("model.application:db")
	s.offerService.EXPECT().GetOfferUUID(gomock.Any(), offerURL).Return(uuid.UUID{}, offererrors.OfferNotFound)

	s.expectNoModelAdminAccessPermissions(modelUUID)
	// authUser has only read permissions for offer.
	s.accessService.EXPECT().ReadUserAccessLevelForTarget(gomock.Any(), user.NameFromTag(authUserTag), gomock.Any()).Return(permission.ReadAccess, nil)

	userTag := names.NewUserTag("jack")
	args := params.ModifyOfferAccessRequest{
		Changes: []params.ModifyOfferAccess{
			{
				UserTag:  userTag.String(),
				Action:   params.GrantOfferAccess,
				Access:   params.OfferConsumeAccess,
				OfferURL: offerURL.String(),
			},
		},
	}

	// Act
	results, err := s.offerAPI(c).ModifyOfferAccess(c.Context(), args)

	// Assert
	c.Assert(err, tc.IsNil)
	c.Assert(results, tc.DeepEquals, params.ErrorResults{Results: []params.ErrorResult{{
		Error: &params.Error{
			Message: "offer \"model.application:db\" not found", Code: "not found"},
	},
	}})
}

func (s *offerSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.accessService = NewMockAccessService(ctrl)
	s.authorizer = NewMockAuthorizer(ctrl)
	s.modelService = NewMockModelService(ctrl)
	s.offerService = NewMockOfferService(ctrl)

	c.Cleanup(func() {
		s.accessService = nil
		s.authorizer = nil
		s.modelService = nil
		s.offerService = nil
	})
	return ctrl
}

func (s *offerSuite) setupAuthUser(name string) names.UserTag {
	authUserTag := names.NewUserTag(name)
	s.authorizer.EXPECT().GetAuthTag().Return(authUserTag)
	return authUserTag
}

func (s *offerSuite) setupCheckAPIUserAdmin(controllerUUID string, modelTag names.ModelTag) {
	s.authorizer.EXPECT().HasPermission(gomock.Any(), permission.SuperuserAccess, names.NewControllerTag(controllerUUID)).Return(authentication.ErrorEntityMissingPermission)
	s.authorizer.EXPECT().HasPermission(gomock.Any(), permission.AdminAccess, modelTag).Return(nil)
}

func (s *offerSuite) expectGetModelByNameAndQualifier(c *tc.C, authUserTag names.UserTag, modelName string) string {
	modelInfo := model.Model{
		UUID: modeltesting.GenModelUUID(c),
	}
	qualifier := model.QualifierFromUserTag(authUserTag)
	s.modelService.EXPECT().GetModelByNameAndQualifier(gomock.Any(), "model", qualifier).Return(modelInfo, nil)
	return modelInfo.UUID.String()
}

func (s *offerSuite) expectNoModelAdminAccessPermissions(modelUUID string) {
	s.authorizer.EXPECT().HasPermission(gomock.Any(), permission.AdminAccess, names.NewModelTag(modelUUID)).Return(authentication.ErrorEntityMissingPermission)
}

func (s *offerSuite) expectNotSuperuser() {
	s.authorizer.EXPECT().HasPermission(gomock.Any(), permission.SuperuserAccess, gomock.Any()).Return(authentication.ErrorEntityMissingPermission)
}

func (s *offerSuite) offerAPI(c *tc.C) *OffersAPI {
	return &OffersAPI{
		controllerUUID: uuid.MustNewUUID().String(),
		modelUUID:      modeltesting.GenModelUUID(c),
		authorizer:     s.authorizer,
		accessService:  s.accessService,
		modelService:   s.modelService,
		offerServiceGetter: func(_ context.Context, _ model.UUID) (OfferService, error) {
			return s.offerService, nil
		},
	}
}
