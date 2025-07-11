// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/collections/transform"

	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/trace"
	domainnetwork "github.com/juju/juju/domain/network"
	"github.com/juju/juju/internal/errors"
)

// MoveSubnetsToSpace moves a list of subnets identified by their UUIDs to a
// specified network space.
// It validates input, computes a new topology, checks its integrity, and
// applies changes if valid.
// Returns the list of moved subnets or an error if any step fails.
func (s *Service) MoveSubnetsToSpace(
	ctx context.Context,
	subnetUUIDs []domainnetwork.SubnetUUID,
	spaceName network.SpaceName,
	force bool,
) ([]domainnetwork.MovedSubnets, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	// Validate subnets
	if err := errors.Join(transform.Slice(subnetUUIDs, domainnetwork.SubnetUUID.Validate)...); err != nil {
		return nil, errors.Errorf("invalid subnet UUIDs: %w", err)
	}

	uuids := transform.Slice(subnetUUIDs, domainnetwork.SubnetUUID.String)
	movingSubnets, err := s.st.MoveSubnetsToSpace(ctx, uuids, spaceName.String(), force)
	if err != nil {
		return nil, errors.Errorf("moving subnets to space %q: %w", spaceName, err)
	}

	return movingSubnets, nil
}
