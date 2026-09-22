// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxd

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/canonical/gomock/gomock"
	"github.com/canonical/lxd/shared/api"
	"github.com/juju/clock"
	"github.com/juju/retry"
	"github.com/juju/tc"

	"github.com/juju/juju/internal/container/lxd"
	"github.com/juju/juju/internal/container/lxd/mocks"
	lxdtesting "github.com/juju/juju/internal/container/lxd/testing"
)

type ovnForwardSuite struct {
	ctrl      *gomock.Controller
	srv       *MockServer
	container *lxd.Container
	state     *api.InstanceState
}

func TestOVNForwardSuite(t *testing.T) {
	tc.Run(t, &ovnForwardSuite{})
}

func (s *ovnForwardSuite) SetUpTest(c *tc.C) {
	s.ctrl = gomock.NewController(c)
	s.srv = NewMockServer(s.ctrl)
	s.container = &lxd.Container{Instance: api.Instance{
		Name: "juju-model-0",
		ExpandedDevices: map[string]map[string]string{
			"eth0": {"type": "nic", "network": "ovn0"},
		},
	}}
	s.state = &api.InstanceState{Network: map[string]api.InstanceStateNetwork{
		"eth0": {Addresses: []api.InstanceStateNetworkAddress{{Family: "inet", Address: "10.0.0.2"}}},
	}}
}

