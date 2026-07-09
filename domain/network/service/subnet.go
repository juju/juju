// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/trace"
	domainnetwork "github.com/juju/juju/domain/network"
	"github.com/juju/juju/internal/errors"
)

// GetAllSubnets returns all the subnets for the model.
func (s *Service) GetAllSubnets(ctx context.Context) (network.SubnetInfos, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()
	allSubnets, err := s.st.GetAllSubnets(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}
	return domainSubnetInfosToCore(allSubnets), nil
}

// SubnetsByCIDR returns the subnets matching the input CIDRs.
func (s *Service) SubnetsByCIDR(ctx context.Context, cidrs ...string) ([]network.SubnetInfo, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()
	subnets, err := s.st.GetSubnetsByCIDR(ctx, cidrs...)
	if err != nil {
		return nil, errors.Capture(err)
	}
	return domainSubnetInfosToCore(subnets), nil
}

// UpdateSubnet updates the spaceUUID of the subnet identified by the input
// UUID.
func (s *Service) UpdateSubnet(ctx context.Context, uuid string, spaceUUID network.SpaceUUID) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()
	return errors.Capture(s.st.UpdateSubnet(ctx, uuid, spaceUUID))
}

// RemoveSubnet deletes a subnet identified by its UUID.
func (s *Service) RemoveSubnet(ctx context.Context, uuid string) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()
	return errors.Capture(s.st.DeleteSubnet(ctx, uuid))
}

// domainSubnetInfoToCore converts a domain subnet info to the core
// representation used at the service boundary.
func domainSubnetInfoToCore(sn domainnetwork.SubnetInfo) network.SubnetInfo {
	return network.SubnetInfo{
		ID:                network.Id(sn.UUID.String()),
		CIDR:              sn.CIDR,
		ProviderId:        sn.ProviderId,
		ProviderSpaceId:   sn.ProviderSpaceId,
		ProviderNetworkId: sn.ProviderNetworkId,
		VLANTag:           sn.VLANTag,
		AvailabilityZones: sn.AvailabilityZones,
		SpaceID:           sn.SpaceID,
		SpaceName:         sn.SpaceName,
	}
}

// domainSubnetInfosToCore converts a slice of domain subnet infos to the core
// representation used at the service boundary.
func domainSubnetInfosToCore(subnets domainnetwork.SubnetInfos) network.SubnetInfos {
	result := make(network.SubnetInfos, len(subnets))
	for i, sn := range subnets {
		result[i] = domainSubnetInfoToCore(sn)
	}
	return result
}
