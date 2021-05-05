// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxd

import (
	"github.com/golang/mock/gomock"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	lxdapi "github.com/lxc/lxd/shared/api"
	gc "gopkg.in/check.v1"

	jujulxd "github.com/juju/juju/container/lxd"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/context"
)

type environNetSuite struct {
	EnvironSuite
}

var _ = gc.Suite(&environNetSuite{})

func (s *environNetSuite) TestSubnetsForUnknownContainer(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	srv := NewMockServer(ctrl)
	srv.EXPECT().FilterContainers("bogus").Return(nil, nil)

	env := s.NewEnviron(c, srv, nil).(*environ)

	ctx := context.NewCloudCallContext()
	_, err := env.Subnets(ctx, "bogus", nil)
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *environNetSuite) TestSubnetsForServersThatLackRequiredAPIExtensions(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	srv := NewMockServer(ctrl)
	srv.EXPECT().GetNetworkNames().Return(nil, errors.New(`server is missing the required "network" API extension`)).AnyTimes()

	env := s.NewEnviron(c, srv, nil).(*environ)
	ctx := context.NewCloudCallContext()

	// Space support and by extension, subnet detection is not available.
	supportsSpaces, err := env.SupportsSpaces(ctx)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(supportsSpaces, jc.IsFalse, gc.Commentf("expected SupportsSpaces to return false when the lxd server lacks the 'network' extension"))

	// Try to grab subnet details anyway!
	srv.EXPECT().GetServer().Return(&lxdapi.Server{
		Environment: lxdapi.ServerEnvironment{
			ServerName: "locutus",
		},
	}, "", nil)
	_, err = env.Subnets(ctx, instance.UnknownId, nil)
	c.Assert(err, jc.Satisfies, errors.IsNotSupported)
}

func (s *environNetSuite) TestSubnetsForKnownContainer(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	srv := NewMockServer(ctrl)
	srv.EXPECT().FilterContainers("woot").Return([]jujulxd.Container{
		{},
	}, nil)
	srv.EXPECT().GetServer().Return(&lxdapi.Server{
		Environment: lxdapi.ServerEnvironment{
			ServerName: "locutus",
		},
	}, "", nil)
	srv.EXPECT().GetNetworkNames().Return([]string{"ovs-system", "lxdbr0", "phys-nic-0"}, nil)
	srv.EXPECT().GetNetwork("ovs-system").Return(&lxdapi.Network{
		Type: "bridge",
	}, "", nil)
	srv.EXPECT().GetNetworkState("ovs-system").Return(&lxdapi.NetworkState{
		Type:  "broadcast",
		State: "down", // should be filtered out because it's down
	}, nil)
	srv.EXPECT().GetNetwork("lxdbr0").Return(&lxdapi.Network{
		Type: "bridge",
	}, "", nil)
	srv.EXPECT().GetNetworkState("lxdbr0").Return(&lxdapi.NetworkState{
		Type:  "broadcast",
		State: "up",
		Addresses: []lxdapi.NetworkStateAddress{
			{
				Family:  "inet",
				Address: "10.55.158.1",
				Netmask: "24",
				Scope:   "global",
			},
			{
				Family:  "inet",
				Address: "10.42.42.1",
				Netmask: "24",
				Scope:   "global",
			},
			{
				Family:  "inet6",
				Address: "fe80::c876:d1ff:fe9c:fa46",
				Netmask: "64",
				Scope:   "link", // ignored because it has link scope
			},
		},
	}, nil)
	srv.EXPECT().GetNetwork("phys-nic-0").Return(&lxdapi.Network{
		Type: "physical", // should be ignored as it is not a bridge.
	}, "", nil)

	env := s.NewEnviron(c, srv, nil).(*environ)

	ctx := context.NewCloudCallContext()
	subnets, err := env.Subnets(ctx, "woot", nil)
	c.Assert(err, jc.ErrorIsNil)

	expSubnets := []network.SubnetInfo{
		{
			CIDR:              "10.55.158.0/24",
			ProviderId:        "subnet-lxdbr0-10.55.158.0/24",
			ProviderNetworkId: "net-lxdbr0",
			AvailabilityZones: []string{"locutus"},
		},
		{
			CIDR:              "10.42.42.0/24",
			ProviderId:        "subnet-lxdbr0-10.42.42.0/24",
			ProviderNetworkId: "net-lxdbr0",
			AvailabilityZones: []string{"locutus"},
		},
	}
	c.Assert(subnets, gc.DeepEquals, expSubnets)
}

