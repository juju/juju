// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common_test

import (
	"context"

	"github.com/juju/names/v5"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	commonmocks "github.com/juju/juju/apiserver/common/mocks"
	facademocks "github.com/juju/juju/apiserver/facade/mocks"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
	"github.com/juju/juju/testing"
)

type modelMachinesWatcherSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&modelMachinesWatcherSuite{})

type fakeModelMachinesWatcher struct {
	state.ModelMachinesWatcher
	initial []string
}

func (f *fakeModelMachinesWatcher) WatchModelMachines() state.StringsWatcher {
	changes := make(chan []string, 1)
	// Simulate initial event.
	changes <- f.initial
	return &fakeStringsWatcher{changes: changes}
}

func (s *modelMachinesWatcherSuite) TestWatchModelMachines(c *gc.C) {
	authorizer := apiservertesting.FakeAuthorizer{
		Tag:        names.NewMachineTag("0"),
		Controller: true,
	}
	ctrl := gomock.NewController(c)
	watcherRegistry := facademocks.NewMockWatcherRegistry(ctrl)
	watcherRegistry.EXPECT().Register(gomock.Any()).Return("11", nil)
	machineService := commonmocks.NewMockMachineService(ctrl)
	changes := make(chan []string, 1)
	// Simulate initial event.
	changes <- []string{"foo"}
	machineService.EXPECT().WatchModelMachines(gomock.Any()).Return(
		&fakeStringsWatcher{changes: changes}, nil)
	e := common.NewModelMachinesWatcher(
		nil,
		nil,
		authorizer,
		watcherRegistry,
		machineService,
	)
	result, err := e.WatchModelMachines(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, params.StringsWatchResult{StringsWatcherId: "11", Changes: []string{"foo"}, Error: nil})
}

func (s *modelMachinesWatcherSuite) TestWatchAuthError(c *gc.C) {
	authorizer := apiservertesting.FakeAuthorizer{
		Tag:        names.NewMachineTag("1"),
		Controller: false,
	}
	ctrl := gomock.NewController(c)
	watcherRegistry := facademocks.NewMockWatcherRegistry(ctrl)
	machineService := commonmocks.NewMockMachineService(ctrl)
	resources := common.NewResources()
	s.AddCleanup(func(_ *gc.C) { resources.StopAll() })
	e := common.NewModelMachinesWatcher(
		&fakeModelMachinesWatcher{},
		resources,
		authorizer,
		watcherRegistry,
		machineService,
	)
	_, err := e.WatchModelMachines(context.Background())
	c.Assert(err, gc.ErrorMatches, "permission denied")
	c.Assert(resources.Count(), gc.Equals, 0)
}
