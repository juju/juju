// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"sort"
	stdtesting "testing"

	"github.com/juju/tc"
	gomock "go.uber.org/mock/gomock"

	"github.com/juju/juju/core/application"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/offer"
	"github.com/juju/juju/core/relation"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/core/unit"
	domainrelation "github.com/juju/juju/domain/relation"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/rpc/params"
)

func TestOfferStatusWatcherSuite(t *stdtesting.T) {
	tc.Run(t, &offerStatusWatcherSuite{})
}

func TestRelationStatusWatcherSuite(t *stdtesting.T) {
	tc.Run(t, &relationStatusWatcherSuite{})
}

func TestRemoteRelationWatcherSuite(t *stdtesting.T) {
	tc.Run(t, &remoteRelationWatcherSuite{})
}

type offerStatusWatcherSuite struct {
	testhelpers.IsolationSuite

	statusService *MockStatusService
	watcher       *MockOfferWatcher

	api *srvOfferStatusWatcher
}

func (s *offerStatusWatcherSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.statusService = NewMockStatusService(ctrl)
	s.watcher = NewMockOfferWatcher(ctrl)

	s.api = &srvOfferStatusWatcher{
		statusService: s.statusService,
		watcher:       s.watcher,
	}

	c.Cleanup(func() {
		s.statusService = nil
		s.watcher = nil
		s.api = nil
	})

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
	watcher         *MockNotifyWatcher

	appUUID application.UUID
	relUUID relation.UUID

	api *srvRemoteRelationWatcher
}

func (s *remoteRelationWatcherSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.relationService = NewMockRelationService(ctrl)
	s.watcher = NewMockNotifyWatcher(ctrl)

	s.appUUID = tc.Must(c, application.NewUUID)
	s.relUUID = tc.Must(c, relation.NewUUID)

	s.api = &srvRemoteRelationWatcher{
		relationService: s.relationService,
		watcher:         s.watcher,
		applicationUUID: s.appUUID,
		relationUUID:    s.relUUID,
	}

	c.Cleanup(func() {
		s.relationService = nil
		s.watcher = nil
		s.api = nil
	})

	return ctrl
}

