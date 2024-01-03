// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/utils/v3"

	"github.com/juju/juju/core/network"
)

// SubnetService provides the API for working with subnets.
type SubnetService struct {
	st SubnetState
}

// NewSunbetService returns a new service reference wrapping the input state.
func NewSubnetService(st SubnetState) *SubnetService {
	return &SubnetService{
		st: st,
	}
}

// AddSubnet creates and returns a new subnet.
func (s *SubnetService) AddSubnet(ctx context.Context, args network.SubnetInfo) (network.Id, error) {
	uuid, err := utils.NewUUID()
	if err != nil {
		return "", errors.Annotatef(err, "creating uuid for new subnet with CIDR %q", args.CIDR)
	}

	if err := s.st.AddSubnet(ctx, uuid.String(), args); err != nil {
		return "", errors.Trace(err)
	}

	return network.Id(uuid.String()), nil
}

// Subnet returns the subnet identified by the input UUID,
// or an error if it is not found.
func (s *SubnetService) Subnet(ctx context.Context, uuid string) (*network.SubnetInfo, error) {
	return s.st.GetSubnet(ctx, uuid)
}

// SubnetsByCIDR returns the subnets matching the input CIDRs.
func (s *SubnetService) SubnetsByCIDR(ctx context.Context, cidrs ...string) ([]network.SubnetInfo, error) {
	return s.st.GetSubnetsByCIDR(ctx, cidrs...)
}

// UpdateSubnet updates the spaceUUID of the subnet identified by the input
// UUID.
func (s *SubnetService) UpdateSubnet(ctx context.Context, uuid, spaceUUID string) error {
	return s.st.UpdateSubnet(ctx, uuid, spaceUUID)
}

// Remove deletes a subnet identified by its uuid.
func (s *SubnetService) Remove(ctx context.Context, uuid string) error {
	return s.st.DeleteSubnet(ctx, uuid)
}
