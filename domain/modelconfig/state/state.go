// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/canonical/sqlair"
	"github.com/juju/collections/transform"
	"github.com/juju/errors"

	coredatabase "github.com/juju/juju/core/database"
	"github.com/juju/juju/domain"
	"github.com/juju/juju/internal/database"
)

// State is a reference to the underlying data accessor for ModelConfig data.
type State struct {
	*domain.StateBase
}

// NewState creates a new ModelConfig state struct for querying the state.
func NewState(factory coredatabase.TxnRunnerFactory) *State {
	return &State{
		StateBase: domain.NewStateBase(factory),
	}
}

// key represents the key column from a model_config row.
// Once SQLair supports scalar types the key can be selected directly into a
// string and this struct will no longer be needed.
type key struct {
	Key string `db:"key"`
}

// AllKeysQuery returns a SQL statement that will return all known model config
// keys.
func (st *State) AllKeysQuery() string {
	return "SELECT key from model_config"
}

// ModelConfigHasAttributes will take a set of model config attributes and
// return the subset of keys that are set and exist in the Model Config.
func (st *State) ModelConfigHasAttributes(
	ctx context.Context,
	attrs []string,
) ([]string, error) {
	rval := []string{}
	if len(attrs) == 0 {
		return rval, nil
	}

	db, err := st.DB()
	if err != nil {
		return rval, errors.Trace(err)
	}

	attrsSlice := sqlair.S(transform.Slice(attrs, func(s string) any { return any(s) }))
	stmt, err := sqlair.Prepare(`
SELECT &key.key FROM model_config WHERE key IN ($S[:])
`, sqlair.S{}, key{})
	if err != nil {
		return rval, errors.Trace(err)
	}

	return rval, db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var keys []key
		err := tx.Query(ctx, stmt, attrsSlice).GetAll(&keys)
		if err != nil {
			return fmt.Errorf("getting model config attrs set: %w", err)
		}

		rval = make([]string, len(keys), len(keys))
		for i, key := range keys {
			rval[i] = key.Key
		}
		return nil
	})
}

// ModelConfig returns the current model config key,value pairs for the model.
func (st *State) ModelConfig(ctx context.Context) (map[string]string, error) {
	config := map[string]string{}

	db, err := st.DB()
	if err != nil {
		return config, errors.Trace(err)
	}

	return config, db.StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		stmt := `
SELECT key,
       value
FROM model_config
`
		rows, err := tx.QueryContext(ctx, stmt)
		if err != nil {
			return fmt.Errorf("getting model config values: %w", err)
		}
		defer rows.Close()

		var (
			key,
			val string
		)
		for rows.Next() {
			if err := rows.Scan(&key, &val); err != nil {
				return errors.Trace(err)
			}
			config[key] = val
		}
		return rows.Err()
	})
}

// SetModelConfig is responsible for overwriting the currently set model config
// with new values. SetModelConfig will remove all existing model config even
// when an empty map is supplied.
func (st *State) SetModelConfig(
	ctx context.Context,
	conf map[string]string,
) error {
	db, err := st.DB()
	if err != nil {
		return errors.Trace(err)
	}

	return db.StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		return SetModelConfig(ctx, conf, tx)
	})
}

// SetModelConfig is responsible for overwriting the currently set model config
// with new values. SetModelConfig will remove all existing model config even
// when an empty map is supplied.
func SetModelConfig(
	ctx context.Context,
	conf map[string]string,
	tx *sql.Tx,
) error {
	insertBinds, insertVals := database.MapToMultiPlaceholder(conf)
	insertStmt := fmt.Sprintf(`
INSERT INTO model_config (key, value) VALUES %s
`, insertBinds)
	deleteStmt := "DELETE FROM model_config"

	_, err := tx.ExecContext(ctx, deleteStmt)
	if err != nil {
		return fmt.Errorf("deleting model config attributes: %w", err)
	}

	if len(insertVals) == 0 {
		return nil
	}

	if _, err := tx.ExecContext(ctx, insertStmt, insertVals...); err != nil {
		return fmt.Errorf("setting model config attributes: %w", err)
	}
	return nil
}

// UpdateModelConfig is responsible for updating the model's config key and
// values. This function will allow the addition and updating of attributes.
// Attributes can also be removed by keys if they exist for the current model.
func (st *State) UpdateModelConfig(
	ctx context.Context,
	updateAttrs map[string]string,
	removeAttrs []string,
) error {
	db, err := st.DB()
	if err != nil {
		return errors.Trace(err)
	}

	removeAttrsSlice := sqlair.S(transform.Slice(removeAttrs, func(s string) any { return any(s) }))
	deleteStmt, err := sqlair.Prepare(`
DELETE FROM model_config
WHERE key IN ($S[:])
`[1:], sqlair.S{})
	if err != nil {
		return errors.Trace(err)
	}

	upsertStmt, err := sqlair.Prepare(`
INSERT INTO model_config (key, value) VALUES ($M.key, $M.value)
ON CONFLICT(key) DO UPDATE
SET value = excluded.value
WHERE key = excluded.key
`[1:], sqlair.M{})
	if err != nil {
		return errors.Trace(err)
	}

	return db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if len(removeAttrsSlice) != 0 {
			if err := tx.Query(ctx, deleteStmt, removeAttrsSlice).Run(); err != nil {
				return fmt.Errorf("removing model config keys: %w", err)
			}
		}

		for k, v := range updateAttrs {
			if err := tx.Query(ctx, upsertStmt, sqlair.M{"key": k, "value": v}).Run(); err != nil {
				return errors.Trace(err)
			}
		}
		return nil
	})
}
