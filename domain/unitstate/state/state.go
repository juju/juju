// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"fmt"

	"github.com/canonical/sqlair"
	"github.com/juju/errors"

	"github.com/juju/juju/core/database"
	"github.com/juju/juju/domain"
	uniterrors "github.com/juju/juju/domain/unitstate/errors"
)

type State struct {
	*domain.StateBase
}

// NewState returns a new state reference.
func NewState(factory database.TxnRunnerFactory) *State {
	return &State{
		StateBase: domain.NewStateBase(factory),
	}
}

// GetUnitUUIDForName returns the UUID corresponding to the input unit name.
// If no unit with the name exists, a [errors.UnitNotFound] error is returned.
func (st *State) GetUnitUUIDForName(ctx domain.AtomicContext, name string) (string, error) {
	uName := unitName{Name: name}
	uuid := unitUUID{}

	q := "SELECT &unitUUID.uuid FROM unit WHERE name = $unitName.name"
	stmt, err := st.Prepare(q, uName, uuid)
	if err != nil {
		return "", fmt.Errorf("preparing UUID query: %w", err)
	}

	err = domain.Run(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt, uName).Get(&uuid)
		if errors.Is(err, sqlair.ErrNoRows) {
			return uniterrors.UnitNotFound
		}
		return err
	})

	return uuid.UUID, err
}

// EnsureUnitStateRecord ensures that there is a row in the unit_state table
// for the input unit UUID. This eliminates the need for upsert statements
// when updating state for uniter, storage and secrets.
func (st *State) EnsureUnitStateRecord(ctx domain.AtomicContext, uuid string) error {
	id := unitUUID{UUID: uuid}

	q := "SELECT unit_uuid AS &unitUUID.uuid FROM unit_state WHERE unit_uuid = $unitUUID.uuid"
	rowStmt, err := st.Prepare(q, id)
	if err != nil {
		return fmt.Errorf("preparing state row query: %w", err)
	}

	q = "INSERT INTO unit_state(unit_uuid) values ($unitUUID.uuid)"
	insertStmt, err := st.Prepare(q, id)
	if err != nil {
		return fmt.Errorf("preparing state insert query: %w", err)
	}

	err = domain.Run(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, rowStmt, id).Get(&id)
		if err == nil {
			return nil
		}
		if !errors.Is(err, sqlair.ErrNoRows) {
			return fmt.Errorf("checking for state row: %w", err)
		}

		err = tx.Query(ctx, insertStmt, id).Run()
		if err != nil {
			return fmt.Errorf("adding state row: %w", err)
		}
		return nil
	})

	return err
}
