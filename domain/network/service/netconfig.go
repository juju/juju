// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/collections/transform"

	"github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/trace"
	"github.com/juju/juju/domain/network"
	"github.com/juju/juju/internal/errors"
)

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
		devs := devByNodeUUIDs[nodeUUID]
		return machine.Name(machineName), devs
	}), nil
}
