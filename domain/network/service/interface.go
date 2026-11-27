// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/juju/core/database"
	"github.com/juju/juju/core/network"
	coreunit "github.com/juju/juju/core/unit"
	domainnetwork "github.com/juju/juju/domain/network"
	networkinternal "github.com/juju/juju/domain/network/internal"
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

// State describes retrieval and persistence methods needed for the network
// domain service.
type State interface {
	SpaceState
	SubnetState
	NetConfigState
	ContainerState
	MigrationState
	NetworkInfoState

	// GetMachineNetNodeUUID returns the net node UUID for the input machine UUID.
	GetMachineNetNodeUUID(ctx context.Context, machineUUID string) (string, error)
}

// SpaceState describes persistence layer methods for the space (sub-) domain.
type SpaceState interface {
	// AddSpace creates a space and atomically associates any subnets
	// matching the supplied CIDRs with the new space. The CIDR-to-subnet
	// resolution and existence check happen inside the same transaction.
	// If any CIDR has no matching subnet an error matching
	// [github.com/juju/juju/domain/network/errors.SubnetNotFound] is
	// returned and no rows are written.
	AddSpace(
		ctx context.Context, uuid network.SpaceUUID, name network.SpaceName, providerID network.Id, cidrs []string,
	) error
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
	// RemoveSpace removes a space from the system, optionally forcing removal,
	// or simulating it via dry run.
	RemoveSpace(
		ctx context.Context, spaceName network.SpaceName, force, dryRun bool,
	) (domainnetwork.RemoveSpaceViolations, error)
	// MoveSubnetsToSpace transfers a list of subnets to a specified network
	// space. It verifies that existing machines will still satisfy their
	// constraints and bindings. The check can be ignored if forced. In this
	// case, failed constraints will be logged.
	// Returns the details of moved subnets or an error if any issue occurs
	// during the operation.
	MoveSubnetsToSpace(ctx context.Context, subnetUUIDs []string, spaceName string,
		force bool) ([]domainnetwork.MovedSubnets, error)
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
	//
	// Deprecated: this method should be removed when we re-work the API
	// for moving subnets.
	GetSubnetsByCIDR(ctx context.Context, cidrs ...string) (network.SubnetInfos, error)
	// UpdateSubnet updates the subnet identified by the passed uuid.
	UpdateSubnet(ctx context.Context, uuid string, spaceID network.SpaceUUID) error
	// DeleteSubnet deletes the subnet identified by the passed uuid.
	DeleteSubnet(ctx context.Context, uuid string) error
	// UpsertSubnets reconciles and adds subnets in one transaction. Existing
	// subnets are matched by provider ID or CIDR and provider network.
	// SubnetInfo.ID is used for inserts; use UpdateSubnet for direct UUID-based
	// space updates. An empty SpaceID preserves an existing subnet's space and
	// defaults a new subnet to the alpha space.
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
	// GetUnitPublicAddresses returns addresses in the best available scope and
	// family for the unit and its cloud service (if any). It falls back from
	// public to cloud-local scope, preferring IPv4 within each scope.
	// Other scopes are excluded. Results are ordered by primary status,
	// IPv4 default-route device, origin and value.
	//
	// The following errors may be returned:
	// - [applicationerrors.UnitNotFound] if the unit does not exist.
	// - [applicationerrors.UnitIsDead] if the unit is dead.
	GetUnitPublicAddresses(ctx context.Context, uuid string) (network.SpaceAddresses, error)

	// GetControllerAPIAddresses returns controller API address candidates for
	// the specified unit, including the supplying device type.
	//
	// The following errors may be returned:
	// - [uniterrors.UnitNotFound] if the unit does not exist.
	GetControllerAPIAddresses(ctx context.Context, uuid string) (domainnetwork.ControllerAPIAddresses, error)

	// GetUnitAddresses returns the addresses of the specified unit.
	//
	// The following errors may be returned:
	// - [uniterrors.UnitNotFound] if the unit does not exist.
	GetUnitAddresses(ctx context.Context, uuid coreunit.UUID) (network.SpaceAddresses, error)

	// GetNetNodeAddresses retrieves network space addresses associated with the
	// given net node UUID. When preferDefaultRoute is true, primary IPv4 addresses
	// on the default route device are preferred within their scope and type.
	GetNetNodeAddresses(
		ctx context.Context, netNodeUUID string, preferDefaultRoute bool,
	) (network.SpaceAddresses, error)

	// GetNetNodePublicAddress returns the preferred public address of a net
	// node, falling back to cloud-local scope. It prefers IPv4 within each
	// scope, then primary status, the IPv4 default-route device,
	// origin and value. It returns a zero address and no error if no suitable
	// address exists. Other scopes are excluded.
	GetNetNodePublicAddress(ctx context.Context, netNodeUUID string) (network.SpaceAddress, error)

	// GetControllerUnitUUIDByName returns the UUID for the named unit if it
	// is a unit of the controller application.
	//
	// The following errors may be returned:
	// - [applicationerrors.UnitNotFound] if the unit does not exist or is not
	//   a controller application unit.
	GetControllerUnitUUIDByName(context.Context, string) (string, error)

	// GetUnitUUIDByName returns the UUID for the named unit, returning an
	// error satisfying [applicationerrors.UnitNotFound] if the unit doesn't
	// exist.
	GetUnitUUIDByName(context.Context, coreunit.Name) (coreunit.UUID, error)

	// IsMachineUnmanaged returns true if the machine
	// with the input UUID is unmanaged.
	IsMachineUnmanaged(ctx context.Context, machineUUID string) (bool, error)

	// SetMachineNetConfig updates the network configuration for the machine
	// with the input net node UUID.
	// If addMissingSubnets is true and any addresses have subnets not yet in
	// the database, those subnets will be inserted.
	SetMachineNetConfig(
		ctx context.Context, nodeUUID string, nics []domainnetwork.NetInterface, addMissingSubnets bool,
	) error

	// GetAllLinkLayerDevicesByNetNodeUUIDs retrieves all link-layer devices
	// grouped by net node UUIDs from the persistence layer.
	// It returns a map where keys are machine UUIDs and values are
	// corresponding network interfaces or an error if retrieval fails.
	GetAllLinkLayerDevicesByNetNodeUUIDs(ctx context.Context) (map[string][]domainnetwork.NetInterface, error)

	// MergeLinkLayerDevice merges the existing link layer devices with the
	// incoming ones.
	MergeLinkLayerDevice(ctx context.Context, machineUUID string, incoming []domainnetwork.NetInterface) error
}

