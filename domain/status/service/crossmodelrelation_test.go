// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"time"

	"github.com/juju/tc"
	gomock "go.uber.org/mock/gomock"

	coreapplicationtesting "github.com/juju/juju/core/application/testing"
	coreoffertesting "github.com/juju/juju/core/offer/testing"
	remoteapplicationtesting "github.com/juju/juju/core/remoteapplication/testing"
	corestatus "github.com/juju/juju/core/status"
	crossmodelrelationerrors "github.com/juju/juju/domain/crossmodelrelation/errors"
	"github.com/juju/juju/domain/status"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/statushistory"
)

func (s *serviceSuite) TestGetOfferStatusNoOffer(c *tc.C) {
	defer s.setupMocks(c).Finish()

	now := s.clock.Now()

	offerUUID := coreoffertesting.GenOfferUUID(c)
	s.modelState.EXPECT().GetApplicationUUIDForOffer(gomock.Any(), offerUUID.String()).Return("", crossmodelrelationerrors.OfferNotFound)

	res, err := s.modelService.GetOfferStatus(c.Context(), offerUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(res, tc.DeepEquals, corestatus.StatusInfo{
		Status:  corestatus.Terminated,
		Message: "offer has been removed",
		Since:   &now,
	})
}

func (s *serviceSuite) TestGetOfferStatus(c *tc.C) {
	defer s.setupMocks(c).Finish()

	now := s.clock.Now()

	offerUUID := coreoffertesting.GenOfferUUID(c)
	applicationUUID := coreapplicationtesting.GenApplicationUUID(c)
	s.modelState.EXPECT().GetApplicationUUIDForOffer(gomock.Any(), offerUUID.String()).Return(applicationUUID.String(), nil)
	s.modelState.EXPECT().GetApplicationStatus(gomock.Any(), applicationUUID).Return(status.StatusInfo[status.WorkloadStatusType]{
		Status:  status.WorkloadStatusActive,
		Message: "message",
		Data:    []byte(`{"foo":"bar"}`),
		Since:   &now,
	}, nil)

	res, err := s.modelService.GetOfferStatus(c.Context(), offerUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(res, tc.DeepEquals, corestatus.StatusInfo{
		Status:  corestatus.Active,
		Message: "message",
		Data:    map[string]any{"foo": "bar"},
		Since:   &now,
	})
}

func (s *serviceSuite) TestSetRemoteApplicationOffererStatus(c *tc.C) {
	defer s.setupMocks(c).Finish()

	now := time.Now().UTC()

	remoteAppUUID := remoteapplicationtesting.GenRemoteApplicationUUID(c)
	s.modelState.EXPECT().GetRemoteApplicationOffererUUIDByName(gomock.Any(), "foo").Return(remoteAppUUID, nil)
	s.modelState.EXPECT().SetRemoteApplicationOffererStatus(gomock.Any(), remoteAppUUID.String(), status.StatusInfo[status.WorkloadStatusType]{
		Status:  status.WorkloadStatusActive,
		Message: "message",
		Data:    []byte(`{"foo":"bar"}`),
		Since:   &now,
	})

	err := s.modelService.SetRemoteApplicationOffererStatus(c.Context(), "foo", corestatus.StatusInfo{
		Status:  corestatus.Active,
		Message: "message",
		Data:    map[string]any{"foo": "bar"},
		Since:   &now,
	})

	c.Assert(err, tc.ErrorIsNil)

	c.Check(s.statusHistory.records, tc.DeepEquals, []statusHistoryRecord{{
		ns: statushistory.Namespace{Kind: corestatus.KindSAAS, ID: "foo"},
		s: corestatus.StatusInfo{
			Status:  corestatus.Active,
			Message: "message",
			Data:    map[string]any{"foo": "bar"},
			Since:   &now,
		},
	}})
}

func (s *serviceSuite) TestSetRemoteApplicationOffererStatusError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	now := time.Now().UTC()

	s.modelState.EXPECT().GetRemoteApplicationOffererUUIDByName(gomock.Any(), "foo").Return("", errors.Errorf("boom"))

	err := s.modelService.SetRemoteApplicationOffererStatus(c.Context(), "foo", corestatus.StatusInfo{
		Status:  corestatus.Active,
		Message: "message",
		Data:    map[string]any{"foo": "bar"},
		Since:   &now,
	})

	c.Assert(err, tc.ErrorMatches, `.*boom`)
}
