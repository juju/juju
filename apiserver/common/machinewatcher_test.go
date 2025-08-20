// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common_test

import (
	"context"
	"errors"
	stdtesting "testing"

	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/common/mocks"
	facademocks "github.com/juju/juju/apiserver/facade/mocks"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/core/machine"
	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/rpc/params"
)

type MachineWatcherSuite struct {
	testing.BaseSuite
	machineService  *mocks.MockWatchableMachineService
	watcherRegistry *facademocks.MockWatcherRegistry
}

func TestMachineWatcherSuite(t *stdtesting.T) {
	tc.Run(t, &MachineWatcherSuite{})
}

func (s *MachineWatcherSuite) TestWatchForRebootEventCannotGetUUID(c *tc.C) {
	// Arrange
	defer s.setup(c).Finish()
	errMachineNotFound := errors.New("machine not found")
	getMachineUUID := func(ctx context.Context) (machine.UUID, error) {
		return "", errMachineNotFound
	}
	rebootWatcher := common.NewMachineRebootWatcher(s.machineService, s.watcherRegistry, getMachineUUID)

	// Act
	_, err := rebootWatcher.WatchForRebootEvent(c.Context())

	// Assert
	c.Assert(err, tc.ErrorIs, errMachineNotFound)
}

func (s *MachineWatcherSuite) TestWatchForRebootEventErrorStartWatcher(c *tc.C) {
	// Arrange
	defer s.setup(c).Finish()
	getMachineUUID := func(ctx context.Context) (machine.UUID, error) {
		return "machine-uuid", nil
	}
	rebootWatcher := common.NewMachineRebootWatcher(s.machineService, s.watcherRegistry, getMachineUUID)
	errStartWatcher := errors.New("start watcher failed")
	s.machineService.EXPECT().WatchMachineReboot(gomock.Any(), machine.UUID("machine-uuid")).Return(nil, errStartWatcher)

	// Act
	_, err := rebootWatcher.WatchForRebootEvent(c.Context())

	// Assert
	c.Assert(err, tc.ErrorIs, errStartWatcher)
}

func (s *MachineWatcherSuite) TestWatchForRebootEvent(c *tc.C) {
	// Arrange
	defer s.setup(c).Finish()
	getMachineUUID := func(ctx context.Context) (machine.UUID, error) {
		return "machine-uuid", nil
	}
	rebootWatcher := common.NewMachineRebootWatcher(s.machineService, s.watcherRegistry, getMachineUUID)
	s.machineService.EXPECT().WatchMachineReboot(gomock.Any(), machine.UUID("machine-uuid")).Return(apiservertesting.NewFakeNotifyWatcher(), nil)
	s.watcherRegistry.EXPECT().Register(gomock.Any(), gomock.Any()).Return("1", nil)

	// Act
	result, err := rebootWatcher.WatchForRebootEvent(c.Context())

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.Equals, params.NotifyWatchResult{
		NotifyWatcherId: "1",
		Error:           nil,
	})
}

func (s *MachineWatcherSuite) setup(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.machineService = mocks.NewMockWatchableMachineService(ctrl)
	s.watcherRegistry = facademocks.NewMockWatcherRegistry(ctrl)

	return ctrl
}
