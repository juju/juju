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
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/core/user"
	"github.com/juju/juju/domain/access"
	accesserrors "github.com/juju/juju/domain/access/errors"
	"github.com/juju/juju/domain/application/architecture"
	"github.com/juju/juju/domain/application/charm"
	modelerrors "github.com/juju/juju/domain/model/errors"
	"github.com/juju/juju/domain/offer"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/uuid"
	"github.com/juju/juju/rpc/params"
)

type offerSuite struct {
	authorizer     *MockAuthorizer
	accessService  *MockAccessService
	modelService   *MockModelService
	offerService   *MockOfferService
	removalService *MockRemovalService
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
		modelUUID:      model.GenUUID(c),
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
		UUID: model.GenUUID(c),
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
	// authUser does not have admin permission on the offer.
	s.authorizer.EXPECT().EntityHasPermission(
		gomock.Any(),
		authUserTag,
		permission.AdminAccess,
		names.NewApplicationOfferTag(offerUUID.String()),
	).Return(nil)

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
	// authUser does not have admin permission on the offer.
	s.authorizer.EXPECT().EntityHasPermission(
		gomock.Any(),
		authUserTag,
		permission.AdminAccess,
		names.NewApplicationOfferTag(offerUUID.String()),
	).Return(authentication.ErrorEntityMissingPermission)

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

func (s *offerSuite) TestDestroyOffers(c *tc.C) {
	s.testDestroyOffers(c, false)
}

func (s *offerSuite) TestDestroyOffersForce(c *tc.C) {
	s.testDestroyOffers(c, true)
}

func (s *offerSuite) testDestroyOffers(c *tc.C, force bool) {
	s.setupMocks(c).Finish()

	// Arrange
	offerAPI := s.offerAPI(c)

	offerURL, _ := corecrossmodel.ParseOfferURL("fred@external/prod.hosted-mysql")
	modelUUID := s.expectGetModelByNameAndQualifier(c, names.NewUserTag("fred@external"), offerURL.ModelName)
	s.setupAuthUser("simon")
	s.setupCheckAPIUserAdmin(offerAPI.controllerUUID, names.NewModelTag(modelUUID))
	offerUUID := uuid.MustNewUUID()
	s.offerService.EXPECT().GetOfferUUID(gomock.Any(), offerURL).Return(offerUUID, nil)
	s.removalService.EXPECT().RemoveOffer(gomock.Any(), offerUUID, force).Return(nil)

	args := params.DestroyApplicationOffers{
		Force:     force,
		OfferURLs: []string{offerURL.String()},
	}

	// Act
	results, err := offerAPI.DestroyOffers(c.Context(), args)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results.Results, tc.HasLen, 1)
	c.Assert(results.Results[0].Error, tc.IsNil)
}

func (s *offerSuite) TestDestroyOffersPermission(c *tc.C) {
	s.setupMocks(c).Finish()

	// Arrange
	offerAPI := s.offerAPI(c)
	offerURL, _ := corecrossmodel.ParseOfferURL("fred@external/prod.hosted-mysql")
	modelUUID := s.expectGetModelByNameAndQualifier(c, names.NewUserTag("fred@external"), offerURL.ModelName)
	s.setupAuthUser("simon")
	s.expectNotSuperuser()
	s.expectNoModelAdminAccessPermissions(modelUUID)

	args := params.DestroyApplicationOffers{
		OfferURLs: []string{offerURL.String()},
	}

	// Act
	results, err := offerAPI.DestroyOffers(c.Context(), args)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results.Results, tc.HasLen, 1)
	c.Assert(results.Results[0].Error, tc.ErrorMatches, `permission denied`)
}

func (s *offerSuite) TestDestroyOffersModelErrors(c *tc.C) {
	s.setupMocks(c).Finish()

	// Arrange
	authUserTag := s.setupAuthUser("simon")
	s.expectNotSuperuser()
	offerAPI := s.offerAPI(c)

	s.modelService.EXPECT().GetModelByNameAndQualifier(
		gomock.Any(),
		"badmodel",
		model.QualifierFromUserTag(authUserTag),
	).Return(model.Model{}, modelerrors.NotFound)
	s.modelService.EXPECT().GetModelByNameAndQualifier(
		gomock.Any(),
		"badmodel",
		model.QualifierFromUserTag(names.NewUserTag("garbage")),
	).Return(model.Model{}, accesserrors.UserNameNotValid)

	args := params.DestroyApplicationOffers{
		OfferURLs: []string{
			"garbage/badmodel.someoffer", "badmodel.someoffer",
		},
	}

	// Act
	results, err := offerAPI.DestroyOffers(c.Context(), args)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results.Results, tc.HasLen, 2)
	c.Assert(results.Results, tc.DeepEquals, []params.ErrorResult{
		{
			Error: &params.Error{Message: `user name "garbage": not valid`, Code: "not valid"},
		}, {
			Error: &params.Error{Message: `model "simon/badmodel": not found`, Code: "not found"},
		},
	})
}

