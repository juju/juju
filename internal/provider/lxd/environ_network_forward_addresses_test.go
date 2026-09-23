// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxd

import (
	"context"
	"errors"
	"testing"

	"github.com/canonical/gomock/gomock"
	"github.com/canonical/lxd/shared/api"
	"github.com/juju/tc"

	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/internal/container/lxd"
)

type forwardAddressSuite struct {
	srv       *MockServer
	env       *environ
	container *api.Instance
}

func TestForwardAddressSuite(t *testing.T) {
	tc.Run(t, &forwardAddressSuite{})
}

func (s *forwardAddressSuite) SetUpTest(c *tc.C) {
	s.srv = NewMockServer(gomock.NewController(c))
	s.env = &environ{serverUnlocked: s.srv}
	s.container = &api.Instance{
		Name: "juju-model-0",
		ExpandedDevices: map[string]map[string]string{
			"device0": {"type": "nic", "network": "ovn0", "name": "eth0"},
			"eth1":    {"type": "nic", "network": "ovn0"},
			"eth2":    {"type": "nic", "parent": "br0"},
			"disk":    {"type": "disk", "network": "other-ovn"},
		},
	}
}

func (s *forwardAddressSuite) TestInstanceAddressesIncludePublicForwards(c *tc.C) {
	guest := network.NewMachineAddress("10.248.0.2").AsProviderAddress()
	s.srv.EXPECT().ContainerAddresses(s.container.Name).Return([]network.ProviderAddress{guest}, nil)
	s.expectInstance()
	s.expectNetworks()
	s.srv.EXPECT().GetNetworkForwards("ovn0").Return([]api.NetworkForward{
		addressForward("10.246.27.235", s.container.Name, "eth0"),
		addressForward("10.246.27.230", s.container.Name, "eth1"),
	}, nil)

	// Resolve current expanded devices from LXD, including profile NICs,
	// rather than depending on the instance's cached devices.
	inst := newInstance(&lxd.Container{Instance: api.Instance{Name: s.container.Name}}, s.env)
	addresses, err := inst.Addresses(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(addresses, tc.DeepEquals, network.ProviderAddresses{
		guest,
		network.NewMachineAddress("10.246.27.235", network.WithScope(network.ScopePublic)).AsProviderAddress(),
		network.NewMachineAddress("10.246.27.230", network.WithScope(network.ScopePublic)).AsProviderAddress(),
	})
	public, ok := addresses.OneMatchingScope(network.ScopeMatchPublic)
	c.Check(ok, tc.IsTrue)
	c.Check(public.Value, tc.Equals, "10.246.27.235")
}

func (s *forwardAddressSuite) TestNetworkInterfacesReportShadowAddresses(c *tc.C) {
	s.srv.EXPECT().GetInstanceState(s.container.Name).Return(&api.InstanceState{
		Network: map[string]api.InstanceStateNetwork{
			"eth0": {
				Type: "broadcast", Mtu: 1442, Hwaddr: "00:16:3e:19:29:cb",
				Addresses: []api.InstanceStateNetworkAddress{{Family: "inet", Address: "10.248.0.2", Netmask: "24"}},
			},
			"eth1": {
				Type: "broadcast", Mtu: 1442, Hwaddr: "00:16:3e:19:29:cc",
				Addresses: []api.InstanceStateNetworkAddress{{Family: "inet", Address: "10.248.0.3", Netmask: "24"}},
			},
		},
	}, "", nil)
	s.expectInstance()
	s.expectNetworks()
	s.srv.EXPECT().GetNetworkForwards("ovn0").Return([]api.NetworkForward{
		addressForward("10.246.27.235", s.container.Name, "eth0"),
		addressForward("10.246.27.230", s.container.Name, "eth1"),
	}, nil)

	infos, err := s.env.NetworkInterfaces(c.Context(), []instance.Id{instance.Id(s.container.Name)})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(infos, tc.DeepEquals, []network.InterfaceInfos{{
		{
			InterfaceName: "eth0", InterfaceType: network.EthernetDevice,
			MACAddress: "00:16:3e:19:29:cb", MTU: 1442, Origin: network.OriginProvider,
			Addresses: network.ProviderAddresses{network.NewMachineAddress(
				"10.248.0.2", network.WithCIDR("10.248.0.0/24"), network.WithConfigType(network.ConfigStatic),
			).AsProviderAddress()},
			ShadowAddresses: network.ProviderAddresses{network.NewMachineAddress(
				"10.246.27.235", network.WithScope(network.ScopePublic),
			).AsProviderAddress()},
		},
		{
			DeviceIndex: 1, InterfaceName: "eth1", InterfaceType: network.EthernetDevice,
			MACAddress: "00:16:3e:19:29:cc", MTU: 1442, Origin: network.OriginProvider,
			Addresses: network.ProviderAddresses{network.NewMachineAddress(
				"10.248.0.3", network.WithCIDR("10.248.0.0/24"), network.WithConfigType(network.ConfigStatic),
			).AsProviderAddress()},
			ShadowAddresses: network.ProviderAddresses{network.NewMachineAddress(
				"10.246.27.230", network.WithScope(network.ScopePublic),
			).AsProviderAddress()},
		},
	}})
}

func (s *forwardAddressSuite) TestOnlyOwnedUsableAddressesForAttachedInterfaces(c *tc.C) {
	s.expectInstance()
	s.expectNetworks()
	forwards := []api.NetworkForward{
		addressForward("10.246.27.235", s.container.Name, "eth0"),
		addressForward("10.246.27.230", s.container.Name, "eth0"),
		addressForward("10.246.27.235", s.container.Name, "eth0"),
		addressForward("10.246.27.231", "other-instance", "eth0"),
		addressForward("10.246.27.232", s.container.Name, "detached"),
		addressForward("10.246.27.233", s.container.Name, "eth2"),
		addressForward("10.246.27.234", s.container.Name, ""),
		{ListenAddress: "10.246.27.236"},
	}
	for _, invalid := range []string{"", "invalid", "0.0.0.0", "127.0.0.1", "169.254.0.1", "224.0.0.1", "255.255.255.255", "::", "2001:db8::1"} {
		forwards = append(forwards, addressForward(invalid, s.container.Name, "eth0"))
	}
	s.srv.EXPECT().GetNetworkForwards("ovn0").Return(forwards, nil)

	addresses, err := ovnForwardAddresses(c.Context(), s.srv, s.container.Name)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(addresses, tc.DeepEquals, map[string]network.ProviderAddresses{
		"eth0": {
			network.NewMachineAddress("10.246.27.230", network.WithScope(network.ScopePublic)).AsProviderAddress(),
			network.NewMachineAddress("10.246.27.235", network.WithScope(network.ScopePublic)).AsProviderAddress(),
		},
	})
}

func (s *forwardAddressSuite) TestNoForwardExtension(c *tc.C) {
	guest := network.NewMachineAddress("10.0.0.2").AsProviderAddress()
	s.srv.EXPECT().ContainerAddresses(s.container.Name).Return([]network.ProviderAddress{guest}, nil)
	s.srv.EXPECT().HasExtension("network_forward").Return(false)
	inst := newInstance(&lxd.Container{Instance: *s.container}, s.env)
	addresses, err := inst.Addresses(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(addresses, tc.DeepEquals, network.ProviderAddresses{guest})
}

func (s *forwardAddressSuite) TestNoNamedNICs(c *tc.C) {
	s.container.ExpandedDevices = map[string]map[string]string{
		"eth0": {"type": "nic", "nictype": "p2p"},
		"disk": {"type": "disk", "network": "ovn0"},
	}
	s.expectInstance()
	addresses, err := ovnForwardAddresses(c.Context(), s.srv, s.container.Name)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(addresses, tc.HasLen, 0)
}

func (s *forwardAddressSuite) TestBridgeOnly(c *tc.C) {
	s.container.ExpandedDevices = map[string]map[string]string{
		"eth0": {"type": "nic", "parent": "br0"},
	}
	s.expectInstance()
	s.expectNetworks()
	addresses, err := ovnForwardAddresses(c.Context(), s.srv, s.container.Name)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(addresses, tc.HasLen, 0)
}

func (s *forwardAddressSuite) TestNoForwards(c *tc.C) {
	s.expectInstance()
	s.expectNetworks()
	s.srv.EXPECT().GetNetworkForwards("ovn0").Return(nil, nil)
	addresses, err := ovnForwardAddresses(c.Context(), s.srv, s.container.Name)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(addresses, tc.HasLen, 0)
}

func (s *forwardAddressSuite) TestInstanceAddressError(c *tc.C) {
	failure := errors.New("state unavailable")
	s.srv.EXPECT().ContainerAddresses(s.container.Name).Return(nil, failure)
	inst := newInstance(&lxd.Container{Instance: *s.container}, s.env)
	_, err := inst.Addresses(c.Context())
	c.Assert(err, tc.ErrorIs, failure)
}

func (s *forwardAddressSuite) TestInstanceLookupError(c *tc.C) {
	failure := errors.New("instance unavailable")
	s.srv.EXPECT().HasExtension("network_forward").Return(true)
	s.srv.EXPECT().GetInstance(s.container.Name).Return(nil, "", failure)
	_, err := ovnForwardAddresses(c.Context(), s.srv, s.container.Name)
	c.Assert(err, tc.ErrorIs, failure)
}

func (s *forwardAddressSuite) TestNetworkLookupError(c *tc.C) {
	failure := errors.New("networks unavailable")
	s.expectInstance()
	s.srv.EXPECT().GetNetworks().Return(nil, failure)
	_, err := ovnForwardAddresses(c.Context(), s.srv, s.container.Name)
	c.Assert(err, tc.ErrorIs, failure)
}

func (s *forwardAddressSuite) TestForwardErrorPropagatedByInstance(c *tc.C) {
	failure := errors.New("forwards unavailable")
	s.srv.EXPECT().ContainerAddresses(s.container.Name).Return(nil, nil)
	s.expectInstance()
	s.expectNetworks()
	s.srv.EXPECT().GetNetworkForwards("ovn0").Return(nil, failure)
	inst := newInstance(&lxd.Container{Instance: *s.container}, s.env)
	_, err := inst.Addresses(c.Context())
	c.Assert(err, tc.ErrorIs, failure)
}

func (s *forwardAddressSuite) TestForwardErrorPropagatedByNetworkInterfaces(c *tc.C) {
	failure := errors.New("forwards unavailable")
	s.srv.EXPECT().GetInstanceState(s.container.Name).Return(&api.InstanceState{
		Network: map[string]api.InstanceStateNetwork{"eth0": {}},
	}, "", nil)
	s.expectInstance()
	s.expectNetworks()
	s.srv.EXPECT().GetNetworkForwards("ovn0").Return(nil, failure)
	_, err := s.env.NetworkInterfaces(c.Context(), []instance.Id{instance.Id(s.container.Name)})
	c.Assert(err, tc.ErrorIs, failure)
}

func (s *forwardAddressSuite) TestCancelled(c *tc.C) {
	ctx, cancel := context.WithCancel(c.Context())
	cancel()
	inst := newInstance(&lxd.Container{Instance: *s.container}, s.env)
	_, err := inst.Addresses(ctx)
	c.Assert(err, tc.ErrorIs, context.Canceled)
	_, err = s.env.NetworkInterfaces(ctx, []instance.Id{instance.Id(s.container.Name)})
	c.Assert(err, tc.ErrorIs, context.Canceled)
}

func (s *forwardAddressSuite) TestCancelledDuringLookup(c *tc.C) {
	ctx, cancel := context.WithCancel(c.Context())
	defer cancel()
	s.expectInstance()
	s.expectNetworks()
	s.srv.EXPECT().GetNetworkForwards("ovn0").DoAndReturn(func(string) ([]api.NetworkForward, error) {
		cancel()
		return []api.NetworkForward{addressForward("10.246.27.235", s.container.Name, "eth0")}, nil
	})
	_, err := ovnForwardAddresses(ctx, s.srv, s.container.Name)
	c.Assert(err, tc.ErrorIs, context.Canceled)
}

func (s *forwardAddressSuite) expectInstance() {
	s.srv.EXPECT().HasExtension("network_forward").Return(true)
	s.srv.EXPECT().GetInstance(s.container.Name).Return(s.container, "", nil)
}

func (s *forwardAddressSuite) expectNetworks() {
	s.srv.EXPECT().GetNetworks().Return([]api.Network{
		{Name: "ovn0", Type: "ovn"},
		{Name: "br0", Type: "bridge"},
		{Name: "other-ovn", Type: "ovn"},
	}, nil)
}

func addressForward(listenAddress, instanceName, iface string) api.NetworkForward {
	return api.NetworkForward{
		ListenAddress: listenAddress,
		Config: map[string]string{
			jujuInstanceForwardKey: instanceName,
			jujuDeviceForwardKey:   iface,
		},
	}
}
