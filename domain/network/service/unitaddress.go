// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/trace"
	"github.com/juju/juju/core/unit"
	domainnetwork "github.com/juju/juju/domain/network"
	"github.com/juju/juju/internal/errors"
)

// GetUnitPrivateAddress returns the private address for the specified unit.
// For k8s provider, it will return the first private address of the cloud
// service if any, the first private address of the cloud container otherwise.
// For machines provider, it will return the first private address of the
// machine.
//
// The following errors may be returned:
// - [applicationerrors.UnitNotFound] if the unit does not exist
// - [network.NoAddressError] if the unit has no private address associated
func (s *Service) GetUnitPrivateAddress(ctx context.Context, unitName unit.Name) (network.SpaceAddress, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	unitUUID, err := s.st.GetUnitUUIDByName(ctx, unitName)
	if err != nil {
		return network.SpaceAddress{}, errors.Capture(err)
	}
	addrs, err := s.st.GetUnitAddresses(ctx, unitUUID)
	if err != nil {
		return network.SpaceAddress{}, errors.Capture(err)
	}
	if len(addrs) == 0 {
		return network.SpaceAddress{}, network.NoAddressError("private")
	}

	matchedAddrs := addrs.AllMatchingScope(network.ScopeMatchCloudLocal)
	if len(matchedAddrs) > 0 {
		return matchedAddrs[0], nil
	}

	// AllMatchingScope already falls back to public addresses if no
	// cloud-local address exists. If we still have no match, only
	// unsuitable addresses (localhost, link-local) remain.
	return network.SpaceAddress{}, network.NoAddressError("private")
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
func (s *Service) GetUnitPublicAddress(ctx context.Context, unitName unit.Name) (network.SpaceAddress, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	publicAddresses, err := s.GetUnitPublicAddresses(ctx, unitName)
	if err != nil {
		return network.SpaceAddress{}, errors.Capture(err)
	}
	return publicAddresses[0], nil
}

// GetUnitPublicAddresses returns all public addresses for the specified unit.
//
// The following errors may be returned:
// - [applicationerrors.UnitNotFound] if the unit does not exist
// - [network.NoAddressError] if the unit has no public address associated
func (s *Service) GetUnitPublicAddresses(ctx context.Context, unitName unit.Name) (network.SpaceAddresses, error) {
	unitUUID, err := s.st.GetUnitUUIDByName(ctx, unitName)
	if err != nil {
		return nil, errors.Capture(err)
	}
	addrs, err := s.st.GetUnitAndK8sServiceAddresses(ctx, unitUUID)
	if err != nil {
		return nil, errors.Capture(err)
	}

	matchedAddrs := addrs.AllMatchingScope(network.ScopeMatchPublic)
	if len(matchedAddrs) == 0 {
		return nil, network.NoAddressError(string(network.ScopePublic))
	}

	return matchedAddrs, nil
}

// GetControllerAPIAddresses returns addresses which can be used for API
// addresses for the specified unit. Local-machine scoped addresses will
// not be returned. Non-virtual Ethernet addresses are preferred, with virtual
// Ethernet addresses retained as a fallback. If a management space is
// configured, its candidates are selected independently so that it is always
// honoured.
//
// The following errors may be returned:
//   - [applicationerrors.UnitNotFound] if the unit does not exist or is
//     not a controller application unit.
//   - [network.NoAddressError] if the unit has no suitable API addresses.
func (s *Service) GetControllerAPIAddresses(
	ctx context.Context,
	unitName unit.Name,
	managementSpace *network.SpaceInfo,
) (network.SpaceAddresses, error) {
	unitUUID, err := s.st.GetControllerUnitUUIDByName(ctx, unitName.String())
	if err != nil {
		return nil, errors.Errorf("getting controller unit UUID for %q: %w", unitName, err)
	}

	candidates, err := s.st.GetControllerAPIAddresses(ctx, unitUUID)
	if err != nil {
		return nil, errors.Errorf("getting API addresses for %q: %w", unitName, err)
	}

	addrs := selectControllerAPIAddresses(candidates, managementSpace)
	if len(addrs) == 0 {
		return nil, network.NoAddressError("API")
	}

	return addrs, nil
}

// selectControllerAPIAddresses selects the preferred client addresses from all
// candidates. When a management space is configured, its preferred addresses
// are added to the selection so that agent connectivity honours the explicit
// operator choice.
func selectControllerAPIAddresses(
	candidates domainnetwork.ControllerAPIAddresses,
	managementSpace *network.SpaceInfo,
) network.SpaceAddresses {
	selected := make([]bool, len(candidates))
	markPreferredControllerAPIAddresses(candidates, nil, selected)
	if managementSpace != nil {
		markPreferredControllerAPIAddresses(candidates, &managementSpace.ID, selected)
	}

	result := make(network.SpaceAddresses, 0, len(candidates))
	for i, candidate := range candidates {
		if selected[i] {
			result = append(result, candidate.SpaceAddress)
		}
	}
	return result
}

func markPreferredControllerAPIAddresses(
	candidates domainnetwork.ControllerAPIAddresses,
	spaceUUID *network.SpaceUUID,
	selected []bool,
) {
	hasNonVeth := false
	for _, candidate := range candidates {
		if (spaceUUID == nil || candidate.SpaceID == *spaceUUID) &&
			candidate.DeviceType != domainnetwork.DeviceTypeVeth {
			hasNonVeth = true
			break
		}
	}

	for i, candidate := range candidates {
		if spaceUUID != nil && candidate.SpaceID != *spaceUUID {
			continue
		}
		if !hasNonVeth || candidate.DeviceType != domainnetwork.DeviceTypeVeth {
			selected[i] = true
		}
	}
}
