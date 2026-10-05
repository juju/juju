// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiaddresssetter

import (
	"context"
	stdtesting "testing"

	"github.com/canonical/gomock/gomock"
	"github.com/juju/tc"
	"github.com/juju/worker/v5/workertest"

	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/unit"
	"github.com/juju/juju/core/watcher/watchertest"
	"github.com/juju/juju/domain/controllernode"
	controllernodeerrors "github.com/juju/juju/domain/controllernode/errors"
	domainnetwork "github.com/juju/juju/domain/network"
	"github.com/juju/juju/internal/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/testhelpers"
)

type workerSuite struct {
	testhelpers.IsolationSuite

	controllerNodeService   *MockControllerNodeService
	networkService          *MockNetworkService
	controllerConfigService *MockControllerConfigService
}

func TestWorkerSuite(t *stdtesting.T) {
	testhelpers.PrintGoroutineLeaks(t, func(t *stdtesting.T) {
		tc.Run(t, &workerSuite{})
	})
}

func (s *workerSuite) setUpMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.controllerConfigService = NewMockControllerConfigService(ctrl)
	s.controllerNodeService = NewMockControllerNodeService(ctrl)
	s.networkService = NewMockNetworkService(ctrl)
	return ctrl
}

func (s *workerSuite) config(c *tc.C) Config {
	return Config{
		ControllerConfigService: s.controllerConfigService,
		ControllerNodeService:   s.controllerNodeService,
		NetworkService:          s.networkService,
		Logger:                  loggertesting.WrapCheckLog(c),
	}
}

func (s *workerSuite) expectWatchers(
	nodeCh <-chan struct{}, configCh <-chan []string, networkCh <-chan struct{},
) {
	s.controllerNodeService.EXPECT().WatchControllerNodes(gomock.Any()).
		Return(watchertest.NewMockNotifyWatcher(nodeCh), nil)
	s.controllerConfigService.EXPECT().WatchControllerConfig(gomock.Any()).
		Return(watchertest.NewMockStringsWatcher(configCh), nil)
	s.networkService.EXPECT().WatchControllerNetwork(gomock.Any()).
		Return(watchertest.NewMockNotifyWatcher(networkCh), nil)
}

func initialNotify() chan struct{} {
	ch := make(chan struct{}, 1)
	ch <- struct{}{}
	return ch
}

func initialConfig() chan []string {
	ch := make(chan []string, 1)
	ch <- nil
	return ch
}

func address(value string) network.SpaceAddresses {
	return network.SpaceAddresses{{
		MachineAddress: network.MachineAddress{Value: value},
	}}
}

func (s *workerSuite) expectEmptyReconcile(published chan<- controllernode.SetAPIAddressArgs) {
	s.controllerNodeService.EXPECT().GetControllerIDs(gomock.Any()).Return(nil, controllernodeerrors.EmptyControllerIDs)
	s.controllerConfigService.EXPECT().GetManagementSpaceAndAPIPort(gomock.Any()).Return(network.SpaceName(""), 17070, nil)
	s.networkService.EXPECT().GetControllerClientAddresses(gomock.Any(), []unit.Name(nil)).
		Return(domainnetwork.ControllerAddressSelection{}, nil)
	s.networkService.EXPECT().GetControllerAgentAddresses(gomock.Any(), []unit.Name(nil), network.SpaceName("")).
		Return(domainnetwork.ControllerAddressSelection{}, nil)
	s.networkService.EXPECT().GetControllerPeerAddresses(gomock.Any(), []unit.Name(nil), network.SpaceName("")).
		Return(domainnetwork.ControllerAddressSelection{}, nil)
	s.controllerNodeService.EXPECT().SetAPIAddresses(gomock.Any(), controllernode.SetAPIAddressArgs{
		APIPort:   17070,
		Addresses: map[string]controllernode.APIAddressSet{},
	}).DoAndReturn(func(_ context.Context, args controllernode.SetAPIAddressArgs) error {
		published <- args
		return nil
	})
}

func (s *workerSuite) TestConsumesAllInitialEventsBeforeReading(c *tc.C) {
	defer s.setUpMocks(c).Finish()

	nodeCh := make(chan struct{})
	configCh := make(chan []string)
	networkCh := make(chan struct{})
	s.expectWatchers(nodeCh, configCh, networkCh)
	published := make(chan controllernode.SetAPIAddressArgs, 1)
	s.expectEmptyReconcile(published)

	w, err := New(s.config(c))
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.DirtyKill(c, w)

	nodeCh <- struct{}{}
	configCh <- nil
	networkCh <- struct{}{}
	select {
	case <-published:
	case <-c.Context().Done():
		c.Fatalf("waiting for initial publication: %v", c.Context().Err())
	}
	workertest.CleanKill(c, w)
}

