// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"fmt"
	"strings"

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

	subnets, err := s.st.GetAllSubnets(ctx)
	if err != nil {
		return errors.Errorf("getting all subnets: %w", err)
	}

	// Create a map of provider subnet ID to subnet info for quick lookup
	subnetByProviderId := make(map[string]network.SubnetInfo)
	for _, subnet := range subnets {
		subnetByProviderId[subnet.ProviderId.String()] = subnet
	}

	// Process each device
	useData := make([]internal.ImportLinkLayerDevice, 0, len(data))
	for _, device := range data {
		dev, err := s.transformImportLinkLayerDevice(ctx, device, namesToUUIDs, subnets, subnetByProviderId)
		if err != nil {
			return errors.Errorf("converting device %q on machine %q: %w", device.Name, device.MachineID, err)
		}
		useData = append(useData, dev)
	}

	return s.st.ImportLinkLayerDevices(ctx, useData)
}

// transformImportLinkLayerDevice transforms an ImportLinkLayerDevice for
// migration by setting net node UUID and transforming addresses.
// It uses the mapping of names to UUIDs and subnet information for processing.
// Returns the transformed device or an error if processing fails.
func (s *MigrationService) transformImportLinkLayerDevice(
	ctx context.Context,
	device internal.ImportLinkLayerDevice,
	namesToUUIDs map[string]string,
	subnets network.SubnetInfos,
	subnetByProviderId map[string]network.SubnetInfo,
) (internal.ImportLinkLayerDevice, error) {
	// Set the net node UUID
	netNodeUUID, ok := namesToUUIDs[device.MachineID]
	if !ok {
		return internal.ImportLinkLayerDevice{}, errors.Errorf("no net node found for machine")
	}
	device.NetNodeUUID = netNodeUUID

	// Process addresses if any
	if len(device.Addresses) > 0 {
		transformedAddresses := make([]internal.ImportIPAddress, 0, len(device.Addresses))
		for _, addr := range device.Addresses {
			transformedAddr, err := s.transformImportIPAddress(ctx, addr, subnets, subnetByProviderId)
			if err != nil {
				return internal.ImportLinkLayerDevice{}, errors.Errorf("converting address %q: %w",
					addr.AddressValue, err)
			}
			transformedAddresses = append(transformedAddresses, transformedAddr)
		}
		device.Addresses = transformedAddresses
	}
	return device, nil
}

// transformImportIPAddress transforms an ImportIPAddress by finding and setting its subnet UUID
func (s *MigrationService) transformImportIPAddress(
	ctx context.Context,
	addr internal.ImportIPAddress,
	subnets network.SubnetInfos,
	subnetByProviderId map[string]network.SubnetInfo,
) (internal.ImportIPAddress, error) {
	var candidateSubnets network.SubnetInfos

	// If provider subnet ID is provided, use it to find the subnet
	if addr.ProviderSubnetID != nil {
		subnet, ok := subnetByProviderId[*addr.ProviderSubnetID]
		if !ok {
			return addr, errors.Errorf("no subnet found for provider subnet ID %q", *addr.ProviderSubnetID)
		}
		candidateSubnets = network.SubnetInfos{subnet}
	} else {
		// Otherwise, use the subnet CIDR to find matching subnets
		var err error
		candidateSubnets, err = subnets.GetByCIDR(addr.SubnetCIDR)
		if err != nil {
			return addr, errors.Capture(err)
		}
	}
	var err error
	addr.SubnetUUID, err = s.ensureOneSubnet(ctx, candidateSubnets)
	return addr, errors.Capture(err)
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
	spaces, err := s.st.GetAllSpaces(ctx)
	if err != nil {
		return nil, errors.Errorf("getting all spaces: %w", err)
	}

	devices := make([]internal.ImportLinkLayerDevice, 0, len(services))
	for _, service := range services {
		transformedAddresses := make([]internal.ImportIPAddress, 0, len(service.Addresses))
		for _, addr := range service.Addresses {
			transformedAddr, err := s.transformCloudServiceAddress(ctx, addr, spaces)
			if err != nil {
				return nil, errors.Errorf("converting address %q for %q cloud service: %w",
					addr.Value,
					service.ApplicationName,
					err)
			}
			transformedAddresses = append(transformedAddresses, transformedAddr)
		}

		device := internal.ImportLinkLayerDevice{
			UUID:            service.DeviceUUID,
			IsAutoStart:     true,
			IsEnabled:       true,
			NetNodeUUID:     service.NetNodeUUID,
			Name:            fmt.Sprintf("placeholder for %q cloud service", service.ApplicationName),
			Type:            network.UnknownDevice,
			VirtualPortType: network.NonVirtualPort,
			Addresses:       transformedAddresses,
		}
		devices = append(devices, device)
	}

	return devices, nil
}

// transformCloudServiceAddress transforms an ImportCloudServiceAddress by
// finding and setting its subnet UUID
func (s *MigrationService) transformCloudServiceAddress(
	ctx context.Context,
	addr internal.ImportCloudServiceAddress,
	spaces network.SpaceInfos,
) (internal.ImportIPAddress, error) {
	// Convert the address to an ImportIPAddress
	result := internal.ImportIPAddress{
		UUID:         addr.UUID,
		Type:         network.AddressType(addr.Type),
		Scope:        network.Scope(addr.Scope),
		AddressValue: addr.Value,
		ConfigType:   network.ConfigStatic,
		Origin:       network.Origin(addr.Origin),
	}

	// Find the space for this address
	space := spaces.GetByID(network.SpaceUUID(addr.SpaceID))
	if space == nil {
		return result, errors.Errorf("unknown space ID %q", addr.SpaceID)
	}

	// Find subnets for this address
	candidateSubnets, err := space.Subnets.GetByAddress(addr.Value)
	if err != nil {
		return result, errors.Errorf("getting subnets: %w", err)
	}
	result.SubnetUUID, err = s.ensureOneSubnet(ctx, candidateSubnets)
	return result, errors.Capture(err)
}

func (s *MigrationService) ensureOneSubnet(ctx context.Context, subnets network.SubnetInfos) (string, error) {

	// Check if we found any subnets
	if len(subnets) == 0 {
		return "", errors.Errorf("no subnet found")
	}

	// Check if we found too many subnets
	if len(subnets) > 1 {
		cidrs := make([]string, 0, len(subnets))
		for _, info := range subnets {
			cidrs = append(cidrs, info.CIDR)
		}
		return "", errors.Errorf("multiple subnets found: %s",
			strings.Join(cidrs, ","))
	}

	return subnets[0].ID.String(), nil
}
