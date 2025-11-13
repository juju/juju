// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"testing"

	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/core/offer"
	"github.com/juju/juju/core/permission"
	relationtesting "github.com/juju/juju/core/relation/testing"
	"github.com/juju/juju/core/status"
	usertesting "github.com/juju/juju/core/user/testing"
	"github.com/juju/juju/domain/application/charm"
	"github.com/juju/juju/domain/crossmodelrelation"
	crossmodelrelationerrors "github.com/juju/juju/domain/crossmodelrelation/errors"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/uuid"
)

type offerServiceSuite struct {
	baseSuite
}

func TestOfferServiceSuite(t *testing.T) {
	tc.Run(t, &offerServiceSuite{})
}

// TestOfferCreate tests that Offer creates a new offer and
// access.
func (s *offerServiceSuite) TestOfferCreate(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	// Expect to call CreateOffer by receiving OfferNotFound from GetOfferUUID.
	applicationName := "test-application"
	offerName := "test-offer"
	s.modelState.EXPECT().GetOfferUUID(gomock.Any(), offerName).Return("", crossmodelrelationerrors.OfferNotFound)

	ownerName := usertesting.GenNewName(c, "admin")
	ownerUUID := uuid.MustNewUUID()
	s.controllerState.EXPECT().GetUserUUIDByName(gomock.Any(), ownerName).Return(ownerUUID, nil)
	args := crossmodelrelation.ApplicationOfferArgs{
		ApplicationName: applicationName,
		OfferName:       offerName,
		Endpoints:       map[string]string{"db": "db"},
		OwnerName:       ownerName,
	}
	createOfferArgs := crossmodelrelation.MakeCreateOfferArgs(args, "")
	m := createOfferArgsMatcher{c: c, expected: createOfferArgs}
	s.modelState.EXPECT().CreateOffer(gomock.Any(), m).Return(nil)

	s.controllerState.EXPECT().CreateOfferAccess(
		gomock.Any(),
		gomock.AssignableToTypeOf(uuid.UUID{}),
		gomock.AssignableToTypeOf(offer.UUID("")),
		ownerUUID,
	).Return(nil)

	// Act
	err := s.service(c).Offer(c.Context(), args)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
}

// TestOfferCreateAccessErr tests that Offer, when creating access for the
// offer fails, the offer is deleted.
func (s *offerServiceSuite) TestOfferCreateAccessErr(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	// Expect to call CreateOffer by receiving OfferNotFound from GetOfferUUID.
	applicationName := "test-application"
	offerName := "test-offer"
	s.modelState.EXPECT().GetOfferUUID(gomock.Any(), offerName).Return("", crossmodelrelationerrors.OfferNotFound)

	ownerName := usertesting.GenNewName(c, "admin")
	ownerUUID := uuid.MustNewUUID()
	s.controllerState.EXPECT().GetUserUUIDByName(gomock.Any(), ownerName).Return(ownerUUID, nil)
	args := crossmodelrelation.ApplicationOfferArgs{
		ApplicationName: applicationName,
		OfferName:       offerName,
		Endpoints:       map[string]string{"db": "db"},
		OwnerName:       ownerName,
	}
	createOfferArgs := crossmodelrelation.MakeCreateOfferArgs(args, "")
	m := createOfferArgsMatcher{c: c, expected: createOfferArgs}
	s.modelState.EXPECT().CreateOffer(gomock.Any(), m).Return(nil)

	// Fail creating offer access and delete the newly created offer
	s.controllerState.EXPECT().CreateOfferAccess(
		gomock.Any(),
		gomock.AssignableToTypeOf(uuid.UUID{}),
		gomock.AssignableToTypeOf(offer.UUID("")),
		ownerUUID,
	).Return(errors.Errorf("boom"))
	s.modelState.EXPECT().DeleteFailedOffer(gomock.Any(), gomock.AssignableToTypeOf(offer.UUID(""))).Return(nil)

	// Act
	err := s.service(c).Offer(c.Context(), args)

	// Assert
	c.Assert(err, tc.ErrorMatches, `creating access for offer "test-offer": boom`)
}

