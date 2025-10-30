// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"sort"
	stdtesting "testing"

	"github.com/juju/tc"
	gomock "go.uber.org/mock/gomock"

	application "github.com/juju/juju/core/application"
	"github.com/juju/juju/core/offer"
	relation "github.com/juju/juju/core/relation"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/core/unit"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/rpc/params"
)

type offerStatusWatcherSuite struct {
	testhelpers.IsolationSuite

	statusService *MockStatusService
	watcher       *MockOfferWatcher

	api *srvOfferStatusWatcher
}

func TestOfferStatusWatcherSuite(t *stdtesting.T) {
	tc.Run(t, &offerStatusWatcherSuite{})
}

func (s *offerStatusWatcherSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.statusService = NewMockStatusService(ctrl)
	s.watcher = NewMockOfferWatcher(ctrl)

	s.api = &srvOfferStatusWatcher{
		statusService: s.statusService,
		watcher:       s.watcher,
	}

	return ctrl
}

func (s *offerStatusWatcherSuite) TestNext(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	offerUUID := tc.Must(c, offer.NewUUID)

	changes := make(chan struct{}, 1)
	changes <- struct{}{}
	s.watcher.EXPECT().Changes().Return(changes)
	s.watcher.EXPECT().OfferUUID().Return(offerUUID)
	s.statusService.EXPECT().GetOfferStatus(gomock.Any(), offerUUID).Return(status.StatusInfo{
		Status:  status.Active,
		Message: "message",
	}, nil)

	res, err := s.api.Next(c.Context())
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(res.Error, tc.IsNil)
	c.Assert(res.Changes, tc.HasLen, 1)
	c.Check(res.Changes[0].Status.Status, tc.Equals, status.Active)
	c.Check(res.Changes[0].Status.Info, tc.Equals, "message")
}

type remoteRelationWatcherSuite struct {
	testhelpers.IsolationSuite

	relationService *MockRelationService
	watcher         *MockRelationChangesWatcher

	api *srvRemoteRelationWatcher
}

func TestRemoteRelationWatcherSuite(t *stdtesting.T) {
	tc.Run(t, &remoteRelationWatcherSuite{})
}

func (s *remoteRelationWatcherSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.relationService = NewMockRelationService(ctrl)
	s.watcher = NewMockRelationChangesWatcher(ctrl)

	s.api = &srvRemoteRelationWatcher{
		relationService: s.relationService,
		watcher:         s.watcher,
	}

	return ctrl
}

func (s *remoteRelationWatcherSuite) TestNext(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	appUUID := tc.Must(c, application.NewUUID)
	relUUID := tc.Must(c, relation.NewUUID)

	changes := make(chan params.RelationUnitsChange, 1)
	changes <- params.RelationUnitsChange{
		Changed: map[string]params.UnitSettings{
			"foo/1": {Version: 1},
			"foo/2": {Version: 2},
		},
		AppChanged: map[string]int64{
			"foo": 1,
		},
		Departed: []string{"foo/0"},
	}
	s.watcher.EXPECT().Changes().Return(changes)
	s.watcher.EXPECT().ApplicationToken().Return(appUUID)
	s.watcher.EXPECT().RelationToken().Return(relUUID)

	inScopeUnitNames := []unit.Name{"foo/1", "foo/2", "foo/3"}
	s.relationService.EXPECT().GetInScopeUnits(gomock.Any(), appUUID, relUUID).Return(inScopeUnitNames, nil)

	unitSettings := map[unit.Name]map[string]interface{}{
		"foo/1": {"thing": 1},
		"foo/2": {"thing": 2},
	}
	s.relationService.EXPECT().GetUnitSettingsForUnits(gomock.Any(), gomock.InAnyOrder([]unit.Name{"foo/1", "foo/2"})).Return(unitSettings, nil)

	appSettings := map[string]interface{}{"foo": 1}
	s.relationService.EXPECT().GetSettingsForApplication(gomock.Any(), appUUID).Return(appSettings, nil)

	res, err := s.api.Next(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(res.Error, tc.IsNil)

	sort.Slice(res.Changes.ChangedUnits, func(i, j int) bool {
		return res.Changes.ChangedUnits[i].UnitId < res.Changes.ChangedUnits[j].UnitId
	})
	c.Check(res.Changes, tc.DeepEquals, params.RemoteRelationChangeEvent{
		RelationToken:           relUUID.String(),
		ApplicationOrOfferToken: appUUID.String(),
		DepartedUnits:           []int{0},
		InScopeUnits:            []int{1, 2, 3},
		UnitCount:               3,
		ApplicationSettings:     appSettings,
		ChangedUnits: []params.RemoteRelationUnitChange{{
			UnitId:   1,
			Settings: unitSettings["foo/1"],
		}, {
			UnitId:   2,
			Settings: unitSettings["foo/2"],
		}},
	})
}

func (s *remoteRelationWatcherSuite) TestNextNoApplicationSettingsChange(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	appUUID := tc.Must(c, application.NewUUID)
	relUUID := tc.Must(c, relation.NewUUID)

	changes := make(chan params.RelationUnitsChange, 1)
	changes <- params.RelationUnitsChange{
		Changed: map[string]params.UnitSettings{
			"foo/1": {Version: 1},
			"foo/2": {Version: 2},
		},
		Departed: []string{"foo/0"},
	}
	s.watcher.EXPECT().Changes().Return(changes)
	s.watcher.EXPECT().ApplicationToken().Return(appUUID)
	s.watcher.EXPECT().RelationToken().Return(relUUID)

	inScopeUnitNames := []unit.Name{"foo/1", "foo/2", "foo/3"}
	s.relationService.EXPECT().GetInScopeUnits(gomock.Any(), appUUID, relUUID).Return(inScopeUnitNames, nil)

	unitSettings := map[unit.Name]map[string]interface{}{
		"foo/1": {"thing": 1},
		"foo/2": {"thing": 2},
	}
	s.relationService.EXPECT().GetUnitSettingsForUnits(gomock.Any(), gomock.InAnyOrder([]unit.Name{"foo/1", "foo/2"})).Return(unitSettings, nil)

	res, err := s.api.Next(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(res.Error, tc.IsNil)

	sort.Slice(res.Changes.ChangedUnits, func(i, j int) bool {
		return res.Changes.ChangedUnits[i].UnitId < res.Changes.ChangedUnits[j].UnitId
	})
	c.Check(res.Changes, tc.DeepEquals, params.RemoteRelationChangeEvent{
		RelationToken:           relUUID.String(),
		ApplicationOrOfferToken: appUUID.String(),
		DepartedUnits:           []int{0},
		InScopeUnits:            []int{1, 2, 3},
		UnitCount:               3,
		ApplicationSettings:     nil,
		ChangedUnits: []params.RemoteRelationUnitChange{{
			UnitId:   1,
			Settings: unitSettings["foo/1"],
		}, {
			UnitId:   2,
			Settings: unitSettings["foo/2"],
		}},
	})
}
