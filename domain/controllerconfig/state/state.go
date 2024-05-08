// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"

	"github.com/canonical/sqlair"
	"github.com/juju/collections/transform"
	"github.com/juju/errors"

	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/database"
	"github.com/juju/juju/domain"
	internaldatabase "github.com/juju/juju/internal/database"
)

// State represents a type for interacting with the underlying state.
type State struct {
	*domain.StateBase
}

// NewState returns a new State for interacting with the underlying state.
func NewState(factory database.TxnRunnerFactory) *State {
	return &State{
		StateBase: domain.NewStateBase(factory),
	}
}

// ControllerConfig returns the current configuration in the database.
func (st *State) ControllerConfig(ctx context.Context) (map[string]string, error) {
	db, err := st.DB()
	if err != nil {
		return nil, errors.Trace(err)
	}

	stmt, err := st.Prepare("SELECT &dbKeyValue.* FROM v_controller_config", dbKeyValue{})
	if err != nil {
		return nil, errors.Trace(err)
	}

	result := make(map[string]string)
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var keyValues []dbKeyValue
		if err := tx.Query(ctx, stmt).GetAll(&keyValues); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return nil
			}
			return errors.Trace(err)
		}

		for _, kv := range keyValues {
			result[kv.Key] = kv.Value
		}

		return nil
	})

	return result, err
}

// UpdateControllerConfig allows changing some of the configuration
// for the controller. Changes passed in updateAttrs will be applied
// to the current config, and keys in removeAttrs will be unset (and
// so revert to their defaults). Only a subset of keys can be changed
// after bootstrapping.
// ValidateModification is a function that will be called with the current
// config, and should return an error if the modification is not allowed.
func (st *State) UpdateControllerConfig(
	ctx context.Context,
	updateAttrs map[string]string, removeAttrs []string,
	validateModification func(map[string]string) error,
) error {
	db, err := st.DB()
	if err != nil {
		return errors.Trace(err)
	}

	selectStmt, err := st.Prepare("SELECT &dbKeyValue.* FROM v_controller_config", dbKeyValue{})
	if err != nil {
		return errors.Trace(err)
	}

	deleteStmt, err := st.Prepare("DELETE FROM controller_config WHERE key IN ($S[:])", sqlair.S{})
	if err != nil {
		return errors.Trace(err)
	}
	removeKeys := sqlair.S(transform.Slice(removeAttrs, func(s string) any { return any(s) }))

	var (
		controllerStmt *sqlair.Statement
		uuid           dbController
	)
	if v, ok := updateAttrs[controller.ControllerUUIDKey]; ok && v != "" {
		controllerStmt, err = st.Prepare(`INSERT INTO controller (uuid) VALUES ($dbController.uuid)`, dbController{})
		if err != nil {
			return errors.Trace(err)
		}
		uuid = dbController{UUID: v}
	}

	updateStmt, err := st.Prepare(`
INSERT INTO controller_config (key, value)
VALUES ($dbKeyValue.*)
  ON CONFLICT(key) DO UPDATE SET value=excluded.value`, dbKeyValue{})
	if err != nil {
		return errors.Trace(err)
	}
	updateKeyValues := make([]dbKeyValue, 0)
	for k, v := range updateAttrs {
		if k == controller.ControllerUUIDKey {
			continue
		}
		updateKeyValues = append(updateKeyValues, dbKeyValue{
			Key:   k,
			Value: v,
		})
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		// Check keys and values are valid between current and new config.
		var keyValues []dbKeyValue
		if err := tx.Query(ctx, selectStmt).GetAll(&keyValues); err != nil {
			if !errors.Is(err, sql.ErrNoRows) {
				return errors.Trace(err)
			}
		}

		current := make(map[string]string)
		for _, kv := range keyValues {
			current[kv.Key] = kv.Value
		}
		if err := validateModification(current); err != nil {
			return errors.Trace(err)
		}

		// Insert the controller UUID, we ignore if it already exists, as
		// the errors will tell use why it might not of worked.
		if controllerStmt != nil {
			if err := tx.Query(ctx, controllerStmt, uuid).Run(); err != nil {
				if !internaldatabase.IsErrConstraintPrimaryKey(err) && !internaldatabase.IsErrConstraintUnique(err) {
					return errors.Trace(err)
				}

				if uuid.UUID != current[controller.ControllerUUIDKey] {
					return errors.Errorf("controller UUID cannot be changed")
				}
			}
		}

		// Update the attributes.
		if err := tx.Query(ctx, updateStmt, updateKeyValues).Run(); err != nil {
			return errors.Trace(err)
		}

		// Remove the attributes
		if len(removeKeys) > 0 {
			if err := tx.Query(ctx, deleteStmt, removeKeys).Run(); err != nil {
				return errors.Trace(err)
			}
		}

		return nil
	})

	return errors.Trace(err)
}

// AllKeysQuery returns a SQL statement that will return
// all known controller configuration keys.
func (*State) AllKeysQuery() string {
	return "SELECT key FROM v_controller_config"
}