func (s *offerSuite) TestListApplicationOffers(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	offerAPI := s.offerAPI(c)
	adminTag := s.setupAuthUser("admin")
	s.authorizer.EXPECT().EntityHasPermission(gomock.Any(), adminTag, permission.SuperuserAccess, names.NewControllerTag(offerAPI.controllerUUID)).Return(nil)
	adminUser := user.User{DisplayName: "fred smith"}
	s.accessService.EXPECT().GetUserByName(gomock.Any(), user.NameFromTag(adminTag)).Return(adminUser, nil)

	modelName := "prod"
	modelOwnerTag := names.NewUserTag("fred@external")

	foundModel := model.Model{
		Name:      modelName,
		Qualifier: model.QualifierFromUserTag(modelOwnerTag),
		UUID:      model.GenUUID(c),
	}
	s.modelService.EXPECT().GetModelByNameAndQualifier(gomock.Any(), modelName, foundModel.Qualifier).Return(foundModel, nil)

	domainFilters := []offer.OfferFilter{
		{
			OfferName:        "hosted-db2",
			Endpoints:        make([]offer.EndpointFilterTerm, 0),
			AllowedConsumers: make([]string, 0),
			ConnectedUsers:   make([]string, 0),
		}, {
			OfferName:        "testing",
			Endpoints:        make([]offer.EndpointFilterTerm, 0),
			AllowedConsumers: make([]string, 0),
			ConnectedUsers:   make([]string, 0),
		},
	}
	charmLocator := charm.CharmLocator{
		Name:         "app",
		Revision:     42,
		Source:       charm.CharmHubSource,
		Architecture: architecture.AMD64,
	}
	offerDetails := []*offer.OfferDetails{
		{
			OfferUUID:              uuid.MustNewUUID().String(),
			OfferName:              domainFilters[0].OfferName,
			ApplicationName:        "test-app",
			ApplicationDescription: "testing application",
			CharmLocator:           charmLocator,
			Endpoints: []offer.OfferEndpoint{
				{Name: "db"},
			},
		}, {
			OfferUUID:              uuid.MustNewUUID().String(),
			OfferName:              domainFilters[1].OfferName,
			ApplicationName:        "test-app",
			ApplicationDescription: "testing application",
			CharmLocator:           charmLocator,
			Endpoints: []offer.OfferEndpoint{
				{Name: "endpoint"},
			},
		},
	}
	s.offerService.EXPECT().GetOffers(gomock.Any(), domainFilters).Return(offerDetails, nil)

	filters := params.OfferFilters{
		Filters: []params.OfferFilter{
			{
				ModelQualifier: modelOwnerTag.Id(),
				ModelName:      modelName,
				OfferName:      "hosted-db2",
			}, {
				ModelQualifier: modelOwnerTag.Id(),
				ModelName:      modelName,
				OfferName:      "testing",
			},
		},
	}

	// Act
	obtained, err := offerAPI.ListApplicationOffers(c.Context(), filters)

	// Assert
	c.Assert(err, tc.IsNil)
	c.Assert(obtained.Results, tc.HasLen, 2)
	mc := tc.NewMultiChecker()
	mc.AddExpr("_.ApplicationOfferDetailsV5.SourceModelTag", tc.Ignore)
	mc.AddExpr("_.ApplicationOfferDetailsV5.OfferUUID", tc.Ignore)
	c.Assert(obtained.Results[0], mc, params.ApplicationOfferAdminDetailsV5{
		ApplicationOfferDetailsV5: params.ApplicationOfferDetailsV5{
			OfferURL:               "fred-external/prod.hosted-db2",
			OfferName:              "hosted-db2",
			ApplicationDescription: "testing application",
			Endpoints:              []params.RemoteEndpoint{{Name: "db"}},
			Users: []params.OfferUserDetails{
				{UserName: "admin", DisplayName: "fred smith", Access: "admin"},
			}},
		ApplicationName: "test-app",
		CharmURL:        "ch:amd64/app-42",
	})
	c.Assert(obtained.Results[1], mc, params.ApplicationOfferAdminDetailsV5{
		ApplicationOfferDetailsV5: params.ApplicationOfferDetailsV5{
			OfferURL:               "fred-external/prod.testing",
			OfferName:              "testing",
			ApplicationDescription: "testing application",
			Endpoints:              []params.RemoteEndpoint{{Name: "endpoint"}},
			Users: []params.OfferUserDetails{
				{UserName: "admin", DisplayName: "fred smith", Access: "admin"},
			}},
		ApplicationName: "test-app",
		CharmURL:        "ch:amd64/app-42",
	})
}

