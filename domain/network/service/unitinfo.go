// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"maps"
	"slices"
	"strings"

	"github.com/juju/collections/transform"

	corenetwork "github.com/juju/juju/core/network"
	corerelation "github.com/juju/juju/core/relation"
	"github.com/juju/juju/core/trace"
	coreunit "github.com/juju/juju/core/unit"
	domainnetwork "github.com/juju/juju/domain/network"
	networkinternal "github.com/juju/juju/domain/network/internal"
	relationerrors "github.com/juju/juju/domain/relation/errors"
	internalerrors "github.com/juju/juju/internal/errors"
)

// GetUnitRelationNetwork retrieves network relation information for a given
// unit and relation UUIDs.
//
// The following errors may be returned:
//   - [applicationerrors.UnitNotFound] if the unit does not exist.
//   - [relationerrors.RelationNotFound] if the relation doesn't belong to the
//     unit.
func (s *ProviderService) GetUnitRelationNetwork(
	ctx context.Context, unitName coreunit.Name, relationUUIDs []corerelation.UUID,
) (map[corerelation.UUID]domainnetwork.UnitNetwork, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	unitUUID, err := s.st.GetUnitUUIDByName(ctx, unitName)
	if err != nil {
		return nil, internalerrors.Capture(err)
	}

	result := make(map[corerelation.UUID]domainnetwork.UnitNetwork, len(relationUUIDs))
	for _, relationUUID := range relationUUIDs {
		endpointName, err := s.st.GetUnitRelationEndpointName(
			ctx, unitUUID.String(), relationUUID.String(),
		)
		if internalerrors.Is(err, relationerrors.RelationNotFound) {
			return nil, relationerrors.RelationNotFound
		} else if err != nil {
			return nil, internalerrors.Errorf(
				"getting endpoint name for relation %q: %w", relationUUID, err,
			)
		}

		egressSubnets, err := s.getRelationEgressSubnets(ctx, relationUUID, unitUUID)
		if err != nil {
			return nil, internalerrors.Capture(err)
		}

		infos, err := s.getUnitEndpointNetworks(
			ctx, unitUUID.String(), []string{endpointName}, egressSubnets,
		)
		if err != nil {
			return nil, internalerrors.Errorf("getting unit endpoint networks: %w", err)
		}
		if len(infos) != 1 {
			return nil, internalerrors.Errorf(
				"expected 1 NetworkInfo for unit %q on endpoint %q, got %d",
				unitName, endpointName, len(infos),
			)
		}
		result[relationUUID] = infos[0]
	}
	return result, nil
}

// GetUnitEndpointNetworks retrieves network relation information for a given
// unit and specified endpoints.
// It returns exactly one info for each endpoint names passed in argument,
// but doesn't enforce the order. Each info has an endpoint name that should
// match one of the endpoint names, one info for each endpoint names.
// If the provider does not support spaces, we choose the best candidate from
// all unit addresses and return it for all the input endpoints.
//
// The following errors may be returned:
// - [applicationerrors.UnitNotFound] if the unit does not exist.
func (s *ProviderService) GetUnitEndpointNetworks(
	ctx context.Context, unitName coreunit.Name, endpointNames []string,
) ([]domainnetwork.UnitNetwork, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	unitUUID, err := s.st.GetUnitUUIDByName(ctx, unitName)
	if err != nil {
		return nil, internalerrors.Capture(err)
	}

	egressSubnets, err := s.getUnitEgressSubnets(ctx, unitUUID)
	if err != nil {
		return nil, internalerrors.Capture(err)
	}

	return s.getUnitEndpointNetworks(
		ctx, unitUUID.String(), endpointNames, egressSubnets,
	)
}

func (s *ProviderService) getUnitEndpointNetworks(
	ctx context.Context,
	unitUUID string,
	endpointNames []string,
	egressSubnets []string,
) ([]domainnetwork.UnitNetwork, error) {
	supportsNetworking, err := s.supportsNetworking(ctx)
	if err != nil {
		return nil, internalerrors.Errorf("checking provider networking support: %w", err)
	}

	isCaas, err := s.st.IsCaasUnit(ctx, unitUUID)
	if err != nil {
		return nil, internalerrors.Errorf("checking if unit is caas: %w", err)
	}

	if !supportsNetworking {
		return s.getUnitEndpointNetworksWithoutProviderNetworking(
			ctx, unitUUID, endpointNames, isCaas, egressSubnets)
	}

	endpointNetworks, err := s.st.GetUnitEndpointNetworkInfo(
		ctx, unitUUID, endpointNames,
	)
	if err != nil {
		return nil, internalerrors.Errorf("getting unit endpoint network info: %w", err)
	}

	result := make([]domainnetwork.UnitNetwork, len(endpointNetworks))
	for i, endpointNetwork := range endpointNetworks {
		info := buildUnitNetworkWithIngressAddresses(
			endpointNetwork.Addresses,
			endpointNetwork.IngressAddresses,
			isCaas,
		)
		info.EndpointName = endpointNetwork.EndpointName
		info.EgressSubnets = egressSubnets
		result[i] = info
	}

	return result, nil
}

