// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/google/uuid"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/trace"
	"github.com/juju/juju/internal/errors"
)

// AddSubnet creates and returns a new subnet.
func (s *Service) AddSubnet(ctx context.Context, args network.SubnetInfo) (network.Id, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if args.ID == "" {
		uuid, err := uuid.NewV7()
		if err != nil {
			return "", errors.Errorf("creating uuid for new subnet with CIDR %q: %w", args.CIDR, err)
		}
		args.ID = network.Id(uuid.String())
	}

	if err := s.st.AddSubnet(ctx, args); err != nil && !errors.Is(err, coreerrors.AlreadyExists) {
		return "", errors.Capture(err)
	}

	return args.ID, nil
}

// GetAllSubnets returns all the subnets for the model.
func (s *Service) GetAllSubnets(ctx context.Context) (network.SubnetInfos, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()
	allSubnets, err := s.st.GetAllSubnets(ctx)
	return allSubnets, errors.Capture(err)
}

// Subnet returns the subnet identified by the input UUID,
// or an error if it is not found.
func (s *Service) Subnet(ctx context.Context, uuid string) (*network.SubnetInfo, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()
	subnet, err := s.st.GetSubnet(ctx, uuid)
	return subnet, errors.Capture(err)
}

// SubnetsByCIDR returns the subnets matching the input CIDRs.
func (s *Service) SubnetsByCIDR(ctx context.Context, cidrs ...string) ([]network.SubnetInfo, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()
	subnets, err := s.st.GetSubnetsByCIDR(ctx, cidrs...)
	return subnets, errors.Capture(err)
}

// UpdateSubnet updates the spaceUUID of the subnet identified by the input
// UUID.
func (s *Service) UpdateSubnet(ctx context.Context, uuid, spaceUUID string) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()
	return errors.Capture(s.st.UpdateSubnet(ctx, uuid, spaceUUID))
}

// Remove deletes a subnet identified by its uuid.
func (s *Service) RemoveSubnet(ctx context.Context, uuid string) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()
	return errors.Capture(s.st.DeleteSubnet(ctx, uuid))
}