func (s *workerSuite) TestPublishesAllIAASAudiences(c *tc.C) {
	defer s.setUpMocks(c).Finish()
	s.expectWatchers(initialNotify(), initialConfig(), initialNotify())

	controller1 := unit.Name("controller/1")
	controller2 := unit.Name("controller/2")
	names := []unit.Name{controller1, controller2}
	clients1, clients2 := address("client-1"), address("client-2")
	agents1, agents2 := address("agent-1"), address("agent-2")
	peers1, peers2 := address("peer-1"), address("peer-2")
	s.controllerNodeService.EXPECT().GetControllerIDs(gomock.Any()).Return([]string{"2", "1"}, nil)
	s.controllerConfigService.EXPECT().GetManagementSpaceAndAPIPort(gomock.Any()).Return(network.SpaceName("management"), 17071, nil)
	s.networkService.EXPECT().GetControllerClientAddresses(gomock.Any(), gomock.InAnyOrder(names)).Return(domainnetwork.ControllerAddressSelection{
		ByUnit: map[unit.Name]network.SpaceAddresses{controller1: clients1, controller2: clients2},
	}, nil)
	s.networkService.EXPECT().GetControllerAgentAddresses(gomock.Any(), gomock.InAnyOrder(names), network.SpaceName("management")).Return(domainnetwork.ControllerAddressSelection{
		ByUnit: map[unit.Name]network.SpaceAddresses{controller1: agents1, controller2: agents2},
	}, nil)
	s.networkService.EXPECT().GetControllerPeerAddresses(gomock.Any(), gomock.InAnyOrder(names), network.SpaceName("management")).Return(domainnetwork.ControllerAddressSelection{
		ByUnit: map[unit.Name]network.SpaceAddresses{controller1: peers1, controller2: peers2},
	}, nil)
	published := make(chan struct{})
	s.controllerNodeService.EXPECT().SetAPIAddresses(gomock.Any(), controllernode.SetAPIAddressArgs{
		APIPort: 17071,
		Addresses: map[string]controllernode.APIAddressSet{
			"1": {Clients: clients1, Agents: agents1, Peers: peers1},
			"2": {Clients: clients2, Agents: agents2, Peers: peers2},
		},
	}).DoAndReturn(func(context.Context, controllernode.SetAPIAddressArgs) error {
		close(published)
		return nil
	})

	w, err := New(s.config(c))
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.DirtyKill(c, w)
	select {
	case <-published:
	case <-c.Context().Done():
		c.Fatalf("waiting for publication: %v", c.Context().Err())
	}
	workertest.CleanKill(c, w)
}

func (s *workerSuite) TestPublishesCAASSharedEndpoints(c *tc.C) {
	defer s.setUpMocks(c).Finish()
	s.expectWatchers(initialNotify(), initialConfig(), initialNotify())

	sharedClients := address("public-service")
	sharedAgents := address("local-service")
	peer := address("pod-address")
	controller0 := unit.Name("controller/0")
	s.controllerNodeService.EXPECT().GetControllerIDs(gomock.Any()).Return([]string{"0"}, nil)
	s.controllerConfigService.EXPECT().GetManagementSpaceAndAPIPort(gomock.Any()).Return(network.SpaceName(""), 17070, nil)
	s.networkService.EXPECT().GetControllerClientAddresses(gomock.Any(), []unit.Name{controller0}).Return(domainnetwork.ControllerAddressSelection{Shared: sharedClients}, nil)
	s.networkService.EXPECT().GetControllerAgentAddresses(gomock.Any(), []unit.Name{controller0}, network.SpaceName("")).Return(domainnetwork.ControllerAddressSelection{Shared: sharedAgents}, nil)
	s.networkService.EXPECT().GetControllerPeerAddresses(gomock.Any(), []unit.Name{controller0}, network.SpaceName("")).Return(domainnetwork.ControllerAddressSelection{
		ByUnit: map[unit.Name]network.SpaceAddresses{controller0: peer},
	}, nil)
	published := make(chan struct{})
	s.controllerNodeService.EXPECT().SetAPIAddresses(gomock.Any(), controllernode.SetAPIAddressArgs{
		APIPort: 17070,
		SharedAddresses: controllernode.SharedAPIAddressSet{
			Clients: sharedClients,
			Agents:  sharedAgents,
		},
		Addresses: map[string]controllernode.APIAddressSet{
			"0": {Peers: peer},
		},
	}).DoAndReturn(func(context.Context, controllernode.SetAPIAddressArgs) error {
		close(published)
		return nil
	})

	w, err := New(s.config(c))
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.DirtyKill(c, w)
	select {
	case <-published:
	case <-c.Context().Done():
		c.Fatalf("waiting for shared publication: %v", c.Context().Err())
	}
	workertest.CleanKill(c, w)
}