func (s *environNetSuite) TestSubnetsForKnownContainerAndSubnetFiltering(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	srv := NewMockServer(ctrl)
	srv.EXPECT().FilterContainers("woot").Return([]jujulxd.Container{
		{},
	}, nil)
	srv.EXPECT().GetServer().Return(&lxdapi.Server{
		Environment: lxdapi.ServerEnvironment{
			ServerName: "locutus",
		},
	}, "", nil)
	srv.EXPECT().GetNetworkNames().Return([]string{"lxdbr0"}, nil)
	srv.EXPECT().GetNetwork("lxdbr0").Return(&lxdapi.Network{
		Type: "bridge",
	}, "", nil)
	srv.EXPECT().GetNetworkState("lxdbr0").Return(&lxdapi.NetworkState{
		Type:  "broadcast",
		State: "up",
		Addresses: []lxdapi.NetworkStateAddress{
			{
				Family:  "inet",
				Address: "10.55.158.1",
				Netmask: "24",
				Scope:   "global",
			},
			{
				Family:  "inet",
				Address: "10.42.42.1",
				Netmask: "24",
				Scope:   "global",
			},
			{
				Family:  "inet6",
				Address: "fe80::c876:d1ff:fe9c:fa46",
				Netmask: "64",
				Scope:   "link", // ignored because it has link scope
			},
		},
	}, nil)

	env := s.NewEnviron(c, srv, nil).(*environ)

	// Filter list so we only get a single subnet
	ctx := context.NewCloudCallContext()
	subnets, err := env.Subnets(ctx, "woot", []network.Id{"subnet-lxdbr0-10.55.158.0/24"})
	c.Assert(err, jc.ErrorIsNil)

	expSubnets := []network.SubnetInfo{
		{
			CIDR:              "10.55.158.0/24",
			ProviderId:        "subnet-lxdbr0-10.55.158.0/24",
			ProviderNetworkId: "net-lxdbr0",
			AvailabilityZones: []string{"locutus"},
		},
	}
	c.Assert(subnets, gc.DeepEquals, expSubnets)
}

func (s *environNetSuite) TestSubnetDiscoveryFallbackForOlderLXDs(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	srv := NewMockServer(ctrl)

	srv.EXPECT().GetServer().Return(&lxdapi.Server{
		Environment: lxdapi.ServerEnvironment{
			ServerName: "locutus",
		},
	}, "", nil)

	// Even though ovsbr0 is returned by the LXD API, it is *not* bridged
	// into the container we will be introspecting and so this subnet will
	// not be reported back. This is a caveat of the fallback code.
	srv.EXPECT().GetNetworkNames().Return([]string{"ovsbr0", "lxdbr0"}, nil).AnyTimes()
	srv.EXPECT().GetNetwork("ovsbr0").Return(&lxdapi.Network{
		Type: "bridge",
	}, "", nil)

	// This error will trigger the fallback codepath
	srv.EXPECT().GetNetworkState("ovsbr0").Return(nil, errors.New(`server is missing the required "network_state" API extension`))

	// When instance.UnknownID is passed to Subnets, juju will pick the
	// first juju-* container and introspect its bridged devices.
	srv.EXPECT().AliveContainers("juju-").Return([]jujulxd.Container{
		{Container: lxdapi.Container{Name: "juju-badn1c"}},
	}, nil)
	srv.EXPECT().GetContainer("juju-badn1c").Return(&lxdapi.Container{
		ExpandedDevices: map[string]map[string]string{
			"eth0": {
				"name":    "eth0",
				"network": "lxdbr0",
				"type":    "nic",
			},
		},
	}, "etag", nil)
	srv.EXPECT().GetContainerState("juju-badn1c").Return(&lxdapi.ContainerState{
		Network: map[string]lxdapi.ContainerStateNetwork{
			"eth0": {
				Type:   "broadcast",
				State:  "up",
				Mtu:    1500,
				Hwaddr: "00:16:3e:19:29:cb",
				Addresses: []lxdapi.ContainerStateNetworkAddress{
					{
						Family:  "inet",
						Address: "10.55.158.99",
						Netmask: "24",
						Scope:   "global",
					},
					{
						Family:  "inet6",
						Address: "fe80::216:3eff:fe19:29cb",
						Netmask: "64",
						Scope:   "link", // should be ignored as it is link-local
					},
				},
			},
			"lo": {
				Type:   "loopback", // skipped as this is a loopback device
				State:  "up",
				Mtu:    1500,
				Hwaddr: "00:16:3e:19:39:39",
				Addresses: []lxdapi.ContainerStateNetworkAddress{
					{
						Family:  "inet",
						Address: "127.0.0.1",
						Netmask: "8",
						Scope:   "local",
					},
				},
			},
		},
	}, "etag", nil)

	env := s.NewEnviron(c, srv, nil).(*environ)

	ctx := context.NewCloudCallContext()

	// Spaces should be supported
	supportsSpaces, err := env.SupportsSpaces(ctx)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(supportsSpaces, jc.IsTrue)

	// List subnets
	subnets, err := env.Subnets(ctx, instance.UnknownId, nil)
	c.Assert(err, jc.ErrorIsNil)

	expSubnets := []network.SubnetInfo{
		{
			CIDR:              "10.55.158.0/24",
			ProviderId:        "subnet-lxdbr0-10.55.158.0/24",
			ProviderNetworkId: "net-lxdbr0",
			AvailabilityZones: []string{"locutus"},
		},
	}
	c.Assert(subnets, gc.DeepEquals, expSubnets)
}

