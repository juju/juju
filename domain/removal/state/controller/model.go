// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller

import (
	"context"

	"github.com/canonical/sqlair"

	"github.com/juju/juju/domain/life"
	modelerrors "github.com/juju/juju/domain/model/errors"
	removalerrors "github.com/juju/juju/domain/removal/errors"
	"github.com/juju/juju/internal/errors"
)

// ModelExists returns true if a model exists with the input UUID.
// This uses the *model* database table, not the *controller* model table.
// The model table with one row should exist until the model is removed.
func (st *State) ModelExists(ctx context.Context, mUUID string) (bool, error) {
	db, err := st.DB()
	if err != nil {
		return false, errors.Capture(err)
	}

	modelUUID := entityUUID{UUID: mUUID}
	existsStmt, err := st.Prepare(`
SELECT uuid AS &entityUUID.uuid
FROM   model
WHERE  uuid = $entityUUID.uuid`, modelUUID)
	if err != nil {
		return false, errors.Errorf("preparing model exists query: %w", err)
	}

	var modelExists bool
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err = tx.Query(ctx, existsStmt, modelUUID).Get(&modelUUID)
		if errors.Is(err, sqlair.ErrNoRows) {
			return nil
		} else if err != nil {
			return errors.Errorf("running model exists query: %w", err)
		}

		modelExists = true
		return nil
	})

	return modelExists, errors.Capture(err)
}

// EnsureModelNotAliveCascade ensures that there is no model identified
// by the input model UUID, that is still alive.
func (st *State) EnsureModelNotAliveCascade(ctx context.Context, modelUUID string, force bool) error {
	db, err := st.DB()
	if err != nil {
		return errors.Capture(err)
	}

	eUUID := entityUUID{
		UUID: modelUUID,
	}

	updateModelLife, err := st.Prepare(`UPDATE model SET life_id = 1 WHERE uuid = $entityUUID.uuid AND life_id = 0`, eUUID)
	if err != nil {
		return errors.Errorf("preparing update model life query: %w", err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		// Update the model life to dying.
		if err := tx.Query(ctx, updateModelLife, eUUID).Run(); err != nil {
			return errors.Errorf("setting model life to dying: %w", err)
		}

		return nil
	})
	if err != nil {
		return errors.Errorf("ensuring model %q is not alive: %w", modelUUID, err)
	}

	return nil
}

// GetModelLife retrieves the life state of a model.
func (st *State) GetModelLife(ctx context.Context, mUUID string) (life.Life, error) {
	db, err := st.DB()
	if err != nil {
		return -1, errors.Capture(err)
	}

	var life life.Life
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var err error
		life, err = st.getModelLife(ctx, tx, mUUID)

		return errors.Capture(err)
	})

	return life, errors.Capture(err)
}

// MarkModelAsDead marks the model with the input UUID as dead.
// If there are model dependents, then this will return an error.
func (st *State) MarkModelAsDead(ctx context.Context, mUUID string) error {
	db, err := st.DB()
	if err != nil {
		return errors.Capture(err)
	}

	modelUUID := entityUUID{UUID: mUUID}
	updateStmt, err := st.Prepare(`
UPDATE model
SET    life_id = 2
WHERE  uuid = $entityUUID.uuid
AND    life_id = 1`, modelUUID)
	if err != nil {
		return errors.Errorf("preparing model life update: %w", err)
	}
	return errors.Capture(db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if l, err := st.getModelLife(ctx, tx, mUUID); err != nil {
			return errors.Errorf("getting model life: %w", err)
		} else if l == life.Dead {
			return nil
		} else if l == life.Alive {
			return removalerrors.EntityStillAlive
		}

		err := tx.Query(ctx, updateStmt, modelUUID).Run()
		if err != nil {
			return errors.Errorf("marking model as dead: %w", err)
		}

		return nil
	}))
}

func (st *State) getModelLife(ctx context.Context, tx *sqlair.TX, mUUID string) (life.Life, error) {
	var model entityLife
	modelUUID := entityUUID{UUID: mUUID}

	stmt, err := st.Prepare(`
SELECT &entityLife.life_id
FROM   model
WHERE  uuid = $entityUUID.uuid;`, model, modelUUID)
	if err != nil {
		return -1, errors.Errorf("preparing model life query: %w", err)
	}

	err = tx.Query(ctx, stmt, modelUUID).Get(&model)
	if errors.Is(err, sqlair.ErrNoRows) {
		return -1, modelerrors.NotFound
	} else if err != nil {
		return -1, errors.Errorf("running model life query: %w", err)
	}

	return life.Life(model.Life), nil
}
