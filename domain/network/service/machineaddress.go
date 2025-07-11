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
