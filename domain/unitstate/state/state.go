// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"

	"github.com/canonical/sqlair"

	"github.com/juju/juju/core/database"
	coreunit "github.com/juju/juju/core/unit"
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

// GetUnitState returns the full unit state. The state may be
// empty.
// If no unit with the namw exists, a [errors.UnitNotFound] error is returned.
func (st *State) GetUnitState(ctx context.Context, name coreunit.Name) (unitstate.RetrievedUnitState, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return unitstate.RetrievedUnitState{}, errors.Capture(err)
	}

	var state unitState
	q := "SELECT &unitState.* FROM unit_state WHERE unit_uuid = $unitUUID.uuid"
	unitStateStmt, err := st.Prepare(q, state, unitUUID{})
	if err != nil {
		return unitstate.RetrievedUnitState{}, errors.Errorf("preparing select unit state statement: %w", err)
	}

	var charmKVs []unitCharmStateKeyVal
	q = `
SELECT &unitCharmStateKeyVal.*
FROM unit_state_charm
WHERE unit_uuid = $unitUUID.uuid`
	charmStateStmt, err := st.Prepare(q, unitCharmStateKeyVal{}, unitUUID{})
	if err != nil {
		return unitstate.RetrievedUnitState{}, errors.Errorf("preparing select unit charm state statement: %w", err)
	}

	var relationKVs []unitRelationStateKeyVal
	q = `
SELECT &unitRelationStateKeyVal.*
FROM unit_state_relation
WHERE unit_uuid = $unitUUID.uuid`
	relationStateStmt, err := st.Prepare(q, unitRelationStateKeyVal{}, unitUUID{})
	if err != nil {
		return unitstate.RetrievedUnitState{}, errors.Errorf("preparing select unit relation state statement: %w", err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		id, err := st.getUnitUUIDForName(ctx, tx, name)
		if err != nil {
			return errors.Errorf("getting unit UUID for %q: %w", name, err)
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

func (st *State) SetUnitState(ctx context.Context, as unitstate.UnitState) error {
	if as.Name.Validate() != nil {
		return errors.Errorf("invalid unit name: %q", as.Name)
	}

	db, err := st.DB(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	return db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		uuid, err := st.getUnitUUIDForName(ctx, tx, as.Name)
		if err != nil {
			return errors.Errorf("getting unit UUID for %q: %w", as.Name, err)
		}

		if err = st.ensureUnitStateRecord(ctx, tx, uuid); err != nil {
			return errors.Errorf("ensuring state record for %q: %w", as.Name, err)
		}

		if as.UniterState != nil {
			if err = st.updateUnitStateUniter(ctx, tx, uuid, *as.UniterState); err != nil {
				return errors.Errorf("setting uniter state for %q: %w", as.Name, err)
			}
		}

		if as.StorageState != nil {
			if err = st.updateUnitStateStorage(ctx, tx, uuid, *as.StorageState); err != nil {
				return errors.Errorf("setting storage state for %q: %w", as.Name, err)
			}
		}

		if as.SecretState != nil {
			if err = st.updateUnitStateSecret(ctx, tx, uuid, *as.SecretState); err != nil {
				return errors.Errorf("setting secret state for %q: %w", as.Name, err)
			}
		}

		if as.CharmState != nil {
			if err = st.setUnitStateCharm(ctx, tx, uuid, *as.CharmState); err != nil {
				return errors.Errorf("setting charm state for %q: %w", as.Name, err)
			}
		}

		if as.RelationState != nil {
			if err = st.setUnitStateRelation(ctx, tx, uuid, *as.RelationState); err != nil {
				return errors.Errorf("setting relation state for %q: %w", as.Name, err)
			}
		}

		return nil
	})
}

// ensureUnitStateRecord ensures that there is a row in the unit_state table
// for the input unit UUID. This eliminates the need for upsert statements
// when updating state for uniter, storage and secrets.
func (st *State) ensureUnitStateRecord(ctx context.Context, tx *sqlair.TX, id unitUUID) error {
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

	err = tx.Query(ctx, rowStmt, id).Get(&id)
	if err == nil {
		return nil
	} else if !errors.Is(err, sqlair.ErrNoRows) {
		return errors.Errorf("checking for state row: %w", err)
	}

	err = tx.Query(ctx, insertStmt, id).Run()
	if err != nil {
		return errors.Errorf("adding state row: %w", err)
	}
	return nil
}

// updateUnitStateUniter sets the input uniter
// state against the input unit UUID.
func (st *State) updateUnitStateUniter(ctx context.Context, tx *sqlair.TX, id unitUUID, state string) error {
	uSt := unitState{UniterState: state}

	q := "UPDATE unit_state SET uniter_state = $unitState.uniter_state WHERE unit_uuid = $unitUUID.uuid"
	stmt, err := st.Prepare(q, id, uSt)
	if err != nil {
		return errors.Errorf("preparing uniter state update query: %w", err)
	}

	return tx.Query(ctx, stmt, id, uSt).Run()
}

// updateUnitStateStorage sets the input storage
// state against the input unit UUID.
func (st *State) updateUnitStateStorage(ctx context.Context, tx *sqlair.TX, id unitUUID, state string) error {
	uSt := unitState{StorageState: state}

	q := "UPDATE unit_state SET storage_state = $unitState.storage_state WHERE unit_uuid = $unitUUID.uuid"
	stmt, err := st.Prepare(q, id, uSt)
	if err != nil {
		return errors.Errorf("preparing storage state update query: %w", err)
	}

	return tx.Query(ctx, stmt, id, uSt).Run()
}

// updateUnitStateSecret sets the input secret
// state against the input unit UUID.
func (st *State) updateUnitStateSecret(ctx context.Context, tx *sqlair.TX, id unitUUID, state string) error {
	uSt := unitState{SecretState: state}

	q := "UPDATE unit_state SET secret_state = $unitState.secret_state WHERE unit_uuid = $unitUUID.uuid"
	stmt, err := st.Prepare(q, id, uSt)
	if err != nil {
		return errors.Errorf("preparing secret state update query: %w", err)
	}

	return tx.Query(ctx, stmt, id, uSt).Run()
}

// setUnitStateCharm sets the input key/value pairs
// as the charm state for the input unit UUID.
func (st *State) setUnitStateCharm(ctx context.Context, tx *sqlair.TX, id unitUUID, state map[string]string) error {
	q := "DELETE from unit_state_charm WHERE unit_uuid = $unitUUID.uuid"
	dStmt, err := st.Prepare(q, id)
	if err != nil {
		return errors.Errorf("preparing charm state delete query: %w", err)
	}

	keyVals := makeUnitCharmStateKeyVals(id, state)

	q = "INSERT INTO unit_state_charm(*) VALUES ($unitCharmStateKeyVal.*)"
	iStmt, err := st.Prepare(q, keyVals[0])
	if err != nil {
		return errors.Errorf("preparing charm state insert query: %w", err)
	}

	if err := tx.Query(ctx, dStmt, id).Run(); err != nil {
		return errors.Errorf("deleting unit charm state: %w", err)
	}

	if err := tx.Query(ctx, iStmt, keyVals).Run(); err != nil {
		return errors.Errorf("setting unit charm state: %w", err)
	}
	return nil
}

// SetUnitStateRelation sets the input key/value pairs
// as the relation state for the input unit UUID.
func (st *State) setUnitStateRelation(ctx context.Context, tx *sqlair.TX, id unitUUID, state map[int]string) error {
	q := "DELETE from unit_state_relation WHERE unit_uuid = $unitUUID.uuid"
	dStmt, err := st.Prepare(q, id)
	if err != nil {
		return errors.Errorf("preparing relation state delete query: %w", err)
	}

	keyVals := makeUnitRelationStateKeyVals(id, state)

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
}

func (st *State) getUnitUUIDForName(ctx context.Context, tx *sqlair.TX, name coreunit.Name) (unitUUID, error) {
	uName := unitName{Name: name}
	uuid := unitUUID{}

	q := "SELECT &unitUUID.uuid FROM unit WHERE name = $unitName.name"
	stmt, err := st.Prepare(q, uName, uuid)
	if err != nil {
		return unitUUID{}, errors.Errorf("preparing UUID query: %w", err)
	}

	err = tx.Query(ctx, stmt, uName).Get(&uuid)
	if errors.Is(err, sqlair.ErrNoRows) {
		return unitUUID{}, uniterrors.UnitNotFound
	} else if err != nil {
		return unitUUID{}, errors.Errorf("getting unit UUID for %q: %w", name, err)
	}

	return uuid, nil
}
