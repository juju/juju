// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"

	"github.com/canonical/sqlair"

	"github.com/juju/juju/core/database"
	"github.com/juju/juju/domain"
	"github.com/juju/juju/domain/unitstate"
	uniterrors "github.com/juju/juju/domain/unitstate/errors"
	"github.com/juju/juju/internal/errors"
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
		return "", errors.Errorf("preparing UUID query: %w", err)
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
		return errors.Errorf("preparing state row query: %w", err)
	}

	q = "INSERT INTO unit_state(unit_uuid) values ($unitUUID.uuid)"
	insertStmt, err := st.Prepare(q, id)
	if err != nil {
		return errors.Errorf("preparing state insert query: %w", err)
	}

	err = domain.Run(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, rowStmt, id).Get(&id)
		if err == nil {
			return nil
		}
		if !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("checking for state row: %w", err)
		}

		err = tx.Query(ctx, insertStmt, id).Run()
		if err != nil {
			return errors.Errorf("adding state row: %w", err)
		}
		return nil
	})

	return err
}

// UpdateUnitStateUniter sets the input uniter
// state against the input unit UUID.
func (st *State) UpdateUnitStateUniter(ctx domain.AtomicContext, uuid, state string) error {
	id := unitUUID{UUID: uuid}
	uSt := unitState{UniterState: state}

	q := "UPDATE unit_state SET uniter_state = $unitState.uniter_state WHERE unit_uuid = $unitUUID.uuid"
	stmt, err := st.Prepare(q, id, uSt)
	if err != nil {
		return errors.Errorf("preparing uniter state update query: %w", err)
	}

	return domain.Run(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		return tx.Query(ctx, stmt, id, uSt).Run()
	})
}

// UpdateUnitStateStorage sets the input storage
// state against the input unit UUID.
func (st *State) UpdateUnitStateStorage(ctx domain.AtomicContext, uuid, state string) error {
	id := unitUUID{UUID: uuid}
	uSt := unitState{StorageState: state}

	q := "UPDATE unit_state SET storage_state = $unitState.storage_state WHERE unit_uuid = $unitUUID.uuid"
	stmt, err := st.Prepare(q, id, uSt)
	if err != nil {
		return errors.Errorf("preparing storage state update query: %w", err)
	}

	return domain.Run(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		return tx.Query(ctx, stmt, id, uSt).Run()
	})
}

// UpdateUnitStateSecret sets the input secret
// state against the input unit UUID.
func (st *State) UpdateUnitStateSecret(ctx domain.AtomicContext, uuid, state string) error {
	id := unitUUID{UUID: uuid}
	uSt := unitState{SecretState: state}

	q := "UPDATE unit_state SET secret_state = $unitState.secret_state WHERE unit_uuid = $unitUUID.uuid"
	stmt, err := st.Prepare(q, id, uSt)
	if err != nil {
		return errors.Errorf("preparing secret state update query: %w", err)
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
		return errors.Errorf("preparing charm state delete query: %w", err)
	}

	keyVals := makeUnitCharmStateKeyVals(uuid, state)

	q = "INSERT INTO unit_state_charm(*) VALUES ($unitCharmStateKeyVal.*)"
	iStmt, err := st.Prepare(q, keyVals[0])
	if err != nil {
		return errors.Errorf("preparing charm state insert query: %w", err)
	}

	return domain.Run(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if err := tx.Query(ctx, dStmt, id).Run(); err != nil {
			return errors.Errorf("deleting unit charm state: %w", err)
		}

		if err := tx.Query(ctx, iStmt, keyVals).Run(); err != nil {
			return errors.Errorf("setting unit charm state: %w", err)
		}
		return nil
	})
}

