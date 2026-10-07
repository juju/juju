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

func (s *workerSuite) expectEmptyReconcile(c *tc.C, published chan<- controllernode.SetAPIAddressArgs) {
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
		select {
		case published <- args:
			return nil
		case <-c.Context().Done():
			return c.Context().Err()
		}
	})
}

type reconcileExpectation struct {
	controllerIDs   []string
	managementSpace network.SpaceName
	apiPort         int
	clients         domainnetwork.ControllerAddressSelection
	agents          domainnetwork.ControllerAddressSelection
	peers           domainnetwork.ControllerAddressSelection
	addresses       map[string]controllernode.APIAddressSet
}

func (s *workerSuite) expectReconcile(
	c *tc.C,
	expectation reconcileExpectation,
	published chan<- controllernode.SetAPIAddressArgs,
) {
	names := make([]unit.Name, len(expectation.controllerIDs))
	for i, controllerID := range expectation.controllerIDs {
		names[i] = unit.Name("controller/" + controllerID)
	}

	s.controllerNodeService.EXPECT().GetControllerIDs(gomock.Any()).
		Return(expectation.controllerIDs, nil)
	s.controllerConfigService.EXPECT().GetManagementSpaceAndAPIPort(gomock.Any()).
		Return(expectation.managementSpace, expectation.apiPort, nil)

	s.networkService.EXPECT().GetControllerClientAddresses(gomock.Any(), gomock.InAnyOrder(names)).
		Return(expectation.clients, nil)
	s.networkService.EXPECT().GetControllerAgentAddresses(
		gomock.Any(), gomock.InAnyOrder(names), expectation.managementSpace,
	).Return(expectation.agents, nil)
	s.networkService.EXPECT().GetControllerPeerAddresses(
		gomock.Any(), gomock.InAnyOrder(names), expectation.managementSpace,
	).Return(expectation.peers, nil)

	s.controllerNodeService.EXPECT().SetAPIAddresses(gomock.Any(), controllernode.SetAPIAddressArgs{
		APIPort:   expectation.apiPort,
		Addresses: expectation.addresses,
	}).DoAndReturn(func(_ context.Context, args controllernode.SetAPIAddressArgs) error {
		select {
		case published <- args:
			return nil
		case <-c.Context().Done():
			return c.Context().Err()
		}
	})
}

func (s *workerSuite) TestConsumesAllInitialEventsBeforeReading(c *tc.C) {
	defer s.setUpMocks(c).Finish()

	nodeCh := make(chan struct{})
	configCh := make(chan []string)
	networkCh := make(chan struct{})
	s.expectWatchers(nodeCh, configCh, networkCh)
	published := make(chan controllernode.SetAPIAddressArgs, 1)
	s.expectEmptyReconcile(c, published)

	w, err := New(s.config(c))
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.DirtyKill(c, w)

	select {
	case nodeCh <- struct{}{}:
	case <-c.Context().Done():
		c.Fatalf("sending initial controller node notification: %v", c.Context().Err())
	}

	select {
	case configCh <- nil:
	case <-c.Context().Done():
		c.Fatalf("sending initial controller config notification: %v", c.Context().Err())
	}

	select {
	case networkCh <- struct{}{}:
	case <-c.Context().Done():
		c.Fatalf("sending initial controller network notification: %v", c.Context().Err())
	}

	select {
	case <-published:
	case <-c.Context().Done():
		c.Fatalf("waiting for initial publication: %v", c.Context().Err())
	}

	workertest.CleanKill(c, w)
}

