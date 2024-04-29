// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/google/uuid"
	"github.com/juju/errors"

	"github.com/juju/juju/core/network"
)

// AddSubnet creates and returns a new subnet.
func (s *Service) AddSubnet(ctx context.Context, args network.SubnetInfo) (network.Id, error) {
	if args.ID == "" {
		uuid, err := uuid.NewV7()
		if err != nil {
			return "", errors.Annotatef(err, "creating uuid for new subnet with CIDR %q", args.CIDR)
		}
		args.ID = network.Id(uuid.String())
	}

	if err := s.st.AddSubnet(ctx, args); err != nil && !errors.Is(err, errors.AlreadyExists) {
		return "", errors.Trace(err)
	}

	return args.ID, nil
}

// GetAllSubnets returns all the subnets for the model.
func (s *Service) GetAllSubnets(ctx context.Context) (network.SubnetInfos, error) {
	allSubnets, err := s.st.GetAllSubnets(ctx)
	return allSubnets, errors.Trace(err)
}

// Subnet returns the subnet identified by the input UUID,
// or an error if it is not found.
func (s *Service) Subnet(ctx context.Context, uuid string) (*network.SubnetInfo, error) {
	subnet, err := s.st.GetSubnet(ctx, uuid)
	return subnet, errors.Trace(err)
}

// SubnetsByCIDR returns the subnets matching the input CIDRs.
func (s *Service) SubnetsByCIDR(ctx context.Context, cidrs ...string) ([]network.SubnetInfo, error) {
	subnets, err := s.st.GetSubnetsByCIDR(ctx, cidrs...)
	return subnets, errors.Trace(err)
}

// UpdateSubnet updates the spaceUUID of the subnet identified by the input
// UUID.
func (s *Service) UpdateSubnet(ctx context.Context, uuid, spaceUUID string) error {
	return errors.Trace(s.st.UpdateSubnet(ctx, uuid, spaceUUID))
}

// Remove deletes a subnet identified by its uuid.
func (s *Service) RemoveSubnet(ctx context.Context, uuid string) error {
	return errors.Trace(s.st.DeleteSubnet(ctx, uuid))
}