// TestOfferCreateError tests that Offer, when creating an offer fails, no
// access is created.
func (s *offerServiceSuite) TestOfferCreateError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	// Expect to call CreateOffer by receiving OfferNotFound from GetOfferUUID.
	applicationName := "test-application"
	offerName := "test-offer"
	s.modelState.EXPECT().GetOfferUUID(gomock.Any(), offerName).Return("", crossmodelrelationerrors.OfferNotFound)

	ownerName := usertesting.GenNewName(c, "admin")
	ownerUUID := uuid.MustNewUUID()
	s.controllerState.EXPECT().GetUserUUIDByName(gomock.Any(), ownerName).Return(ownerUUID, nil)
	args := crossmodelrelation.ApplicationOfferArgs{
		ApplicationName: applicationName,
		OfferName:       offerName,
		Endpoints:       map[string]string{"db": "db"},
		OwnerName:       ownerName,
	}
	createOfferArgs := crossmodelrelation.MakeCreateOfferArgs(args, "")
	m := createOfferArgsMatcher{c: c, expected: createOfferArgs}
	s.modelState.EXPECT().CreateOffer(gomock.Any(), m).Return(errors.Errorf("boom"))

	// Act
	err := s.service(c).Offer(c.Context(), args)

	// Assert
	c.Assert(err, tc.ErrorMatches, "create offer: boom")
}

// TestOfferAlreadyExists tests that Offer returns an error when an offer
// with the same name already exists.
func (s *offerServiceSuite) TestOfferAlreadyExists(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	applicationName := "test-application"
	offerName := "test-offer"
	ownerName := usertesting.GenNewName(c, "admin")
	existingOfferUUID := uuid.MustNewUUID().String()
	args := crossmodelrelation.ApplicationOfferArgs{
		ApplicationName: applicationName,
		OfferName:       offerName,
		Endpoints:       map[string]string{"db": "db"},
		OwnerName:       ownerName,
	}

	// Return an existing offer UUID to simulate the offer already exists
	s.modelState.EXPECT().GetOfferUUID(gomock.Any(), offerName).Return(existingOfferUUID, nil)

	// Act
	err := s.service(c).Offer(c.Context(), args)

	// Assert
	c.Assert(err, tc.ErrorMatches, `create offer: offer "test-offer" already exists with UUID ".*"`)
	c.Assert(err, tc.ErrorIs, crossmodelrelationerrors.OfferAlreadyExists)
}

// TestOfferGetOfferUUIDError tests that Offer returns an error when
// GetOfferUUID fails with an error other than OfferNotFound.
func (s *offerServiceSuite) TestOfferGetOfferUUIDError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	applicationName := "test-application"
	offerName := "test-offer"
	ownerName := usertesting.GenNewName(c, "admin")
	args := crossmodelrelation.ApplicationOfferArgs{
		ApplicationName: applicationName,
		OfferName:       offerName,
		Endpoints:       map[string]string{"db": "db"},
		OwnerName:       ownerName,
	}

	// Return a database error when checking if offer exists
	s.modelState.EXPECT().GetOfferUUID(gomock.Any(), offerName).Return("", errors.Errorf("database error"))

	// Act
	err := s.service(c).Offer(c.Context(), args)

	// Assert
	c.Assert(err, tc.ErrorMatches, "create offer: database error")
}

// TestOfferOwnerNotFound tests that Offer returns an error when
// the owner user doesn't exist.
func (s *offerServiceSuite) TestOfferOwnerNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	applicationName := "test-application"
	offerName := "test-offer"
	ownerName := usertesting.GenNewName(c, "nonexistent")
	args := crossmodelrelation.ApplicationOfferArgs{
		ApplicationName: applicationName,
		OfferName:       offerName,
		Endpoints:       map[string]string{"db": "db"},
		OwnerName:       ownerName,
	}

	// Offer doesn't exist yet
	s.modelState.EXPECT().GetOfferUUID(gomock.Any(), offerName).Return("", crossmodelrelationerrors.OfferNotFound)
	// Owner user doesn't exist
	s.controllerState.EXPECT().GetUserUUIDByName(gomock.Any(), ownerName).Return(uuid.UUID{}, errors.Errorf("user not found"))

	// Act
	err := s.service(c).Offer(c.Context(), args)

	// Assert
	c.Assert(err, tc.ErrorMatches, "create offer: user not found")
}

