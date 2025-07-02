// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"strings"

	corerelation "github.com/juju/juju/core/relation"
	"github.com/juju/juju/core/trace"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain/network"
	relationerrors "github.com/juju/juju/domain/relation/errors"
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

	unitUUID, err := s.st.GetUnitUUIDByName(ctx, unitName)
	if err != nil {
		return nil, internalerrors.Capture(err)
	}

	result, err := s.st.GetUnitEndpointNetworks(ctx, unitUUID.String(), endpointNames)
	if err != nil {
		return nil, internalerrors.Errorf("getting unit endpoint networks: %w", err)
	}

	return result, nil
}

// GetUnitRelationNetwork retrieves network relation information for a given
// unit and relation key.
//
// The following errors may be returned:
//   - [applicationerrors.UnitNotFound] if the unit does not exist
//   - [relationerrors.RelationNotFound] if the relation key doesn't belong to
//     the unit.
func (s *Service) GetUnitRelationNetwork(ctx context.Context, unitName coreunit.Name,
	relKey corerelation.Key) (network.UnitNetwork, error) {
	var endpoint string
	for _, epIdentifier := range relKey.EndpointIdentifiers() {
		if strings.HasPrefix(unitName.String(), epIdentifier.ApplicationName) {
			endpoint = epIdentifier.EndpointName
			break
		}
	}
	if endpoint == "" {
		s.logger.Errorf(ctx, "could not find endpoint for unit %s in the relation %+v", unitName, relKey)
		return network.UnitNetwork{}, relationerrors.RelationNotFound
	}

	infos, err := s.GetUnitEndpointNetworks(ctx, unitName, []string{endpoint})
	if err != nil {
		return network.UnitNetwork{}, internalerrors.Errorf("getting unit endpoint networks: %w", err)
	}
	if len(infos) != 1 {
		// Should not happen unless the interface contract for
		// GetUnitEndpointNetworks is broken.
		// If not broken, providing exactly one endpoint as a parameter for
		// GetUnitEndpointNetworks should return exactly one info.
		return network.UnitNetwork{}, internalerrors.Errorf("expected 1 NetworkInfo for unit %q on endpoint %q, got %d", unitName, endpoint,
			len(infos))
	}
	return infos[0], nil
}
