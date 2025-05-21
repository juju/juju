// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/juju/core/trace"
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
