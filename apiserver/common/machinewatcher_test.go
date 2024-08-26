package common_test

import (
	"context"
	"errors"

	"github.com/juju/clock"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v4/workertest"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/common/mocks"
	"github.com/juju/juju/apiserver/facade"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/core/watcher/registry"
	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/rpc/params"
)

type MachineWatcherSuite struct {
	testing.BaseSuite
	mockWatchRebootService *mocks.MockWatchableMachineService
	watcherRegistry        facade.WatcherRegistry
}

var _ = gc.Suite(&MachineWatcherSuite{})

func (s *MachineWatcherSuite) setup(c *gc.C) *gomock.Controller {
	var err error
	ctrl := gomock.NewController(c)
	s.mockWatchRebootService = mocks.NewMockWatchableMachineService(ctrl)
	s.watcherRegistry, err = registry.NewRegistry(clock.WallClock)
	c.Assert(err, jc.ErrorIsNil)
	s.AddCleanup(func(c *gc.C) { workertest.DirtyKill(c, s.watcherRegistry) })
	return ctrl
}

func (s *MachineWatcherSuite) TestWatchForRebootEventCannotGetUUID(c *gc.C) {
	// Arrange
	defer s.setup(c).Finish()
	errMachineNotFound := errors.New("machine not found")
	getMachineUUID := func(ctx context.Context) (string, error) {
		return "", errMachineNotFound
	}
	rebootWatcher := common.NewMachineRebootWatcher(s.mockWatchRebootService, s.watcherRegistry, getMachineUUID)

	// Act
	_, err := rebootWatcher.WatchForRebootEvent(context.Background())

	// Assert
	c.Assert(err, jc.ErrorIs, errMachineNotFound)
	c.Assert(s.watcherRegistry.Count(), gc.Equals, 0)
}

func (s *MachineWatcherSuite) TestWatchForRebootEventErrorStartWatcher(c *gc.C) {
	// Arrange
	defer s.setup(c).Finish()
	getMachineUUID := func(ctx context.Context) (string, error) {
		return "machine-uuid", nil
	}
	rebootWatcher := common.NewMachineRebootWatcher(s.mockWatchRebootService, s.watcherRegistry, getMachineUUID)
	errStartWatcher := errors.New("start watcher failed")
	s.mockWatchRebootService.EXPECT().WatchMachineReboot(gomock.Any(), "machine-uuid").Return(nil, errStartWatcher)

	// Act
	_, err := rebootWatcher.WatchForRebootEvent(context.Background())

	// Assert
	c.Assert(err, jc.ErrorIs, errStartWatcher)
	c.Assert(s.watcherRegistry.Count(), gc.Equals, 0)
}

func (s *MachineWatcherSuite) TestWatchForRebootEvent(c *gc.C) {
	// Arrange
	defer s.setup(c).Finish()
	getMachineUUID := func(ctx context.Context) (string, error) {
		return "machine-uuid", nil
	}
	rebootWatcher := common.NewMachineRebootWatcher(s.mockWatchRebootService, s.watcherRegistry, getMachineUUID)
	s.mockWatchRebootService.EXPECT().WatchMachineReboot(gomock.Any(), "machine-uuid").Return(apiservertesting.NewFakeNotifyWatcher(), nil)

	// Act
	result, err := rebootWatcher.WatchForRebootEvent(context.Background())

	// Assert
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.watcherRegistry.Count(), gc.Equals, 1)
	c.Assert(result, gc.Equals, params.NotifyWatchResult{
		NotifyWatcherId: registry.DefaultNamespace + "-1",
		Error:           nil,
	})
}
