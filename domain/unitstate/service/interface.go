// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	coreunit "github.com/juju/juju/core/unit"
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
