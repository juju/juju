// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"net"

	"github.com/juju/juju/core/network"
	corerelation "github.com/juju/juju/core/relation"
	"github.com/juju/juju/core/trace"
	"github.com/juju/juju/core/watcher/eventsource"
	crossmodelrelationerrors "github.com/juju/juju/domain/crossmodelrelation/errors"
	"github.com/juju/juju/internal/errors"
	internalnetwork "github.com/juju/juju/internal/network"
)

// ModelRelationNetworkState describes retrieval and persistence methods for
// relation network ingress in the model database.
type ModelRelationNetworkState interface {
	// AddRelationNetworkIngress adds ingress network CIDRs for the specified
	// relation.
	AddRelationNetworkIngress(ctx context.Context, relationUUID string, cidrs []string) error

	// GetRelationNetworkIngress retrieves all ingress network CIDRs for the
	// specified relation.
	GetRelationNetworkIngress(ctx context.Context, relationUUID string) ([]string, error)

	// GetRelationNetworkEgress retrieves all egress network CIDRs for the
	// specified relation.
	GetRelationNetworkEgress(ctx context.Context, relationUUID string) ([]string, error)

	// NamespaceForRelationIngressNetworksWatcher returns the namespace of the
	// relation_network_ingress table, used for the watcher.
	NamespaceForRelationIngressNetworksWatcher() string

	// NamespaceForRelationEgressNetworksWatcher returns the namespaces of the
	// tables needed for the relation egress networks watcher.
	NamespaceForRelationEgressNetworksWatcher() (string, string, string)

	// InitialWatchStatementForRelationEgressNetworks returns the initial query
	// for watching relation egress networks.
	InitialWatchStatementForRelationEgressNetworks(relationUUID string) eventsource.NamespaceQuery

	// GetUnitAddressesForRelation returns all unit addresses for units that are
	// part of the specified relation, grouped by unit UUID.
	GetUnitAddressesForRelation(ctx context.Context, relationUUID string) (map[string]network.SpaceAddresses, error)

	// GetModelEgressSubnets returns the egress-subnets configuration from model config.
	GetModelEgressSubnets(ctx context.Context) ([]string, error)
}

// AddRelationNetworkIngress adds ingress network CIDRs for the specified
// relation.
// The CIDRs are added to the relation_network_ingress table.
func (s *Service) AddRelationNetworkIngress(ctx context.Context, relationUUID corerelation.UUID, saasIngressAllow []string, cidrs []string) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := relationUUID.Validate(); err != nil {
		return errors.Capture(err)
	}

	// Validate CIDRs are not empty and are valid
	if err := s.validateIngressNetworks(saasIngressAllow, cidrs); err != nil {
		return errors.Capture(err)
	}

	if err := s.modelState.AddRelationNetworkIngress(ctx, relationUUID.String(), cidrs); err != nil {
		return errors.Capture(err)
	}

	return nil
}

// GetRelationNetworkIngress retrieves all ingress network CIDRs for the
// specified relation.
// The CIDRs are retrieved from the relation_network_ingress table.
func (s *Service) GetRelationNetworkIngress(ctx context.Context, relationUUID corerelation.UUID) ([]string, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := relationUUID.Validate(); err != nil {
		return nil, errors.Capture(err)
	}

	cidrs, err := s.modelState.GetRelationNetworkIngress(ctx, relationUUID.String())
	if err != nil {
		return nil, errors.Capture(err)
	}

	return cidrs, nil
}

func (s *Service) validateIngressNetworks(saasIngressAllow []string, networks []string) error {
	if len(networks) == 0 {
		return nil
	}

	whitelistCIDRs, err := parseCIDRs(saasIngressAllow)
	if err != nil {
		return errors.Capture(err)
	}
	requestedCIDRs, err := parseCIDRs(networks)
	if err != nil {
		return errors.Capture(err)
	}
	if len(whitelistCIDRs) > 0 {
		for _, n := range requestedCIDRs {
			if !internalnetwork.SubnetInAnyRange(whitelistCIDRs, n) {
				return errors.Errorf("subnet %v not in firewall whitelist", n).Add(crossmodelrelationerrors.SubnetNotInWhitelist)
			}
		}
	}
	return nil
}

func parseCIDRs(values []string) ([]*net.IPNet, error) {
	res := make([]*net.IPNet, len(values))
	for i, cidrStr := range values {
		_, ipNet, err := net.ParseCIDR(cidrStr)
		if err != nil {
			return nil, errors.Capture(err)
		}
		res[i] = ipNet
	}
	return res, nil
}
