// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/juju/core/changestream"
	"github.com/juju/juju/core/database"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/eventsource"
	"github.com/juju/juju/environs"
)

// Provider is the interface that the network service requires to be able to
// interact with the underlying provider.
type Provider interface {
	environs.Networking
}

// WatcherFactory describes methods for creating watchers.
type WatcherFactory interface {
	// NewNamespaceMapperWatcher returns a new namespace watcher
	// for events based on the input change mask and mapper.
	NewNamespaceMapperWatcher(namespace string, changeMask changestream.ChangeType, initialStateQuery eventsource.NamespaceQuery, mapper eventsource.Mapper) (watcher.StringsWatcher, error)
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
	// GetSpace returns the space by UUID. If the space is not found, an error
	// is returned matching
	// [github.com/juju/juju/domain/network/errors.SpaceNotFound].
	GetSpace(ctx context.Context, uuid string) (*network.SpaceInfo, error)
	// GetSpaceByName returns the space by name. If the space is not found, an
	// error is returned matching
	// [github.com/juju/juju/domain/network/errors.SpaceNotFound].
	GetSpaceByName(ctx context.Context, name string) (*network.SpaceInfo, error)
	// GetAllSpaces returns all spaces for the model.
	GetAllSpaces(ctx context.Context) (network.SpaceInfos, error)
	// UpdateSpace updates the space identified by the passed uuid. If the
	// space is not found, an error is returned matching
	// [github.com/juju/juju/domain/network/errors.SpaceNotFound].
	UpdateSpace(ctx context.Context, uuid string, name string) error
	// DeleteSpace deletes the space identified by the passed uuid. If the
	// space is not found, an error is returned matching
	// [github.com/juju/juju/domain/network/errors.SpaceNotFound].
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
	// Deprecated: this method should be removed when we re-work the API
	// for moving subnets.
	GetSubnetsByCIDR(ctx context.Context, cidrs ...string) (network.SubnetInfos, error)
	// UpdateSubnet updates the subnet identified by the passed uuid.
	UpdateSubnet(ctx context.Context, uuid string, spaceID string) error
	// DeleteSubnet deletes the subnet identified by the passed uuid.
	DeleteSubnet(ctx context.Context, uuid string) error
	// UpsertSubnets updates or adds each one of the provided subnets in one
	// transaction.
	UpsertSubnets(ctx context.Context, subnets []network.SubnetInfo) error
	// AllSubnetsQuery returns the SQL query that finds all subnet UUIDs from the
	// subnet table, needed for the subnets watcher.
	AllSubnetsQuery(ctx context.Context, db database.TxnRunner) ([]string, error)

	// NamespaceForWatchSubnet returns the namespace identifier used for
	// observing changes to subnets.
	NamespaceForWatchSubnet() string
}
