// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	coremachine "github.com/juju/juju/core/machine"
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
			return errors.Errorf("no net node found for machineID %q",
				device.MachineID)
		}
		useData[i].NetNodeUUID = netNodeUUID
	}
	return s.st.ImportLinkLayerDevices(ctx, useData)
}

// MergeLinkLayerDevice merges the existing link layer devices with the
// incoming ones.
func (s *Service) MergeLinkLayerDevice(
	ctx context.Context,
	machineUUID coremachine.UUID,
	incoming []network.NetInterface,
) error {
	if err := machineUUID.Validate(); err != nil {
		return errors.Errorf("invalid machine UUID: %w", err)
	}
	return errors.Capture(s.st.MergeLinkLayerDevice(ctx, machineUUID.String(),
		incoming))
}
