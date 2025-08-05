// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"

	"github.com/canonical/sqlair"

	"github.com/juju/juju/core/database"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/domain"
	modelerrors "github.com/juju/juju/domain/model/errors"
	"github.com/juju/juju/internal/errors"
)

// State is responsible for accessing the controller/model DB to retrieve the
// controller/model config keys required for the container config.
type State struct {
	*domain.StateBase
}

// NewState creates a new State object.
func NewState(modelFactory database.TxnRunnerFactory) *State {
	return &State{
		StateBase: domain.NewStateBase(modelFactory),
	}
}

// GetModelConfigKeyValues returns the values of the specified model config
// keys from the model database. If a key cannot be found in model config, it
// will be omitted from the result. If no keys are specified, then this method
// returns an empty map.
func (s *State) GetModelConfigKeyValues(
	ctx context.Context,
	keys ...string,
) (map[string]string, error) {
	if len(keys) == 0 {
		return map[string]string{}, nil
	}

	db, err := s.DB(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}

	input := make(sqlair.S, 0, len(keys))
	for _, key := range keys {
		input = append(input, key)
	}

	stmt, err := s.Prepare(`
SELECT (key, value) AS (&modelConfigRow.*)
FROM model_config
WHERE key in ($S[:])
`, input, modelConfigRow{})

	if err != nil {
		return nil, errors.Errorf(
			"preparing get model config key values: %w", domain.CoerceError(err))

	}

	result := make([]modelConfigRow, 0, len(keys))
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt, &input).GetAll(&result)
		if errors.Is(err, sqlair.ErrNoRows) {
			return nil
		} else if err != nil {
			return errors.Errorf(
				"getting model config key values: %w",
				domain.CoerceError(err))

		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	rval := make(map[string]string, len(result))
	for _, row := range result {
		rval[row.Key] = row.Value
	}

	return rval, nil
}

// ModelID returns the UUID of the current model. If the model cannot be found,
// an error is returned satisfying [modelerrors.NotFound].
func (s *State) ModelID(ctx context.Context) (model.UUID, error) {
	db, err := s.DB(ctx)
	if err != nil {
		return "", errors.Capture(err)
	}

	stmt, err := s.Prepare(`
SELECT (uuid) AS (&modelInfo.*)
FROM model
`, modelInfo{})

	if err != nil {
		return "", errors.Errorf(
			"preparing get model statement: %w", domain.CoerceError(err))

	}

	result := make([]modelInfo, 0, 1)
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		return tx.Query(ctx, stmt).GetAll(&result)
	})

	if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
		return "", modelerrors.NotFound
	}
	if len(result) == 0 {
		return "", modelerrors.NotFound
	}

	return result[0].ID, nil
}