func (s *ovnForwardSuite) TestNoNICs(c *tc.C) {
	s.container.ExpandedDevices = nil
	err := ensureOVNNetworkForwards(c.Context(), s.srv, s.container, clock.WallClock)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *ovnForwardSuite) TestNonOVN(c *tc.C) {
	s.container.ExpandedDevices["disk"] = map[string]string{"type": "disk", "network": "ovn1"}
	s.srv.EXPECT().GetNetworks().Return([]api.Network{{Name: "ovn0", Type: "bridge"}, {Name: "ovn1", Type: "ovn"}}, nil)
	err := ensureOVNNetworkForwards(c.Context(), s.srv, s.container, clock.WallClock)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *ovnForwardSuite) TestAllocateForEachOVNNIC(c *tc.C) {
	// The device and guest interface names need not match. The second NIC
	// exercises the fallback to the device name and shares the same network.
	s.container.ExpandedDevices = map[string]map[string]string{
		"device0": {"type": "nic", "network": "ovn0", "name": "eth0"},
		"eth1":    {"type": "nic", "network": "ovn0"},
		"eth2":    {"type": "nic", "parent": "br0"},
	}
	s.state.Network["eth1"] = api.InstanceStateNetwork{
		Addresses: []api.InstanceStateNetworkAddress{{Family: "inet", Address: "10.0.0.3"}},
	}
	s.expectNetworksAndState()
	s.srv.EXPECT().GetNetworkForwards("ovn0").Return(nil, nil).Times(2)
	s.expectCreate(c, "eth0", "10.0.0.2")
	s.expectCreate(c, "eth1", "10.0.0.3")
	err := ensureOVNNetworkForwards(c.Context(), s.srv, s.container, clock.WallClock)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *ovnForwardSuite) TestReuseAndRemoveStaleOrDuplicateForwards(c *tc.C) {
	s.expectNetworksAndState()
	foreign := s.forward("192.0.2.10", "10.0.0.2")
	foreign.Config[jujuInstanceForwardKey] = "other-instance"
	otherNIC := s.forward("192.0.2.11", "10.0.0.2")
	otherNIC.Config[jujuDeviceForwardKey] = "eth1"
	s.srv.EXPECT().GetNetworkForwards("ovn0").Return([]api.NetworkForward{
		s.forward("192.0.2.1", "10.0.0.2"),
		s.forward("192.0.2.2", "10.0.0.9"),
		s.forward("192.0.2.3", "10.0.0.2"),
		foreign, otherNIC, {ListenAddress: "192.0.2.12"},
	}, nil)
	for _, address := range []string{"192.0.2.2", "192.0.2.3"} {
		op := lxdtesting.NewMockOperation(s.ctrl)
		s.srv.EXPECT().DeleteNetworkForward("ovn0", address).Return(op, nil)
		op.EXPECT().WaitContext(c.Context()).Return(nil)
	}
	err := ensureOVNNetworkForwards(c.Context(), s.srv, s.container, clock.WallClock)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *ovnForwardSuite) TestReplaceStaleForward(c *tc.C) {
	s.expectNetworksAndState()
	deleteOp := lxdtesting.NewMockOperation(s.ctrl)
	createOp := lxdtesting.NewMockOperation(s.ctrl)
	gomock.InOrder(
		s.srv.EXPECT().GetNetworkForwards("ovn0").Return([]api.NetworkForward{s.forward("192.0.2.1", "10.0.0.9")}, nil),
		s.srv.EXPECT().DeleteNetworkForward("ovn0", "192.0.2.1").Return(deleteOp, nil),
		deleteOp.EXPECT().WaitContext(c.Context()).Return(nil),
		s.srv.EXPECT().CreateNetworkForward("ovn0", s.request("eth0", "10.0.0.2")).Return(createOp, nil),
		createOp.EXPECT().WaitContext(c.Context()).Return(nil),
	)
	err := ensureOVNNetworkForwards(c.Context(), s.srv, s.container, clock.WallClock)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *ovnForwardSuite) TestRepeatedStartReusesAllocatedForward(c *tc.C) {
	s.expectNetworksAndState()
	s.srv.EXPECT().GetNetworkForwards("ovn0").Return(nil, nil)
	s.expectCreate(c, "eth0", "10.0.0.2")
	err := ensureOVNNetworkForwards(c.Context(), s.srv, s.container, clock.WallClock)
	c.Assert(err, tc.ErrorIsNil)

	s.expectNetworksAndState()
	s.srv.EXPECT().GetNetworkForwards("ovn0").Return([]api.NetworkForward{s.forward("192.0.2.1", "10.0.0.2")}, nil)
	err = ensureOVNNetworkForwards(c.Context(), s.srv, s.container, clock.WallClock)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *ovnForwardSuite) TestStaleForwardAlreadyDeleted(c *tc.C) {
	s.expectNetworksAndState()
	s.srv.EXPECT().GetNetworkForwards("ovn0").Return([]api.NetworkForward{s.forward("192.0.2.1", "10.0.0.9")}, nil)
	s.srv.EXPECT().DeleteNetworkForward("ovn0", "192.0.2.1").Return(nil, api.StatusErrorf(http.StatusNotFound, "gone"))
	s.expectCreate(c, "eth0", "10.0.0.2")
	err := ensureOVNNetworkForwards(c.Context(), s.srv, s.container, clock.WallClock)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *ovnForwardSuite) TestNetworkError(c *tc.C) {
	failure := errors.New("networks unavailable")
	s.srv.EXPECT().GetNetworks().Return(nil, failure)
	err := ensureOVNNetworkForwards(c.Context(), s.srv, s.container, clock.WallClock)
	c.Assert(err, tc.ErrorIs, failure)
}

func (s *ovnForwardSuite) TestStateErrorIsNotRetried(c *tc.C) {
	failure := errors.New("not authorized")
	s.expectNetworks()
	s.srv.EXPECT().GetInstanceState(s.container.Name).Return(nil, "", failure)
	err := ensureOVNNetworkForwards(c.Context(), s.srv, s.container, clock.WallClock)
	c.Assert(err, tc.ErrorIs, failure)
}

func (s *ovnForwardSuite) TestListError(c *tc.C) {
	failure := errors.New("forwards unavailable")
	s.expectNetworksAndState()
	s.srv.EXPECT().GetNetworkForwards("ovn0").Return(nil, failure)
	err := ensureOVNNetworkForwards(c.Context(), s.srv, s.container, clock.WallClock)
	c.Assert(err, tc.ErrorIs, failure)
}

func (s *ovnForwardSuite) TestCreateError(c *tc.C) {
	failure := errors.New("no free external addresses")
	s.expectNetworksAndState()
	s.srv.EXPECT().GetNetworkForwards("ovn0").Return(nil, nil)
	s.srv.EXPECT().CreateNetworkForward("ovn0", s.request("eth0", "10.0.0.2")).Return(nil, failure)
	err := ensureOVNNetworkForwards(c.Context(), s.srv, s.container, clock.WallClock)
	c.Assert(err, tc.ErrorIs, failure)
}

