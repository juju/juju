// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/juju/core/database"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/eventsource"
	"github.com/juju/juju/domain/network/internal"
	"github.com/juju/juju/environs"
)

// ProviderWithNetworking describes the interface needed from providers that
// support networking capabilities.
type ProviderWithNetworking interface {
	environs.Networking
}

// ProviderWithZones describes the interface needed from providers that
// support reporting the zones available for use.
type ProviderWithZones interface {
	// AvailabilityZones returns all availability zones in the provider.
	AvailabilityZones(ctx context.Context) (network.AvailabilityZones, error)
}

// WatcherFactory describes methods for creating watchers.
type WatcherFactory interface {
	// NewNamespaceMapperWatcher returns a new watcher that receives changes
	// from the input base watcher's db/queue. Change-log events will be emitted
	// only if the filter accepts them, and dispatching the notifications via
	// the Changes channel, once the mapper has processed them. Filtering of
	// values is done first by the filter, and then by the mapper. Based on the
	// mapper's logic a subset of them (or none) may be emitted. A filter option
	// is required, though additional filter options can be provided.
	NewNamespaceMapperWatcher(
		initialQuery eventsource.NamespaceQuery,
		mapper eventsource.Mapper,
		filterOption eventsource.FilterOption, filterOptions ...eventsource.FilterOption,
	) (watcher.StringsWatcher, error)
}

// State describes retrieval and persistence methods needed for the network
// domain service.
type State interface {
	LinkLayerDeviceState
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
	// IsSpaceUsedInConstraints checks if the provided space name is used in any
	// constraints.
	// This method doesn't check if the provided space name exists, it returns
	// false in that case.
	IsSpaceUsedInConstraints(ctx context.Context, name network.SpaceName) (bool, error)
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

// LinkLayerDeviceState describes persistence layer methods for the
// link layer device (sub-) domain.
type LinkLayerDeviceState interface {
	// AllMachinesAndNetNodes returns all machine names mapped to their
	// net mode UUIDs in the model.
	AllMachinesAndNetNodes(ctx context.Context) (map[string]string, error)

	// DeleteImportedLinkLayerDevices deletes all data added via the ImportLinkLayerDevices
	// method.
	DeleteImportedLinkLayerDevices(ctx context.Context) error

	// ImportLinkLayerDevices adds link layer devices into the model as part
	// of the migration import process.
	ImportLinkLayerDevices(ctx context.Context, input []internal.ImportLinkLayerDevice) error
}
