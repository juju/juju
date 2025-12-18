// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model

import (
	"context"

	"github.com/canonical/sqlair"
	"github.com/juju/collections/set"

	"github.com/juju/juju/core/database"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/domain"
	"github.com/juju/juju/internal/errors"
)

// State represents the access method for interacting the underlying model
// during model migration.
type State struct {
	*domain.StateBase

	modelUUID model.UUID
}

// New creates a new [State]
func New(modelFactory database.TxnRunnerFactory, modelUUID model.UUID) *State {
	return &State{
		StateBase: domain.NewStateBase(modelFactory),
		modelUUID: modelUUID,
	}
}

// GetControllerUUID is responsible for returning the controller's unique id
// from state.
func (s *State) GetControllerUUID(
	ctx context.Context,
) (string, error) {
	db, err := s.DB(ctx)
	if err != nil {
		return "", errors.Errorf("cannot get database to retrieve controller uuid: %w", err)
	}

	stmt, err := s.Prepare(`
SELECT (controller_uuid) AS (&modelInfo.*)
FROM model`, modelInfo{})

	if err != nil {
		return "", errors.Errorf("preparing get controller uuid statement: %w", err)
	}

	result := modelInfo{}
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
func (s *State) GetAllInstanceIDs(ctx context.Context) (set.Strings, error) {
	db, err := s.DB(ctx)
	if err != nil {
		return nil, errors.Errorf("cannot get database to retrieve instance IDs: %w", err)
	}

	query := `
SELECT &instanceID.instance_id
FROM   machine_cloud_instance`
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

// DeleteModelImportingStatus removes the entry from the model_migrating table
// in the model database, indicating that the model import has completed or been
// aborted.
func (s *State) DeleteModelImportingStatus(ctx context.Context) error {
	db, err := s.DB(ctx)
	if err != nil {
		return errors.Errorf("cannot get database to delete importing status: %w", err)
	}

	modelUUIDArg := entityUUID{
		UUID: s.modelUUID.String(),
	}

	stmt, err := s.Prepare(`
DELETE FROM model_migrating
WHERE model_uuid = $entityUUID.uuid
	`, modelUUIDArg)
	if err != nil {
		return errors.Errorf("preparing delete importing status statement: %w", err)
	}

	return db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if err := tx.Query(ctx, stmt, modelUUIDArg).Run(); err != nil {
			return errors.Errorf("deleting importing status for model %q: %w", s.modelUUID, err)
		}
		return nil
	})
}

// GetModelTargetAgentVersion returns the target agent version currently set for
// the model. This func expects that the target agent version for
// the model has already been set.
func (s *State) GetModelTargetAgentVersion(
	ctx context.Context,
) (semversion.Number, error) {
	db, err := s.DB(ctx)
	if err != nil {
		return semversion.Zero, errors.Capture(err)
	}

	var currentVersion string
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		currentVersion, err = s.getModelTargetAgentVersion(ctx, tx)
		return err
	})

	if err != nil {
		return semversion.Zero, errors.Capture(err)
	}

	rval, err := semversion.Parse(currentVersion)
	if err != nil {
		return semversion.Zero, errors.Errorf(
			"parsing controller model target agent version %q: %w",
			currentVersion, err,
		)
	}
	return rval, nil
}

// SetModelTargetAgentVersion is responsible for setting the current target
// agent version of the model. This function expects a precondition
// version to be supplied. The model's target agent version at the time the
// operation is applied must match the preCondition version or else an error is
// returned.
func (s *State) SetModelTargetAgentVersion(
	ctx context.Context,
	preCondition semversion.Number,
	toVersion semversion.Number,
) error {
	db, err := s.DB(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	toVersionInput := setAgentVersionTarget{TargetVersion: toVersion.String()}
	setAgentVersionStmt, err := s.Prepare(`
UPDATE agent_version
SET    target_version = $setAgentVersionTarget.target_version
`,
		toVersionInput,
	)
	if err != nil {
		return errors.Capture(err)
	}

	preConditionVersionStr := preCondition.String()
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		currentAgentVersion, err := s.getModelTargetAgentVersion(ctx, tx)
		if err != nil {
			return errors.Errorf(
				"checking current target agent version for model to validate precondition: %w", err,
			)
		}

		if currentAgentVersion != preConditionVersionStr {
			return errors.Errorf(
				"unable to set agent version for model. The agent version has changed to %q",
				currentAgentVersion,
			)
		}

		// If the current version is the same as the toVersion we don't need to
		// perform the set operation. This avoids creating any churn in the
		// change log.
		if currentAgentVersion == toVersionInput.TargetVersion {
			return nil
		}

		err = tx.Query(ctx, setAgentVersionStmt, toVersionInput).Run()
		if err != nil {
			return errors.Errorf(
				"setting target agent version to %q for model: %w",
				toVersion.String(), err,
			)
		}
		return nil
	})

	if err != nil {
		return errors.Capture(err)
	}

	return nil
}

func (s *State) getModelTargetAgentVersion(
	ctx context.Context,
	tx *sqlair.TX,
) (string, error) {
	var dbVal agentVersionTarget
	stmt, err := s.Prepare(
		"SELECT &agentVersionTarget.* FROM agent_version",
		dbVal,
	)
	if err != nil {
		return "", errors.Capture(err)
	}

	err = tx.Query(ctx, stmt).Get(&dbVal)
	if errors.Is(err, sqlair.ErrNoRows) {
		return "", errors.New("no target agent version has previously been set for the controller's model")
	}

	return dbVal.TargetVersion, err
}
