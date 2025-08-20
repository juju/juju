// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model_test

import (
	stdtesting "testing"

	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/apiserver/common/model"
	facademocks "github.com/juju/juju/apiserver/facade/mocks"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/core/watcher/watchertest"
	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/rpc/params"
)

type modelMachinesWatcherSuite struct {
	testing.BaseSuite

	machineService  *MockMachineService
	watcherRegistry *facademocks.MockWatcherRegistry
}

func TestModelMachinesWatcherSuite(t *stdtesting.T) {
	tc.Run(t, &modelMachinesWatcherSuite{})
}

func (s *modelMachinesWatcherSuite) setUpMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.machineService = NewMockMachineService(ctrl)
	s.watcherRegistry = facademocks.NewMockWatcherRegistry(ctrl)
	return ctrl
}

func (s *modelMachinesWatcherSuite) TestWatchModelMachines(c *tc.C) {
	defer s.setUpMocks(c).Finish()

	authorizer := apiservertesting.FakeAuthorizer{
		Tag:        names.NewMachineTag("0"),
		Controller: true,
	}

	ch := make(chan []string, 1)
	w := watchertest.NewMockStringsWatcher(ch)
	ch <- []string{"foo"}
	s.machineService.EXPECT().WatchModelMachines(gomock.Any()).Return(w, nil)
	s.watcherRegistry.EXPECT().Register(gomock.Any(), gomock.Any()).Return("1", nil)

	e := model.NewModelMachinesWatcher(
		s.machineService,
		s.watcherRegistry,
		authorizer,
	)
	result, err := e.WatchModelMachines(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.StringsWatchResult{StringsWatcherId: "1", Changes: []string{"foo"}, Error: nil})
}

func (s *modelMachinesWatcherSuite) TestWatchAuthError(c *tc.C) {
	authorizer := apiservertesting.FakeAuthorizer{
		Tag:        names.NewMachineTag("1"),
		Controller: false,
	}

	e := model.NewModelMachinesWatcher(
		s.machineService,
		s.watcherRegistry,
		authorizer,
	)
	_, err := e.WatchModelMachines(c.Context())
	c.Assert(err, tc.ErrorMatches, "permission denied")
}