func (s *offerServiceSuite) TestGetOffersEmptyFilters(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	offerUUIDs := []string{uuid.MustNewUUID().String()}

	inputFilter := crossmodelrelation.OfferFilter{}
	offerDetails := []*crossmodelrelation.OfferDetail{
		{
			OfferUUID:              offerUUIDs[0],
			OfferName:              "test-offer",
			ApplicationName:        "test-offer",
			ApplicationDescription: "this is a test",
			Endpoints: []crossmodelrelation.OfferEndpoint{
				{
					Name:      "endpoint",
					Interface: "interface",
					Role:      charm.RoleProvider,
				},
			},
			TotalConnections:       2,
			TotalActiveConnections: 1,
		},
	}
	s.modelState.EXPECT().GetOfferDetails(gomock.Any(), inputFilter).Return(offerDetails, nil)

	offerUsers := map[string][]crossmodelrelation.OfferUser{
		offerUUIDs[0]: {
			{
				Name:   "fred",
				Access: permission.ConsumeAccess,
			},
		},
	}
	s.controllerState.EXPECT().GetUsersForOfferUUIDs(gomock.Any(), offerUUIDs).Return(offerUsers, nil)

	filters := []OfferFilter{{}}

	// Act
	result, err := s.service(c).GetOffers(c.Context(), filters)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.SameContents, []*crossmodelrelation.OfferDetail{
		{
			OfferUUID:              offerUUIDs[0],
			OfferName:              "test-offer",
			ApplicationName:        "test-offer",
			ApplicationDescription: "this is a test",
			Endpoints: []crossmodelrelation.OfferEndpoint{
				{
					Name:      "endpoint",
					Interface: "interface",
					Role:      charm.RoleProvider,
				},
			},
			OfferUsers: []crossmodelrelation.OfferUser{
				{
					Name:   "fred",
					Access: permission.ConsumeAccess,
				},
			},
			TotalConnections:       2,
			TotalActiveConnections: 1,
		},
	})
}

func (s *offerServiceSuite) TestGetOffers(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	offerUUIDs := []string{uuid.MustNewUUID().String()}

	inputFilter := crossmodelrelation.OfferFilter{ApplicationName: "test-offer"}
	offerDetails := []*crossmodelrelation.OfferDetail{
		{
			OfferUUID:              offerUUIDs[0],
			OfferName:              "test-offer",
			ApplicationName:        "test-offer",
			ApplicationDescription: "this is a test",
			Endpoints: []crossmodelrelation.OfferEndpoint{
				{
					Name:      "endpoint",
					Interface: "interface",
					Role:      charm.RoleProvider,
				},
			},
			TotalConnections:       2,
			TotalActiveConnections: 1,
		},
	}
	s.modelState.EXPECT().GetOfferDetails(gomock.Any(), inputFilter).Return(offerDetails, nil)

	offerUsers := map[string][]crossmodelrelation.OfferUser{
		offerUUIDs[0]: {
			{
				Name:   "fred",
				Access: permission.ConsumeAccess,
			},
		},
	}
	s.controllerState.EXPECT().GetUsersForOfferUUIDs(gomock.Any(), offerUUIDs).Return(offerUsers, nil)

	filters := []OfferFilter{{
		ApplicationName: inputFilter.ApplicationName,
	}}

	// Act
	result, err := s.service(c).GetOffers(c.Context(), filters)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.SameContents, []*crossmodelrelation.OfferDetail{
		{
			OfferUUID:              offerUUIDs[0],
			OfferName:              "test-offer",
			ApplicationName:        "test-offer",
			ApplicationDescription: "this is a test",
			Endpoints: []crossmodelrelation.OfferEndpoint{
				{
					Name:      "endpoint",
					Interface: "interface",
					Role:      charm.RoleProvider,
				},
			},
			OfferUsers: []crossmodelrelation.OfferUser{
				{
					Name:   "fred",
					Access: permission.ConsumeAccess,
				},
			},
			TotalConnections:       2,
			TotalActiveConnections: 1,
		},
	})
}