func (s *ProviderService) getRelationEgressSubnets(
	ctx context.Context,
	relationUUID corerelation.UUID,
	unitUUID coreunit.UUID,
) ([]string, error) {
	egressSubnets, err := s.st.GetRelationEgressSubnets(ctx, relationUUID.String())
	if err != nil {
		return nil, internalerrors.Errorf(
			"getting egress subnets for relation %q: %w", relationUUID, err,
		)
	}
	if len(egressSubnets) > 0 {
		return egressSubnets, nil
	}

	modelEgressSubnets, err := s.st.GetModelEgressSubnets(ctx)
	if err != nil {
		return nil, internalerrors.Errorf("getting model egress subnets: %w", err)
	}
	if len(modelEgressSubnets) > 0 {
		return modelEgressSubnets, nil
	}

	publicEgressSubnets, err := s.getUnitPublicEgressSubnets(ctx, unitUUID)
	if err != nil {
		return nil, internalerrors.Errorf(
			"getting fallback egress subnet for unit %q: %w", unitUUID, err,
		)
	}
	return publicEgressSubnets, nil
}

func (s *ProviderService) getUnitEgressSubnets(
	ctx context.Context,
	unitUUID coreunit.UUID,
) ([]string, error) {
	egressSubnets, err := s.st.GetUnitEgressSubnets(ctx, unitUUID.String())
	if err != nil {
		return nil, internalerrors.Errorf("getting unit egress subnets: %w", err)
	}
	if len(egressSubnets) > 0 {
		return egressSubnets, nil
	}

	modelEgressSubnets, err := s.st.GetModelEgressSubnets(ctx)
	if err != nil {
		return nil, internalerrors.Errorf("getting model egress subnets: %w", err)
	}
	if len(modelEgressSubnets) > 0 {
		return modelEgressSubnets, nil
	}

	return s.getUnitPublicEgressSubnets(ctx, unitUUID)
}

func (s *ProviderService) getUnitPublicEgressSubnets(
	ctx context.Context,
	unitUUID coreunit.UUID,
) ([]string, error) {
	address, err := s.st.GetUnitPublicAddressForEgress(ctx, unitUUID.String())
	if err != nil {
		return nil, internalerrors.Errorf(
			"getting unit public address for egress fallback: %w", err,
		)
	}
	if address == "" {
		return []string{}, nil
	}
	return corenetwork.SubnetsForAddresses([]string{normaliseAddress(address)}), nil
}

func buildUnitNetworkWithIngressAddresses(
	addresses []networkinternal.UnitAddress,
	ingressAddresses []string,
	isCaas bool,
) domainnetwork.UnitNetwork {
	byDevice := map[string]domainnetwork.DeviceInfo{}
	for _, addr := range addresses {
		// The purpose of the method is to get connectivity information for
		// the unit. Skip loopback addresses to focus on external connectivity.
		if addr.IP().IsLoopback() {
			continue
		}

		devInfo, ok := byDevice[addr.DeviceName]
		if !ok {
			devInfo.Name = addr.DeviceName
			devInfo.MACAddress = addr.MACAddress
		}

		if !isCaas || addr.Scope == corenetwork.ScopeMachineLocal {
			devInfo.Addresses = append(devInfo.Addresses, domainnetwork.AddressInfo{
				Hostname: addr.Host(),
				Value:    addr.IP().String(),
				CIDR:     addr.AddressCIDR(),
			})
		}
		byDevice[addr.DeviceName] = devInfo
	}
	return domainnetwork.UnitNetwork{
		DeviceInfos:      slices.Collect(maps.Values(byDevice)),
		IngressAddresses: normaliseIngressAddresses(ingressAddresses),
	}
}

func normaliseIngressAddresses(addresses []string) []string {
	return transform.Slice(addresses, func(address string) string {
		return normaliseAddress(address)
	})
}

func normaliseAddress(address string) string {
	before, _, _ := strings.Cut(address, "/")
	return before
}

func (s *ProviderService) getUnitEndpointNetworksWithoutProviderNetworking(
	ctx context.Context, unitUUID string, endpointNames []string, isCaas bool, egressSubnets []string,
) ([]domainnetwork.UnitNetwork, error) {
	unitNetwork, err := s.st.GetUnitNetworkInfo(ctx, unitUUID)
	if err != nil {
		return nil, internalerrors.Errorf(
			"getting unit network info: %w", err,
		)
	}
	info := buildUnitNetworkWithIngressAddresses(
		unitNetwork.Addresses, unitNetwork.IngressAddresses, isCaas,
	)
	info.EgressSubnets = egressSubnets

	infos := make([]domainnetwork.UnitNetwork, len(endpointNames))
	for i, endpointName := range endpointNames {
		infos[i] = info
		infos[i].EndpointName = endpointName
	}
	return infos, nil
}
