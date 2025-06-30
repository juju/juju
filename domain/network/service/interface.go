// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/juju/core/database"
	"github.com/juju/juju/core/network"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/eventsource"
	domainnetwork "github.com/juju/juju/domain/network"
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
	SpaceState
	SubnetState
	NetConfigState
	ContainerState
	MigrationState

	// GetMachineNetNodeUUID returns the net node UUID for the input machine UUID.
	GetMachineNetNodeUUID(ctx context.Context, machineUUID string) (string, error)
}

// SpaceState describes persistence layer methods for the space (sub-) domain.
type SpaceState interface {
	// AddSpace creates a space.
	AddSpace(ctx context.Context, uuid network.SpaceUUID, name network.SpaceName, providerID network.Id, subnetIDs []string) error
	// GetSpace returns the space by UUID. If the space is not found, an error
	// is returned matching
	// [github.com/juju/juju/domain/network/errors.SpaceNotFound].
	GetSpace(ctx context.Context, uuid network.SpaceUUID) (*network.SpaceInfo, error)
	// GetSpaceByName returns the space by name. If the space is not found, an
	// error is returned matching
	// [github.com/juju/juju/domain/network/errors.SpaceNotFound].
	GetSpaceByName(ctx context.Context, name network.SpaceName) (*network.SpaceInfo, error)
	// GetAllSpaces returns all spaces for the model.
	GetAllSpaces(ctx context.Context) (network.SpaceInfos, error)
	// UpdateSpace updates the space identified by the passed uuid. If the
	// space is not found, an error is returned matching
	// [github.com/juju/juju/domain/network/errors.SpaceNotFound].
	UpdateSpace(ctx context.Context, uuid network.SpaceUUID, name network.SpaceName) error
	// DeleteSpace deletes the space identified by the passed uuid. If the
	// space is not found, an error is returned matching
	// [github.com/juju/juju/domain/network/errors.SpaceNotFound].
	DeleteSpace(ctx context.Context, uuid network.SpaceUUID) error
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
	UpdateSubnet(ctx context.Context, uuid string, spaceID network.SpaceUUID) error
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

// NetConfigState describes persistence layer methods for
// working with link-layer devices and IP addresses.
type NetConfigState interface {
	// GetUnitAndK8sServiceAddresses returns the addresses of the specified unit.
	// The addresses are taken by unioning the net node UUIDs of the cloud service
	// (if any) and the net node UUIDs of the unit, where each net node has an
	// associated address.
	// This approach allows us to get the addresses regardless of the substrate
	// (k8s or machines).
	//
	// The following errors may be returned:
	// - [uniterrors.UnitNotFound] if the unit does not exist
	GetUnitAndK8sServiceAddresses(ctx context.Context, uuid coreunit.UUID) (network.SpaceAddresses, error)

	// GetUnitAddresses returns the addresses of the specified unit.
	//
	// The following errors may be returned:
	// - [uniterrors.UnitNotFound] if the unit does not exist
	GetUnitAddresses(ctx context.Context, uuid coreunit.UUID) (network.SpaceAddresses, error)

	// GetControllerUnitUUIDByName returns the UUID for the named unit if it
	// is a unit of the controller application.
	//
	// The following errors may be returned:
	// - [applicationerrors.UnitNotFound] if the unit does not exist or is not
	//   a controller application unit.
	GetControllerUnitUUIDByName(context.Context, coreunit.Name) (coreunit.UUID, error)

	// GetUnitUUIDByName returns the UUID for the named unit, returning an
	// error satisfying [applicationerrors.UnitNotFound] if the unit doesn't
	// exist.
	GetUnitUUIDByName(context.Context, coreunit.Name) (coreunit.UUID, error)

	// SetMachineNetConfig updates the network configuration for the machine with
	// the input net node UUID.
	SetMachineNetConfig(ctx context.Context, nodeUUID string, nics []domainnetwork.NetInterface) error

	// GetAllLinkLayerDevicesByNetNodeUUIDs retrieves all link-layer devices
	// grouped by net node UUIDs from the persistence layer.
	// It returns a map where keys are machine UUIDs and values are
	// corresponding network interfaces or an error if retrieval fails.
	GetAllLinkLayerDevicesByNetNodeUUIDs(ctx context.Context) (map[string][]domainnetwork.NetInterface, error)

	// MergeLinkLayerDevice merges the existing link layer devices with the
	// incoming ones.
	MergeLinkLayerDevice(ctx context.Context, machineUUID string, incoming []domainnetwork.NetInterface) error
}