func (s *offerServiceSuite) TestGetOffersWithAllowedConsumers(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	allowedConsumers := []string{"fred"}
	offerUUIDs := []string{uuid.MustNewUUID().String(), uuid.MustNewUUID().String()}
	s.controllerState.EXPECT().GetOfferUUIDsForUsersWithConsume(gomock.Any(), allowedConsumers).Return(offerUUIDs, nil)

	inputFilter := crossmodelrelation.OfferFilter{
		OfferUUIDs: offerUUIDs,
	}
	offerDetails := []*crossmodelrelation.OfferDetail{
		{
			OfferUUID:              offerUUIDs[0],
			OfferName:              "test-offer",
			ApplicationName:        "test-offer",
			ApplicationDescription: "this is a test",
			Endpoints: []crossmodelrelation.OfferEndpoint{
				{
					Name:      "endpoint",
					Interface: "interface",
					Role:      charm.RoleProvider,
				},
			},
		}, {
			OfferUUID:              offerUUIDs[1],
			OfferName:              "different offer",
			ApplicationName:        "test-offer",
			ApplicationDescription: "this is a test",
			Endpoints: []crossmodelrelation.OfferEndpoint{
				{
					Name:      "second",
					Interface: "second",
					Role:      charm.RoleProvider,
				},
			},
		},
	}
	s.modelState.EXPECT().GetOfferDetails(gomock.Any(), inputFilter).Return(offerDetails, nil)

	offerUsers := map[string][]crossmodelrelation.OfferUser{
		offerUUIDs[0]: {
			{
				Name:   allowedConsumers[0],
				Access: permission.ConsumeAccess,
			},
		},
		offerUUIDs[1]: {
			{
				Name:   allowedConsumers[0],
				Access: permission.AdminAccess,
			},
		},
	}
	s.controllerState.EXPECT().GetUsersForOfferUUIDs(gomock.Any(), offerUUIDs).Return(offerUsers, nil)

	filters := []OfferFilter{
		{
			AllowedConsumers: allowedConsumers,
		},
	}

	// Act
	result, err := s.service(c).GetOffers(c.Context(), filters)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.SameContents, []*crossmodelrelation.OfferDetail{
		{
			OfferUUID:              offerUUIDs[0],
			OfferName:              "test-offer",
			ApplicationName:        "test-offer",
			ApplicationDescription: "this is a test",
			Endpoints: []crossmodelrelation.OfferEndpoint{
				{
					Name:      "endpoint",
					Interface: "interface",
					Role:      charm.RoleProvider,
				},
			},
			OfferUsers: []crossmodelrelation.OfferUser{
				{
					Name:   allowedConsumers[0],
					Access: permission.ConsumeAccess,
				},
			},
		}, {
			OfferUUID:              offerUUIDs[1],
			OfferName:              "different offer",
			ApplicationName:        "test-offer",
			ApplicationDescription: "this is a test",
			Endpoints: []crossmodelrelation.OfferEndpoint{
				{
					Name:      "second",
					Interface: "second",
					Role:      charm.RoleProvider,
				},
			},
			OfferUsers: []crossmodelrelation.OfferUser{
				{
					Name:   allowedConsumers[0],
					Access: permission.AdminAccess,
				},
			},
		},
	})
}

