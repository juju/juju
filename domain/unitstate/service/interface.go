// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	corerelation "github.com/juju/juju/core/relation"
	"github.com/juju/juju/domain/unitstate"
	"github.com/juju/juju/domain/unitstate/internal"
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

	// GetCommitHookUnitInfo returns the unit UUID and machine UUID if assigned,
	// returning an error satisfying
	// [applicationerrors.UnitNotFound] if the unit doesn't exist.
	GetCommitHookUnitInfo(ctx context.Context, unitName string) (internal.CommitHookUnitInfo, error)
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
