// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/juju/core/machine"
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
	useData := data

	// Net node uuids were created when machines were imported.
	for i, device := range data {
		netNodeUUID, ok := namesToUUIDs[device.MachineID]
		if !ok {
			return errors.Errorf("no net node found for machineID %q", device.MachineID)
		}
		useData[i].NetNodeUUID = netNodeUUID
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
	s.logger.Debugf(ctx, "setting network config for machine %q: %#v", mUUID, nics)

	if err := mUUID.Validate(); err != nil {
		return errors.Capture(err)
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