// TestGetOffersWithAllowedConsumersNotFound tests that if no allowed consumers
// are found, and the filter has other content. Filter is an OR.
func (s *offerServiceSuite) TestGetOffersWithAllowedConsumersNotFoundMoreInFilter(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	allowedConsumers := []string{"fred"}
	s.controllerState.EXPECT().GetOfferUUIDsForUsersWithConsume(gomock.Any(), allowedConsumers).Return(nil, nil)

	offerUUIDs := []string{uuid.MustNewUUID().String()}
	offerDetails := []*crossmodelrelation.OfferDetail{
		{
			OfferUUID:              offerUUIDs[0],
			OfferName:              "test-offer",
			ApplicationName:        "test-offer",
			ApplicationDescription: "this is a test",
			Endpoints: []crossmodelrelation.OfferEndpoint{
				{
					Name:      "endpoint",
					Interface: "interface",
					Role:      charm.RoleProvider,
				},
			},
		},
	}
	s.modelState.EXPECT().GetOfferDetails(gomock.Any(), crossmodelrelation.OfferFilter{OfferName: "test-offer"}).Return(offerDetails, nil)

	offerUsers := map[string][]crossmodelrelation.OfferUser{
		offerUUIDs[0]: {
			{
				Name:   "joe",
				Access: permission.AdminAccess,
			},
		},
	}
	s.controllerState.EXPECT().GetUsersForOfferUUIDs(gomock.Any(), offerUUIDs).Return(offerUsers, nil)

	filters := []OfferFilter{
		{
			OfferName:        "test-offer",
			AllowedConsumers: allowedConsumers,
		},
	}

	// Act
	result, err := s.service(c).GetOffers(c.Context(), filters)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.SameContents, []*crossmodelrelation.OfferDetail{
		{
			OfferUUID:              offerUUIDs[0],
			OfferName:              "test-offer",
			ApplicationName:        "test-offer",
			ApplicationDescription: "this is a test",
			Endpoints: []crossmodelrelation.OfferEndpoint{
				{
					Name:      "endpoint",
					Interface: "interface",
					Role:      charm.RoleProvider,
				},
			},
			OfferUsers: []crossmodelrelation.OfferUser{
				{
					Name:   "joe",
					Access: permission.AdminAccess,
				},
			},
		},
	})
}

// TestGetOffersWithAllowedConsumersNotFound tests that if no allowed consumers
// are found, and no other content of the same filter, nothing is returned.
func (s *offerServiceSuite) TestGetOffersWithAllowedConsumersNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	allowedConsumers := []string{"fred"}
	s.controllerState.EXPECT().GetOfferUUIDsForUsersWithConsume(gomock.Any(), allowedConsumers).Return(nil, nil)

	filters := []OfferFilter{
		{
			AllowedConsumers: allowedConsumers,
		},
	}

	// Act
	result, err := s.service(c).GetOffers(c.Context(), filters)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.SameContents, []*crossmodelrelation.OfferDetail{})
}

// TestGetOffersWithConnections focuses on the GetOfferConnections state call.
// The other functionality has been tested in TestGetOffers tests.
func (s *offerServiceSuite) TestGetOffersWithConnections(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	offerUUIDs := []string{uuid.MustNewUUID().String()}

	inputFilter := crossmodelrelation.OfferFilter{ApplicationName: "test-offer"}
	offerDetails := []*crossmodelrelation.OfferDetail{
		{
			OfferUUID:              offerUUIDs[0],
			OfferName:              "test-offer",
			ApplicationName:        "test-offer",
			ApplicationDescription: "this is a test",
			Endpoints: []crossmodelrelation.OfferEndpoint{
				{
					Name:      "endpoint",
					Interface: "interface",
					Role:      charm.RoleProvider,
				},
			},
			TotalConnections:       2,
			TotalActiveConnections: 1,
		},
	}
	s.modelState.EXPECT().GetOfferDetails(gomock.Any(), inputFilter).Return(offerDetails, nil)

	offerUsers := map[string][]crossmodelrelation.OfferUser{
		offerUUIDs[0]: {
			{
				Name:   "fred",
				Access: permission.ConsumeAccess,
			},
		},
	}
	s.controllerState.EXPECT().GetUsersForOfferUUIDs(gomock.Any(), offerUUIDs).Return(offerUsers, nil)

	offerConnections := map[string][]crossmodelrelation.OfferConnection{
		offerUUIDs[0]: {
			{
				Username:       "bob",
				RelationId:     0,
				Endpoint:       "db-admin-r",
				Status:         status.Joined,
				IngressSubnets: []string{"203.0.113.42/24"},
			},
		},
	}
	s.modelState.EXPECT().GetOfferConnections(gomock.Any(), offerUUIDs).Return(offerConnections, nil)

	filters := []OfferFilter{{
		ApplicationName: inputFilter.ApplicationName,
	}}

	// Act
	result, err := s.service(c).GetOffersWithConnections(c.Context(), filters)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.SameContents, []*crossmodelrelation.OfferDetailWithConnections{
		{
			OfferDetail: crossmodelrelation.OfferDetail{
				OfferUUID:              offerUUIDs[0],
				OfferName:              "test-offer",
				ApplicationName:        "test-offer",
				ApplicationDescription: "this is a test",
				Endpoints: []crossmodelrelation.OfferEndpoint{
					{
						Name:      "endpoint",
						Interface: "interface",
						Role:      charm.RoleProvider,
					},
				},
				OfferUsers: []crossmodelrelation.OfferUser{
					{
						Name:   "fred",
						Access: permission.ConsumeAccess,
					},
				},
				TotalConnections:       2,
				TotalActiveConnections: 1,
			},
			OfferConnections: []crossmodelrelation.OfferConnection{{
				Username:       "bob",
				RelationId:     0,
				Endpoint:       "db-admin-r",
				Status:         status.Joined,
				IngressSubnets: []string{"203.0.113.42/24"},
			}},
		},
	})
}

