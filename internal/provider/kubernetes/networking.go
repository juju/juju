// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package kubernetes

import (
	"context"

	"github.com/juju/errors"

	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/environs"
)

var _ environs.Networking = (*environNetworking)(nil)

type environNetworking struct {
	environs.NoContainerAddressesEnviron
	environs.NoSpaceDiscoveryEnviron
}

// Subnets is part of the [environs.Networking] interface.
func (environNetworking) Subnets(_ context.Context, _ []network.Id) ([]network.SubnetInfo, error) {
	// Respond with place holder subnets for RI in dqlite until networking
	// in Kubernetes is improved.
	return []network.SubnetInfo{
		{
			CIDR: "0.0.0.0/0",
		},
	}, nil
}

// NetworkInterfaces is part of the [environs.Networking] interface.
func (environNetworking) NetworkInterfaces(ctx context.Context, ids []instance.Id) ([]network.InterfaceInfos, error) {
	return nil, errors.NotSupportedf("network interfaces")
}

// SupportsSpaces is part of the [environs.Networking] interface.
func (environNetworking) SupportsSpaces() (bool, error) {
	return false, nil
}
