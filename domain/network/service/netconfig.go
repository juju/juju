// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/collections/transform"

	"github.com/juju/juju/core/machine"
	corenetwork "github.com/juju/juju/core/network"
	"github.com/juju/juju/core/trace"
	"github.com/juju/juju/domain/network"
	"github.com/juju/juju/domain/network/internal"
	"github.com/juju/juju/internal/errors"
)

// DeleteImportedLinkLayerDevices is part of the [modelmigration.MigrationService]
// interface.
func (s *MigrationService) DeleteImportedLinkLayerDevices(ctx context.Context) error {
	return s.st.DeleteImportedLinkLayerDevices(ctx)
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
	subnetUUIDByProviderId := transform.SliceToMap(subnets, func(f corenetwork.SubnetInfo) (string, string) {
		return f.ProviderId.String(), f.ID.String()
	})

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

			device.Addresses, err = transform.SliceOrErr(device.Addresses, func(addr internal.ImportIPAddress) (internal.ImportIPAddress, error) {
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
			})
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

// SetProviderNetConfig merges the existing link layer devices with the
// incoming ones.
func (s *Service) SetProviderNetConfig(
	ctx context.Context,
	machineUUID machine.UUID,
	incoming []network.NetInterface,
) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := machineUUID.Validate(); err != nil {
		return errors.Errorf("invalid machine UUID: %w", err)
	}

	nodeUUID, err := s.st.GetMachineNetNodeUUID(ctx, machineUUID.String())
	if err != nil {
		return errors.Errorf("retrieving net node for machine %q: %w", machineUUID, err)
	}

	return errors.Capture(s.st.MergeLinkLayerDevice(ctx, nodeUUID, incoming))
}

// SetMachineNetConfig updates the detected network configuration for
// the machine with the input UUID.
func (s *Service) SetMachineNetConfig(ctx context.Context, mUUID machine.UUID, nics []network.NetInterface) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	s.logger.Debugf(ctx, "setting network config for machine %q: %#v", mUUID, nics)

	if err := mUUID.Validate(); err != nil {
		return errors.Errorf("invalid machine UUID: %w", err)
	}

	if len(nics) == 0 {
		return nil
	}

	nodeUUID, err := s.st.GetMachineNetNodeUUID(ctx, mUUID.String())
	if err != nil {
		return errors.Errorf("retrieving net node for machine %q: %w", mUUID, err)
	}

	if err := s.st.SetMachineNetConfig(ctx, nodeUUID, nics); err != nil {
		return errors.Errorf("setting net config for machine %q: %w", mUUID, err)
	}

	return nil
}

// GetAllDevicesByMachineNames retrieves all network devices grouped by machine
// names from stored data.
// It fetches devices by node UUIDs and maps them to their respective machine
// names.
// Returns a map of machine names to their network devices or an error if the operation fails.
func (s *Service) GetAllDevicesByMachineNames(ctx context.Context) (map[machine.Name][]network.NetInterface,
	error) {
	devByNodeUUIDs, err := s.st.GetAllLinkLayerDevicesByNetNodeUUIDs(ctx)
	if err != nil {
		return nil, errors.Errorf("retrieving devices by node UUIDs: %w", err)
	}
	machineNamesToNodeUUIDs, err := s.st.AllMachinesAndNetNodes(ctx)
	if err != nil {
		return nil, errors.Errorf("retrieving machine names to UUIDs: %w", err)
	}
	return transform.Map(machineNamesToNodeUUIDs, func(machineName string,
		nodeUUID string) (machine.Name, []network.NetInterface) {
		devs, _ := devByNodeUUIDs[nodeUUID]
		return machine.Name(machineName), devs
	}), nil
}
