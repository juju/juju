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

// State implements persistence for unit state.
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

// UpdateUnitStateUniter sets the input uniter
// state against the input unit UUID.
func (st *State) UpdateUnitStateUniter(ctx domain.AtomicContext, uuid, state string) error {
	id := unitUUID{UUID: uuid}
	uSt := unitState{State: state}

	q := "UPDATE unit_state SET uniter_state = $unitState.state WHERE unit_uuid = $unitUUID.uuid"
	stmt, err := st.Prepare(q, id, uSt)
	if err != nil {
		return fmt.Errorf("preparing uniter state update query: %w", err)
	}

	return domain.Run(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		return tx.Query(ctx, stmt, id, uSt).Run()
	})
}

// UpdateUnitStateStorage sets the input storage
// state against the input unit UUID.
func (st *State) UpdateUnitStateStorage(ctx domain.AtomicContext, uuid, state string) error {
	id := unitUUID{UUID: uuid}
	uSt := unitState{State: state}

	q := "UPDATE unit_state SET storage_state = $unitState.state WHERE unit_uuid = $unitUUID.uuid"
	stmt, err := st.Prepare(q, id, uSt)
	if err != nil {
		return fmt.Errorf("preparing storage state update query: %w", err)
	}

	return domain.Run(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		return tx.Query(ctx, stmt, id, uSt).Run()
	})
}

// UpdateUnitStateSecret sets the input secret
// state against the input unit UUID.
func (st *State) UpdateUnitStateSecret(ctx domain.AtomicContext, uuid, state string) error {
	id := unitUUID{UUID: uuid}
	uSt := unitState{State: state}

	q := "UPDATE unit_state SET secret_state = $unitState.state WHERE unit_uuid = $unitUUID.uuid"
	stmt, err := st.Prepare(q, id, uSt)
	if err != nil {
		return fmt.Errorf("preparing secret state update query: %w", err)
	}

	return domain.Run(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		return tx.Query(ctx, stmt, id, uSt).Run()
	})
}

// SetUnitStateCharm sets the input key/value pairs
// as the charm state for the input unit UUID.
func (st *State) SetUnitStateCharm(ctx domain.AtomicContext, uuid string, state map[string]string) error {
	id := unitUUID{UUID: uuid}

	q := "DELETE from unit_state_charm WHERE unit_uuid = $unitUUID.uuid"
	dStmt, err := st.Prepare(q, id)
	if err != nil {
		return fmt.Errorf("preparing charm state delete query: %w", err)
	}

	keyVals := makeUnitStateKeyVals(uuid, state)

	q = "INSERT INTO unit_state_charm(*) VALUES ($unitStateKeyVal.*)"
	iStmt, err := st.Prepare(q, keyVals[0])
	if err != nil {
		return fmt.Errorf("preparing charm state insert query: %w", err)
	}

	return domain.Run(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if err := tx.Query(ctx, dStmt, id).Run(); err != nil {
			return fmt.Errorf("deleting unit charm state: %w", err)
		}

		if err := tx.Query(ctx, iStmt, keyVals).Run(); err != nil {
			return fmt.Errorf("setting unit charm state: %w", err)
		}
		return nil
	})
}

// SetUnitStateRelation sets the input key/value pairs
// as the relation state for the input unit UUID.
func (st *State) SetUnitStateRelation(ctx domain.AtomicContext, uuid string, state map[string]string) error {
	id := unitUUID{UUID: uuid}

	q := "DELETE from unit_state_relation WHERE unit_uuid = $unitUUID.uuid"
	dStmt, err := st.Prepare(q, id)
	if err != nil {
		return fmt.Errorf("preparing relation state delete query: %w", err)
	}

	keyVals := makeUnitStateKeyVals(uuid, state)

	q = "INSERT INTO unit_state_relation(*) VALUES ($unitStateKeyVal.*)"
	iStmt, err := st.Prepare(q, keyVals[0])
	if err != nil {
		return fmt.Errorf("preparing relation state insert query: %w", err)
	}

	return domain.Run(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if err := tx.Query(ctx, dStmt, id).Run(); err != nil {
			return fmt.Errorf("deleting unit relation state: %w", err)
		}

		if err := tx.Query(ctx, iStmt, keyVals).Run(); err != nil {
			return fmt.Errorf("setting unit relation state: %w", err)
		}
		return nil
	})
}
