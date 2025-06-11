// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"sort"

	"github.com/juju/juju/core/machine"
	corenetwork "github.com/juju/juju/core/network"
	"github.com/juju/juju/core/trace"
	"github.com/juju/juju/core/unit"
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

// GetUnitPublicAddress returns the public address for the specified unit.
// For k8s provider, it will return the first public address of the cloud
// service if any, the first public address of the cloud container otherwise.
// For machines provider, it will return the first public address of the
// machine.
//
// The following errors may be returned:
// - [applicationerrors.UnitNotFound] if the unit does not exist
// - [network.NoAddressError] if the unit has no public address associated
func (s *Service) GetUnitPublicAddress(ctx context.Context, unitName unit.Name) (corenetwork.SpaceAddress, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	publicAddresses, err := s.GetUnitPublicAddresses(ctx, unitName)
	if err != nil {
		return corenetwork.SpaceAddress{}, errors.Capture(err)
	}
	return publicAddresses[0], nil
}

// GetUnitPublicAddresses returns all public addresses for the specified unit.
//
// The following errors may be returned:
// - [applicationerrors.UnitNotFound] if the unit does not exist
// - [network.NoAddressError] if the unit has no public address associated
func (s *Service) GetUnitPublicAddresses(ctx context.Context, unitName unit.Name) (corenetwork.SpaceAddresses, error) {
	unitUUID, err := s.st.GetUnitUUIDByName(ctx, unitName)
	if err != nil {
		return nil, errors.Capture(err)
	}
	addrs, err := s.st.GetUnitAndK8sServiceAddresses(ctx, unitUUID)
	if err != nil {
		return nil, errors.Capture(err)
	}

	// First match the scope, then sort by origin.
	matchedAddrs := addrs.AllMatchingScope(corenetwork.ScopeMatchPublic)
	if len(matchedAddrs) == 0 {
		return nil, corenetwork.NoAddressError(string(corenetwork.ScopePublic))
	}
	sort.Slice(matchedAddrs, matchedAddrs.Less)

	return matchedAddrs, nil
}

// GetUnitPrivateAddress returns the private address for the specified unit.
// For k8s provider, it will return the first private address of the cloud
// service if any, the first private address of the cloud container otherwise.
// For machines provider, it will return the first private address of the
// machine.
//
// The following errors may be returned:
// - [applicationerrors.UnitNotFound] if the unit does not exist
// - [network.NoAddressError] if the unit has no private address associated
func (s *Service) GetUnitPrivateAddress(ctx context.Context, unitName unit.Name) (corenetwork.SpaceAddress, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	unitUUID, err := s.st.GetUnitUUIDByName(ctx, unitName)
	if err != nil {
		return corenetwork.SpaceAddress{}, errors.Capture(err)
	}
	addrs, err := s.st.GetUnitAddresses(ctx, unitUUID)
	if err != nil {
		return corenetwork.SpaceAddress{}, errors.Capture(err)
	}
	if len(addrs) == 0 {
		return corenetwork.SpaceAddress{}, corenetwork.NoAddressError("private")
	}

	// First match the scope.
	matchedAddrs := addrs.AllMatchingScope(corenetwork.ScopeMatchCloudLocal)
	if len(matchedAddrs) == 0 {
		// If no address matches the scope, return the first private address.
		return addrs[0], nil
	}
	// Then sort by origin.
	sort.Slice(matchedAddrs, matchedAddrs.Less)

	return matchedAddrs[0], nil
}
