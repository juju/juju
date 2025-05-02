// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter_test

import (
	"context"

	"github.com/juju/names/v6"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facades/agent/uniter"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/core/lxdprofile"
	internallogger "github.com/juju/juju/internal/logger"
	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/rpc/params"
)

type lxdProfileSuite struct {
	testing.BaseSuite

	machineTag1 names.MachineTag
	unitTag1    names.UnitTag
}

var _ = gc.Suite(&lxdProfileSuite{})

func (s *lxdProfileSuite) SetUpTest(c *gc.C) {
	s.machineTag1 = names.NewMachineTag("1")
	s.unitTag1 = names.NewUnitTag("mysql/1")
}

func (s *lxdProfileSuite) assertBackendAPI(c *gc.C, tag names.Tag) (*uniter.LXDProfileAPI, *gomock.Controller, *MockLXDProfileBackend) {
	resources := common.NewResources()
	authorizer := apiservertesting.FakeAuthorizer{
		Tag: tag,
	}

	ctrl := gomock.NewController(c)
	mockBackend := NewMockLXDProfileBackend(ctrl)

	unitAuthFunc := func(ctx context.Context) (common.AuthFunc, error) {
		return func(tag names.Tag) bool {
			return tag.Id() == s.unitTag1.Id()
		}, nil
	}

	api := uniter.NewLXDProfileAPI(
		mockBackend, resources, authorizer, unitAuthFunc,
		internallogger.GetLogger("juju.apiserver.facades.agent.uniter"),
	)
	return api, ctrl, mockBackend
}

func (s *lxdProfileSuite) TestWatchLXDProfileUpgradeNotifications(c *gc.C) {
	api, ctrl, mockBackend := s.assertBackendAPI(c, s.unitTag1)
	defer ctrl.Finish()

	lxdProfileWatcher := &mockStringsWatcher{
		changes: make(chan []string, 1),
	}
	lxdProfileWatcher.changes <- []string{lxdprofile.EmptyStatus}

	mockMachine1 := NewMockLXDProfileMachine(ctrl)
	mockUnit1 := NewMockLXDProfileUnit(ctrl)

	mockBackend.EXPECT().Machine(s.machineTag1.Id()).Return(mockMachine1, nil)
	mockBackend.EXPECT().Unit(s.unitTag1.Id()).Return(mockUnit1, nil)
	mockMachine1.EXPECT().WatchLXDProfileUpgradeNotifications("foo-bar").Return(lxdProfileWatcher, nil)
	mockUnit1.EXPECT().AssignedMachineId().Return(s.machineTag1.Id(), nil)

	args := params.LXDProfileUpgrade{
		Entities: []params.Entity{
			{Tag: names.NewUnitTag("mysql/2").String()},
			{Tag: s.unitTag1.String()},
			{Tag: names.NewMachineTag("2").String()},
		},
		ApplicationName: "foo-bar",
	}
	watches, err := api.WatchLXDProfileUpgradeNotifications(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(watches, gc.DeepEquals, params.StringsWatchResults{
		Results: []params.StringsWatchResult{
			{StringsWatcherId: "", Error: &params.Error{Message: "permission denied", Code: "unauthorized access"}},
			{StringsWatcherId: "1", Changes: []string{""}, Error: nil},
			{StringsWatcherId: "", Error: &params.Error{Message: "permission denied", Code: "unauthorized access"}},
		},
	})
}

func (s *lxdProfileSuite) TestWatchUnitLXDProfileUpgradeNotifications(c *gc.C) {
	api, ctrl, mockBackend := s.assertBackendAPI(c, s.unitTag1)
	defer ctrl.Finish()

	lxdProfileWatcher := &mockStringsWatcher{
		changes: make(chan []string, 1),
	}
	lxdProfileWatcher.changes <- []string{lxdprofile.EmptyStatus}

	mockUnit1 := NewMockLXDProfileUnit(ctrl)
	mockBackend.EXPECT().Unit(s.unitTag1.Id()).Return(mockUnit1, nil)
	mockUnit1.EXPECT().WatchLXDProfileUpgradeNotifications().Return(lxdProfileWatcher, nil)

	args := params.Entities{
		Entities: []params.Entity{
			{Tag: names.NewUnitTag("mysql/2").String()},
			{Tag: s.unitTag1.String()},
			{Tag: names.NewMachineTag("2").String()},
		},
	}
	watches, err := api.WatchUnitLXDProfileUpgradeNotifications(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(watches, gc.DeepEquals, params.StringsWatchResults{
		Results: []params.StringsWatchResult{
			{StringsWatcherId: "", Error: &params.Error{Message: "permission denied", Code: "unauthorized access"}},
			{StringsWatcherId: "1", Changes: []string{""}, Error: nil},
			{StringsWatcherId: "", Error: &params.Error{Message: "permission denied", Code: "unauthorized access"}},
		},
	})
}