func (s *environNetSuite) TestNetworkInterfaces(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	srv := NewMockServer(ctrl)
	srv.EXPECT().GetContainer("woot").Return(&lxdapi.Container{
		ExpandedDevices: map[string]map[string]string{
			"eth0": {
				"name":    "eth0",
				"network": "lxdbr0",
				"type":    "nic",
			},
			"eth1": {
				"name":    "eth1",
				"network": "ovsbr0",
				"type":    "nic",
			},
		},
	}, "etag", nil)
	srv.EXPECT().GetContainerState("woot").Return(&lxdapi.ContainerState{
		Network: map[string]lxdapi.ContainerStateNetwork{
			"eth0": {
				Type:   "broadcast",
				State:  "up",
				Mtu:    1500,
				Hwaddr: "00:16:3e:19:29:cb",
				Addresses: []lxdapi.ContainerStateNetworkAddress{
					{
						Family:  "inet",
						Address: "10.55.158.99",
						Netmask: "24",
						Scope:   "global",
					},
					{
						Family:  "inet6",
						Address: "fe80::216:3eff:fe19:29cb",
						Netmask: "64",
						Scope:   "link", // should be ignored as it is link-local
					},
				},
			},
			"lo": {
				Type:   "loopback", // skipped as this is a loopback device
				State:  "up",
				Mtu:    1500,
				Hwaddr: "00:16:3e:19:39:39",
				Addresses: []lxdapi.ContainerStateNetworkAddress{
					{
						Family:  "inet",
						Address: "127.0.0.1",
						Netmask: "8",
						Scope:   "local",
					},
				},
			},
			"eth1": {
				Type:   "broadcast",
				State:  "up",
				Mtu:    1500,
				Hwaddr: "00:16:3e:fe:fe:fe",
				Addresses: []lxdapi.ContainerStateNetworkAddress{
					{
						Family:  "inet",
						Address: "10.42.42.99",
						Netmask: "24",
						Scope:   "global",
					},
				},
			},
		},
	}, "etag", nil)

	env := s.NewEnviron(c, srv, nil).(*environ)

	ctx := context.NewCloudCallContext()
	infos, err := env.NetworkInterfaces(ctx, []instance.Id{"woot"})
	c.Assert(err, jc.ErrorIsNil)
	expInfos := []network.InterfaceInfos{
		{
			{
				DeviceIndex:         0,
				MACAddress:          "00:16:3e:19:29:cb",
				MTU:                 1500,
				InterfaceName:       "eth0",
				ParentInterfaceName: "lxdbr0",
				InterfaceType:       network.EthernetDevice,
				Origin:              network.OriginProvider,
				ProviderId:          "nic-00:16:3e:19:29:cb",
				ProviderSubnetId:    "subnet-lxdbr0-10.55.158.0/24",
				ProviderNetworkId:   "net-lxdbr0",
				Addresses: network.ProviderAddresses{network.NewProviderAddress(
					"10.55.158.99", network.WithCIDR("10.55.158.0/24"), network.WithConfigType(network.ConfigStatic),
				)},
			},
			{
				DeviceIndex:         1,
				MACAddress:          "00:16:3e:fe:fe:fe",
				MTU:                 1500,
				InterfaceName:       "eth1",
				ParentInterfaceName: "ovsbr0",
				InterfaceType:       network.EthernetDevice,
				Origin:              network.OriginProvider,
				ProviderId:          "nic-00:16:3e:fe:fe:fe",
				ProviderSubnetId:    "subnet-ovsbr0-10.42.42.0/24",
				ProviderNetworkId:   "net-ovsbr0",
				Addresses: network.ProviderAddresses{network.NewProviderAddress(
					"10.42.42.99", network.WithCIDR("10.42.42.0/24"), network.WithConfigType(network.ConfigStatic),
				)},
			},
		},
	}
	c.Assert(infos, gc.DeepEquals, expInfos)
}

