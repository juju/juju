// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"fmt"

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

	transformAddress, err := s.newAddressTransformer(ctx)
	if err != nil {
		return errors.Capture(err)
	}

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

// newAddressTransformer creates a function to transform an ImportIPAddress
// by resolving associated subnet details.
func (s *MigrationService) newAddressTransformer(
	ctx context.Context,
) (func(addr internal.ImportIPAddress) (internal.ImportIPAddress, error), error) {
	subnets, err := s.st.GetAllSubnets(ctx)
	if err != nil {
		return nil, errors.Errorf("getting all subnets: %w", err)
	}
	subnetUUIDByProviderId := transform.SliceToMap(subnets, func(f network.SubnetInfo) (string, string) {
		return f.ProviderId.String(), f.ID.String()
	})

	return func(addr internal.ImportIPAddress) (internal.ImportIPAddress, error) {
		if addr.ProviderSubnetID != nil {
			subnetUUID, ok := subnetUUIDByProviderId[*addr.ProviderSubnetID]
			if !ok {
				return addr, errors.Errorf("no subnet found for provider subnet ID %q", *addr.ProviderSubnetID)
			}
			addr.SubnetUUID = subnetUUID
			return addr, nil
		}
		info, err := subnets.GetByCIDR(addr.SubnetCIDR)
		if err != nil {
			return addr, errors.Errorf("getting subnet by CIDR %q: %w", addr.SubnetCIDR, err)
		}
		if len(info) == 0 {
			return addr, errors.Errorf("no subnet found for CIDR %q", addr.SubnetCIDR)
		}
		if len(info) > 1 {
			return addr, errors.Errorf("multiple subnets found for CIDR %q", addr.SubnetCIDR)
		}
		addr.SubnetUUID = info[0].ID.String()
		return addr, nil
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

// getPlaceholderLinkLayerDevices processes the list of cloud services to generate placeholder link layer devices.
func (s *MigrationService) getPlaceholderLinkLayerDevices(
	ctx context.Context,
	services []internal.ImportCloudService,
) ([]internal.ImportLinkLayerDevice, error) {
	spaces, err := s.st.GetAllSpaces(ctx)
	if err != nil {
		return nil, errors.Errorf("getting all spaces: %w", err)
	}

	devices, err := transform.SliceOrErr(services,
		func(service internal.ImportCloudService) (internal.ImportLinkLayerDevice, error) {
			addresses, err := transform.SliceOrErr(service.Addresses,
				func(addr internal.ImportCloudServiceAddress) (internal.ImportIPAddress, error) {
					space := spaces.GetByID(network.SpaceUUID(addr.SpaceID))
					if space == nil {
						return internal.ImportIPAddress{}, errors.Errorf("getting no space for space ID %q",
							addr.SpaceID)
					}
					subnet, err := space.Subnets.GetByAddress(addr.Value)
					if err != nil {
						return internal.ImportIPAddress{},
							errors.Errorf("getting subnet by address %q in space %q: %w", addr.Value, space.Name, err)
					}
					if len(subnet) == 0 {
						return internal.ImportIPAddress{},
							errors.Errorf("no subnet found for address %q in space %q", addr.Value, space.Name)
					}
					if len(subnet) > 1 {
						return internal.ImportIPAddress{}, errors.Errorf("multiple subnets found for address %q in space %q",
							addr.Value, space.Name)
					}
					return internal.ImportIPAddress{
						UUID:         addr.UUID,
						Type:         network.AddressType(addr.Type),
						Scope:        network.Scope(addr.Scope),
						AddressValue: addr.Value,
						ConfigType:   network.ConfigStatic,
						Origin:       network.Origin(addr.Origin),
						SubnetUUID:   subnet[0].ID.String(),
					}, nil
				})
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
