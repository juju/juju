// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"
	"fmt"

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

	binds, vals := database.SliceToPlaceholder(attrs)
	stmt := fmt.Sprintf(`
SELECT key FROM model_config WHERE key IN (%s)
`, binds)

	return rval, db.StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, stmt, vals...)
		if err != nil {
			return fmt.Errorf("deducing model config attrs set: %w", err)
		}
		defer rows.Close()

		var key string
		for rows.Next() {
			if err := rows.Scan(&key); err != nil {
				return fmt.Errorf(
					"scanning model config attribute into result set: %w",
					err,
				)
			}
			rval = append(rval, key)
		}
		return rows.Err()
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

	insertBinds, insertVals := database.MapToMultiPlaceholder(conf)
	insertStmt := fmt.Sprintf(`
INSERT INTO model_config (key, value) VALUES %s
`, insertBinds)
	deleteStmt := "DELETE FROM model_config"

	return db.StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
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
	})
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

	deleteBinds, deleteVals := database.SliceToPlaceholder(removeAttrs)
	deleteStmt := fmt.Sprintf(`
DELETE FROM model_config
WHERE key IN (%s)
`[1:], deleteBinds)

	upsertBinds, upsertVals := database.MapToMultiPlaceholder(updateAttrs)
	upsertStmt := fmt.Sprintf(`
INSERT INTO model_config (key, value) VALUES %s
ON CONFLICT(key) DO UPDATE
SET value = excluded.value
WHERE key = excluded.key
`[1:], upsertBinds)

	return db.StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		if len(deleteVals) != 0 {
			_, err := tx.ExecContext(ctx, deleteStmt, deleteVals...)
			if err != nil {
				return fmt.Errorf("removing model config keys: %w", err)
			}
		}

		if len(upsertVals) == 0 {
			return nil
		}

		_, err := tx.ExecContext(ctx, upsertStmt, upsertVals...)
		return errors.Trace(err)
	})
}