func (s *offerSuite) TestListApplicationOffersError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	offerAPI := s.offerAPI(c)
	adminTag := s.setupAuthUser("admin")
	s.authorizer.EXPECT().EntityHasPermission(gomock.Any(), adminTag, permission.SuperuserAccess, names.NewControllerTag(offerAPI.controllerUUID)).Return(nil)
	adminUser := user.User{DisplayName: "fred smith"}
	s.accessService.EXPECT().GetUserByName(gomock.Any(), user.NameFromTag(adminTag)).Return(adminUser, nil)

	modelName := "prod"
	modelOwnerTag := names.NewUserTag("fred@external")

	foundModel := model.Model{
		Name:      modelName,
		Qualifier: model.QualifierFromUserTag(modelOwnerTag),
		UUID:      model.GenUUID(c),
	}
	s.modelService.EXPECT().GetModelByNameAndQualifier(gomock.Any(), modelName, foundModel.Qualifier).Return(foundModel, nil)

	domainFilters := []offer.OfferFilter{
		{
			OfferName:        "hosted-db2",
			Endpoints:        make([]offer.EndpointFilterTerm, 0),
			AllowedConsumers: make([]string, 0),
			ConnectedUsers:   make([]string, 0),
		}, {
			OfferName:        "testing",
			Endpoints:        make([]offer.EndpointFilterTerm, 0),
			AllowedConsumers: make([]string, 0),
			ConnectedUsers:   make([]string, 0),
		},
	}
	s.offerService.EXPECT().GetOffers(gomock.Any(), domainFilters).Return(nil, errors.New("some error"))

	filters := params.OfferFilters{
		Filters: []params.OfferFilter{
			{
				ModelQualifier: modelOwnerTag.Id(),
				ModelName:      modelName,
				OfferName:      "hosted-db2",
			}, {
				ModelQualifier: modelOwnerTag.Id(),
				ModelName:      modelName,
				OfferName:      "testing",
			},
		},
	}

	// Act
	_, err := offerAPI.ListApplicationOffers(c.Context(), filters)

	// Assert
	c.Assert(err, tc.ErrorMatches, "some error")

}

func (s *offerSuite) TestListApplicationOffersPermission(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	offerAPI := s.offerAPI(c)
	adminTag := s.setupAuthUser("admin")
	s.authorizer.EXPECT().EntityHasPermission(gomock.Any(), adminTag, permission.SuperuserAccess, names.NewControllerTag(offerAPI.controllerUUID)).Return(authentication.ErrorEntityMissingPermission)
	adminUser := user.User{DisplayName: "fred smith"}
	s.accessService.EXPECT().GetUserByName(gomock.Any(), user.NameFromTag(adminTag)).Return(adminUser, nil)

	modelName := "prod"
	foundModel := model.Model{
		Name: modelName,
		UUID: model.GenUUID(c),
	}
	s.modelService.EXPECT().GetModelByNameAndQualifier(gomock.Any(), modelName, model.QualifierFromUserTag(adminTag)).Return(foundModel, nil)
	s.authorizer.EXPECT().EntityHasPermission(gomock.Any(), adminTag, permission.AdminAccess, names.NewModelTag(foundModel.UUID.String())).Return(authentication.ErrorEntityMissingPermission)

	filters := params.OfferFilters{
		Filters: []params.OfferFilter{
			{
				ModelName: modelName,
				OfferName: "hosted-db2",
			}, {
				ModelName: modelName,
				OfferName: "testing",
			},
		},
	}

	// Act
	_, err := offerAPI.ListApplicationOffers(c.Context(), filters)

	// Assert
	c.Assert(err, tc.DeepEquals, &params.Error{
		Message: "permission denied", Code: "unauthorized access"},
	)
}

func (s *offerSuite) TestListFilterRequiresModel(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	s.setupAuthUser("admin")
	filter := params.OfferFilters{
		Filters: []params.OfferFilter{
			{
				OfferName:       "hosted-db2",
				ApplicationName: "test",
			},
		},
	}

	// Act
	_, err := s.offerAPI(c).ListApplicationOffers(c.Context(), filter)

	// Assert
	c.Assert(err, tc.ErrorMatches, "application offer filter must specify a model name")
}

func (s *offerSuite) TestListRequiresFilter(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	s.setupAuthUser("admin")

	// Act
	_, err := s.offerAPI(c).ListApplicationOffers(c.Context(), params.OfferFilters{})

	// Assert
	c.Assert(err, tc.ErrorMatches, "at least one offer filter is required")
}

func (s *offerSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.accessService = NewMockAccessService(ctrl)
	s.authorizer = NewMockAuthorizer(ctrl)
	s.modelService = NewMockModelService(ctrl)
	s.offerService = NewMockOfferService(ctrl)
	s.removalService = NewMockRemovalService(ctrl)

	c.Cleanup(func() {
		s.accessService = nil
		s.authorizer = nil
		s.modelService = nil
		s.offerService = nil
		s.removalService = nil
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
		UUID: model.GenUUID(c),
	}
	qualifier := model.QualifierFromUserTag(authUserTag)
	s.modelService.EXPECT().GetModelByNameAndQualifier(gomock.Any(), modelName, qualifier).Return(modelInfo, nil)
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
		modelUUID:      model.GenUUID(c),
		authorizer:     s.authorizer,
		accessService:  s.accessService,
		modelService:   s.modelService,
		offerServiceGetter: func(_ context.Context, _ model.UUID) (OfferService, error) {
			return s.offerService, nil
		},
		removalServiceGetter: func(_ context.Context, _ model.UUID) (RemovalService, error) {
			return s.removalService, nil
		},
	}
}
