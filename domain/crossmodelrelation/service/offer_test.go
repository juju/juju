// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"testing"

	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	usertesting "github.com/juju/juju/core/user/testing"
	"github.com/juju/juju/domain/crossmodelrelation"
	crossmodelrelationerrors "github.com/juju/juju/domain/crossmodelrelation/errors"
	"github.com/juju/juju/domain/crossmodelrelation/internal"
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
	s.modelDBState.EXPECT().UpdateOffer(gomock.Any(), gomock.Any(), gomock.Any()).Return(crossmodelrelationerrors.OfferNotFound)

	applicationName := "test-application"
	offerName := "test-offer"
	ownerName := usertesting.GenNewName(c, "admin")
	ownerUUID := uuid.MustNewUUID()
	s.controllerDBState.EXPECT().GetUserUUIDByName(gomock.Any(), ownerName).Return(ownerUUID, nil)
	args := crossmodelrelation.ApplicationOfferArgs{
		ApplicationName: applicationName,
		OfferName:       offerName,
		Endpoints:       map[string]string{"db": "db"},
		OwnerName:       ownerName,
	}
	createOfferArgs := internal.MakeCreateOfferArgs(args, uuid.UUID{})
	m := createOfferArgsMatcher{c: c, expected: createOfferArgs}
	s.modelDBState.EXPECT().CreateOffer(gomock.Any(), m).Return(nil)

	s.controllerDBState.EXPECT().CreateOfferAccess(
		gomock.Any(),
		gomock.AssignableToTypeOf(uuid.UUID{}),
		gomock.AssignableToTypeOf(uuid.UUID{}),
		ownerUUID,
	).Return(nil)

	// Act
	err := s.service(c).Offer(c.Context(), args)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
}

// TestOfferCreateAccessErr tests that Offer, when creating access for the
// offer fails, the offer is deleted.
func (s *serviceSuite) TestOfferCreateAccessErr(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	// Expect to call CreateOffer by receiving OfferNotFound from UpdateOffer.
	s.modelDBState.EXPECT().UpdateOffer(gomock.Any(), gomock.Any(), gomock.Any()).Return(crossmodelrelationerrors.OfferNotFound)

	applicationName := "test-application"
	offerName := "test-offer"
	ownerName := usertesting.GenNewName(c, "admin")
	ownerUUID := uuid.MustNewUUID()
	s.controllerDBState.EXPECT().GetUserUUIDByName(gomock.Any(), ownerName).Return(ownerUUID, nil)
	args := crossmodelrelation.ApplicationOfferArgs{
		ApplicationName: applicationName,
		OfferName:       offerName,
		Endpoints:       map[string]string{"db": "db"},
		OwnerName:       ownerName,
	}
	createOfferArgs := internal.MakeCreateOfferArgs(args, uuid.UUID{})
	m := createOfferArgsMatcher{c: c, expected: createOfferArgs}
	s.modelDBState.EXPECT().CreateOffer(gomock.Any(), m).Return(nil)

	// Fail creating offer access and delete the newly created offer
	s.controllerDBState.EXPECT().CreateOfferAccess(
		gomock.Any(),
		gomock.AssignableToTypeOf(uuid.UUID{}),
		gomock.AssignableToTypeOf(uuid.UUID{}),
		ownerUUID,
	).Return(errors.Errorf("boom"))
	s.modelDBState.EXPECT().DeleteFailedOffer(gomock.Any(), gomock.AssignableToTypeOf(uuid.UUID{})).Return(nil)

	// Act
	err := s.service(c).Offer(c.Context(), args)

	// Assert
	c.Assert(err, tc.ErrorMatches, `creating access for offer "test-offer": boom`)
}

// TestOfferCreateError tests that Offer, when creating an offer fails, no
// access is created.
func (s *serviceSuite) TestOfferCreateError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange

	// Expect to call CreateOffer by receiving OfferNotFound from UpdateOffer.
	s.modelDBState.EXPECT().UpdateOffer(gomock.Any(), gomock.Any(), gomock.Any()).Return(crossmodelrelationerrors.OfferNotFound)

	applicationName := "test-application"
	offerName := "test-offer"
	ownerName := usertesting.GenNewName(c, "admin")
	ownerUUID := uuid.MustNewUUID()
	s.controllerDBState.EXPECT().GetUserUUIDByName(gomock.Any(), ownerName).Return(ownerUUID, nil)
	args := crossmodelrelation.ApplicationOfferArgs{
		ApplicationName: applicationName,
		OfferName:       offerName,
		Endpoints:       map[string]string{"db": "db"},
		OwnerName:       ownerName,
	}
	createOfferArgs := internal.MakeCreateOfferArgs(args, uuid.UUID{})
	m := createOfferArgsMatcher{c: c, expected: createOfferArgs}
	s.modelDBState.EXPECT().CreateOffer(gomock.Any(), m).Return(errors.Errorf("boom"))

	// Act
	err := s.service(c).Offer(c.Context(), args)

	// Assert
	c.Assert(err, tc.ErrorMatches, "create offer: boom")
}

// TestOfferUpdate tests that Offer creates updates an existing offer.
func (s *serviceSuite) TestOfferUpdate(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	applicationName := "test-application"
	offerName := "test-offer"
	ownerName := usertesting.GenNewName(c, "admin")
	createOfferArgs := crossmodelrelation.ApplicationOfferArgs{
		ApplicationName: applicationName,
		OfferName:       offerName,
		Endpoints:       map[string]string{"db": "db"},
		OwnerName:       ownerName,
	}

	s.modelDBState.EXPECT().UpdateOffer(gomock.Any(), offerName, []string{"db"}).Return(nil)

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

type createOfferArgsMatcher struct {
	c        *tc.C
	expected internal.CreateOfferArgs
}

func (m createOfferArgsMatcher) Matches(x interface{}) bool {
	obtained, ok := x.(internal.CreateOfferArgs)
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
