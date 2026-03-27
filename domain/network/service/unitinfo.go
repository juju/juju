// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"maps"
	"slices"
	"sort"

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
// unit and relation UUID.
//
// The following errors may be returned:
//   - [applicationerrors.UnitNotFound] if the unit does not exist.
//   - [relationerrors.RelationNotFound] if the relation doesn't belong to the
//     unit.
func (s *ProviderService) GetUnitRelationNetwork(ctx context.Context, unitName coreunit.Name,
	relationUUID corerelation.UUID) (domainnetwork.UnitNetwork, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	unitUUID, err := s.st.GetUnitUUIDByName(ctx, unitName)
	if err != nil {
		return domainnetwork.UnitNetwork{}, internalerrors.Capture(err)
	}

	endpointName, err := s.st.GetUnitRelationEndpointName(
		ctx, unitUUID.String(), relationUUID.String(),
	)
	if internalerrors.Is(err, relationerrors.RelationNotFound) {
		return domainnetwork.UnitNetwork{}, relationerrors.RelationNotFound
	} else if err != nil {
		return domainnetwork.UnitNetwork{}, internalerrors.Errorf(
			"getting endpoint name for relation %q: %w", relationUUID, err,
		)
	}

	egressSubnets, err := s.getRelationEgressSubnets(ctx, relationUUID, unitUUID)
	if err != nil {
		return domainnetwork.UnitNetwork{}, internalerrors.Capture(err)
	}

	infos, err := s.getUnitEndpointNetworks(
		ctx, unitUUID.String(), []string{endpointName}, egressSubnets,
	)
	if err != nil {
		return domainnetwork.UnitNetwork{}, internalerrors.Errorf("getting unit endpoint networks: %w", err)
	}
	if len(infos) != 1 {
		// Should not happen unless the interface contract for
		// GetUnitEndpointNetworks is broken.
		// If not broken, providing exactly one endpoint as a parameter for
		// GetUnitEndpointNetworks should return exactly one info.
		return domainnetwork.UnitNetwork{}, internalerrors.Errorf(
			"expected 1 NetworkInfo for unit %q on endpoint %q, got %d",
			unitName, endpointName, len(infos),
		)
	}
	return infos[0], nil
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

	endpointAddresses, err := s.st.GetUnitEndpointNetworkAddresses(
		ctx, unitUUID, endpointNames,
	)
	if err != nil {
		return nil, internalerrors.Errorf("getting unit endpoint addresses: %w", err)
	}

	result := make([]domainnetwork.UnitNetwork, len(endpointAddresses))
	for i, endpointAddresses := range endpointAddresses {
		info := buildUnitNetworkFromAddresses(endpointAddresses.Addresses, isCaas)
		info.EndpointName = endpointAddresses.EndpointName
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

	modelEgressSubnets, err := s.getModelEgressSubnets(ctx)
	if err != nil {
		return nil, internalerrors.Errorf(
			"getting egress subnets for relation %q: %w", relationUUID, err,
		)
	}
	if len(modelEgressSubnets) > 0 {
		return modelEgressSubnets, nil
	}

	publicEgressSubnets, err := s.getUnitPublicEgressSubnets(ctx, unitUUID)
	if err != nil {
		return nil, internalerrors.Errorf(
			"getting egress subnets for relation %q: %w", relationUUID, err,
		)
	}
	return publicEgressSubnets, nil
}

func (s *ProviderService) getModelEgressSubnets(
	ctx context.Context,
) ([]string, error) {
	modelEgressSubnets, err := s.st.GetModelEgressSubnets(ctx)
	if err != nil {
		return nil, internalerrors.Errorf("getting model egress subnets: %w", err)
	}
	return modelEgressSubnets, nil
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

	modelEgressSubnets, err := s.getModelEgressSubnets(ctx)
	if err != nil {
		return nil, internalerrors.Capture(err)
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
	addrs, err := s.st.GetUnitAndK8sServiceAddresses(ctx, unitUUID)
	if err != nil {
		return nil, internalerrors.Errorf(
			"getting unit public addresses for egress fallback: %w", err,
		)
	}

	matchedAddrs := addrs.AllMatchingScope(corenetwork.ScopeMatchPublic)
	if len(matchedAddrs) == 0 {
		return []string{}, nil
	}
	sort.Sort(matchedAddrs)
	return corenetwork.SubnetsForAddresses([]string{matchedAddrs[0].Value}), nil
}

func buildUnitNetworkFromAddresses(
	addresses []networkinternal.UnitAddress,
	isCaas bool,
) domainnetwork.UnitNetwork {
	byDevice := map[string]domainnetwork.DeviceInfo{}
	var ingressAddresses corenetwork.SpaceAddresses
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
		if (!isCaas || addr.Scope != corenetwork.ScopeMachineLocal) &&
			// Addresses on virtual Ethernet devices are never suitable for
			// ingress addresses.
			addr.DeviceType != corenetwork.VirtualEthernetDevice {
			ingressAddresses = append(ingressAddresses, addr.SpaceAddress)
		}

		byDevice[addr.DeviceName] = devInfo
	}

	// We use the same sorting algorithm as in GetUnitAddresses.
	// It is important that the selected address is the same every time for a
	// given set of bindings/devices/addresses.
	sortedIngressAddresses := ingressAddresses.AllMatchingScope(
		corenetwork.ScopeMatchCloudLocal,
	).Values()
	return domainnetwork.UnitNetwork{
		DeviceInfos:      slices.Collect(maps.Values(byDevice)),
		IngressAddresses: sortedIngressAddresses,
	}
}

func (s *ProviderService) getUnitEndpointNetworksWithoutProviderNetworking(
	ctx context.Context, unitUUID string, endpointNames []string, isCaas bool, egressSubnets []string,
) ([]domainnetwork.UnitNetwork, error) {
	addresses, err := s.st.GetUnitNetworkAddresses(ctx, unitUUID)
	if err != nil {
		return nil, internalerrors.Errorf("getting unit addresses: %w", err)
	}
	info := buildUnitNetworkFromAddresses(addresses, isCaas)
	info.EgressSubnets = egressSubnets

	infos := make([]domainnetwork.UnitNetwork, len(endpointNames))
	for i, endpointName := range endpointNames {
		infos[i] = info
		infos[i].EndpointName = endpointName
	}
	return infos, nil
}
