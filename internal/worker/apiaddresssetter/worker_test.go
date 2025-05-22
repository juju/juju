// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiaddresssetter

import (
	"context"
	stdtesting "testing"
	"time"

	"github.com/juju/tc"
	"github.com/juju/testing"
	"github.com/juju/worker/v4/workertest"
	gomock "go.uber.org/mock/gomock"

	controller "github.com/juju/juju/controller"
	"github.com/juju/juju/core/network"
	networktesting "github.com/juju/juju/core/network/testing"
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

func TestWorkerSuite(t *stdtesting.T) { tc.Run(t, &workerSuite{}) }
func (s *workerSuite) setUpMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.applicationService = NewMockApplicationService(ctrl)
	s.controllerConfigService = NewMockControllerConfigService(ctrl)
	s.controllerNodeService = NewMockControllerNodeService(ctrl)
	s.networkService = NewMockNetworkService(ctrl)

	return ctrl
}

func (s *workerSuite) TestWorkerCleanKill(c *tc.C) {
	defer s.setUpMocks(c).Finish()

	nodeWatcher := watchertest.NewMockNotifyWatcher(make(chan struct{}))
	s.controllerNodeService.EXPECT().WatchControllerNodes(gomock.Any()).Return(nodeWatcher, nil)
	// We use the consume of the initial event as a sync point to decide
	// whether the worker has started. This channel is then used to stop
	// waiting for the worker to start.
	notifyInitialConfigConsumed := make(chan struct{})
	// Send an initial change to the (mocked) controller config watcher.
	s.controllerConfigService.EXPECT().WatchControllerConfig(gomock.Any()).DoAndReturn(func(ctx context.Context) (watcher.Watcher[[]string], error) {
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
	c.Assert(err, tc.ErrorIsNil)

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
func (s *workerSuite) TestNewControllerNode(c *tc.C) {
	defer s.setUpMocks(c).Finish()

	// Mock the controller node watcher.
	nodeCh := make(chan struct{})
	nodeWatcher := watchertest.NewMockNotifyWatcher(nodeCh)
	s.controllerNodeService.EXPECT().WatchControllerNodes(gomock.Any()).Return(nodeWatcher, nil)

	cfgCh := make(chan []string)
	cfgWatcher := watchertest.NewMockStringsWatcher(cfgCh)
	s.controllerConfigService.EXPECT().WatchControllerConfig(gomock.Any()).Return(cfgWatcher, nil)

	// Starts the controller tracker for the new node.
	s.controllerNodeService.EXPECT().GetControllerIDs(gomock.Any()).Return([]string{"1"}, nil)
	netNodeUUID := networktesting.GenNetNodeUUID(c)
	s.applicationService.EXPECT().GetUnitNetNodes(gomock.Any(), unit.Name("controller/1")).Return([]network.NetNodeUUID{netNodeUUID}, nil)
	s.applicationService.EXPECT().WatchNetNodeAddress(gomock.Any(), netNodeUUID).Return(watchertest.NewMockNotifyWatcher(make(chan struct{})), nil)
	// Updates the API addresses for the new node.
	addrs := network.SpaceAddresses{
		{
			MachineAddress: network.MachineAddress{
				Value: "10.0.0.1",
			},
			SpaceID: "space0",
		},
	}
	s.controllerConfigService.EXPECT().ControllerConfig(gomock.Any()).Return(controller.Config{
		controller.JujuManagementSpace: "space0",
	}, nil)
	sp := network.SpaceInfo{
		ID: "space0",
	}
	s.applicationService.EXPECT().GetUnitPublicAddresses(gomock.Any(), unit.Name("controller/1")).Return(addrs, nil)
	s.networkService.EXPECT().SpaceByName(gomock.Any(), "space0").Return(&sp, nil)
	// Synchronization point to ensure the worker processes the event.
	sync := make(chan struct{})
	hostPorts := network.SpaceAddressesWithPort(addrs, 17070)
	s.controllerNodeService.EXPECT().SetAPIAddresses(gomock.Any(), "1", hostPorts, sp).DoAndReturn(func(ctx context.Context, controllerID string, addrs network.SpaceHostPorts, sp network.SpaceInfo) error {
		close(sync)
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
	c.Assert(err, tc.ErrorIsNil)
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

// TestConfigChange tests that when the controller config changes, the worker
// will update the api addresses for the controller.
func (s *workerSuite) TestConfigChange(c *tc.C) {
	defer s.setUpMocks(c).Finish()

	// Mock the controller node watcher.
	nodeCh := make(chan struct{})
	nodeWatcher := watchertest.NewMockNotifyWatcher(nodeCh)
	s.controllerNodeService.EXPECT().WatchControllerNodes(gomock.Any()).Return(nodeWatcher, nil)

	cfgCh := make(chan []string)
	cfgWatcher := watchertest.NewMockStringsWatcher(cfgCh)
	s.controllerConfigService.EXPECT().WatchControllerConfig(gomock.Any()).Return(cfgWatcher, nil)

	// Starts the controller tracker for the new node.
	s.controllerNodeService.EXPECT().GetControllerIDs(gomock.Any()).Return([]string{"1"}, nil)
	netNodeUUID := networktesting.GenNetNodeUUID(c)
	s.applicationService.EXPECT().GetUnitNetNodes(gomock.Any(), unit.Name("controller/1")).Return([]network.NetNodeUUID{netNodeUUID}, nil)
	s.applicationService.EXPECT().WatchNetNodeAddress(gomock.Any(), netNodeUUID).Return(watchertest.NewMockNotifyWatcher(make(chan struct{})), nil)

	// Updates the API addresses for the new node.
	addrs := network.SpaceAddresses{
		{
			MachineAddress: network.MachineAddress{
				Value: "10.0.0.1",
			},
			SpaceID: "space0",
		},
	}
	s.controllerConfigService.EXPECT().ControllerConfig(gomock.Any()).Return(controller.Config{
		controller.JujuManagementSpace: "space0",
	}, nil)
	sp0 := network.SpaceInfo{
		ID: "space0",
	}
	s.applicationService.EXPECT().GetUnitPublicAddresses(gomock.Any(), unit.Name("controller/1")).Return(addrs, nil)
	s.networkService.EXPECT().SpaceByName(gomock.Any(), "space0").Return(&sp0, nil)
	// Synchronization point to ensure the worker processes the event.
	sync := make(chan struct{})
	hostPorts := network.SpaceAddressesWithPort(addrs, 17070)
	s.controllerNodeService.EXPECT().SetAPIAddresses(gomock.Any(), "1", hostPorts, sp0).DoAndReturn(func(ctx context.Context, controllerID string, addrs network.SpaceHostPorts, sp network.SpaceInfo) error {
		close(sync)
		return nil
	})
	// Expected calls after the controller config change.
	s.controllerConfigService.EXPECT().ControllerConfig(gomock.Any()).Return(controller.Config{
		controller.JujuManagementSpace: "space1",
	}, nil)
	sp1 := network.SpaceInfo{
		ID: "space1",
	}
	s.applicationService.EXPECT().GetUnitPublicAddresses(gomock.Any(), unit.Name("controller/1")).Return(addrs, nil)
	s.networkService.EXPECT().SpaceByName(gomock.Any(), "space1").Return(&sp1, nil)
	// Synchronization point to ensure the worker processes the config event.
	cfgSync := make(chan struct{})
	s.controllerNodeService.EXPECT().SetAPIAddresses(gomock.Any(), "1", hostPorts, sp1).DoAndReturn(func(ctx context.Context, controllerID string, addrs network.SpaceHostPorts, sp network.SpaceInfo) error {
		close(cfgSync)
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
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.DirtyKill(c, w)

	// Simulate a new controller node event.
	select {
	case nodeCh <- struct{}{}:
	case <-time.After(testing.ShortWait):
		c.Fatalf("timed out sending controller node event")
	}

	// Wait for the worker to process the initial (new node) event.
	select {
	case <-sync:
	case <-time.After(testing.ShortWait):
		c.Fatalf("timed out waiting for API address update")
	}

	// Now we can trigger the config change on the cfgWatcher channel, and sync
	// on the second set api addresses call.
	select {
	case cfgCh <- []string{}:
	case <-time.After(testing.ShortWait):
		c.Fatalf("timed out sending controller config change")
	}

	// Wait for the worker to process the config event.
	select {
	case <-cfgSync:
	case <-time.After(testing.ShortWait):
		c.Fatalf("timed out waiting for API address update after config change")
	}

	workertest.CleanKill(c, w)
}

// TestNodeAddressChange tests that when the controller node address changes,
// the worker will update the api addresses for the controller.
func (s *workerSuite) TestNodeAddressChange(c *tc.C) {
	defer s.setUpMocks(c).Finish()

	// Mock the controller node watcher.
	nodeCh := make(chan struct{})
	nodeWatcher := watchertest.NewMockNotifyWatcher(nodeCh)
	s.controllerNodeService.EXPECT().WatchControllerNodes(gomock.Any()).Return(nodeWatcher, nil)

	s.controllerConfigService.EXPECT().WatchControllerConfig(gomock.Any()).Return(watchertest.NewMockStringsWatcher(make(chan []string)), nil)

	// Starts the controller tracker for the new node.
	s.controllerNodeService.EXPECT().GetControllerIDs(gomock.Any()).Return([]string{"1"}, nil)
	netNodeUUID := networktesting.GenNetNodeUUID(c)
	s.applicationService.EXPECT().GetUnitNetNodes(gomock.Any(), unit.Name("controller/1")).Return([]network.NetNodeUUID{netNodeUUID}, nil)
	addrCh := make(chan struct{})
	netNodeAddressWatcher := watchertest.NewMockNotifyWatcher(addrCh)
	s.applicationService.EXPECT().WatchNetNodeAddress(gomock.Any(), netNodeUUID).Return(netNodeAddressWatcher, nil)

	// Updates the API addresses for the new node.
	addrs := network.SpaceAddresses{
		{
			MachineAddress: network.MachineAddress{
				Value: "10.0.0.1",
			},
			SpaceID: "space0",
		},
	}
	s.controllerConfigService.EXPECT().ControllerConfig(gomock.Any()).Return(controller.Config{
		controller.JujuManagementSpace: "space0",
	}, nil).MaxTimes(2)
	sp0 := network.SpaceInfo{
		ID: "space0",
	}
	s.applicationService.EXPECT().GetUnitPublicAddresses(gomock.Any(), unit.Name("controller/1")).Return(addrs, nil)
	s.networkService.EXPECT().SpaceByName(gomock.Any(), "space0").Return(&sp0, nil).MaxTimes(2)
	// Synchronization point to ensure the worker processes the event.
	sync := make(chan struct{})
	hostPorts := network.SpaceAddressesWithPort(addrs, 17070)
	s.controllerNodeService.EXPECT().SetAPIAddresses(gomock.Any(), "1", hostPorts, sp0).DoAndReturn(func(ctx context.Context, controllerID string, addrs network.SpaceHostPorts, sp network.SpaceInfo) error {
		close(sync)
		return nil
	})
	// Expected calls after the controller node address change.
	newAddrs := network.SpaceAddresses{
		{
			MachineAddress: network.MachineAddress{
				Value: "192.168.0.1",
			},
			SpaceID: "space0",
		},
	}
	s.applicationService.EXPECT().GetUnitPublicAddresses(gomock.Any(), unit.Name("controller/1")).Return(newAddrs, nil)
	// Synchronization point to ensure the worker processes the config event.
	addrSync := make(chan struct{})
	newHP := network.SpaceAddressesWithPort(newAddrs, 17070)
	s.controllerNodeService.EXPECT().SetAPIAddresses(gomock.Any(), "1", newHP, sp0).DoAndReturn(func(ctx context.Context, controllerID string, addrs network.SpaceHostPorts, sp network.SpaceInfo) error {
		close(addrSync)
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
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.DirtyKill(c, w)

	// Simulate a new controller node event.
	select {
	case nodeCh <- struct{}{}:
	case <-time.After(testing.ShortWait):
		c.Fatalf("timed out sending controller node event")
	}

	// Wait for the worker to process the initial (new node) event.
	select {
	case <-sync:
	case <-time.After(testing.ShortWait):
		c.Fatalf("timed out waiting for API address update")
	}

	// Now we can trigger the node address change on the watcher, and sync
	// on the second set api addresses call.
	select {
	case addrCh <- struct{}{}:
	case <-time.After(testing.ShortWait):
		c.Fatalf("timed out sending controller node address change")
	}

	// Wait for the worker to process the new addrs event.
	select {
	case <-addrSync:
	case <-time.After(testing.ShortWait):
		c.Fatalf("timed out waiting for API address update after address change")
	}

	workertest.CleanKill(c, w)
}
