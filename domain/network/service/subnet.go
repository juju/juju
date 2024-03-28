// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/errors"

	"github.com/juju/juju/core/network"
	"github.com/juju/juju/internal/uuid"
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
	if args.ID == "" {
		uuid, err := uuid.NewUUID()
		if err != nil {
			return "", errors.Annotatef(err, "creating uuid for new subnet with CIDR %q", args.CIDR)
		}
		args.ID = network.Id(uuid.String())
	}

	if err := s.st.AddSubnet(ctx, args); err != nil {
		return "", errors.Trace(err)
	}

	return args.ID, nil
}

// GetAllSubnets returns all the subnets for the model.
func (s *SubnetService) GetAllSubnets(ctx context.Context) (network.SubnetInfos, error) {
	allSubnets, err := s.st.GetAllSubnets(ctx)
	return allSubnets, errors.Trace(err)
}

// Subnet returns the subnet identified by the input UUID,
// or an error if it is not found.
func (s *SubnetService) Subnet(ctx context.Context, uuid string) (*network.SubnetInfo, error) {
	subnet, err := s.st.GetSubnet(ctx, uuid)
	return subnet, errors.Trace(err)
}

// SubnetsByCIDR returns the subnets matching the input CIDRs.
func (s *SubnetService) SubnetsByCIDR(ctx context.Context, cidrs ...string) ([]network.SubnetInfo, error) {
	subnets, err := s.st.GetSubnetsByCIDR(ctx, cidrs...)
	return subnets, errors.Trace(err)
}

// UpdateSubnet updates the spaceUUID of the subnet identified by the input
// UUID.
func (s *SubnetService) UpdateSubnet(ctx context.Context, uuid, spaceUUID string) error {
	return errors.Trace(s.st.UpdateSubnet(ctx, uuid, spaceUUID))
}

// Remove deletes a subnet identified by its uuid.
func (s *SubnetService) Remove(ctx context.Context, uuid string) error {
	return errors.Trace(s.st.DeleteSubnet(ctx, uuid))
}
