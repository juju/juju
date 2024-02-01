// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"

	"github.com/juju/errors"

	"github.com/juju/juju/core/database"
	"github.com/juju/juju/domain"
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

	query := "SELECT key, value FROM controller_config"

	var result map[string]string
	err = db.StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, query)
		if err != nil {
			return errors.Trace(err)
		}
		defer rows.Close()

		result, err = controllerConfigFromRows(rows)
		return errors.Trace(err)
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
func (st *State) UpdateControllerConfig(ctx context.Context, updateAttrs map[string]string, removeAttrs []string, validateModification func(map[string]string) error) error {
	db, err := st.DB()
	if err != nil {
		return errors.Trace(err)
	}

	selectQuery := "SELECT key, value FROM controller_config"
	deleteQuery := "DELETE FROM controller_config WHERE key = ?"
	updateQuery := `
INSERT INTO controller_config (key, value)
VALUES (?, ?)
  ON CONFLICT(key) DO UPDATE SET value=?`

	err = db.StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		// Check keys and values are valid between current and new config.
		rows, err := tx.QueryContext(ctx, selectQuery)
		if err != nil {
			return errors.Trace(err)
		}
		current, err := controllerConfigFromRows(rows)
		if err != nil {
			return errors.Trace(err)
		}
		if err := validateModification(current); err != nil {
			return errors.Trace(err)
		}

		// Remove the attributes
		for _, r := range removeAttrs {
			if _, err := tx.ExecContext(ctx, deleteQuery, r); err != nil {
				return errors.Trace(err)
			}
		}

		// Update the attributes.
		for key := range updateAttrs {
			value := updateAttrs[key]
			if _, err := tx.ExecContext(ctx, updateQuery, key, value, value); err != nil {
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
	return "SELECT key FROM controller_config"
}

// controllerConfigFromRows returns controller config info from rows returned from the backing DB.
func controllerConfigFromRows(rows *sql.Rows) (map[string]string, error) {
	result := make(map[string]string)

	for rows.Next() {
		var key string
		var value string

		if err := rows.Scan(&key, &value); err != nil {
			return nil, errors.Trace(err)
		}

		result[key] = value
	}

	return result, errors.Trace(rows.Err())
}
