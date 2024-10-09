// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"

	"github.com/canonical/sqlair"
	"github.com/juju/collections/set"
	"github.com/juju/names/v5"

	"github.com/juju/juju/core/controller"
	"github.com/juju/juju/core/database"
	"github.com/juju/juju/core/migration"
	"github.com/juju/juju/domain"
	"github.com/juju/juju/domain/modelmigration"
	"github.com/juju/juju/internal/errors"
)

// ModelState represents the access method for interacting the underlying model
// during model migration.
type ModelState struct {
	*domain.StateBase
}

// NewModelState creates a new model state for model migration.
func NewModelState(modelFactory database.TxnRunnerFactory) *ModelState {
	return &ModelState{
		StateBase: domain.NewStateBase(modelFactory),
	}
}

// CreateMigration creates a migration record in the model state.
func (s *ModelState) CreateMigration(ctx context.Context, initiatedBy names.UserTag, targetInfo migration.TargetInfo) error {

	return nil
}

// GetControllerUUID is responsible for returning the controller's unique id
// from state.
func (s *ModelState) GetControllerUUID(
	ctx context.Context,
) (string, error) {
	db, err := s.DB()
	if err != nil {
		return "", errors.Errorf("cannot get database to retrieve controller uuid: %w", err)
	}

	stmt, err := s.Prepare(`SELECT &modelInfo.* FROM model`, modelInfo{})
	if err != nil {
		return "", errors.Errorf("preparing get controller uuid statement: %w", err)
	}

	var result modelInfo
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt).Get(&result)
		if errors.Is(err, sqlair.ErrNoRows) {
			return errors.New(
				"cannot get controller uuid, model information is missing from database",
			).Add(err)
		} else if err != nil {
			return errors.Errorf(
				"cannot get controller uuid on model database: %w",
				err,
			)
		}
		return nil
	})

	if err != nil {
		return "", err
	}

	return result.ControllerUUID, nil
}

// GetAllInstanceIDs returns all instance IDs from the current model as
// juju/collections set.
func (s *ModelState) GetAllInstanceIDs(ctx context.Context) (set.Strings, error) {
	db, err := s.DB()
	if err != nil {
		return nil, errors.Errorf("cannot get database to retrieve instance IDs: %w", err)
	}

	query := `SELECT &instanceID.instance_id FROM machine_cloud_instance`
	queryStmt, err := s.Prepare(query, instanceID{})
	if err != nil {
		return nil, errors.Errorf("preparing retrieve all instance IDs statement: %w", err)
	}

	var result []instanceID
	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, queryStmt).GetAll(&result)
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("retrieving all instance IDs: %w", err)
		}
		return nil
	}); err != nil {
		return nil, err
	}

	instanceIDs := make(set.Strings, len(result))
	for _, instanceID := range result {
		instanceIDs.Add(instanceID.ID)
	}
	return instanceIDs, nil
}

// ModelControllerInfo returns the information about the model in relation to the
// controller.
func (s *ModelState) ModelControllerInfo(ctx context.Context) (modelmigration.ModelControllerInfo, error) {
	db, err := s.DB()
	if err != nil {
		return modelmigration.ModelControllerInfo{}, errors.Errorf("cannot get database to retrieve model controller info: %w", err)
	}

	stmt, err := s.Prepare(`SELECT &modelControllerInfo.* FROM model`, modelControllerInfo{})
	if err != nil {
		return modelmigration.ModelControllerInfo{}, errors.Errorf("preparing model controller info statement: %w", err)
	}

	var result modelControllerInfo
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt).Get(&result)
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
		return modelmigration.ModelControllerInfo{}, err
	}

	controllerUUID, err := controller.ParseUUID(result.ControllerUUID)
	if err != nil {
		return modelmigration.ModelControllerInfo{}, errors.Errorf(
			"cannot parse controller UUID: %w",
			err,
		)
	}

	return modelmigration.ModelControllerInfo{
		ControllerUUID:    controllerUUID,
		IsControllerModel: result.IsControllerModel,
	}, nil
}