// TestGetOffersWithConnections focuses on the GetOfferConnections state call.
// The other functionality has been tested in TestGetOffers tests. Ensure that
// if there are no connections, the call does not fail.
func (s *offerServiceSuite) TestGetOffersWithConnectionsNoConnections(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	offerUUIDs := []string{uuid.MustNewUUID().String()}

	inputFilter := crossmodelrelation.OfferFilter{ApplicationName: "test-offer"}
	offerDetails := []*crossmodelrelation.OfferDetail{
		{
			OfferUUID:              offerUUIDs[0],
			OfferName:              "test-offer",
			ApplicationName:        "test-offer",
			ApplicationDescription: "this is a test",
			Endpoints: []crossmodelrelation.OfferEndpoint{
				{
					Name:      "endpoint",
					Interface: "interface",
					Role:      charm.RoleProvider,
				},
			},
		},
	}
	s.modelState.EXPECT().GetOfferDetails(gomock.Any(), inputFilter).Return(offerDetails, nil)

	offerUsers := map[string][]crossmodelrelation.OfferUser{
		offerUUIDs[0]: {
			{
				Name:   "fred",
				Access: permission.ConsumeAccess,
			},
		},
	}
	s.controllerState.EXPECT().GetUsersForOfferUUIDs(gomock.Any(), offerUUIDs).Return(offerUsers, nil)

	s.modelState.EXPECT().GetOfferConnections(gomock.Any(), offerUUIDs).Return(nil, nil)

	filters := []OfferFilter{{
		ApplicationName: inputFilter.ApplicationName,
	}}

	// Act
	result, err := s.service(c).GetOffersWithConnections(c.Context(), filters)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.SameContents, []*crossmodelrelation.OfferDetailWithConnections{
		{
			OfferDetail: crossmodelrelation.OfferDetail{
				OfferUUID:              offerUUIDs[0],
				OfferName:              "test-offer",
				ApplicationName:        "test-offer",
				ApplicationDescription: "this is a test",
				Endpoints: []crossmodelrelation.OfferEndpoint{
					{
						Name:      "endpoint",
						Interface: "interface",
						Role:      charm.RoleProvider,
					},
				},
				OfferUsers: []crossmodelrelation.OfferUser{
					{
						Name:   "fred",
						Access: permission.ConsumeAccess,
					},
				},
			},
		},
	})
}

func (s *offerServiceSuite) TestGetOfferUUID(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	offerURL, err := crossmodel.ParseOfferURL("postgresql.db-admin")
	c.Assert(err, tc.IsNil)
	offerUUID := tc.Must(c, offer.NewUUID)
	s.modelState.EXPECT().GetOfferUUID(gomock.Any(), offerURL.Name).Return(offerUUID.String(), nil)

	// Act
	obtainedOfferUUID, err := s.service(c).GetOfferUUID(c.Context(), offerURL)

	// Assert
	c.Assert(err, tc.IsNil)
	c.Assert(obtainedOfferUUID, tc.Equals, offerUUID)
}