func (s *workerSuite) TestPublishesAllIAASAudiences(c *tc.C) {
	defer s.setUpMocks(c).Finish()
	s.expectWatchers(initialNotify(c), initialConfig(c), initialNotify(c))

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

func (s *workerSuite) TestControllerConfigChangeRepublishesAddresses(c *tc.C) {
	defer s.setUpMocks(c).Finish()

	nodeCh := initialNotify(c)
	configCh := initialConfig(c)
	networkCh := initialNotify(c)

	s.expectWatchers(nodeCh, configCh, networkCh)

	controller0 := unit.Name("controller/0")
	clients := address("client")
	initialAgents := address("initial-agent")
	initialPeers := address("initial-peer")
	updatedAgents := address("updated-agent")
	updatedPeers := address("updated-peer")

	published := make(chan controllernode.SetAPIAddressArgs, 2)

	s.expectReconcile(c, reconcileExpectation{
		controllerIDs:   []string{"0"},
		managementSpace: "initial",
		apiPort:         17070,
		clients: domainnetwork.ControllerAddressSelection{
			ByUnit: map[unit.Name]network.SpaceAddresses{controller0: clients},
		},
		agents: domainnetwork.ControllerAddressSelection{
			ByUnit: map[unit.Name]network.SpaceAddresses{controller0: initialAgents},
		},
		peers: domainnetwork.ControllerAddressSelection{
			ByUnit: map[unit.Name]network.SpaceAddresses{controller0: initialPeers},
		},
		addresses: map[string]controllernode.APIAddressSet{
			"0": {Clients: clients, Agents: initialAgents, Peers: initialPeers},
		},
	}, published)

	s.expectReconcile(c, reconcileExpectation{
		controllerIDs:   []string{"0"},
		managementSpace: "updated",
		apiPort:         17071,
		clients: domainnetwork.ControllerAddressSelection{
			ByUnit: map[unit.Name]network.SpaceAddresses{controller0: clients},
		},
		agents: domainnetwork.ControllerAddressSelection{
			ByUnit: map[unit.Name]network.SpaceAddresses{controller0: updatedAgents},
		},
		peers: domainnetwork.ControllerAddressSelection{
			ByUnit: map[unit.Name]network.SpaceAddresses{controller0: updatedPeers},
		},
		addresses: map[string]controllernode.APIAddressSet{
			"0": {Clients: clients, Agents: updatedAgents, Peers: updatedPeers},
		},
	}, published)

	w, err := New(s.config(c))
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.DirtyKill(c, w)

	waitForPublication(c, published)

	select {
	case configCh <- []string{"juju-mgmt-space", "api-port"}:
	case <-c.Context().Done():
		c.Fatalf("sending controller config notification: %v", c.Context().Err())
	}

	waitForPublication(c, published)

	workertest.CleanKill(c, w)
}

func (s *workerSuite) TestNetworkChangeRepublishesAddresses(c *tc.C) {
	defer s.setUpMocks(c).Finish()

	nodeCh := initialNotify(c)
	configCh := initialConfig(c)
	networkCh := initialNotify(c)

	s.expectWatchers(nodeCh, configCh, networkCh)

	controller0 := unit.Name("controller/0")
	initialClients := address("initial-client")
	initialAgents := address("initial-agent")
	initialPeers := address("initial-peer")
	updatedClients := address("updated-client")
	updatedAgents := address("updated-agent")
	updatedPeers := address("updated-peer")

	published := make(chan controllernode.SetAPIAddressArgs, 2)

	s.expectReconcile(c, reconcileExpectation{
		controllerIDs:   []string{"0"},
		managementSpace: "management",
		apiPort:         17070,
		clients: domainnetwork.ControllerAddressSelection{
			ByUnit: map[unit.Name]network.SpaceAddresses{controller0: initialClients},
		},
		agents: domainnetwork.ControllerAddressSelection{
			ByUnit: map[unit.Name]network.SpaceAddresses{controller0: initialAgents},
		},
		peers: domainnetwork.ControllerAddressSelection{
			ByUnit: map[unit.Name]network.SpaceAddresses{controller0: initialPeers},
		},
		addresses: map[string]controllernode.APIAddressSet{
			"0": {Clients: initialClients, Agents: initialAgents, Peers: initialPeers},
		},
	}, published)

	s.expectReconcile(c, reconcileExpectation{
		controllerIDs:   []string{"0"},
		managementSpace: "management",
		apiPort:         17070,
		clients: domainnetwork.ControllerAddressSelection{
			ByUnit: map[unit.Name]network.SpaceAddresses{controller0: updatedClients},
		},
		agents: domainnetwork.ControllerAddressSelection{
			ByUnit: map[unit.Name]network.SpaceAddresses{controller0: updatedAgents},
		},
		peers: domainnetwork.ControllerAddressSelection{
			ByUnit: map[unit.Name]network.SpaceAddresses{controller0: updatedPeers},
		},
		addresses: map[string]controllernode.APIAddressSet{
			"0": {Clients: updatedClients, Agents: updatedAgents, Peers: updatedPeers},
		},
	}, published)

	w, err := New(s.config(c))
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.DirtyKill(c, w)

	waitForPublication(c, published)

	select {
	case networkCh <- struct{}{}:
	case <-c.Context().Done():
		c.Fatalf("sending controller network notification: %v", c.Context().Err())
	}

	waitForPublication(c, published)

	workertest.CleanKill(c, w)
}

func (s *workerSuite) TestControllerMembershipChangeRepublishesAddresses(c *tc.C) {
	defer s.setUpMocks(c).Finish()

	nodeCh := initialNotify(c)
	configCh := initialConfig(c)
	networkCh := initialNotify(c)

	s.expectWatchers(nodeCh, configCh, networkCh)

	controller0 := unit.Name("controller/0")
	controller1 := unit.Name("controller/1")
	clients0, clients1 := address("client-0"), address("client-1")
	agents0, agents1 := address("agent-0"), address("agent-1")
	peers0, peers1 := address("peer-0"), address("peer-1")

	published := make(chan controllernode.SetAPIAddressArgs, 2)

	s.expectReconcile(c, reconcileExpectation{
		controllerIDs: []string{"0"},
		apiPort:       17070,
		clients: domainnetwork.ControllerAddressSelection{
			ByUnit: map[unit.Name]network.SpaceAddresses{controller0: clients0},
		},
		agents: domainnetwork.ControllerAddressSelection{
			ByUnit: map[unit.Name]network.SpaceAddresses{controller0: agents0},
		},
		peers: domainnetwork.ControllerAddressSelection{
			ByUnit: map[unit.Name]network.SpaceAddresses{controller0: peers0},
		},
		addresses: map[string]controllernode.APIAddressSet{
			"0": {Clients: clients0, Agents: agents0, Peers: peers0},
		},
	}, published)

	s.expectReconcile(c, reconcileExpectation{
		controllerIDs: []string{"0", "1"},
		apiPort:       17070,
		clients: domainnetwork.ControllerAddressSelection{
			ByUnit: map[unit.Name]network.SpaceAddresses{controller0: clients0, controller1: clients1},
		},
		agents: domainnetwork.ControllerAddressSelection{
			ByUnit: map[unit.Name]network.SpaceAddresses{controller0: agents0, controller1: agents1},
		},
		peers: domainnetwork.ControllerAddressSelection{
			ByUnit: map[unit.Name]network.SpaceAddresses{controller0: peers0, controller1: peers1},
		},
		addresses: map[string]controllernode.APIAddressSet{
			"0": {Clients: clients0, Agents: agents0, Peers: peers0},
			"1": {Clients: clients1, Agents: agents1, Peers: peers1},
		},
	}, published)

	w, err := New(s.config(c))
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.DirtyKill(c, w)

	waitForPublication(c, published)

	select {
	case nodeCh <- struct{}{}:
	case <-c.Context().Done():
		c.Fatalf("sending controller node notification: %v", c.Context().Err())
	}

	waitForPublication(c, published)

	workertest.CleanKill(c, w)
}

func (s *workerSuite) TestPublishesCAASSharedEndpoints(c *tc.C) {
	defer s.setUpMocks(c).Finish()
	s.expectWatchers(initialNotify(c), initialConfig(c), initialNotify(c))

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
	s.expectWatchers(initialNotify(c), initialConfig(c), initialNotify(c))

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
	s.expectWatchers(initialNotify(c), initialConfig(c), initialNotify(c))

	s.controllerNodeService.EXPECT().GetControllerIDs(gomock.Any()).Return(nil, errors.New("boom"))
	w, err := New(s.config(c))
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.DirtyKill(c, w)

	err = workertest.CheckKilled(c, w)
	c.Check(err, tc.ErrorMatches, "getting controller IDs: boom")
}

func (s *workerSuite) TestMissingPeerAddressesPublishesAuthoritativeEmptySet(c *tc.C) {
	defer s.setUpMocks(c).Finish()
	s.expectWatchers(initialNotify(c), initialConfig(c), initialNotify(c))

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

func (s *workerSuite) TestEmptyDiscoveryAddressesPreserveBootstrapAddresses(c *tc.C) {
	s.assertIncompleteDiscoveryPreservesBootstrap(c,
		domainnetwork.ControllerAddressSelection{},
		domainnetwork.ControllerAddressSelection{})
}

func (s *workerSuite) TestMissingClientDiscoveryPreservesBootstrapAddresses(c *tc.C) {
	controller0 := unit.Name("controller/0")
	s.assertIncompleteDiscoveryPreservesBootstrap(c,
		domainnetwork.ControllerAddressSelection{},
		domainnetwork.ControllerAddressSelection{
			ByUnit: map[unit.Name]network.SpaceAddresses{controller0: address("agent")},
		})
}

func (s *workerSuite) TestMissingAgentDiscoveryPreservesBootstrapAddresses(c *tc.C) {
	controller0 := unit.Name("controller/0")
	s.assertIncompleteDiscoveryPreservesBootstrap(c,
		domainnetwork.ControllerAddressSelection{
			ByUnit: map[unit.Name]network.SpaceAddresses{controller0: address("client")},
		},
		domainnetwork.ControllerAddressSelection{})
}

func (s *workerSuite) assertIncompleteDiscoveryPreservesBootstrap(
	c *tc.C,
	clients, agents domainnetwork.ControllerAddressSelection,
) {
	defer s.setUpMocks(c).Finish()
	s.expectWatchers(initialNotify(c), initialConfig(c), initialNotify(c))

	controller0 := unit.Name("controller/0")
	names := []unit.Name{controller0}
	s.controllerNodeService.EXPECT().GetControllerIDs(gomock.Any()).Return([]string{"0"}, nil)
	s.controllerConfigService.EXPECT().GetManagementSpaceAndAPIPort(gomock.Any()).Return(network.SpaceName(""), 17070, nil)
	s.networkService.EXPECT().GetControllerClientAddresses(gomock.Any(), names).Return(clients, nil)
	s.networkService.EXPECT().GetControllerAgentAddresses(gomock.Any(), names, network.SpaceName("")).Return(agents, nil)
	reconciled := make(chan struct{})
	s.networkService.EXPECT().GetControllerPeerAddresses(gomock.Any(), names, network.SpaceName("")).DoAndReturn(
		func(context.Context, []unit.Name, network.SpaceName) (domainnetwork.ControllerAddressSelection, error) {
			close(reconciled)
			return domainnetwork.ControllerAddressSelection{}, nil
		},
	)

	w, err := New(s.config(c))
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.DirtyKill(c, w)
	select {
	case <-reconciled:
	case <-c.Context().Done():
		c.Fatalf("waiting for empty discovery reconciliation: %v", c.Context().Err())
	}
	workertest.CleanKill(c, w)
}

func (s *workerSuite) TestWatcherClosureStopsWorker(c *tc.C) {
	defer s.setUpMocks(c).Finish()
	networkCh := initialNotify(c)
	s.expectWatchers(initialNotify(c), initialConfig(c), networkCh)
	published := make(chan controllernode.SetAPIAddressArgs, 1)
	s.expectEmptyReconcile(c, published)

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

func waitForPublication(c *tc.C, published <-chan controllernode.SetAPIAddressArgs) {
	select {
	case <-published:
	case <-c.Context().Done():
		c.Fatalf("waiting for API address publication: %v", c.Context().Err())
	}
}

func initialNotify(c *tc.C) chan struct{} {
	ch := make(chan struct{}, 1)
	select {
	case ch <- struct{}{}:
	case <-c.Context().Done():
		c.Fatalf("sending initial notification: %v", c.Context().Err())
	}
	return ch
}

func initialConfig(c *tc.C) chan []string {
	ch := make(chan []string, 1)
	select {
	case ch <- nil:
	case <-c.Context().Done():
		c.Fatalf("sending initial config notification: %v", c.Context().Err())
	}
	return ch
}

func address(value string) network.SpaceAddresses {
	return network.SpaceAddresses{{
		MachineAddress: network.MachineAddress{Value: value},
	}}
}
