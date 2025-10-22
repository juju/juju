// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"

	"github.com/canonical/sqlair"
	coredatabase "github.com/juju/juju/core/database"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain"
	"github.com/juju/juju/internal/errors"
)

// State represents database interactions dealing with storage.
type State struct {
	*domain.StateBase
}

// checkUnitExists checks if a unit with the given UUID exists in the model.
func (s *State) checkUnitExists(
	ctx context.Context, tx *sqlair.TX, uuid coreunit.UUID,
) (bool, error) {

	entityUUIDInput := entityUUID{UUID: uuid.String()}
	stmt, err := s.Prepare(
		"SELECT &entityUUID.* FROM unit WHERE  uuid = $entityUUID.uuid",
		entityUUIDInput,
	)
	if err != nil {
		return false, errors.Capture(err)
	}

	err = tx.Query(ctx, stmt, entityUUIDInput).Get(&entityUUIDInput)
	if errors.Is(err, sqlair.ErrNoRows) {
		return false, nil
	} else if err != nil {
		return false, errors.Capture(err)
	}

	return true, nil
}

// NewState returns a new storage state
// based on the input database factory method.
func NewState(factory coredatabase.TxnRunnerFactory) *State {
	return &State{
		StateBase: domain.NewStateBase(factory),
	}
}