func (s *offerServiceSuite) TestGetOfferUUIDError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	offerURL, err := crossmodel.ParseOfferURL("postgresql.db-admin")
	c.Assert(err, tc.IsNil)
	s.modelState.EXPECT().GetOfferUUID(gomock.Any(), offerURL.Name).Return("", crossmodelrelationerrors.OfferNotFound)

	// Act
	_, err = s.service(c).GetOfferUUID(c.Context(), offerURL)

	// Assert
	c.Assert(err, tc.ErrorIs, crossmodelrelationerrors.OfferNotFound)
}

func (s *offerServiceSuite) TestGetOfferUUIDOfferURLNotValid(c *tc.C) {
	// Act
	_, err := s.service(c).GetOfferUUID(c.Context(), crossmodel.OfferURL{})

	// Assert
	c.Assert(err, tc.ErrorIs, crossmodelrelationerrors.OfferURLNotValid)
}

func (s *offerServiceSuite) TestGetConsumeDetails(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	offerURL, err := crossmodel.ParseOfferURL("postgresql.db-admin")
	c.Assert(err, tc.IsNil)
	expected := crossmodelrelation.ConsumeDetails{
		OfferUUID: tc.Must(c, offer.NewUUID).String(),
		Endpoints: []crossmodelrelation.OfferEndpoint{{
			Name:      "test",
			Role:      charm.RoleProvider,
			Interface: "db",
			Limit:     7},
		},
	}
	s.modelState.EXPECT().GetConsumeDetails(gomock.Any(), offerURL.Name).Return(expected, nil)

	// Act
	obtained, err := s.service(c).GetConsumeDetails(c.Context(), offerURL)

	// Assert
	c.Assert(err, tc.IsNil)
	c.Assert(obtained, tc.DeepEquals, expected)
}

func (s *offerServiceSuite) TestGetConsumeDetailsError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	offerURL, err := crossmodel.ParseOfferURL("postgresql.db-admin")
	c.Assert(err, tc.IsNil)
	s.modelState.EXPECT().GetConsumeDetails(gomock.Any(), offerURL.Name).Return(crossmodelrelation.ConsumeDetails{}, crossmodelrelationerrors.OfferNotFound)

	// Act
	_, err = s.service(c).GetConsumeDetails(c.Context(), offerURL)

	// Assert
	c.Assert(err, tc.ErrorIs, crossmodelrelationerrors.OfferNotFound)
}

func (s *offerServiceSuite) TestGetConsumeDetailsOfferURLNotValid(c *tc.C) {
	// Act
	_, err := s.service(c).GetConsumeDetails(c.Context(), crossmodel.OfferURL{})

	// Assert
	c.Assert(err, tc.ErrorIs, crossmodelrelationerrors.OfferURLNotValid)
}

type createOfferArgsMatcher struct {
	c        *tc.C
	expected crossmodelrelation.CreateOfferArgs
}

func (m createOfferArgsMatcher) Matches(x interface{}) bool {
	obtained, ok := x.(crossmodelrelation.CreateOfferArgs)
	m.c.Assert(ok, tc.IsTrue)
	if !ok {
		return false
	}
	mc := tc.NewMultiChecker()
	mc.AddExpr("_.UUID", tc.Not(tc.HasLen), 0)
	m.c.Check(obtained, mc, m.expected)
	return true
}

func (m createOfferArgsMatcher) String() string {
	return "match CreateOfferArgs"
}

func (s *offerServiceSuite) TestGetOfferUUIDByRelationUUID(c *tc.C) {
	defer s.setupMocks(c).Finish()

	offerUUID := tc.Must(c, offer.NewUUID)
	relUUID := relationtesting.GenRelationUUID(c)
	s.modelState.EXPECT().GetOfferUUIDByRelationUUID(gomock.Any(), relUUID.String()).Return(offerUUID.String(), nil)

	got, err := s.service(c).GetOfferUUIDByRelationUUID(c.Context(), relUUID)
	c.Assert(err, tc.IsNil)
	c.Assert(got, tc.Equals, offerUUID)
}
