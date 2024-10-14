// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"

	"github.com/canonical/sqlair"

	"github.com/juju/juju/core/controller"
	"github.com/juju/juju/core/database"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/user"
	"github.com/juju/juju/domain"
	accesserrors "github.com/juju/juju/domain/access/errors"
	"github.com/juju/juju/domain/life"
	"github.com/juju/juju/domain/modelmigration"
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

// GetUserUUIDByName will retrieve the user uuid for the user identifier by
// name. If the user does not exist an error that satisfies
// [accesserrors.UserNotFound] will be returned.
func (s *ControllerState) GetUserUUIDByName(ctx context.Context, name user.Name) (user.UUID, error) {
	db, err := s.DB()
	if err != nil {
		return "", errors.Errorf("cannot get database to retrieve user: %w", err)
	}

	uName := userName{Name: name.Name()}

	stmt := `
SELECT user.uuid AS &M.userUUID
FROM user
WHERE user.name = $userName.name
AND user.removed = false`

	selectUserUUIDStmt, err := sqlair.Prepare(stmt, userUUID{}, uName)
	if err != nil {
		return "", err
	}

	var result userUUID
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err = tx.Query(ctx, selectUserUUIDStmt, uName).Get(&result)
		if errors.Is(err, sql.ErrNoRows) {
			return errors.Errorf("%w when finding user uuid for name %q", accesserrors.UserNotFound, name)
		} else if err != nil {
			return errors.Errorf("looking up user uuid for name %q: %w", name, err)
		}
		return nil
	})

	uuid, err := user.ParseUUID(result.UUID)
	if err != nil {
		return "", errors.Errorf("cannot parse user UUID: %w", err)
	}

	return uuid, nil
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
		err := tx.Query(ctx, stmt, mUUID).Get(&result)
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

// ModelMigrationInfo returns the information about the model migration in
// relation to the controller.
func (s *ControllerState) ModelMigrationInfo(ctx context.Context, uuid model.UUID) (modelmigration.ModelMigrationInfo, error) {
	db, err := s.DB()
	if err != nil {
		return modelmigration.ModelMigrationInfo{}, errors.Errorf("cannot get database to retrieve model controller info: %w", err)
	}

	mUUID := modelUUID{UUID: uuid.String()}

	// We don't consider activated boolean here because we want to know if
	// the model is in the process of being migrated. If
	stmt, err := s.Prepare(`
SELECT &modelMigrationInfo.* 
FROM v_model_migration_info
WHERE uuid = $modelUUID.uuid
`, modelMigrationInfo{}, mUUID)
	if err != nil {
		return modelmigration.ModelMigrationInfo{}, errors.Errorf("preparing model controller info statement: %w", err)
	}

	var result modelMigrationInfo
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt, mUUID).Get(&result)
		if errors.Is(err, sqlair.ErrNoRows) {
			return errors.New(
				"cannot get model controller info, model information is missing from database",
			).Add(err)
		} else if err != nil {
			return errors.Errorf(
				"cannot get model controller info on model database: %w",
				err,
			)
		}
		return nil
	})

	if err != nil {
		return modelmigration.ModelMigrationInfo{}, err
	}

	controllerUUID, err := controller.ParseUUID(result.ControllerUUID)
	if err != nil {
		return modelmigration.ModelMigrationInfo{}, errors.Errorf(
			"cannot parse controller UUID: %w",
			err,
		)
	}

	controllerModelUUID, err := model.ParseUUID(result.ControllerModelUUID)
	if err != nil {
		return modelmigration.ModelMigrationInfo{}, errors.Errorf(
			"cannot parse controller model UUID: %w",
			err,
		)
	}

	return modelmigration.ModelMigrationInfo{
		ControllerUUID:    controllerUUID,
		IsControllerModel: controllerModelUUID == uuid,
		MigrationActive:   result.MigrationActive,
	}, nil
}
