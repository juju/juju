// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/collections/transform"

	"github.com/juju/juju/core/trace"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain/network"
	internalerrors "github.com/juju/juju/internal/errors"
)

// GetUnitEndpointNetworks retrieves network relation information for a given unit
// and specified endpoints.
// It returns exactly one info for each endpoint names passed in argument,
// but doesn't enforce the order. Each info has an endpoint name that should match
// one of the endpoint names, one info for each endpoint names.
//
// The following errors may be returned:
// - [applicationerrors.UnitNotFound] if the unit does not exist
func (s *Service) GetUnitEndpointNetworks(
	ctx context.Context,
	unitName coreunit.Name,
	endpointNames []string,
) ([]network.UnitNetwork, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	// todo(gfouillet): Implements the whole functionality
	//   this is just a placeholder for the facade works
	addr, err := s.GetUnitPublicAddress(ctx, unitName)
	if err != nil {
		return nil, internalerrors.Errorf("getting unit public address: %w", err)
	}

	return transform.Slice(endpointNames, func(endpoint string) network.UnitNetwork {
		return network.UnitNetwork{
			EndpointName:     endpoint,
			IngressAddresses: []string{addr.IP().String()},
		}
	}), nil
}

// SetUnitRelationNetworks updates the relation network information for
// the specified unit.
func (s *Service) SetUnitRelationNetworks(context context.Context, name coreunit.Name) error {
	return nil // To be implemented
}