func (s *remoteRelationWatcherSuite) TestNext(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	changes := make(chan struct{}, 1)
	changes <- struct{}{}
	s.watcher.EXPECT().Changes().Return(changes)

	s.api.data = domainrelation.ConsumerRelationUnitsChange{
		UnitsSettingsVersions: map[string]int64{
			"foo/1": 0,
			"foo/2": 1,
		},
		AppSettingsVersion: map[string]int64{
			"foo": 0,
		},
	}

	s.relationService.EXPECT().GetConsumerRelationUnitsChange(gomock.Any(), s.relUUID, s.appUUID).Return(
		domainrelation.ConsumerRelationUnitsChange{
			UnitsSettingsVersions: map[string]int64{
				"foo/1": 1,
				"foo/2": 2,
			},
			AppSettingsVersion: map[string]int64{
				"foo": 1,
			},
			DepartedUnits: []string{"foo/0"},
		}, nil)

	inScopeUnitNames := []unit.Name{"foo/1", "foo/2", "foo/3"}
	s.relationService.EXPECT().GetInScopeUnits(gomock.Any(), s.appUUID, s.relUUID).Return(inScopeUnitNames, nil)

	unitSettings := []domainrelation.UnitSettings{{
		UnitID:   1,
		Settings: map[string]string{"thing1": "thing2"},
	}, {
		UnitID:   2,
		Settings: map[string]string{"thing2": "thing1"},
	}}
	s.relationService.EXPECT().GetUnitSettingsForUnits(gomock.Any(), s.relUUID, gomock.InAnyOrder([]unit.Name{"foo/1", "foo/2"})).Return(unitSettings, nil)

	appSettings := map[string]string{"foo": "bar"}
	s.relationService.EXPECT().GetRelationApplicationSettings(gomock.Any(), s.relUUID, s.appUUID).Return(appSettings, nil)

	res, err := s.api.Next(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(res.Error, tc.IsNil)

	sort.Slice(res.Changes.ChangedUnits, func(i, j int) bool {
		return res.Changes.ChangedUnits[i].UnitId < res.Changes.ChangedUnits[j].UnitId
	})
	c.Check(res.Changes, tc.DeepEquals, params.RemoteRelationChangeEvent{
		RelationToken:           s.relUUID.String(),
		ApplicationOrOfferToken: s.appUUID.String(),
		DepartedUnits:           []int{0},
		InScopeUnits:            []int{1, 2, 3},
		UnitCount:               3,
		ApplicationSettings:     map[string]any{"foo": "bar"},
		ChangedUnits: []params.RemoteRelationUnitChange{{
			UnitId:   1,
			Settings: map[string]any{"thing1": "thing2"},
		}, {
			UnitId:   2,
			Settings: map[string]any{"thing2": "thing1"},
		}},
	})
}

func (s *remoteRelationWatcherSuite) TestNextNoApplicationSettingsChange(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	changes := make(chan struct{}, 1)
	changes <- struct{}{}
	s.watcher.EXPECT().Changes().Return(changes)

	s.api.data = domainrelation.ConsumerRelationUnitsChange{
		UnitsSettingsVersions: map[string]int64{
			"foo/1": 0,
			"foo/2": 1,
		},
		AppSettingsVersion: map[string]int64{
			"foo": 1,
		},
	}

	s.relationService.EXPECT().GetConsumerRelationUnitsChange(gomock.Any(), s.relUUID, s.appUUID).Return(
		domainrelation.ConsumerRelationUnitsChange{
			UnitsSettingsVersions: map[string]int64{
				"foo/1": 1,
				"foo/2": 2,
			},
			AppSettingsVersion: map[string]int64{
				"foo": 1,
			},
			DepartedUnits: []string{"foo/0"},
		}, nil)

	inScopeUnitNames := []unit.Name{"foo/1", "foo/2", "foo/3"}
	s.relationService.EXPECT().GetInScopeUnits(gomock.Any(), s.appUUID, s.relUUID).Return(inScopeUnitNames, nil)

	unitSettings := []domainrelation.UnitSettings{{
		UnitID:   1,
		Settings: map[string]string{"thing1": "thing2"},
	}, {
		UnitID:   2,
		Settings: map[string]string{"thing2": "thing1"},
	}}
	s.relationService.EXPECT().GetUnitSettingsForUnits(gomock.Any(), s.relUUID, gomock.InAnyOrder([]unit.Name{"foo/1", "foo/2"})).Return(unitSettings, nil)

	res, err := s.api.Next(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(res.Error, tc.IsNil)

	sort.Slice(res.Changes.ChangedUnits, func(i, j int) bool {
		return res.Changes.ChangedUnits[i].UnitId < res.Changes.ChangedUnits[j].UnitId
	})
	c.Check(res.Changes, tc.DeepEquals, params.RemoteRelationChangeEvent{
		RelationToken:           s.relUUID.String(),
		ApplicationOrOfferToken: s.appUUID.String(),
		DepartedUnits:           []int{0},
		InScopeUnits:            []int{1, 2, 3},
		UnitCount:               3,
		ApplicationSettings:     nil,
		ChangedUnits: []params.RemoteRelationUnitChange{{
			UnitId:   1,
			Settings: map[string]any{"thing1": "thing2"},
		}, {
			UnitId:   2,
			Settings: map[string]any{"thing2": "thing1"},
		}},
	})
}

func (s *remoteRelationWatcherSuite) TestNextNoChangeThenChange(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	changes := make(chan struct{}, 2)
	changes <- struct{}{}
	changes <- struct{}{}
	s.watcher.EXPECT().Changes().Return(changes).MinTimes(1)

	s.api.data = domainrelation.ConsumerRelationUnitsChange{
		UnitsSettingsVersions: map[string]int64{
			"foo/1": 0,
			"foo/2": 1,
		},
		AppSettingsVersion: map[string]int64{
			"foo": 1,
		},
	}

	s.relationService.EXPECT().GetConsumerRelationUnitsChange(gomock.Any(), s.relUUID, s.appUUID).Return(s.api.data, nil)
	s.relationService.EXPECT().GetConsumerRelationUnitsChange(gomock.Any(), s.relUUID, s.appUUID).Return(
		domainrelation.ConsumerRelationUnitsChange{
			UnitsSettingsVersions: map[string]int64{
				"foo/1": 1,
				"foo/2": 2,
			},
			AppSettingsVersion: map[string]int64{
				"foo": 1,
			},
			DepartedUnits: []string{"foo/0"},
		}, nil)

	inScopeUnitNames := []unit.Name{"foo/1", "foo/2", "foo/3"}
	s.relationService.EXPECT().GetInScopeUnits(gomock.Any(), s.appUUID, s.relUUID).Return(inScopeUnitNames, nil)

	unitSettings := []domainrelation.UnitSettings{{
		UnitID:   1,
		Settings: map[string]string{"thing1": "thing2"},
	}, {
		UnitID:   2,
		Settings: map[string]string{"thing2": "thing1"},
	}}
	s.relationService.EXPECT().GetUnitSettingsForUnits(gomock.Any(), s.relUUID, gomock.InAnyOrder([]unit.Name{"foo/1", "foo/2"})).Return(unitSettings, nil)

	res, err := s.api.Next(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(res.Error, tc.IsNil)

	sort.Slice(res.Changes.ChangedUnits, func(i, j int) bool {
		return res.Changes.ChangedUnits[i].UnitId < res.Changes.ChangedUnits[j].UnitId
	})
	c.Check(res.Changes, tc.DeepEquals, params.RemoteRelationChangeEvent{
		RelationToken:           s.relUUID.String(),
		ApplicationOrOfferToken: s.appUUID.String(),
		DepartedUnits:           []int{0},
		InScopeUnits:            []int{1, 2, 3},
		UnitCount:               3,
		ApplicationSettings:     nil,
		ChangedUnits: []params.RemoteRelationUnitChange{{
			UnitId:   1,
			Settings: map[string]any{"thing1": "thing2"},
		}, {
			UnitId:   2,
			Settings: map[string]any{"thing2": "thing1"},
		}},
	})
}

type relationStatusWatcherSuite struct {
	testhelpers.IsolationSuite

	relationService *MockRelationService
	watcher         *MockRelationStatusWatcher

	api *srvRelationStatusWatcher
}

func (s *relationStatusWatcherSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.relationService = NewMockRelationService(ctrl)
	s.watcher = NewMockRelationStatusWatcher(ctrl)

	s.api = &srvRelationStatusWatcher{
		relationService: s.relationService,
		watcher:         s.watcher,
	}

	c.Cleanup(func() {
		s.relationService = nil
		s.watcher = nil
		s.api = nil
	})

	return ctrl
}
func (s *relationStatusWatcherSuite) TestNext(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	relationUUID := tc.Must(c, relation.NewUUID)

	changes := make(chan struct{}, 1)
	changes <- struct{}{}
	s.watcher.EXPECT().Changes().Return(changes)
	s.watcher.EXPECT().RelationUUID().Return(relationUUID)
	s.relationService.EXPECT().GetRelationLifeSuspendedStatus(gomock.Any(), relationUUID).Return(
		domainrelation.RelationLifeSuspendedStatus{
			Key:             "key",
			Life:            life.Alive,
			Suspended:       true,
			SuspendedReason: "it's a test",
		}, nil)

	res, err := s.api.Next(c.Context())
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(res.Error, tc.IsNil)
	c.Assert(res.Changes, tc.HasLen, 1)
	c.Check(res.Changes[0], tc.DeepEquals, params.RelationLifeSuspendedStatusChange{
		Key:             "key",
		Life:            life.Alive,
		Suspended:       true,
		SuspendedReason: "it's a test",
	})
}