func (s *ovnForwardSuite) TestCreateOperationError(c *tc.C) {
	failure := errors.New("allocation failed")
	s.expectNetworksAndState()
	s.srv.EXPECT().GetNetworkForwards("ovn0").Return(nil, nil)
	op := lxdtesting.NewMockOperation(s.ctrl)
	s.srv.EXPECT().CreateNetworkForward("ovn0", s.request("eth0", "10.0.0.2")).Return(op, nil)
	op.EXPECT().WaitContext(c.Context()).Return(failure)
	err := ensureOVNNetworkForwards(c.Context(), s.srv, s.container, clock.WallClock)
	c.Assert(err, tc.ErrorIs, failure)
}

func (s *ovnForwardSuite) TestDeleteError(c *tc.C) {
	failure := errors.New("delete failed")
	s.expectNetworksAndState()
	s.srv.EXPECT().GetNetworkForwards("ovn0").Return([]api.NetworkForward{s.forward("192.0.2.1", "10.0.0.9")}, nil)
	s.srv.EXPECT().DeleteNetworkForward("ovn0", "192.0.2.1").Return(nil, failure)
	err := ensureOVNNetworkForwards(c.Context(), s.srv, s.container, clock.WallClock)
	c.Assert(err, tc.ErrorIs, failure)
}

func (s *ovnForwardSuite) TestDeleteOperationError(c *tc.C) {
	failure := errors.New("delete failed")
	s.expectNetworksAndState()
	s.srv.EXPECT().GetNetworkForwards("ovn0").Return([]api.NetworkForward{s.forward("192.0.2.1", "10.0.0.9")}, nil)
	op := lxdtesting.NewMockOperation(s.ctrl)
	s.srv.EXPECT().DeleteNetworkForward("ovn0", "192.0.2.1").Return(op, nil)
	op.EXPECT().WaitContext(c.Context()).Return(failure)
	err := ensureOVNNetworkForwards(c.Context(), s.srv, s.container, clock.WallClock)
	c.Assert(err, tc.ErrorIs, failure)
}

func (s *ovnForwardSuite) TestWaitForAllAddresses(c *tc.C) {
	s.expectNetworks()
	s.container.ExpandedDevices["eth1"] = map[string]string{"type": "nic", "network": "ovn0"}
	ready := &api.InstanceState{Network: map[string]api.InstanceStateNetwork{
		"eth0": s.state.Network["eth0"],
		"eth1": {Addresses: []api.InstanceStateNetworkAddress{{Family: "inet", Address: "10.0.0.3"}}},
	}}
	clk := s.retryClock()
	tick := make(chan time.Time, 1)
	tick <- time.Now()
	gomock.InOrder(
		s.srv.EXPECT().GetInstanceState(s.container.Name).Return(s.state, "", nil),
		clk.EXPECT().After(time.Second).Return(tick),
		s.srv.EXPECT().GetInstanceState(s.container.Name).Return(ready, "", nil),
		s.srv.EXPECT().GetNetworkForwards("ovn0").Return(nil, nil).Times(2),
	)
	s.expectCreate(c, "eth0", "10.0.0.2")
	s.expectCreate(c, "eth1", "10.0.0.3")
	err := ensureOVNNetworkForwards(c.Context(), s.srv, s.container, clk)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *ovnForwardSuite) TestAddressWaitBounded(c *tc.C) {
	s.expectNetworks()
	s.srv.EXPECT().GetInstanceState(s.container.Name).Return(&api.InstanceState{}, "", nil).Times(60)
	clk := s.retryClock()
	ticks := make(chan time.Time)
	close(ticks)
	clk.EXPECT().After(time.Second).Return(ticks).Times(59)
	err := ensureOVNNetworkForwards(c.Context(), s.srv, s.container, clk)
	c.Check(retry.IsAttemptsExceeded(err), tc.IsTrue)
	c.Check(err, tc.ErrorMatches, `.*IPv4 address for OVN interface "eth0" not assigned.*`)
}

