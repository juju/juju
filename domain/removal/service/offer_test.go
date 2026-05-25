// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"testing"
	"time"

	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/core/offer"
	"github.com/juju/juju/core/relation"
	"github.com/juju/juju/domain/removal/internal"
)

type removeOfferSuite struct {
	baseSuite
}

func TestRemoveOfferSuite(t *testing.T) {
	tc.Run(t, &removeOfferSuite{})
}

func (s *removeOfferSuite) TestRemoveOfferDeletesDisconnectedOffer(c *tc.C) {
	defer s.setupMocks(c).Finish()

	offerUUID := tc.Must(c, offer.NewUUID)

	exp := s.modelState.EXPECT()
	exp.OfferExists(gomock.Any(), offerUUID.String()).Return(true, nil)
	exp.GetOfferRelationUUIDs(gomock.Any(), offerUUID.String()).Return(nil, nil)
	exp.DeleteOffer(gomock.Any(), offerUUID.String(), false).Return(nil)
	s.controllerState.EXPECT().DeleteOfferAccess(gomock.Any(), offerUUID.String()).Return(nil)

	err := s.newService(c).RemoveOffer(c.Context(), offerUUID, false)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *removeOfferSuite) TestRemoveOfferSchedulesConnectedRelationsAndHidesOffer(c *tc.C) {
	defer s.setupMocks(c).Finish()

	offerUUID := tc.Must(c, offer.NewUUID)
	relUUID0 := tc.Must(c, relation.NewUUID)
	relUUID1 := tc.Must(c, relation.NewUUID)
	when := time.Now()

	s.clock.EXPECT().Now().Return(when).Times(2)

	exp := s.modelState.EXPECT()
	gomock.InOrder(
		exp.OfferExists(gomock.Any(), offerUUID.String()).Return(true, nil),
		exp.GetOfferRelationUUIDs(gomock.Any(), offerUUID.String()).Return([]string{relUUID0.String(), relUUID1.String()}, nil),
		exp.RelationWithRemoteConsumerExists(gomock.Any(), relUUID0.String()).Return(true, nil),
		exp.EnsureRelationWithRemoteConsumerNotAliveCascade(gomock.Any(), relUUID0.String()).Return(internal.CascadedRelationWithRemoteConsumerLives{}, nil),
		exp.RelationWithRemoteConsumerScheduleRemoval(gomock.Any(), gomock.Any(), relUUID0.String(), false, when.UTC()).Return(nil),
		exp.RelationWithRemoteConsumerExists(gomock.Any(), relUUID1.String()).Return(true, nil),
		exp.EnsureRelationWithRemoteConsumerNotAliveCascade(gomock.Any(), relUUID1.String()).Return(internal.CascadedRelationWithRemoteConsumerLives{}, nil),
		exp.RelationWithRemoteConsumerScheduleRemoval(gomock.Any(), gomock.Any(), relUUID1.String(), false, when.UTC()).Return(nil),
		exp.HideOffer(gomock.Any(), offerUUID.String()).Return(nil),
	)
	s.controllerState.EXPECT().DeleteOfferAccess(gomock.Any(), offerUUID.String()).Return(nil)

	err := s.newService(c).RemoveOffer(c.Context(), offerUUID, false)
	c.Assert(err, tc.ErrorIsNil)
}
