// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"testing"

	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	usertesting "github.com/juju/juju/core/user/testing"
	"github.com/juju/juju/domain/offer"
	offererrors "github.com/juju/juju/domain/offer/errors"
	"github.com/juju/juju/internal/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/uuid"
)

type serviceSuite struct {
	controllerDBState *MockControllerDBState
	modelDBState      *MockModelDBState
}

func TestServiceSuite(t *testing.T) {
	tc.Run(t, &serviceSuite{})
}

// TestOfferCreate tests that Offer creates a new offer and
// access.
func (s *serviceSuite) TestOfferCreate(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange

	// Expect to call CreateOffer by receiving OfferNotFound from UpdateOffer.
	s.modelDBState.EXPECT().UpdateOffer(gomock.Any(), gomock.Any()).Return(offererrors.OfferNotFound)

	applicationName := "test-application"
	offerName := "test-offer"
	ownerName := usertesting.GenNewName(c, "admin")
	createOfferArgs := offer.ApplicationOfferArgs{
		ApplicationName: applicationName,
		OfferName:       offerName,
		Endpoints:       map[string]string{"db": "db"},
		OwnerName:       ownerName,
	}
	offerUUID := uuid.MustNewUUID()
	s.modelDBState.EXPECT().CreateOffer(gomock.Any(), createOfferArgs).Return(offerUUID, nil)

	s.controllerDBState.EXPECT().CreateOfferAccess(gomock.Any(), offerUUID, ownerName).Return(nil)

	// Act
	err := s.service(c).Offer(c.Context(), createOfferArgs)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
}

// TestOfferCreateAccessErr tests that Offer, when creating access for the
// offer fails, the offer is deleted.
func (s *serviceSuite) TestOfferCreateAccessErr(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange

	// Expect to call CreateOffer by receiving OfferNotFound from UpdateOffer.
	s.modelDBState.EXPECT().UpdateOffer(gomock.Any(), gomock.Any()).Return(offererrors.OfferNotFound)

	applicationName := "test-application"
	offerName := "test-offer"
	ownerName := usertesting.GenNewName(c, "admin")
	createOfferArgs := offer.ApplicationOfferArgs{
		ApplicationName: applicationName,
		OfferName:       offerName,
		Endpoints:       map[string]string{"db": "db"},
		OwnerName:       ownerName,
	}
	offerUUID := uuid.MustNewUUID()
	s.modelDBState.EXPECT().CreateOffer(gomock.Any(), createOfferArgs).Return(offerUUID, nil)

	// Fail creating offer access and delete the newly created offer
	s.controllerDBState.EXPECT().CreateOfferAccess(gomock.Any(), offerUUID, ownerName).Return(errors.Errorf("boom"))
	s.modelDBState.EXPECT().DeleteOffer(gomock.Any(), offerUUID).Return(nil)

	// Act
	err := s.service(c).Offer(c.Context(), createOfferArgs)

	// Assert
	c.Assert(err, tc.ErrorMatches, `failed to create access for offer "test-offer": boom`)
}

// TestOfferCreateError tests that Offer, when creating an offer fails, no
// access is created.
func (s *serviceSuite) TestOfferCreateError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange

	// Expect to call CreateOffer by receiving OfferNotFound from UpdateOffer.
	s.modelDBState.EXPECT().UpdateOffer(gomock.Any(), gomock.Any()).Return(offererrors.OfferNotFound)

	applicationName := "test-application"
	offerName := "test-offer"
	ownerName := usertesting.GenNewName(c, "admin")
	createOfferArgs := offer.ApplicationOfferArgs{
		ApplicationName: applicationName,
		OfferName:       offerName,
		Endpoints:       map[string]string{"db": "db"},
		OwnerName:       ownerName,
	}
	s.modelDBState.EXPECT().CreateOffer(gomock.Any(), createOfferArgs).Return(uuid.UUID{}, errors.Errorf("boom"))

	// Act
	err := s.service(c).Offer(c.Context(), createOfferArgs)

	// Assert
	c.Assert(err, tc.ErrorMatches, "failed to create offer: boom")
}

// TestOfferUpdate tests that Offer creates updates an existing offer.
func (s *serviceSuite) TestOfferUpdate(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	applicationName := "test-application"
	offerName := "test-offer"
	ownerName := usertesting.GenNewName(c, "admin")
	createOfferArgs := offer.ApplicationOfferArgs{
		ApplicationName: applicationName,
		OfferName:       offerName,
		Endpoints:       map[string]string{"db": "db"},
		OwnerName:       ownerName,
	}

	s.modelDBState.EXPECT().UpdateOffer(gomock.Any(), createOfferArgs).Return(nil)

	// Act
	err := s.service(c).Offer(c.Context(), createOfferArgs)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
}

func (s *serviceSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.controllerDBState = NewMockControllerDBState(ctrl)
	s.modelDBState = NewMockModelDBState(ctrl)

	c.Cleanup(func() {
		s.controllerDBState = nil
		s.modelDBState = nil
	})
	return ctrl
}

func (s *serviceSuite) service(c *tc.C) *Service {
	return &Service{
		controllerState: s.controllerDBState,
		modelState:      s.modelDBState,
		logger:          loggertesting.WrapCheckLog(c),
	}
}
