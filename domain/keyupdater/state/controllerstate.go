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

// ControllerState provides a state access layer for accessing a controller's
// ssh keys via controller config.
type ControllerState struct {
	*domain.StateBase
}

// GetControllerConfigKeys returns the controller config key and values for the
// keys supplied. If one or more keys supplied do not exist in the controller's
// config they will be omitted from the final result.
func (st *ControllerState) GetControllerConfigKeys(
	ctx context.Context,
	keys []string,
) (map[string]string, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return nil, errors.Errorf(
			"getting database when getting controller config keys: %w", err,
		)
	}

	sqlKeys := make(sqlair.S, 0, len(keys))
	for _, key := range keys {
		sqlKeys = append(sqlKeys, key)
	}

	stmt, err := st.Prepare(`
SELECT &keyValue.*
FROM v_controller_config
WHERE key IN ($S[:])
`, keyValue{}, sqlKeys)
	if err != nil {
		return nil, errors.Errorf(
			"preparing statement for getting keys from controller config: %w",
			err,
		)
	}

	keyValues := []keyValue{}
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt, sqlKeys).GetAll(&keyValues)
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return err
		}
		return nil
	})

	if err != nil {
		return nil, errors.Errorf(
			"getting controller config for keys %v: %w",
			keys, err,
		)
	}

	rval := make(map[string]string, len(keyValues))
	for _, kv := range keyValues {
		rval[kv.Key] = kv.Value
	}

	return rval, nil
}

// GetUserAuthorizedKeysForModel is responsible for returning all of the user
// authorized keys for a model.
// The following errors can be expected:
// - [modelerrors.NotFound] if the model does not exist.
func (s *ControllerState) GetUserAuthorizedKeysForModel(
	ctx context.Context,
	modelUUID model.UUID,
) ([]string, error) {
	db, err := s.DB(ctx)
	if err != nil {
		return nil, errors.Errorf(
			"getting database when getting all user public keys for model %q: %w",
			modelUUID, err,
		)
	}

	modelUUIDVal := modelUUIDValue{modelUUID.String()}

	modelExistsStmt, err := s.Prepare(`
SELECT (uuid) AS (&modelUUIDValue.model_uuid)
FROM v_model
WHERE uuid = $modelUUIDValue.model_uuid
`, modelUUIDVal)
	if err != nil {
		return nil, errors.Errorf(
			"preparing model exists statement when getting public keys for model %q: %w",
			modelUUID, err,
		)
	}

	stmt, err := s.Prepare(`
SELECT &authorizedKey.*
FROM v_model_authorized_keys
WHERE model_uuid = $modelUUIDValue.model_uuid
`, modelUUIDVal, authorizedKey{})
	if err != nil {
		return nil, errors.Errorf(
			"preparing model authorized keys statement when getting public keys for model %q: %w",
			modelUUID, err,
		)
	}

	authorizedKeys := []authorizedKey{}
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, modelExistsStmt, modelUUIDVal).Get(&modelUUIDVal)
		if errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf(
				"getting user authorized keys for model %q because the model does not exist",
				modelUUID,
			).Add(modelerrors.NotFound)
		}
		if err != nil {
			return errors.Errorf(
				"checking that model %q exists when getting user authorized keys: %w",
				modelUUID, err,
			)
		}

		err = tx.Query(ctx, stmt, modelUUIDVal).GetAll(&authorizedKeys)
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf(
				"getting user authorized keys on model %q: %w",
				modelUUID, err,
			)
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	rval := make([]string, 0, len(authorizedKeys))
	for _, authKey := range authorizedKeys {
		rval = append(rval, authKey.PublicKey)
	}

	return rval, nil
}

// NewControllerState constructs a new state for interacting with the
// underlying authorised keys of a controller via controller config.
func NewControllerState(factory database.TxnRunnerFactory) *ControllerState {
	return &ControllerState{
		StateBase: domain.NewStateBase(factory),
	}
}