func (s *workerSuite) TestPreservesSuccessfulEmptySharedSelection(c *tc.C) {
	defer s.setUpMocks(c).Finish()
	s.expectWatchers(initialNotify(), initialConfig(), initialNotify())

	s.controllerNodeService.EXPECT().GetControllerIDs(gomock.Any()).Return(nil, controllernodeerrors.EmptyControllerIDs)
	s.controllerConfigService.EXPECT().GetManagementSpaceAndAPIPort(gomock.Any()).Return(network.SpaceName(""), 17070, nil)
	s.networkService.EXPECT().GetControllerClientAddresses(gomock.Any(), []unit.Name(nil)).Return(domainnetwork.ControllerAddressSelection{
		Shared: network.SpaceAddresses{},
	}, nil)
	s.networkService.EXPECT().GetControllerAgentAddresses(gomock.Any(), []unit.Name(nil), network.SpaceName("")).Return(domainnetwork.ControllerAddressSelection{}, nil)
	s.networkService.EXPECT().GetControllerPeerAddresses(gomock.Any(), []unit.Name(nil), network.SpaceName("")).Return(domainnetwork.ControllerAddressSelection{}, nil)
	published := make(chan struct{})
	s.controllerNodeService.EXPECT().SetAPIAddresses(gomock.Any(), controllernode.SetAPIAddressArgs{
		APIPort:   17070,
		Addresses: map[string]controllernode.APIAddressSet{},
		SharedAddresses: controllernode.SharedAPIAddressSet{
			Clients: network.SpaceAddresses{},
		},
	}).DoAndReturn(func(context.Context, controllernode.SetAPIAddressArgs) error {
		close(published)
		return nil
	})

	w, err := New(s.config(c))
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.DirtyKill(c, w)
	select {
	case <-published:
	case <-c.Context().Done():
		c.Fatalf("waiting for empty shared publication: %v", c.Context().Err())
	}
	workertest.CleanKill(c, w)
}

func (s *workerSuite) TestSourceFailureStopsWorker(c *tc.C) {
	defer s.setUpMocks(c).Finish()
	s.expectWatchers(initialNotify(), initialConfig(), initialNotify())

	s.controllerNodeService.EXPECT().GetControllerIDs(gomock.Any()).Return(nil, errors.New("boom"))
	w, err := New(s.config(c))
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.DirtyKill(c, w)

	err = workertest.CheckKilled(c, w)
	c.Check(err, tc.ErrorMatches, "getting controller IDs: boom")
}

func (s *workerSuite) TestMissingPeerAddressesPublishesAuthoritativeEmptySet(c *tc.C) {
	defer s.setUpMocks(c).Finish()
	s.expectWatchers(initialNotify(), initialConfig(), initialNotify())

	controller0 := unit.Name("controller/0")
	controller1 := unit.Name("controller/1")
	names := []unit.Name{controller0, controller1}
	clients0, clients1 := address("client-0"), address("client-1")
	agents0, agents1 := address("agent-0"), address("agent-1")
	peers1 := address("peer-1")
	s.controllerNodeService.EXPECT().GetControllerIDs(gomock.Any()).Return([]string{"0", "1"}, nil)
	s.controllerConfigService.EXPECT().GetManagementSpaceAndAPIPort(gomock.Any()).Return(network.SpaceName(""), 17070, nil)
	s.networkService.EXPECT().GetControllerClientAddresses(gomock.Any(), gomock.InAnyOrder(names)).Return(domainnetwork.ControllerAddressSelection{
		ByUnit: map[unit.Name]network.SpaceAddresses{controller0: clients0, controller1: clients1},
	}, nil)
	s.networkService.EXPECT().GetControllerAgentAddresses(gomock.Any(), gomock.InAnyOrder(names), network.SpaceName("")).Return(domainnetwork.ControllerAddressSelection{
		ByUnit: map[unit.Name]network.SpaceAddresses{controller0: agents0, controller1: agents1},
	}, nil)
	s.networkService.EXPECT().GetControllerPeerAddresses(gomock.Any(), gomock.InAnyOrder(names), network.SpaceName("")).Return(domainnetwork.ControllerAddressSelection{
		ByUnit: map[unit.Name]network.SpaceAddresses{controller1: peers1},
	}, nil)
	published := make(chan struct{})
	s.controllerNodeService.EXPECT().SetAPIAddresses(gomock.Any(), controllernode.SetAPIAddressArgs{
		APIPort: 17070,
		Addresses: map[string]controllernode.APIAddressSet{
			"0": {Clients: clients0, Agents: agents0},
			"1": {Clients: clients1, Agents: agents1, Peers: peers1},
		},
	}).DoAndReturn(func(context.Context, controllernode.SetAPIAddressArgs) error {
		close(published)
		return nil
	})

	w, err := New(s.config(c))
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.DirtyKill(c, w)
	select {
	case <-published:
	case <-c.Context().Done():
		c.Fatalf("waiting for publication with empty peer addresses: %v", c.Context().Err())
	}
	workertest.CleanKill(c, w)
}

func (s *workerSuite) TestWatcherClosureStopsWorker(c *tc.C) {
	defer s.setUpMocks(c).Finish()
	networkCh := initialNotify()
	s.expectWatchers(initialNotify(), initialConfig(), networkCh)
	published := make(chan controllernode.SetAPIAddressArgs, 1)
	s.expectEmptyReconcile(published)

	w, err := New(s.config(c))
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.DirtyKill(c, w)
	select {
	case <-published:
	case <-c.Context().Done():
		c.Fatalf("waiting for initial publication: %v", c.Context().Err())
	}
	close(networkCh)
	err = workertest.CheckKilled(c, w)
	c.Check(err, tc.ErrorMatches, "controller network watcher closed")
}
