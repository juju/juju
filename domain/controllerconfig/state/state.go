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

// ControllerConfig ...
func (st *State) ControllerConfig(ctx context.Context) (jujucontroller.Config, error) {
	return nil, nil
}

// UpdateControllerConfig allows changing some of the configuration
// for the controller. Changes passed in updateAttrs will be applied
// to the current config, and keys in removeAttrs will be unset (and
// so revert to their defaults). Only a subset of keys can be changed
// after bootstrapping.
func (st *State) UpdateControllerConfig(ctx context.Context, updateAttrs map[string]interface{}, removeAttrs []string) error {
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
		// Remove the attributes first, so that we don't end up with
		for _, r := range removeAttrs {
			q := `
DELETE FROM controller_config
WHERE key = ?`[1:]
			if _, err := tx.ExecContext(ctx, q, r); err != nil {
				return errors.Trace(err)
			}
		}

		for k := range updateAttrs {
			q := `
INSERT INTO controller_config (key, value)
VALUES (?, ?)
  ON CONFLICT(key) DO UPDATE SET value=?`[1:]
			if _, err := tx.ExecContext(ctx, q, k, updateAttrs[k]); err != nil {
				return errors.Trace(err)
			}
		}

		return nil
	})

	return errors.Trace(err)
}

func checkUpdateControllerConfig(name string) error {
	if !jujucontroller.ControllerOnlyAttribute(name) {
		return errors.Errorf("unknown controller config setting %q", name)
	}
	if !jujucontroller.AllowedUpdateConfigAttributes.Contains(name) {
		return errors.Errorf("can't change %q after bootstrap", name)
	}
	return nil
}
