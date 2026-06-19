// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	coremodel "github.com/juju/juju/core/model"
	corerelation "github.com/juju/juju/core/relation"
	"github.com/juju/juju/core/secrets"
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

	// GetCommitHookUnitInfo returns the unit UUID and machine UUID if assigned,
	// returning an error satisfying
	// [applicationerrors.UnitNotFound] if the unit doesn't exist.
	GetCommitHookUnitInfo(ctx context.Context, unitName string) (internal.CommitHookUnitInfo, error)

	// GetModelUUID returns the UUID of the model for the unit state domain.
	GetModelUUID(ctx context.Context) (string, error)

	// GetSecretRotatePolicy returns the current rotate policy for the
	// secret identified by the given secret ID. If the secret does not
	// exist, an error satisfying [secreterrors.SecretNotFound] is returned.
	GetSecretRotatePolicy(ctx context.Context, secretID string) (secrets.RotatePolicy, error)
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

// SecretBackendReferenceMutator describes methods for modifying secret
// backend references in the controller database.
type SecretBackendReferenceMutator interface {
	// AddSecretBackendReference adds a reference to the secret backend
	// for the given secret revision. It returns a rollback function which
	// can be used to revert the changes.
	AddSecretBackendReference(
		ctx context.Context, valueRef *secrets.ValueRef, modelID coremodel.UUID, revisionID string, secretID string,
	) (func() error, error)

	// UpdateSecretBackendReference updates the reference to the secret
	// backend for the given secret revision. It returns a rollback function
	// which can be used to revert the changes.
	UpdateSecretBackendReference(
		ctx context.Context, valueRef *secrets.ValueRef, modelID coremodel.UUID, revisionID string, secretID string,
	) (func() error, error)
}

// ProviderWithNetworking describes the interface needed from providers that
// support networking capabilities.
type ProviderWithNetworking interface {
	environs.Networking
}
