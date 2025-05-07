// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxd_test

import (
	"context"

	lxdapi "github.com/canonical/lxd/shared/api"
	"github.com/juju/errors"
	"github.com/juju/tc"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/environs"
	environscloudspec "github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/internal/provider/lxd"
)

type environNetSuite struct {
	lxd.EnvironSuite
}

var _ = tc.Suite(&environNetSuite{})

func (s *environNetSuite) TestSubnetsForClustered(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	srv := lxd.NewMockServer(ctrl)

	srv.EXPECT().IsClustered().Return(true)
	srv.EXPECT().GetClusterMembers().Return([]lxdapi.ClusterMember{
		{
			ServerName: "server0",
		},
		{
			ServerName: "server1",
		},
		{
			ServerName: "server2",
		},
	}, nil)

	srv.EXPECT().GetNetworks().Return([]lxdapi.Network{
		{
			Name: "ovs-system",
			Type: "bridge",
		},
		{
			Name: "lxdbr0",
			Type: "bridge",
		},
		// This should be filtered, as it is not a bridge.
		{
			Name: "phys-nic-0",
			Type: "physical",
		},
	}, nil)
	srv.EXPECT().GetNetworkState("ovs-system").Return(&lxdapi.NetworkState{
		Type:  "broadcast",
		State: "down", // should be filtered out because it's down
	}, nil)
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

	invalidator := lxd.NewMockCredentialInvalidator(ctrl)

	env := s.NewEnviron(c, srv, nil, environscloudspec.CloudSpec{}, invalidator).(environs.Networking)

	ctx := context.Background()
	subnets, err := env.Subnets(ctx, nil)
	c.Assert(err, jc.ErrorIsNil)

	expSubnets := []network.SubnetInfo{
		{
			CIDR:              "10.55.158.0/24",
			ProviderId:        "subnet-lxdbr0-10.55.158.0/24",
			ProviderNetworkId: "net-lxdbr0",
			AvailabilityZones: []string{"server0", "server1", "server2"},
		},
		{
			CIDR:              "10.42.42.0/24",
			ProviderId:        "subnet-lxdbr0-10.42.42.0/24",
			ProviderNetworkId: "net-lxdbr0",
			AvailabilityZones: []string{"server0", "server1", "server2"},
		},
	}
	c.Assert(subnets, tc.DeepEquals, expSubnets)
}
func (s *environNetSuite) TestSubnetsForSubnetFiltering(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	srv := lxd.NewMockServer(ctrl)
	srv.EXPECT().GetNetworks().Return([]lxdapi.Network{{
		Name: "lxdbr0",
		Type: "bridge",
	}}, nil)
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
	srv.EXPECT().IsClustered().Return(false)
	srv.EXPECT().Name().Return("locutus")

	invalidator := lxd.NewMockCredentialInvalidator(ctrl)

	env := s.NewEnviron(c, srv, nil, environscloudspec.CloudSpec{}, invalidator).(environs.Networking)

	// Filter list so we only get a single subnet
	ctx := context.Background()
	subnets, err := env.Subnets(ctx, []network.Id{"subnet-lxdbr0-10.55.158.0/24"})
	c.Assert(err, jc.ErrorIsNil)

	expSubnets := []network.SubnetInfo{
		{
			CIDR:              "10.55.158.0/24",
			ProviderId:        "subnet-lxdbr0-10.55.158.0/24",
			ProviderNetworkId: "net-lxdbr0",
			AvailabilityZones: []string{"locutus"},
		},
	}
	c.Assert(subnets, tc.DeepEquals, expSubnets)
}

func (s *environNetSuite) TestNetworkInterfaces(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	srv := lxd.NewMockServer(ctrl)
	srv.EXPECT().GetInstance("woot").Return(&lxdapi.Instance{
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
	srv.EXPECT().GetInstanceState("woot").Return(&lxdapi.InstanceState{
		Network: map[string]lxdapi.InstanceStateNetwork{
			"eth0": {
				Type:   "broadcast",
				State:  "up",
				Mtu:    1500,
				Hwaddr: "00:16:3e:19:29:cb",
				Addresses: []lxdapi.InstanceStateNetworkAddress{
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
				Addresses: []lxdapi.InstanceStateNetworkAddress{
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
				Addresses: []lxdapi.InstanceStateNetworkAddress{
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

	invalidator := lxd.NewMockCredentialInvalidator(ctrl)

	env := s.NewEnviron(c, srv, nil, environscloudspec.CloudSpec{}, invalidator).(environs.Networking)

	ctx := context.Background()
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
				Addresses: network.ProviderAddresses{network.NewMachineAddress(
					"10.55.158.99", network.WithCIDR("10.55.158.0/24"), network.WithConfigType(network.ConfigStatic),
				).AsProviderAddress()},
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
				Addresses: network.ProviderAddresses{network.NewMachineAddress(
					"10.42.42.99", network.WithCIDR("10.42.42.0/24"), network.WithConfigType(network.ConfigStatic),
				).AsProviderAddress()},
			},
		},
	}
	c.Assert(infos, tc.DeepEquals, expInfos)
}

func (s *environNetSuite) TestNetworkInterfacesPartialResults(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	srv := lxd.NewMockServer(ctrl)
	srv.EXPECT().GetInstance("woot").Return(&lxdapi.Instance{
		ExpandedDevices: map[string]map[string]string{
			"eth0": {
				"name":    "eth0",
				"network": "lxdbr0",
				"type":    "nic",
			},
		},
	}, "etag", nil)
	srv.EXPECT().GetInstance("unknown").Return(nil, "", errors.New("not found"))
	srv.EXPECT().GetInstanceState("woot").Return(&lxdapi.InstanceState{
		Network: map[string]lxdapi.InstanceStateNetwork{
			"eth0": {
				Type:   "broadcast",
				State:  "up",
				Mtu:    1500,
				Hwaddr: "00:16:3e:19:29:cb",
				Addresses: []lxdapi.InstanceStateNetworkAddress{
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

	invalidator := lxd.NewMockCredentialInvalidator(ctrl)

	env := s.NewEnviron(c, srv, nil, environscloudspec.CloudSpec{}, invalidator).(environs.Networking)

	ctx := context.Background()
	infos, err := env.NetworkInterfaces(ctx, []instance.Id{"woot", "unknown"})
	c.Assert(err, tc.Equals, environs.ErrPartialInstances, tc.Commentf("expected a partial instances error to be returned if some of the instances were not found"))
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
				Addresses: network.ProviderAddresses{network.NewMachineAddress(
					"10.55.158.99", network.WithCIDR("10.55.158.0/24"), network.WithConfigType(network.ConfigStatic),
				).AsProviderAddress()},
			},
		},
		nil, // slot for second instance is nil as the container was not found
	}
	c.Assert(infos, tc.DeepEquals, expInfos)
}

func (s *environNetSuite) TestNetworkInterfacesNoResults(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	srv := lxd.NewMockServer(ctrl)
	srv.EXPECT().GetInstance("unknown1").Return(nil, "", errors.New("not found"))
	srv.EXPECT().GetInstance("unknown2").Return(nil, "", errors.New("not found"))

	invalidator := lxd.NewMockCredentialInvalidator(ctrl)

	env := s.NewEnviron(c, srv, nil, environscloudspec.CloudSpec{}, invalidator).(environs.Networking)

	ctx := context.Background()
	_, err := env.NetworkInterfaces(ctx, []instance.Id{"unknown1", "unknown2"})
	c.Assert(err, tc.Equals, environs.ErrNoInstances, tc.Commentf("expected a no instances error to be returned if none of the requested instances exists"))
}
