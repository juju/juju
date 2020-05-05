// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common_test

import (
	"github.com/juju/loggo"
	"github.com/juju/names/v4"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/tomb.v2"

	"github.com/golang/mock/gomock"
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/common/mocks"
	"github.com/juju/juju/apiserver/params"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/testing"
)

type upgradeSeriesSuite struct {
	testing.BaseSuite

	machineTag1 names.MachineTag
	unitTag1    names.UnitTag
	unitTag2    names.UnitTag
}

var _ = gc.Suite(&upgradeSeriesSuite{})

func (s *upgradeSeriesSuite) SetUpTest(c *gc.C) {
	s.machineTag1 = names.NewMachineTag("1")
	s.unitTag1 = names.NewUnitTag("mysql/1")
	s.unitTag2 = names.NewUnitTag("redis/1")
}

func (s *upgradeSeriesSuite) assertBackendApi(c *gc.C, tag names.Tag) (*common.UpgradeSeriesAPI, *gomock.Controller, *mocks.MockUpgradeSeriesBackend) {
	resources := common.NewResources()
	authorizer := apiservertesting.FakeAuthorizer{
		Tag: tag,
	}

	ctrl := gomock.NewController(c)
	mockBackend := mocks.NewMockUpgradeSeriesBackend(ctrl)

	unitAuthFunc := func() (common.AuthFunc, error) {
		return func(tag names.Tag) bool {
			if tag.Id() == s.unitTag1.Id() {
				return true
			}
			return false
		}, nil
	}

	machineAuthFunc := func() (common.AuthFunc, error) {
		return func(tag names.Tag) bool {
			if tag.Id() == s.machineTag1.Id() {
				return true
			}
			return false
		}, nil
	}

	api := common.NewUpgradeSeriesAPI(
		mockBackend, resources, authorizer, machineAuthFunc, unitAuthFunc, loggo.GetLogger("juju.apiserver.common"))
	return api, ctrl, mockBackend
}

