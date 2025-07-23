// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/juju/collections/transform"

	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/trace"
	"github.com/juju/juju/domain/network/internal"
	"github.com/juju/juju/internal/errors"
)

// MigrationState describes methods required
// for migrating machine network configuration.
type MigrationState interface {
	// AllMachinesAndNetNodes returns all machine names mapped to their
	// net mode UUIDs in the model.
	AllMachinesAndNetNodes(ctx context.Context) (map[string]string, error)

	// ImportLinkLayerDevices adds link layer devices into the model as part
	// of the migration import process.
	ImportLinkLayerDevices(ctx context.Context, input []internal.ImportLinkLayerDevice) error

	// GetAllSubnets returns all known subnets in the model.
	GetAllSubnets(ctx context.Context) (network.SubnetInfos, error)

	// GetAllSpaces returns all spaces for the model.
	GetAllSpaces(ctx context.Context) (network.SpaceInfos, error)

	// CreateCloudServices creates cloud service in state.
	// It creates the associated netnode and link it to the application
	// through the provided application name.
	CreateCloudServices(ctx context.Context, cloudservices []internal.ImportCloudService) error
}

// MigrationService provides the API for model migration actions within
// the network domain.
type MigrationService struct {
	st     MigrationState
	logger logger.Logger
}

// NewMigrationService returns a new migration service reference wrapping
// the input state. These methods are specific to migration only and not
// intended to be used outside the domain.
func NewMigrationService(st MigrationState, logger logger.Logger) *MigrationService {
	return &MigrationService{
		st:     st,
		logger: logger,
	}
}

// ImportLinkLayerDevices is part of the [modelmigration.MigrationService]
// interface.
func (s *MigrationService) ImportLinkLayerDevices(ctx context.Context, data []internal.ImportLinkLayerDevice) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if len(data) == 0 {
		return nil
	}

	namesToUUIDs, err := s.st.AllMachinesAndNetNodes(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	getSubnets, err := s.findSubnetsForImportIpAddress(ctx)
	if err != nil {
		return errors.Capture(err)
	}
	transformAddress := newAddressTransformer(ctx, ident, getSubnets)
	useData, err := transform.SliceOrErr(data,
		func(device internal.ImportLinkLayerDevice) (internal.ImportLinkLayerDevice, error) {
			netNodeUUID, ok := namesToUUIDs[device.MachineID]
			if !ok {
				return device, errors.Errorf("no net node found for machineID %q", device.MachineID)
			}
			device.NetNodeUUID = netNodeUUID

			if len(device.Addresses) == 0 {
				return device, nil
			}

			device.Addresses, err = transform.SliceOrErr(device.Addresses, transformAddress)
			if err != nil {
				return device, errors.Errorf("converting addresses: %w", err)
			}

			return device, nil
		})
	if err != nil {
		return errors.Errorf("converting devices: %w", err)
	}

	return s.st.ImportLinkLayerDevices(ctx, useData)
}

// findSubnetsForImportIpAddress returns a finder function to find subnets for
// a given imported IP address.
// This function returns a subnet UUID if a ProviderSubnetID is provided, or
// tries to match the subnet through the address subnetCIDR if not.
func (s *MigrationService) findSubnetsForImportIpAddress(ctx context.Context,
) (subnetFinder[internal.ImportIPAddress], error) {
	subnets, err := s.st.GetAllSubnets(ctx)
	if err != nil {
		return nil, errors.Errorf("getting all subnets: %w", err)
	}
	subnetByProviderId := transform.SliceToMap(subnets, func(f network.SubnetInfo) (string, network.SubnetInfo) {
		return f.ProviderId.String(), f
	})

	return func(ctx context.Context, addr internal.ImportIPAddress) (network.SubnetInfos, error) {
		if addr.ProviderSubnetID != nil {
			subnet, ok := subnetByProviderId[*addr.ProviderSubnetID]
			if !ok {
				return nil, errors.Errorf("no subnet found for provider subnet ID %q", *addr.ProviderSubnetID)
			}
			return network.SubnetInfos{subnet}, nil
		}
		result, err := subnets.GetByCIDR(addr.SubnetCIDR)
		return result, errors.Capture(err)
	}, nil
}

// ImportCloudServices is part of the [modelmigration.MigrationService]
// interface.
func (s *MigrationService) ImportCloudServices(ctx context.Context, services []internal.ImportCloudService) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	// Convert services parameter in internal.ImportLinkLayerDevice with placeholder device to host addresses then call
	//   st.ImportLinkLayerDevices
	llds, err := s.getPlaceholderLinkLayerDevices(ctx, services)
	if err != nil {
		return errors.Errorf("converting services: %w", err)
	}

	// Create the k8s_services and nodes through a call to state (can take directly []ImportCloudService)
	err = s.st.CreateCloudServices(ctx, services)
	if err != nil {
		return errors.Errorf("creating cloud services: %w", err)
	}
	err = s.st.ImportLinkLayerDevices(ctx, llds)
	if err != nil {
		return errors.Errorf("importing link layer devices: %w", err)
	}
	return nil
}

