// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"

	"github.com/canonical/sqlair"

	"github.com/juju/juju/core/database"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/domain"
	"github.com/juju/juju/domain/life"
	modelmigrationerrors "github.com/juju/juju/domain/modelmigration/errors"
	"github.com/juju/juju/internal/errors"
)

// ControllerState represents the access method for interacting the underlying
// controller during model migration.
type ControllerState struct {
	*domain.StateBase
}

// NewControllerState creates a new controller state for model migration.
func NewControllerState(modelFactory database.TxnRunnerFactory) *ControllerState {
	return &ControllerState{
		StateBase: domain.NewStateBase(modelFactory),
	}
}

// ModelAvailable returns true if the model is available.
// This checks if the model is activated and the model is alive.
// Returns [errors.NotFound] if the model is not found.
func (s *ControllerState) ModelAvailable(ctx context.Context, uuid model.UUID) (bool, error) {
	db, err := s.DB()
	if err != nil {
		return false, errors.Errorf("cannot get database to retrieve model: %w", err)
	}

	mUUID := modelUUID{UUID: uuid.String()}

	stmt, err := s.Prepare(`
SELECT &modelLife.*
FROM model
WHERE
uuid = $modelUUID.uuid AND
activated = TRUE
	`, modelLife{}, mUUID)
	if err != nil {
		return false, errors.Errorf("preparing get model statement: %w", err)
	}

	var result modelLife
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt, uuid).Get(&result)
		if errors.Is(err, sqlair.ErrNoRows) {
			return modelmigrationerrors.ModelNotFound
		} else if err != nil {
			return errors.Errorf("cannot get model: %w", err)
		}
		return nil
	})
	if err != nil {
		return false, err
	}
	return result.Life == life.Alive, nil
}
