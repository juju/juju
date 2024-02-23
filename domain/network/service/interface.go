// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/juju/core/network"
)

// Environ provides access to the environment.
type Environ interface {
}

// EnvironFactory provides access to the environment identified by the
// environment UUID.
type EnvironFactory interface {
	// Environ returns the environment identified by the passed uuid.
	Environ(ctx context.Context) (Environ, error)
}

// State describes retrieval and persistence methods needed for the network
// domain service.
type State interface {
	SpaceState
	SubnetState
}

// SpaceState describes persistence layer methods for the space (sub-) domain.
type SpaceState interface {
	// AddSpace creates a space.
	AddSpace(ctx context.Context, uuid string, name string, providerID network.Id, subnetIDs []string) error
	// GetSpace returns the space by UUID.
	GetSpace(ctx context.Context, uuid string) (*network.SpaceInfo, error)
	// GetSpaceByName returns the space by name.
	GetSpaceByName(ctx context.Context, name string) (*network.SpaceInfo, error)
	// GetAllSpaces returns all spaces for the model.
	GetAllSpaces(ctx context.Context) (network.SpaceInfos, error)
	// UpdateSpace updates the space identified by the passed uuid.
	UpdateSpace(ctx context.Context, uuid string, name string) error
	// DeleteSpace deletes the space identified by the passed uuid.
	DeleteSpace(ctx context.Context, uuid string) error
}

// SubnetState describes persistence layer methods for the subnet (sub-) domain.
type SubnetState interface {
	// AddSubnet creates a subnet.
	AddSubnet(ctx context.Context, subnet network.SubnetInfo) error
	// GetAllSubnets returns all known subnets in the model.
	GetAllSubnets(ctx context.Context) (network.SubnetInfos, error)
	// GetSubnet returns the subnet by UUID.
	GetSubnet(ctx context.Context, uuid string) (*network.SubnetInfo, error)
	// GetSubnetsByCIDR returns the subnets by CIDR.
	// Deprecated, this method should be removed when we re-work the API
	// for moving subnets.
	GetSubnetsByCIDR(ctx context.Context, cidrs ...string) (network.SubnetInfos, error)
	// UpdateSubnet updates the subnet identified by the passed uuid.
	UpdateSubnet(ctx context.Context, uuid string, spaceID string) error
	// DeleteSubnet deletes the subnet identified by the passed uuid.
	DeleteSubnet(ctx context.Context, uuid string) error
	// UpsertSubnets updates or adds each one of the provided subnets in one
	// transaction.
	UpsertSubnets(ctx context.Context, subnets []network.SubnetInfo) error
}

// Logger facilitates emitting log messages.
type Logger interface {
	Debugf(string, ...interface{})
}
