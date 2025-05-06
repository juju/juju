// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiaddresssetter

import (
	"context"
	"time"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v4/workertest"
	gomock "go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	controller "github.com/juju/juju/controller"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/unit"
	watcher "github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/watchertest"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

type workerSuite struct {
	testing.IsolationSuite

	applicationService      *MockApplicationService
	controllerNodeService   *MockControllerNodeService
	networkService          *MockNetworkService
	controllerConfigService *MockControllerConfigService
}

var _ = gc.Suite(&workerSuite{})

func (s *workerSuite) setUpMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.applicationService = NewMockApplicationService(ctrl)
	s.controllerConfigService = NewMockControllerConfigService(ctrl)
	s.controllerNodeService = NewMockControllerNodeService(ctrl)
	s.networkService = NewMockNetworkService(ctrl)

	return ctrl
}

func (s *workerSuite) TestWorkerCleanKill(c *gc.C) {
	defer s.setUpMocks(c).Finish()

	nodeWatcher := watchertest.NewMockNotifyWatcher(make(chan struct{}))
	s.controllerNodeService.EXPECT().WatchControllerNodes().Return(nodeWatcher, nil)
	// We use the consume of the initial event as a sync point to decide
	// whether the worker has started. This channel is then used to stop
	// waiting for the worker to start.
	notifyInitialConfigConsumed := make(chan struct{})
	// Send an initial change to the (mocked) controller config watcher.
	s.controllerConfigService.EXPECT().WatchControllerConfig().DoAndReturn(func() (watcher.Watcher[[]string], error) {
		ch := make(chan []string)
		go func() {
			select {
			case ch <- []string{}:
				notifyInitialConfigConsumed <- struct{}{}
			case <-time.After(testing.ShortWait):
				c.Fatalf("timed out sending initial change")
			}
		}()
		return watchertest.NewMockStringsWatcher(ch), nil
	})

	cfg := Config{
		ControllerConfigService: s.controllerConfigService,
		ApplicationService:      s.applicationService,
		ControllerNodeService:   s.controllerNodeService,
		NetworkService:          s.networkService,
		APIPort:                 17070,
		ControllerAPIPort:       17070,
		Logger:                  loggertesting.WrapCheckLog(c),
	}
	w, err := New(cfg)
	defer workertest.DirtyKill(c, w)
	c.Assert(err, jc.ErrorIsNil)

	select {
	case <-notifyInitialConfigConsumed:
	case <-time.After(testing.ShortWait):
		c.Fatalf("timed out waiting for worker to start")
	}
	workertest.CleanKill(c, w)
}

// TestNewControllerNode tests that when there is an event on the controller
// node watcher (i.e. a new controller node is added or removed), the worker
// will start tracking the new controller node, and since we mock a new
// controller being added, the worker should also update the api address for
// the new controller.
func (s *workerSuite) TestNewControllerNode(c *gc.C) {
	defer s.setUpMocks(c).Finish()

	// Mock the controller node watcher.
	nodeCh := make(chan struct{})
	nodeWatcher := watchertest.NewMockNotifyWatcher(nodeCh)
	s.controllerNodeService.EXPECT().WatchControllerNodes().Return(nodeWatcher, nil)

	// Send an initial change to the (mocked) controller config watcher.
	s.controllerConfigService.EXPECT().WatchControllerConfig().DoAndReturn(func() (watcher.Watcher[[]string], error) {
		ch := make(chan []string)
		go func() {
			select {
			case ch <- []string{}:
			case <-time.After(testing.ShortWait):
				c.Fatalf("timed out sending initial change")
			}
		}()
		return watchertest.NewMockStringsWatcher(ch), nil
	})

	// Starts the controller tracker for the new node.
	s.controllerNodeService.EXPECT().GetControllerIDs(gomock.Any()).Return([]string{"1"}, nil)
	s.applicationService.EXPECT().GetUnitNetNodes(gomock.Any(), unit.Name("controller/1")).Return([]string{"net-node-0"}, nil)
	s.applicationService.EXPECT().WatchNetNodeAddress(gomock.Any(), "net-node-0").Return(watchertest.NewMockNotifyWatcher(make(chan struct{})), nil)
	// Updates the API addresses for the new node.
	addrs := network.SpaceAddresses{
		{
			MachineAddress: network.MachineAddress{
				Value: "10.0.0.1",
			},
			SpaceID: "space0",
		},
	}
	s.applicationService.EXPECT().GetUnitPublicAddresses(gomock.Any(), unit.Name("controller/1")).Return(addrs, nil)
	s.controllerConfigService.EXPECT().ControllerConfig(gomock.Any()).Return(controller.Config{
		controller.JujuManagementSpace: "space0",
	}, nil)
	sp := network.SpaceInfo{
		ID: "space0",
	}
	s.networkService.EXPECT().SpaceByName(gomock.Any(), "space0").Return(&sp, nil)
	// Synchronization point to ensure the worker processes the event.
	sync := make(chan struct{})
	hostPorts := network.SpaceAddressesWithPort(addrs, 17070)
	s.controllerNodeService.EXPECT().SetAPIAddresses(gomock.Any(), "1", hostPorts, sp).DoAndReturn(func(ctx context.Context, controllerID string, addrs network.SpaceHostPorts, sp network.SpaceInfo) error {
		sync <- struct{}{}
		return nil
	})

	cfg := Config{
		ControllerConfigService: s.controllerConfigService,
		ApplicationService:      s.applicationService,
		ControllerNodeService:   s.controllerNodeService,
		NetworkService:          s.networkService,
		APIPort:                 17070,
		ControllerAPIPort:       17070,
		Logger:                  loggertesting.WrapCheckLog(c),
	}
	w, err := New(cfg)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.DirtyKill(c, w)

	// Simulate a new controller node event.
	select {
	case nodeCh <- struct{}{}:
	case <-time.After(testing.ShortWait):
		c.Fatalf("timed out sending controller node event")
	}

	// Wait for the worker to process the event.
	select {
	case <-sync:
	case <-time.After(testing.ShortWait):
		c.Fatalf("timed out waiting for API address update")
	}

	workertest.CleanKill(c, w)
}
