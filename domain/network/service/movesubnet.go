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

// MoveSubnetToSpace moves a list of subnets identified by their UUIDs to a
// specified network space.
// It validates input, computes a new topology, checks its integrity, and
// applies changes if valid.
// Returns the list of moved subnets or an error if any step fails.
func (s *Service) MoveSubnetToSpace(
	ctx context.Context,
	subnetUUIDs []domainnetwork.SubnetUUID,
	spaceName network.SpaceName,
) ([]domainnetwork.MovedSubnets, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	// Validate subnet
	if err := errors.Join(transform.Slice(subnetUUIDs, domainnetwork.SubnetUUID.Validate)...); err != nil {
		return nil, errors.Errorf("invalid subnet UUIDs: %w", err)
	}

	currentTopology, err := s.st.GetAllSpaces(ctx)
	if err != nil {
		return nil, errors.Errorf("getting current topology: %w", err)
	}

	// Validate space
	spaceTo := currentTopology.GetByName(spaceName)
	if spaceTo == nil {
		return nil, errors.Errorf("space %q not found", spaceName)
	}

	// Compute future topology
	movingSubnets, err := s.st.GetSubnets(ctx, transform.Slice(subnetUUIDs, domainnetwork.SubnetUUID.String))
	if err != nil {
		return nil, errors.Errorf("getting moving subnets: %w", err)
	}
	subnetSetIDs := transform.SliceToMap(movingSubnets, func(subnet network.SubnetInfo) (network.Id, struct{}) {
		return subnet.ID, struct{}{}
	})

	newTopology, err := currentTopology.MoveSubnets(subnetSetIDs, spaceName)
	if err != nil {
		return nil, errors.Errorf("building new topology: %w", err)
	}

	// Check the changes are ok
	boundToSpaceMachines, err := s.st.GetMachinesBoundToSpaces(ctx, movingSubnets.SpaceIDs().Values())
	if err != nil {
		return nil, errors.Errorf("getting machines bound to the source spaces: %w", err)
	}
	notCompatibleToSpaceMachines, err := s.st.GetMachinesNotAllowedInSpace(ctx, spaceTo.ID.String())
	if err != nil {
		return nil, errors.Errorf("getting machines allergic to the destination space: %w", err)
	}
	err = errors.Join(
		boundToSpaceMachines.Accept(newTopology),
		notCompatibleToSpaceMachines.Accept(newTopology),
	)
	if err != nil {
		return nil, errors.Capture(err)
	}

	// Apply changes
	subnets, err := newTopology.AllSubnetInfos()
	if err != nil {
		return nil, errors.Errorf("getting subnets: %w", err)
	}
	filtered := subnets[:0]
	for _, subnet := range subnets {
		if _, ok := subnetSetIDs[subnet.ID]; ok {
			filtered = append(filtered, subnet)
		}
	}
	if err := s.st.UpsertSubnets(ctx, filtered); err != nil {
		return nil, errors.Errorf("upserting subnets: %w", err)
	}

	return transform.Slice(movingSubnets, func(subnet network.SubnetInfo) domainnetwork.MovedSubnets {
		return domainnetwork.MovedSubnets{
			UUID:      domainnetwork.SubnetUUID(subnet.ID.String()),
			CIDR:      subnet.CIDR,
			FromSpace: subnet.SpaceName,
			ToSpace:   spaceName,
		}
	}), nil
}
