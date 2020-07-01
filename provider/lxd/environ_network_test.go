// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxd

import (
	jujulxd "github.com/juju/juju/container/lxd"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/environs/context"

	"github.com/golang/mock/gomock"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	lxdapi "github.com/lxc/lxd/shared/api"
	gc "gopkg.in/check.v1"
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
	_, err := env.Subnets(ctx, instance.Id("bogus"), nil)
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *environNetSuite) TestSubnetsForKnownContainer(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	srv := NewMockServer(ctrl)
	srv.EXPECT().FilterContainers("woot").Return([]jujulxd.Container{
		jujulxd.Container{},
	}, nil)
	srv.EXPECT().GetNetworkNames().Return([]string{"lo", "ovs-system", "lxdbr0"}, nil)
	srv.EXPECT().GetNetworkState("lo").Return(&lxdapi.NetworkState{
		Type:  "loopback", // should be filtered out because it's loopback
		State: "up",
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

	env := s.NewEnviron(c, srv, nil).(*environ)

	ctx := context.NewCloudCallContext()
	subnets, err := env.Subnets(ctx, instance.Id("woot"), nil)
	c.Assert(err, jc.ErrorIsNil)

	expSubnets := []network.SubnetInfo{
		{
			CIDR:              "10.55.158.0/24",
			ProviderId:        "subnet-lxdbr0-10.55.158.0/24",
			ProviderNetworkId: "net-lxdbr0",
		},
		{
			CIDR:              "10.42.42.0/24",
			ProviderId:        "subnet-lxdbr0-10.42.42.0/24",
			ProviderNetworkId: "net-lxdbr0",
		},
	}
	c.Assert(subnets, gc.DeepEquals, expSubnets)
}

func (s *environNetSuite) TestSubnetsForKnownContainerAndSubnetFiltering(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	srv := NewMockServer(ctrl)
	srv.EXPECT().FilterContainers("woot").Return([]jujulxd.Container{
		jujulxd.Container{},
	}, nil)
	srv.EXPECT().GetNetworkNames().Return([]string{"lxdbr0"}, nil)
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
	subnets, err := env.Subnets(ctx, instance.Id("woot"), []network.Id{"subnet-lxdbr0-10.55.158.0/24"})
	c.Assert(err, jc.ErrorIsNil)

	expSubnets := []network.SubnetInfo{
		{
			CIDR:              "10.55.158.0/24",
			ProviderId:        "subnet-lxdbr0-10.55.158.0/24",
			ProviderNetworkId: "net-lxdbr0",
		},
	}
	c.Assert(subnets, gc.DeepEquals, expSubnets)
}