// getPlaceholderLinkLayerDevices processes the list of cloud services to
// generate placeholder link layer devices for migrated CloudServices
func (s *MigrationService) getPlaceholderLinkLayerDevices(
	ctx context.Context,
	services []internal.ImportCloudService,
) ([]internal.ImportLinkLayerDevice, error) {

	addressToImport := func(addr internal.ImportCloudServiceAddress) internal.ImportIPAddress {
		return internal.ImportIPAddress{
			UUID:         addr.UUID,
			Type:         network.AddressType(addr.Type),
			Scope:        network.Scope(addr.Scope),
			AddressValue: addr.Value,
			ConfigType:   network.ConfigStatic,
			Origin:       network.Origin(addr.Origin),
		}
	}
	getSubnets, err := s.findSubnetsForImportCloudServiceAddress(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}
	transformAddress := newAddressTransformer(ctx, addressToImport, getSubnets)

	devices, err := transform.SliceOrErr(services,
		func(service internal.ImportCloudService) (internal.ImportLinkLayerDevice, error) {
			addresses, err := transform.SliceOrErr(service.Addresses, transformAddress)
			if err != nil {
				return internal.ImportLinkLayerDevice{}, errors.Errorf("converting addresses: %w", err)
			}
			return internal.ImportLinkLayerDevice{
				UUID:            service.DeviceUUID,
				IsAutoStart:     true,
				IsEnabled:       true,
				NetNodeUUID:     service.NetNodeUUID,
				Name:            fmt.Sprintf("placeholder for %q cloud service", service.ApplicationName),
				Type:            network.UnknownDevice,
				VirtualPortType: network.NonVirtualPort,
				Addresses:       addresses,
			}, nil
		})

	return devices, errors.Capture(err)
}

// findSubnetsForImportCloudServiceAddress returns a finder function to locate
// subnets for a given imported cloud service address.
// It solves the subnet by searching a matching subnet for the address value in
// the input address space.
func (s *MigrationService) findSubnetsForImportCloudServiceAddress(ctx context.Context,
) (subnetFinder[internal.ImportCloudServiceAddress], error) {
	spaces, err := s.st.GetAllSpaces(ctx)
	if err != nil {
		return nil, errors.Errorf("getting all spaces: %w", err)
	}

	return func(ctx context.Context, addr internal.ImportCloudServiceAddress) (network.SubnetInfos, error) {
		space := spaces.GetByID(network.SpaceUUID(addr.SpaceID))
		if space == nil {
			return nil, errors.Errorf("getting no space for space ID %q",
				addr.SpaceID)
		}
		result, err := space.Subnets.GetByAddress(addr.Value)
		return result, errors.Capture(err)
	}, nil
}

// subnetFinder defines a generic function type for finding network subnets
// based on input criteria.
type subnetFinder[T any] func(context.Context, T) (network.SubnetInfos, error)

// toImportIpAddress defines a generic function type that transforms an input of
// any type T into an ImportIPAddress to migrates LinkLayerDevices
type toImportIpAddress[T any] func(T) internal.ImportIPAddress

// ident returns the input value unchanged.
// It is a generic identity function for any type.
func ident[T any](t T) T { return t }

// newAddressTransformer creates a function to transform an address of type T
// into an ImportIPAddress for import operations.
// It uses generic function to transform input to expected output, and a
// specific logic to resolve the subnetUUID, which can vary with the various
// migration use cases.
func newAddressTransformer[T any](
	ctx context.Context,
	toImport toImportIpAddress[T],
	getSubnets subnetFinder[T],
) func(addr T) (internal.ImportIPAddress, error) {
	return func(addr T) (internal.ImportIPAddress, error) {
		result := toImport(addr)
		candidateSubnets, err := getSubnets(ctx, addr)
		if err != nil {
			return result, errors.Errorf("getting subnets for address %q: %w", result.AddressValue, err)
		}
		if len(candidateSubnets) == 0 {
			return result, errors.Errorf("no subnet found for address %q", result.AddressValue)
		}
		if len(candidateSubnets) > 1 {
			return result, errors.Errorf("multiple subnets found for address %q: %s", result.AddressValue,
				strings.Join(transform.Slice(candidateSubnets, func(info network.SubnetInfo) string {
					return info.CIDR
				}), ","))
		}
		result.SubnetUUID = candidateSubnets[0].ID.String()
		return result, nil
	}
}