// SetUnitStateRelation sets the input key/value pairs
// as the relation state for the input unit UUID.
func (st *State) SetUnitStateRelation(ctx domain.AtomicContext, uuid string, state map[int]string) error {
	id := unitUUID{UUID: uuid}

	q := "DELETE from unit_state_relation WHERE unit_uuid = $unitUUID.uuid"
	dStmt, err := st.Prepare(q, id)
	if err != nil {
		return errors.Errorf("preparing relation state delete query: %w", err)
	}

	keyVals := makeUnitRelationStateKeyVals(uuid, state)

	return domain.Run(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if err := tx.Query(ctx, dStmt, id).Run(); err != nil {
			return errors.Errorf("deleting unit relation state: %w", err)
		}

		if len(keyVals) != 0 {
			q = "INSERT INTO unit_state_relation(*) VALUES ($unitRelationStateKeyVal.*)"
			iStmt, err := st.Prepare(q, keyVals[0])
			if err != nil {
				return errors.Errorf("preparing relation state insert query: %w", err)
			}

			if err := tx.Query(ctx, iStmt, keyVals).Run(); err != nil {
				return errors.Errorf("setting unit relation state: %w", err)
			}
		}
		return nil
	})
}

// GetUnitState returns the full unit state. The state may be
// empty.
// If no unit with the uuid exists, a [errors.UnitNotFound] error is returned.
func (st *State) GetUnitState(ctx context.Context, uuid string) (unitstate.RetrievedUnitState, error) {
	db, err := st.DB()
	if err != nil {
		return unitstate.RetrievedUnitState{}, errors.Errorf("getting db: %w", err)
	}

	id := unitUUID{UUID: uuid}
	var count count
	q := "SELECT COUNT(uuid) AS &count.count FROM unit WHERE uuid = $unitUUID.uuid"
	unitNameStmt, err := st.Prepare(q, count, id)
	if err != nil {
		return unitstate.RetrievedUnitState{}, errors.Errorf("preparing select unit name statement: %w", err)
	}

	var state unitState
	q = "SELECT &unitState.* FROM unit_state WHERE unit_uuid = $unitUUID.uuid"
	unitStateStmt, err := st.Prepare(q, state, id)
	if err != nil {
		return unitstate.RetrievedUnitState{}, errors.Errorf("preparing select unit state statement: %w", err)
	}

	var charmKVs []unitCharmStateKeyVal
	q = `
SELECT &unitCharmStateKeyVal.*
FROM unit_state_charm
WHERE unit_uuid = $unitUUID.uuid`
	charmStateStmt, err := st.Prepare(q, unitCharmStateKeyVal{}, id)
	if err != nil {
		return unitstate.RetrievedUnitState{}, errors.Errorf("preparing select unit charm state statement: %w", err)
	}

	var relationKVs []unitRelationStateKeyVal
	q = `
SELECT &unitRelationStateKeyVal.*
FROM unit_state_relation
WHERE unit_uuid = $unitUUID.uuid`
	relationStateStmt, err := st.Prepare(q, unitRelationStateKeyVal{}, id)
	if err != nil {
		return unitstate.RetrievedUnitState{}, errors.Errorf("preparing select unit relation state statement: %w", err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, unitNameStmt, id).Get(&count)
		if count.Count < 1 {
			return uniterrors.UnitNotFound
		} else if err != nil {
			return errors.Errorf("getting unit name: %w", err)
		}

		err = tx.Query(ctx, unitStateStmt, id).Get(&state)
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("getting unit state: %w", err)
		}

		err = tx.Query(ctx, charmStateStmt, id).GetAll(&charmKVs)
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("getting unit charm state: %w", err)
		}

		err = tx.Query(ctx, relationStateStmt, id).GetAll(&relationKVs)
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("getting unit relation state: %w", err)
		}

		return nil
	})
	if err != nil {
		return unitstate.RetrievedUnitState{}, err
	}

	unitState := unitstate.RetrievedUnitState{
		UniterState:  state.UniterState,
		StorageState: state.StorageState,
		SecretState:  state.SecretState,
	}
	if len(charmKVs) > 0 {
		unitState.CharmState = makeMapFromCharmUnitStateKeyVals(charmKVs)
	}
	if len(relationKVs) > 0 {
		unitState.RelationState = makeMapFromRelationUnitStateKeyVals(relationKVs)
	}

	return unitState, nil
}
