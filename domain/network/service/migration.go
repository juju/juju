// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/juju/juju/core/logger"
	corenetwork "github.com/juju/juju/core/network"
	"github.com/juju/juju/core/trace"
	"github.com/juju/juju/domain/network"
	"github.com/juju/juju/domain/network/internal"
	"github.com/juju/juju/internal/errors"
)

// MigrationState describes methods required
// for migrating machine network configuration.
type MigrationState interface {
	// AllMachinesAndNetNodes returns all machine names mapped to their
	// net mode UUIDs in the model.
	AllMachinesAndNetNodes(ctx context.Context) (map[string]string, error)

	// CreateCloudServices creates cloud service in state.
	// It creates the associated netnode and link it to the application
	// through the provided application name.
	CreateCloudServices(ctx context.Context, cloudservices []internal.ImportCloudService) error

	// GetAllSpaces returns all spaces for the model.
	GetAllSpaces(ctx context.Context) (corenetwork.SpaceInfos, error)

	// GetAllSubnets returns all known subnets in the model.
	GetAllSubnets(ctx context.Context) (corenetwork.SubnetInfos, error)

	// GetModelCloudType returns the type of the cloud that is in use by this model.
	GetModelCloudType(context.Context) (string, error)

	// ImportLinkLayerDevices adds link layer devices into the model as part
	// of the migration import process.
	ImportLinkLayerDevices(ctx context.Context, input []internal.ImportLinkLayerDevice) error
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

// GetModelCloudType returns the type of the cloud that is in use by this model.
func (s *MigrationService) GetModelCloudType(ctx context.Context) (string, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()
	cloudType, err := s.st.GetModelCloudType(ctx)
	return cloudType, errors.Capture(err)
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
	subnetByProviderId := make(map[string]corenetwork.SubnetInfo)
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
	subnets corenetwork.SubnetInfos,
	subnetByProviderId map[string]corenetwork.SubnetInfo,
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
			transformedAddr, err := s.transformImportIPAddress(addr, subnets, subnetByProviderId)
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
	addr internal.ImportIPAddress,
	subnets corenetwork.SubnetInfos,
	subnetByProviderId map[string]corenetwork.SubnetInfo,
) (internal.ImportIPAddress, error) {
	if addr.ConfigType == corenetwork.ConfigLoopback {
		// Loopback addresses will not have an associated subnet, return the
		// original address.
		return addr, nil
	}

	var candidateSubnets corenetwork.SubnetInfos

	// If provider subnet ID is provided, use it to find the subnet
	if addr.ProviderSubnetID != nil {
		subnet, ok := subnetByProviderId[*addr.ProviderSubnetID]
		if !ok {
			return addr, errors.Errorf("no subnet found for provider subnet ID %q", *addr.ProviderSubnetID)
		}
		candidateSubnets = corenetwork.SubnetInfos{subnet}
	} else {
		// Otherwise, use the subnet CIDR to find matching subnets
		var err error
		candidateSubnets, err = subnets.GetByCIDR(addr.SubnetCIDR)
		if err != nil {
			return addr, errors.Capture(err)
		}
	}
	var err error
	addr.SubnetUUID, err = s.ensureOneSubnet(candidateSubnets)
	return addr, errors.Capture(err)
}

// ImportCloudServices is part of the [modelmigration.MigrationService]
// interface.
func (s *MigrationService) ImportCloudServices(ctx context.Context, services []internal.ImportCloudService) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	// Convert services parameter in internal.ImportLinkLayerDevice with
	// placeholder device to host addresses then call
	//   st.ImportLinkLayerDevices
	llds, err := s.getPlaceholderLinkLayerDevices(ctx, services)
	if err != nil {
		return errors.Errorf("converting services: %w", err)
	}

	// Create the k8s_services and nodes through a call to state (can take
	// directly []ImportCloudService)
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

func (s *MigrationService) getPlaceholderSubnetUUIDByAddressType(ctx context.Context) (map[corenetwork.AddressType]string, error) {
	subnets, err := s.st.GetAllSubnets(ctx)
	if err != nil {
		return nil, errors.Errorf("getting all subnets: %w", err)
	}

	// Note: Today there are only two k8s subnets, which are a placeholders.
	// Finding the subnet for the ip address will be more complex
	// in the future.
	if len(subnets) != 2 {
		return nil, errors.Errorf("expected 2 subnet uuid, got %d", len(subnets))
	}

	result := make(map[corenetwork.AddressType]string)
	for _, subnet := range subnets {
		switch subnet.CIDR {
		case "0.0.0.0/0":
			result[corenetwork.IPv4Address] = subnet.ID.String()
		case "::/0":
			result[corenetwork.IPv6Address] = subnet.ID.String()
		default:
			return nil, errors.Errorf("unexpected k8s subnet CIDR %q", subnet.CIDR)
		}
	}
	return result, nil
}

// getPlaceholderLinkLayerDevices processes the list of cloud services to
// generate placeholder link layer devices for migrated CloudServices
func (s *MigrationService) getPlaceholderLinkLayerDevices(
	ctx context.Context,
	services []internal.ImportCloudService,
) ([]internal.ImportLinkLayerDevice, error) {
	subnetUUIDByAddressType, err := s.getPlaceholderSubnetUUIDByAddressType(ctx)
	if err != nil {
		return nil, errors.Errorf("getting placeholder subnet UUIDs: %w", err)
	}

	devices := make([]internal.ImportLinkLayerDevice, 0, len(services))
	for _, service := range services {
		transformedAddresses := make([]internal.ImportIPAddress, 0, len(service.Addresses))
		for _, addr := range service.Addresses {
			transformedAddr, err := s.transformCloudServiceAddress(addr, subnetUUIDByAddressType)
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
			Type:            network.DeviceTypeUnknown,
			VirtualPortType: corenetwork.NonVirtualPort,
			Addresses:       transformedAddresses,
		}
		devices = append(devices, device)
	}

	return devices, nil
}

// transformCloudServiceAddress transforms an ImportCloudServiceAddress by
// finding and setting its subnet UUID
func (s *MigrationService) transformCloudServiceAddress(
	addr internal.ImportCloudServiceAddress,
	addressTypeSubnetUUID map[corenetwork.AddressType]string,
) (internal.ImportIPAddress, error) {
	addressType := corenetwork.AddressType(addr.Type)
	if err := addressType.Validate(); err != nil {
		return internal.ImportIPAddress{}, err
	}
	subnetUUID, ok := addressTypeSubnetUUID[addressType]
	if !ok {
		return internal.ImportIPAddress{}, errors.Errorf("no subnet UUID found for address type %q", addr.Type)
	}

	// Convert the address to an ImportIPAddress
	return internal.ImportIPAddress{
		UUID:         addr.UUID,
		Type:         addressType,
		Scope:        corenetwork.Scope(addr.Scope),
		AddressValue: addr.Value,
		ConfigType:   corenetwork.ConfigStatic,
		Origin:       corenetwork.Origin(addr.Origin),
		SubnetUUID:   subnetUUID,
	}, nil
}

func (s *MigrationService) ensureOneSubnet(subnets corenetwork.SubnetInfos) (string, error) {
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
