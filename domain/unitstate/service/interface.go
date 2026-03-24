// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	corerelation "github.com/juju/juju/core/relation"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain/unitstate"
	"github.com/juju/juju/domain/unitstate/internal"
	"github.com/juju/juju/environs"
)

// State defines an interface for interacting with the underlying state.
type State interface {
	CommitHookState
	UnitStateState
}

// CommitHookState defines a persistence layer interface for commit hook changes.
type CommitHookState interface {
	// CommitHookChanges persists a set of changes after a hook successfully
	// completes and executes them in a single transaction.
	CommitHookChanges(ctx context.Context, arg internal.CommitHookChangesArg) error

	// GetPeerRelationUUIDByEndpointIdentifiers gets the UUID of a peer
	// relation specified by a single endpoint identifier.
	//
	// The following error types can be expected to be returned:
	//   - [relationerrors.RelationNotFound] is returned if endpoint cannot be
	//     found.
	GetPeerRelationUUIDByEndpointIdentifiers(
		ctx context.Context,
		endpoint corerelation.EndpointIdentifier,
	) (corerelation.UUID, error)

	// GetRegularRelationUUIDByEndpointIdentifiers gets the UUID of a regular
	// relation specified by two endpoint identifiers.
	//
	// The following error types can be expected to be returned:
	//   - [relationerrors.RelationNotFound] is returned if endpoints cannot be
	//     found.
	GetRegularRelationUUIDByEndpointIdentifiers(
		ctx context.Context,
		endpoint1, endpoint2 corerelation.EndpointIdentifier,
	) (corerelation.UUID, error)

	// GetUnitRelationNetworkInfosNetworkingNotSupported retrieves egress and
	// ingress addresses for the specified unit by selecting the best candidate
	// from *all* unit addresses. These addresses are linked with all relations
	// where the given unit is in scope.
	// This is used on providers that do not support networking, and therefore
	// can not factor endpoint bindings.
	GetUnitRelationNetworkInfosNetworkingNotSupported(
		ctx context.Context, unitUUID coreunit.UUID,
	) ([]internal.RelationNetworkInfo, error)

	// GetUnitRelationNetworkInfos retrieves network info for all relations
	// where the the unit is in scope.
	GetUnitRelationNetworkInfos(
		ctx context.Context, unitUUID coreunit.UUID,
	) ([]internal.RelationNetworkInfo, error)

	// GetUnitUUIDByName returns the UUID for the named unit, returning an
	// error satisfying [applicationerrors.UnitNotFound] if the unit doesn't
	// exist.
	GetUnitUUIDByName(ctx context.Context, name coreunit.Name) (coreunit.UUID, error)
}

// UnitStateState defines a persistence layer interface for retrieving
// and persisting unit agent state.
type UnitStateState interface {
	// GetUnitState returns the full unit agent state.
	// If no unit with the uuid exists, a [unitstateerrors.UnitNotFound] error
	// is returned.
	// If the units state is empty [unitstateerrors.EmptyUnitState] error is
	// returned.
	GetUnitState(context.Context, string) (unitstate.RetrievedUnitState, error)

	// SetUnitState persists the input unit state selectively,
	// based on its populated values.
	SetUnitState(context.Context, unitstate.UnitState) error
}

// ProviderWithNetworking describes the interface needed from providers that
// support networking capabilities.
type ProviderWithNetworking interface {
	environs.Networking
}
