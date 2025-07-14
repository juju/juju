// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/trace"
	"github.com/juju/juju/internal/errors"
)

// GetMachineAddresses retrieves the network space addresses of a machine
// identified by its UUID.
// It performs validation on the UUID and fetches the corresponding network
// node and its associated addresses.
// Returns the network space addresses or an error if any issues are encountered.
func (s *Service) GetMachineAddresses(ctx context.Context, uuid machine.UUID) (network.SpaceAddresses, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	return s.getMachineAddresses(ctx, uuid)
}

// GetMachinePublicAddress retrieves the public address of a machine identified
// by its UUID within the given context.
// It returns the address that matches the public scope or an error if
// no suitable address is found or another error occurs.
func (s *Service) GetMachinePublicAddress(ctx context.Context, uuid machine.UUID) (network.SpaceAddress, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	addresses, err := s.getMachineAddresses(ctx, uuid)
	if err != nil {
		return network.SpaceAddress{}, errors.Capture(err)
	}

	addr, ok := addresses.OneMatchingScope(network.ScopeMatchPublic)
	if !ok {
		return network.SpaceAddress{}, network.NoAddressError(network.ScopePublic.String())
	}
	return addr, nil
}

// GetMachinePrivateAddress retrieves the private address of a machine
// identified by its UUID within a specific context.
// It uses cloud-local scope matching to determine the appropriate address.
// Returns an error if the address is not found.
func (s *Service) GetMachinePrivateAddress(ctx context.Context, uuid machine.UUID) (network.SpaceAddress, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	addresses, err := s.getMachineAddresses(ctx, uuid)
	if err != nil {
		return network.SpaceAddress{}, errors.Capture(err)
	}

	addr, ok := addresses.OneMatchingScope(network.ScopeMatchCloudLocal)
	if !ok {
		return network.SpaceAddress{}, network.NoAddressError("private")
	}
	return addr, nil
}

func (s *Service) getMachineAddresses(ctx context.Context, uuid machine.UUID) (network.SpaceAddresses, error) {
	if err := uuid.Validate(); err != nil {
		return nil, errors.Capture(err)
	}

	netNodeUUID, err := s.st.GetMachineNetNodeUUID(ctx, uuid.String())
	if err != nil {
		return nil, errors.Errorf("retrieving net node for machine %q: %w", uuid, err)
	}

	addresses, err := s.st.GetNetNodeAddresses(ctx, netNodeUUID)
	if err != nil {
		return nil, errors.Errorf("retrieving addresses for net node %q: %w", netNodeUUID, err)
	}

	return addresses, nil
}
