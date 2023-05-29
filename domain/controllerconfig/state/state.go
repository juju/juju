// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"

	"github.com/juju/errors"

	jujucontroller "github.com/juju/juju/controller"
	"github.com/juju/juju/domain"
)

// State represents a type for interacting with the underlying state.
type State struct {
	*domain.StateBase
}

// NewState returns a new State for interacting with the underlying state.
func NewState(factory domain.DBFactory) *State {
	return &State{
		StateBase: domain.NewStateBase(factory),
	}
}

// ControllerConfig returns the current configuration in the database.
func (st *State) ControllerConfig(ctx context.Context) (jujucontroller.Config, error) {
	db, err := st.DB()
	if err != nil {
		return nil, errors.Trace(err)
	}

	q := `
SELECT key,value FROM controller_config`[1:]

	var result jujucontroller.Config
	err = db.StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, q)
		if err != nil {
			return errors.Trace(err)
		}

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
func (st *State) UpdateControllerConfig(ctx context.Context, updateAttrs jujucontroller.Config, removeAttrs []string) error {
	db, err := st.DB()
	if err != nil {
		return errors.Trace(err)
	}

	// Validate the updateAttrs.
	fields, _, err := jujucontroller.ConfigSchema.ValidationSchema()
	if err != nil {
		return errors.Trace(err)
	}
	for k := range updateAttrs {
		if field, ok := fields[k]; ok {
			v, err := field.Coerce(updateAttrs[k], []string{k})
			if err != nil {
				return err
			}
			updateAttrs[k] = v
		}
	}

	err = db.StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		// Remove the attributes
		for _, r := range removeAttrs {
			q := `
DELETE FROM controller_config
WHERE key = ?`[1:]
			if _, err := tx.ExecContext(ctx, q, r); err != nil {
				return errors.Trace(err)
			}
		}

		// Update the attributes.
		for k := range updateAttrs {
			q := `
INSERT INTO controller_config (key, value)
VALUES (?, ?)
  ON CONFLICT(key) DO UPDATE SET value=?`[1:]
			if _, err := tx.ExecContext(ctx, q, k, updateAttrs[k], updateAttrs[k]); err != nil {
				return errors.Trace(err)
			}
		}

		return nil
	})

	return errors.Trace(err)
}

// controllerConfigFromRows returns controller config info from rows returned from the backing DB.
func controllerConfigFromRows(rows *sql.Rows) (jujucontroller.Config, error) {
	result := jujucontroller.Config{}

	// Get ValidationSchema to coerce values.
	fields, _, err := jujucontroller.ConfigSchema.ValidationSchema()
	if err != nil {
		return nil, errors.Trace(err)
	}

	for rows.Next() {
		var key string
		var value interface{}

		if err := rows.Scan(&key, &value); err != nil {
			_ = rows.Close()
			return nil, errors.Trace(err)
		}

		// Coerce the value to the correct type.
		if field, ok := fields[key]; ok {
			v, err := field.Coerce(value, []string{key})
			if err != nil {
				return nil, err
			}
			result[key] = v
		}
	}

	return result, errors.Trace(rows.Err())
}
