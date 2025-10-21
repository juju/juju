// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"net"

	coreerrors "github.com/juju/juju/core/errors"
	corerelation "github.com/juju/juju/core/relation"
	"github.com/juju/juju/core/trace"
	"github.com/juju/juju/internal/errors"
)

// ModelRelationNetworkState describes retrieval and persistence methods for
// relation network ingress in the model database.
type ModelRelationNetworkState interface {
	// AddRelationNetworkIngress adds ingress network CIDRs for the specified
	// relation.
	AddRelationNetworkIngress(ctx context.Context, relationUUID string, cidrs ...string) error

	// GetRelationNetworkIngress retrieves all ingress network CIDRs for the
	// specified relation.
	GetRelationNetworkIngress(ctx context.Context, relationUUID string) ([]string, error)

	// NamespaceForRelationIngressNetworksWatcher returns the namespace of the
	// relation_network_ingress table, used for the watcher.
	NamespaceForRelationIngressNetworksWatcher() string
}

// AddRelationNetworkIngress adds ingress network CIDRs for the specified
// relation.
// The CIDRs are added to the relation_network_ingress table.
func (s *Service) AddRelationNetworkIngress(ctx context.Context, relationUUID corerelation.UUID, cidrs ...string) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if relationUUID == "" {
		return errors.Errorf("relation UUID cannot be empty")
	}

	if err := relationUUID.Validate(); err != nil {
		return errors.Capture(err)
	}

	if len(cidrs) == 0 {
		return errors.Errorf("at least one CIDR must be provided")
	}

	// Validate CIDRs are not empty and are valid
	for _, cidr := range cidrs {
		if cidr == "" {
			return errors.Errorf("CIDR cannot be empty")
		}
		if _, _, err := net.ParseCIDR(cidr); err != nil {
			return errors.Errorf("CIDR %q is not valid: %w", cidr, err).Add(coreerrors.NotValid)
		}
	}

	if err := s.modelState.AddRelationNetworkIngress(ctx, relationUUID.String(), cidrs...); err != nil {
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

	if relationUUID == "" {
		return nil, errors.Errorf("relation UUID cannot be empty")
	}

	if err := relationUUID.Validate(); err != nil {
		return nil, errors.Capture(err)
	}

	cidrs, err := s.modelState.GetRelationNetworkIngress(ctx, relationUUID.String())
	if err != nil {
		return nil, errors.Capture(err)
	}

	return cidrs, nil
}