func (s *environNetSuite) TestNetworkInterfacesPartialResults(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	srv := NewMockServer(ctrl)
	srv.EXPECT().GetContainer("woot").Return(&lxdapi.Container{
		ExpandedDevices: map[string]map[string]string{
			"eth0": {
				"name":    "eth0",
				"network": "lxdbr0",
				"type":    "nic",
			},
		},
	}, "etag", nil)
	srv.EXPECT().GetContainer("unknown").Return(nil, "", errors.New("not found"))
	srv.EXPECT().GetContainerState("woot").Return(&lxdapi.ContainerState{
		Network: map[string]lxdapi.ContainerStateNetwork{
			"eth0": {
				Type:   "broadcast",
				State:  "up",
				Mtu:    1500,
				Hwaddr: "00:16:3e:19:29:cb",
				Addresses: []lxdapi.ContainerStateNetworkAddress{
					{
						Family:  "inet",
						Address: "10.55.158.99",
						Netmask: "24",
						Scope:   "global",
					},
				},
			},
		},
	}, "etag", nil)

	env := s.NewEnviron(c, srv, nil).(*environ)

	ctx := context.NewCloudCallContext()
	infos, err := env.NetworkInterfaces(ctx, []instance.Id{"woot", "unknown"})
	c.Assert(err, gc.Equals, environs.ErrPartialInstances, gc.Commentf("expected a partial instances error to be returned if some of the instances were not found"))
	expInfos := []network.InterfaceInfos{
		{
			{
				DeviceIndex:         0,
				MACAddress:          "00:16:3e:19:29:cb",
				MTU:                 1500,
				InterfaceName:       "eth0",
				ParentInterfaceName: "lxdbr0",
				InterfaceType:       network.EthernetDevice,
				Origin:              network.OriginProvider,
				ProviderId:          "nic-00:16:3e:19:29:cb",
				ProviderSubnetId:    "subnet-lxdbr0-10.55.158.0/24",
				ProviderNetworkId:   "net-lxdbr0",
				Addresses: network.ProviderAddresses{network.NewProviderAddress(
					"10.55.158.99", network.WithCIDR("10.55.158.0/24"), network.WithConfigType(network.ConfigStatic),
				)},
			},
		},
		nil, // slot for second instance is nil as the container was not found
	}
	c.Assert(infos, gc.DeepEquals, expInfos)
}

func (s *environNetSuite) TestNetworkInterfacesNoResults(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	srv := NewMockServer(ctrl)
	srv.EXPECT().GetContainer("unknown1").Return(nil, "", errors.New("not found"))
	srv.EXPECT().GetContainer("unknown2").Return(nil, "", errors.New("not found"))

	env := s.NewEnviron(c, srv, nil).(*environ)

	ctx := context.NewCloudCallContext()
	_, err := env.NetworkInterfaces(ctx, []instance.Id{"unknown1", "unknown2"})
	c.Assert(err, gc.Equals, environs.ErrNoInstances, gc.Commentf("expected a no instances error to be returned if none of the requested instances exists"))
}
