// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"sort"

	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/trace"
	"github.com/juju/juju/core/unit"
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

	// First match the scope.
	matchedAddrs := addrs.AllMatchingScope(network.ScopeMatchCloudLocal)
	if len(matchedAddrs) == 0 {
		// If no address matches the scope, return the first private address.
		return addrs[0], nil
	}
	// Then sort by origin.
	sort.Slice(matchedAddrs, matchedAddrs.Less)

	return matchedAddrs[0], nil
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

	// First match the scope, then sort by origin.
	matchedAddrs := addrs.AllMatchingScope(network.ScopeMatchPublic)
	if len(matchedAddrs) == 0 {
		return nil, network.NoAddressError(string(network.ScopePublic))
	}
	sort.Slice(matchedAddrs, matchedAddrs.Less)

	return matchedAddrs, nil
}

// GetUnitAddressesForAPI returns all addresses which can be used for
// API addresses for the specified unit. local-machine scoped addresses
// will not be returned.
//
// The following errors may be returned:
// - [applicationerrors.UnitNotFound] if the unit does not exist
// - [network.NoAddressError] if the unit has no api address associated
func (s *Service) GetUnitAddressesForAPI(ctx context.Context, unitName unit.Name) (network.SpaceAddresses, error) {
	unitUUID, err := s.st.GetUnitUUIDByName(ctx, unitName)
	if err != nil {
		return nil, errors.Capture(err)
	}
	addrs, err := s.st.GetUnitAndK8sServiceAddresses(ctx, unitUUID)
	if err != nil {
		return nil, errors.Capture(err)
	}

	// First remove local-machine scoped addresses.
	matchedAddrs := make(network.SpaceAddresses, 0)
	for _, addr := range addrs {
		if addr.Scope == network.ScopeMachineLocal {
			continue
		}
		matchedAddrs = append(matchedAddrs, addr)
	}
	if len(matchedAddrs) == 0 {
		return nil, network.NoAddressError("API")
	}

	return matchedAddrs, nil
}
