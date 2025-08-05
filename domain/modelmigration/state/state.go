// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"

	"github.com/canonical/sqlair"
	"github.com/juju/collections/set"

	"github.com/juju/juju/core/database"
	"github.com/juju/juju/domain"
	"github.com/juju/juju/internal/errors"
)

// State represents the access method for interacting the underlying model
// during model migration.
type State struct {
	*domain.StateBase
}

// New creates a new [State]
func New(modelFactory database.TxnRunnerFactory) *State {
	return &State{
		StateBase: domain.NewStateBase(modelFactory),
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
SELECT (controller_uuid) AS (&ModelInfo.*)
FROM model`, ModelInfo{})

	if err != nil {
		return "", errors.Errorf("preparing get controller uuid statement: %w", err)
	}

	result := ModelInfo{}
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