func (s *upgradeSeriesSuite) TestWatchUpgradeSeriesNotificationsUnitTag(c *gc.C) {
	api, ctrl, mockBackend := s.assertBackendApi(c, s.unitTag1)
	defer ctrl.Finish()

	upgradeSeriesWatcher := &mockNotifyWatcher{
		changes: make(chan struct{}, 1),
	}
	upgradeSeriesWatcher.changes <- struct{}{}

	mockMachine1 := mocks.NewMockUpgradeSeriesMachine(ctrl)
	mockUnit1 := mocks.NewMockUpgradeSeriesUnit(ctrl)

	mockBackend.EXPECT().Machine(s.machineTag1.Id()).Return(mockMachine1, nil)
	mockBackend.EXPECT().Unit(s.unitTag1.Id()).Return(mockUnit1, nil)
	mockMachine1.EXPECT().WatchUpgradeSeriesNotifications().Return(upgradeSeriesWatcher, nil)
	mockUnit1.EXPECT().AssignedMachineId().Return(s.machineTag1.Id(), nil)

	args := params.Entities{Entities: []params.Entity{
		{Tag: names.NewUnitTag("mysql/2").String()},
		{Tag: s.unitTag1.String()},
	}}
	watches, err := api.WatchUpgradeSeriesNotifications(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(watches, gc.DeepEquals, params.NotifyWatchResults{
		Results: []params.NotifyWatchResult{
			{NotifyWatcherId: "", Error: &params.Error{Message: "permission denied", Code: "unauthorized access"}},
			{NotifyWatcherId: "1", Error: nil},
		},
	})
}

func (s *upgradeSeriesSuite) TestWatchUpgradeSeriesNotificationsMachineTag(c *gc.C) {
	api, ctrl, mockBackend := s.assertBackendApi(c, s.machineTag1)
	defer ctrl.Finish()

	mockMachine := mocks.NewMockUpgradeSeriesMachine(ctrl)

	upgradeSeriesWatcher := &mockNotifyWatcher{
		changes: make(chan struct{}, 1),
	}
	upgradeSeriesWatcher.changes <- struct{}{}

	mockBackend.EXPECT().Machine(s.machineTag1.Id()).Return(mockMachine, nil)
	mockMachine.EXPECT().WatchUpgradeSeriesNotifications().Return(upgradeSeriesWatcher, nil)

	watches, err := api.WatchUpgradeSeriesNotifications(
		params.Entities{
			Entities: []params.Entity{
				{Tag: s.machineTag1.String()},
				{Tag: names.NewMachineTag("7").String()},
			},
		},
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(watches, gc.DeepEquals, params.NotifyWatchResults{
		Results: []params.NotifyWatchResult{
			{NotifyWatcherId: "1"},
			{NotifyWatcherId: "", Error: &params.Error{Message: "permission denied", Code: "unauthorized access"}},
		},
	})
}

func (s *upgradeSeriesSuite) TestSetUpgradeSeriesStatusUnitTag(c *gc.C) {
	api, ctrl, mockBackend := s.assertBackendApi(c, s.unitTag1)
	defer ctrl.Finish()

	mockUnit := mocks.NewMockUpgradeSeriesUnit(ctrl)

	mockBackend.EXPECT().Unit(s.unitTag1.Id()).Return(mockUnit, nil)
	mockUnit.EXPECT().SetUpgradeSeriesStatus(model.UpgradeSeriesPrepareCompleted, gomock.Any()).Return(nil)

	args := params.UpgradeSeriesStatusParams{
		Params: []params.UpgradeSeriesStatusParam{
			{
				Entity: params.Entity{Tag: s.unitTag1.String()},
				Status: model.UpgradeSeriesPrepareCompleted,
			},
			{
				Entity: params.Entity{Tag: names.NewUnitTag("mysql/2").String()},
				Status: model.UpgradeSeriesPrepareCompleted,
			},
		},
	}
	watches, err := api.SetUpgradeSeriesUnitStatus(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(watches, gc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{Error: nil},
			{Error: &params.Error{Message: "permission denied", Code: "unauthorized access"}},
		},
	})
}

func (s *upgradeSeriesSuite) TestUpgradeSeriesStatusUnitTag(c *gc.C) {
	api, ctrl, mockBackend := s.assertBackendApi(c, s.unitTag1)
	defer ctrl.Finish()

	mockUnit := mocks.NewMockUpgradeSeriesUnit(ctrl)

	mockBackend.EXPECT().Unit(s.unitTag1.Id()).Return(mockUnit, nil)
	mockUnit.EXPECT().UpgradeSeriesStatus().Return(model.UpgradeSeriesPrepareCompleted, nil)

	args := params.Entities{
		Entities: []params.Entity{
			{Tag: s.unitTag1.String()},
			{Tag: names.NewUnitTag("mysql/2").String()},
		},
	}

	results, err := api.UpgradeSeriesUnitStatus(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.DeepEquals, params.UpgradeSeriesStatusResults{
		Results: []params.UpgradeSeriesStatusResult{
			{Status: model.UpgradeSeriesPrepareCompleted},
			{Error: &params.Error{Message: "permission denied", Code: "unauthorized access"}},
		},
	})
}

type mockNotifyWatcher struct {
	tomb    tomb.Tomb
	changes chan struct{}
}

func (m *mockNotifyWatcher) Stop() error {
	m.Kill()
	return m.Wait()
}

func (m *mockNotifyWatcher) Kill() {
	m.tomb.Kill(nil)
}

func (m *mockNotifyWatcher) Wait() error {
	return m.tomb.Wait()
}

func (m *mockNotifyWatcher) Err() error {
	return m.tomb.Err()
}

func (m *mockNotifyWatcher) Changes() <-chan struct{} {
	return m.changes
}
