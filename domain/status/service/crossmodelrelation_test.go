// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"time"

	"github.com/juju/tc"
	gomock "go.uber.org/mock/gomock"

	"github.com/juju/juju/core/application"
	"github.com/juju/juju/core/crossmodel"
	corelife "github.com/juju/juju/core/life"
	"github.com/juju/juju/core/offer"
	corerelation "github.com/juju/juju/core/relation"
	remoteapplicationtesting "github.com/juju/juju/core/remoteapplication/testing"
	corestatus "github.com/juju/juju/core/status"
	crossmodelrelationerrors "github.com/juju/juju/domain/crossmodelrelation/errors"
	"github.com/juju/juju/domain/life"
	"github.com/juju/juju/domain/status"
	internalcharm "github.com/juju/juju/internal/charm"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/statushistory"
)

func (s *serviceSuite) TestGetOfferStatusNoOffer(c *tc.C) {
	defer s.setupMocks(c).Finish()

	now := s.clock.Now().UTC()

	offerUUID := tc.Must(c, offer.NewUUID)
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

	now := s.clock.Now().UTC()

	offerUUID := tc.Must(c, offer.NewUUID)
	applicationUUID := tc.Must(c, application.NewUUID)
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

func (s *serviceSuite) TestGetRemoteApplicationOffererStatuses(c *tc.C) {
	defer s.setupMocks(c).Finish()

	now := s.clock.Now().UTC()

	relUUID1 := tc.Must(c, corerelation.NewUUID)
	relUUID2 := tc.Must(c, corerelation.NewUUID)
	relUUID3 := tc.Must(c, corerelation.NewUUID)

	s.modelState.EXPECT().GetRemoteApplicationOffererStatuses(gomock.Any()).Return(map[string]status.RemoteApplicationOfferer{
		"foo": {
			Status: status.StatusInfo[status.WorkloadStatusType]{
				Status:  status.WorkloadStatusActive,
				Message: "it's active",
				Data:    []byte(`{"foo":"bar"}`),
				Since:   &now,
			},
			Life:     life.Alive,
			OfferURL: "controller:qualifier/model.offername",
			Endpoints: []status.Endpoint{
				{Name: "endpoint-1", Role: "provider", Interface: "interface-1", Limit: 10},
				{Name: "endpoint-2", Role: "requirer", Interface: "interface-2", Limit: 20},
			},
			Relations: []string{relUUID1.String(), relUUID2.String()},
		},
		"bar": {
			Status: status.StatusInfo[status.WorkloadStatusType]{
				Status:  status.WorkloadStatusError,
				Message: "it's error",
				Data:    []byte(`{"bar":"foo"}`),
				Since:   &now,
			},
			Life:     life.Dying,
			OfferURL: "controller:qualifier/model.offername",
			Endpoints: []status.Endpoint{
				{Name: "endpoint-3", Role: "peer", Interface: "interface-3", Limit: 30},
				{Name: "endpoint-4", Role: "peer", Interface: "interface-4", Limit: 40},
			},
			Relations: []string{relUUID3.String()},
		},
	}, nil)

	res, err := s.modelService.GetRemoteApplicationOffererStatuses(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(res, tc.DeepEquals, map[string]RemoteApplicationOfferer{
		"foo": {
			Status: corestatus.StatusInfo{
				Status:  corestatus.Active,
				Message: "it's active",
				Data:    map[string]interface{}{"foo": "bar"},
				Since:   &now,
			},
			Life:     corelife.Alive,
			OfferURL: tc.Must1(c, crossmodel.ParseOfferURL, "controller:qualifier/model.offername"),
			Endpoints: []Endpoint{
				{Name: "endpoint-1", Role: internalcharm.RoleProvider, Interface: "interface-1", Limit: 10},
				{Name: "endpoint-2", Role: internalcharm.RoleRequirer, Interface: "interface-2", Limit: 20},
			},
			Relations: []corerelation.UUID{relUUID1, relUUID2},
		},
		"bar": {
			Status: corestatus.StatusInfo{
				Status:  corestatus.Error,
				Message: "it's error",
				Data:    map[string]interface{}{"bar": "foo"},
				Since:   &now,
			},
			Life:     corelife.Dying,
			OfferURL: tc.Must1(c, crossmodel.ParseOfferURL, "controller:qualifier/model.offername"),
			Endpoints: []Endpoint{
				{Name: "endpoint-3", Role: internalcharm.RolePeer, Interface: "interface-3", Limit: 30},
				{Name: "endpoint-4", Role: internalcharm.RolePeer, Interface: "interface-4", Limit: 40},
			},
			Relations: []corerelation.UUID{relUUID3},
		},
	})
}

func (s *serviceSuite) TestGetRemoteApplicationOffererStatusesError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.modelState.EXPECT().GetRemoteApplicationOffererStatuses(gomock.Any()).Return(nil, errors.Errorf("boom"))

	_, err := s.modelService.GetRemoteApplicationOffererStatuses(c.Context())
	c.Assert(err, tc.ErrorMatches, ".*boom")
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