func (s *ovnForwardSuite) TestAlreadyCancelled(c *tc.C) {
	ctx, cancel := context.WithCancel(c.Context())
	cancel()
	err := ensureOVNNetworkForwards(ctx, s.srv, s.container, clock.WallClock)
	c.Assert(err, tc.ErrorIs, context.Canceled)
}

func (s *ovnForwardSuite) TestCancelAddressWait(c *tc.C) {
	ctx, cancel := context.WithCancel(c.Context())
	defer cancel()
	s.expectNetworks()
	s.srv.EXPECT().GetInstanceState(s.container.Name).Return(&api.InstanceState{}, "", nil)
	clk := s.retryClock()
	clk.EXPECT().After(time.Second).DoAndReturn(func(time.Duration) <-chan time.Time {
		cancel()
		return make(chan time.Time)
	})
	err := ensureOVNNetworkForwards(ctx, s.srv, s.container, clk)
	c.Assert(err, tc.ErrorIs, context.Canceled)
}

func (s *ovnForwardSuite) TestCancelBeforeAllocation(c *tc.C) {
	ctx, cancel := context.WithCancel(c.Context())
	defer cancel()
	s.expectNetworksAndState()
	s.srv.EXPECT().GetNetworkForwards("ovn0").DoAndReturn(func(string) ([]api.NetworkForward, error) {
		cancel()
		return nil, nil
	})
	err := ensureOVNNetworkForwards(ctx, s.srv, s.container, clock.WallClock)
	c.Assert(err, tc.ErrorIs, context.Canceled)
}

func (s *ovnForwardSuite) TestIPv4AddressFiltering(c *tc.C) {
	state := api.InstanceStateNetwork{Addresses: []api.InstanceStateNetworkAddress{
		{Family: "inet", Address: "invalid"},
		{Family: "inet6", Address: "2001:db8::1"},
		{Family: "inet", Address: "127.0.0.1"},
		{Family: "inet", Address: "169.254.0.1"},
		{Family: "inet", Address: "0.0.0.0"},
		{Family: "inet", Address: "224.0.0.1"},
	}}
	c.Check(interfaceIPv4Address(state), tc.Equals, "")
	state.Addresses = append(state.Addresses, api.InstanceStateNetworkAddress{Family: "inet", Address: "10.0.0.2"})
	c.Check(interfaceIPv4Address(state), tc.Equals, "10.0.0.2")
}

func (s *ovnForwardSuite) expectNetworksAndState() {
	s.expectNetworks()
	s.srv.EXPECT().GetInstanceState(s.container.Name).Return(s.state, "", nil)
}

func (s *ovnForwardSuite) expectNetworks() {
	s.srv.EXPECT().GetNetworks().Return([]api.Network{{Name: "ovn0", Type: "ovn"}, {Name: "br0", Type: "bridge"}}, nil)
}

func (s *ovnForwardSuite) retryClock() *mocks.MockClock {
	clk := mocks.NewMockClock(s.ctrl)
	clk.EXPECT().Now().Return(time.Now()).AnyTimes()
	return clk
}

func (s *ovnForwardSuite) expectCreate(c *tc.C, iface, address string) {
	op := lxdtesting.NewMockOperation(s.ctrl)
	s.srv.EXPECT().CreateNetworkForward("ovn0", s.request(iface, address)).Return(op, nil)
	op.EXPECT().WaitContext(c.Context()).Return(nil)
}

func (s *ovnForwardSuite) forward(listen, target string) api.NetworkForward {
	return api.NetworkForward{
		ListenAddress: listen,
		Config: map[string]string{
			"target_address":     target,
			"user.juju-instance": s.container.Name,
			"user.juju-device":   "eth0",
		},
	}
}

func (s *ovnForwardSuite) request(iface, address string) api.NetworkForwardsPost {
	return api.NetworkForwardsPost{
		ListenAddress: "0.0.0.0",
		NetworkForwardPut: api.NetworkForwardPut{
			Description: "Juju instance juju-model-0 interface " + iface,
			Config: map[string]string{
				"target_address":     address,
				"user.juju-instance": s.container.Name,
				"user.juju-device":   iface,
			},
		},
	}
}
