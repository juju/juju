// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common_test

import (
	"context"
	"errors"
	stdtesting "testing"

	"github.com/juju/clock"
	"github.com/juju/tc"
	"github.com/juju/worker/v4/workertest"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/common/mocks"
	"github.com/juju/juju/apiserver/facade"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/watcher/registry"
	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/rpc/params"
)

type MachineWatcherSuite struct {
	testing.BaseSuite
	mockWatchRebootService *mocks.MockWatchableMachineService
	watcherRegistry        facade.WatcherRegistry
}

func TestMachineWatcherSuite(t *stdtesting.T) {
	tc.Run(t, &MachineWatcherSuite{})
}

func (s *MachineWatcherSuite) setup(c *tc.C) *gomock.Controller {
	var err error
	ctrl := gomock.NewController(c)
	s.mockWatchRebootService = mocks.NewMockWatchableMachineService(ctrl)
	s.watcherRegistry, err = registry.NewRegistry(clock.WallClock)
	c.Assert(err, tc.ErrorIsNil)
	s.AddCleanup(func(c *tc.C) { workertest.DirtyKill(c, s.watcherRegistry) })
	return ctrl
}

func (s *MachineWatcherSuite) TestWatchForRebootEventCannotGetUUID(c *tc.C) {
	// Arrange
	defer s.setup(c).Finish()
	errMachineNotFound := errors.New("machine not found")
	getMachineUUID := func(ctx context.Context) (machine.UUID, error) {
		return "", errMachineNotFound
	}
	rebootWatcher := common.NewMachineRebootWatcher(s.mockWatchRebootService, s.watcherRegistry, getMachineUUID)

	// Act
	_, err := rebootWatcher.WatchForRebootEvent(c.Context())

	// Assert
	c.Assert(err, tc.ErrorIs, errMachineNotFound)
	c.Assert(s.watcherRegistry.Count(), tc.Equals, 0)
}

func (s *MachineWatcherSuite) TestWatchForRebootEventErrorStartWatcher(c *tc.C) {
	// Arrange
	defer s.setup(c).Finish()
	getMachineUUID := func(ctx context.Context) (machine.UUID, error) {
		return "machine-uuid", nil
	}
	rebootWatcher := common.NewMachineRebootWatcher(s.mockWatchRebootService, s.watcherRegistry, getMachineUUID)
	errStartWatcher := errors.New("start watcher failed")
	s.mockWatchRebootService.EXPECT().WatchMachineReboot(gomock.Any(), machine.UUID("machine-uuid")).Return(nil, errStartWatcher)

	// Act
	_, err := rebootWatcher.WatchForRebootEvent(c.Context())

	// Assert
	c.Assert(err, tc.ErrorIs, errStartWatcher)
	c.Assert(s.watcherRegistry.Count(), tc.Equals, 0)
}

func (s *MachineWatcherSuite) TestWatchForRebootEvent(c *tc.C) {
	// Arrange
	defer s.setup(c).Finish()
	getMachineUUID := func(ctx context.Context) (machine.UUID, error) {
		return "machine-uuid", nil
	}
	rebootWatcher := common.NewMachineRebootWatcher(s.mockWatchRebootService, s.watcherRegistry, getMachineUUID)
	s.mockWatchRebootService.EXPECT().WatchMachineReboot(gomock.Any(), machine.UUID("machine-uuid")).Return(apiservertesting.NewFakeNotifyWatcher(), nil)

	// Act
	result, err := rebootWatcher.WatchForRebootEvent(c.Context())

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(s.watcherRegistry.Count(), tc.Equals, 1)
	c.Assert(result, tc.Equals, params.NotifyWatchResult{
		NotifyWatcherId: registry.DefaultNamespace + "-1",
		Error:           nil,
	})
}
