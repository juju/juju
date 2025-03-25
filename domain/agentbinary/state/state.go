// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"

	"github.com/juju/juju/core/database"
	"github.com/juju/juju/domain"
	"github.com/juju/juju/domain/agentbinary"
)

// State represents a type for interacting with the underlying state.
type State struct {
	*domain.StateBase
}

// NewState returns a new State for interacting with the underlying state.
func NewState(factory database.TxnRunnerFactory) *State {
	return &State{
		StateBase: domain.NewStateBase(factory),
	}
}

// Add adds a new agent binary's metadata to the database.
// It always overwrites the metadata for the given version and arch if it already exists.
// It returns [coreerrors.NotSupported] if the architecture is not found in the database.
// It returns [coreerrors.NotFound] if object store UUID is not found in the database.
func (s *State) Add(ctx context.Context, metadata agentbinary.Metadata) error {
	// TODO: implement this method.
	return nil
}