// NetworkInfoState defines a persistence layer interface for retrieving
// network relationship details.
type NetworkInfoState interface {
	// IsCaasUnit returns whether the specified unit is backed by a
	// Kubernetes service.
	IsCaasUnit(ctx context.Context, unitUUID string) (bool, error)

	// GetUnitRelationEndpointName retrieves the endpoint name used by the
	// specified unit in the specified relation.
	GetUnitRelationEndpointName(
		ctx context.Context, unitUUID, relationUUID string,
	) (string, error)

	// GetRelationEgressSubnets retrieves the egress subnets for the specified
	// relation.
	GetRelationEgressSubnets(ctx context.Context, relationUUID string) ([]string, error)

	// GetModelEgressSubnets retrieves the egress-subnets configuration from
	// model config.
	GetModelEgressSubnets(ctx context.Context) ([]string, error)

	// GetUnitEgressSubnets retrieves the egress subnets for the specified
	// unit.
	GetUnitEgressSubnets(ctx context.Context, unitUUID string) ([]string, error)

	// GetUnitPublicAddressForEgress retrieves the best unit address to use
	// when deriving fallback egress subnets.
	GetUnitPublicAddressForEgress(ctx context.Context, unitUUID string) (string, error)

	// GetUnitEndpointNetworkInfo retrieves raw unit addresses and selected
	// ingress addresses for the specified endpoints. It returns one result per
	// endpoint name.
	GetUnitEndpointNetworkInfo(
		ctx context.Context, unitUUID string, endpointNames []string,
	) ([]networkinternal.EndpointNetworkInfo, error)

	// GetUnitNetworkInfo retrieves raw unit addresses and selected ingress
	// addresses for the specified unit when provider networking is not
	// supported.
	GetUnitNetworkInfo(
		ctx context.Context, unitUUID string,
	) (networkinternal.UnitNetworkInfo, error)
}
