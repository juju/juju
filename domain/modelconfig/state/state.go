// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/canonical/sqlair"
	"github.com/juju/errors"

	coredatabase "github.com/juju/juju/core/database"
	"github.com/juju/juju/domain"
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

// AgentVersion returns the current models agent version. If no agent version
// can be found an error satisfying [errors.NotFound] will be returned.
func (st *State) AgentVersion(ctx context.Context) (string, error) {
	db, err := st.DB()
	if err != nil {
		return "", errors.Trace(err)
	}

	q := `SELECT &dbAgentVersion.target_version FROM agent_version`

	rval := dbAgentVersion{}

	stmt, err := st.Prepare(q, rval)
	if err != nil {
		return "", errors.Trace(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt).Get(&rval)
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("agent version %w", errors.NotFound)
		} else if err != nil {
			return fmt.Errorf("retrieving current agent version: %w", err)
		}
		return nil
	})
	if err != nil {
		return "", errors.Trace(err)
	}

	return rval.TargetAgentVersion, nil
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

	stmt, err := st.Prepare(`
SELECT &dbKey.key FROM model_config WHERE key IN ($dbKeys[:])
`, dbKeys{}, dbKey{})
	if err != nil {
		return rval, errors.Trace(err)
	}

	return rval, db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var keys []dbKey
		err := tx.Query(ctx, stmt, dbKeys(attrs)).GetAll(&keys)
		if err != nil {
			return fmt.Errorf("getting model config attrs set: %w", err)
		}

		rval = make([]string, len(keys))
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

	stmt, err := st.Prepare(`SELECT &dbKeyValue.* FROM model_config`, dbKeyValue{})
	if err != nil {
		return config, errors.Trace(err)
	}

	return config, db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var result []dbKeyValue
		if err := tx.Query(ctx, stmt).GetAll(&result); err != nil && !errors.Is(err, sql.ErrNoRows) {
			return errors.Trace(err)
		}

		for _, kv := range result {
			config[kv.Key] = kv.Value
		}
		return nil
	})
}

// SetModelConfig is responsible for overwriting the currently set model config
// with new values. SetModelConfig will remove all existing model config even
// when an empty map is supplied.
func (st *State) SetModelConfig(
	ctx context.Context,
	config map[string]string,
) error {
	db, err := st.DB()
	if err != nil {
		return errors.Trace(err)
	}

	selectQuery := `SELECT &dbKeyValue.* FROM model_config`
	selectStmt, err := st.Prepare(selectQuery, dbKeyValue{})
	if err != nil {
		return fmt.Errorf("preparing select query: %w", err)
	}

	insertQuery := `INSERT INTO model_config (*) VALUES ($dbKeyValue.*)`
	insertStmt, err := st.Prepare(insertQuery, dbKeyValue{})
	if err != nil {
		return fmt.Errorf("preparing insert query: %w", err)
	}

	updateQuery := `UPDATE model_config SET value = $dbKeyValue.value WHERE key = $dbKeyValue.key`
	updateStmt, err := st.Prepare(updateQuery, dbKeyValue{})
	if err != nil {
		return fmt.Errorf("preparing update query: %w", err)
	}

	deleteQuery := `DELETE FROM model_config WHERE key IN ($dbKeys[:])`
	deleteStmt, err := st.Prepare(deleteQuery, dbKeys{})
	if err != nil {
		return fmt.Errorf("preparing delete query: %w", err)
	}

	return db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var keyValues []dbKeyValue
		if err := tx.Query(ctx, selectStmt).GetAll(&keyValues); err != nil {
			if !errors.Is(err, sql.ErrNoRows) {
				return fmt.Errorf("getting model config values: %w", err)
			}
		}

		current := make(map[string]string)
		for _, kv := range keyValues {
			current[kv.Key] = kv.Value
		}

		// Work out what to insert, update and delete from the current config
		// and the new config.
		var (
			insert = make(map[string]string)
			update = make(map[string]string)
			delete = make(map[string]struct{})
		)
		for k, v := range config {
			cv, ok := current[k]

			// If the key is known and isn't the same value, update it.
			if ok {
				if cv != v {
					update[k] = v
				}
				// We already have the correct value, do nothing.
				continue
			}

			// If the key is unknown, insert it.
			insert[k] = v
		}
		for k := range current {
			if _, ok := config[k]; !ok {
				delete[k] = struct{}{}
			}
		}

		// The order of operations is important here. We must insert new keys
		// before updating existing keys, as the update statement will fail if
		// the key does not exist. Deleting keys must be done last, as the
		// update statement will fail if the key is deleted. It shouldn't
		// happen, but it's better to be safe in that case.

		// Insert any new keys.
		if len(insert) > 0 {
			insertKV := make([]dbKeyValue, 0, len(insert))
			for k, v := range insert {
				insertKV = append(insertKV, dbKeyValue{Key: k, Value: v})
			}
			if err := tx.Query(ctx, insertStmt, insertKV).Run(); err != nil {
				return fmt.Errorf("inserting model config values: %w", err)
			}
		}

		// Update any keys that have changed.
		for k, v := range update {
			if err := tx.Query(ctx, updateStmt, dbKeyValue{Key: k, Value: v}).Run(); err != nil {
				return fmt.Errorf("updating model config key %q: %w", k, err)
			}
		}

		// Delete any keys that are no longer in the config.
		if len(delete) > 0 {
			deleteKeys := make(dbKeys, 0, len(delete))
			for k := range delete {
				deleteKeys = append(deleteKeys, k)
			}
			if err := tx.Query(ctx, deleteStmt, deleteKeys).Run(); err != nil {
				return fmt.Errorf("deleting model config keys: %w", err)
			}
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

	deleteStmt, err := st.Prepare(`DELETE FROM model_config WHERE key IN ($dbKeys[:])`, dbKeys{})
	if err != nil {
		return errors.Trace(err)
	}

	upsertStmt, err := st.Prepare(`
INSERT INTO model_config (*) VALUES ($dbKeyValue.*)
ON CONFLICT(key) DO UPDATE
SET value = excluded.value
WHERE key = excluded.key
`[1:], dbKeyValue{})
	if err != nil {
		return errors.Trace(err)
	}

	return db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if len(removeAttrs) != 0 {
			if err := tx.Query(ctx, deleteStmt, dbKeys(removeAttrs)).Run(); err != nil {
				return fmt.Errorf("removing model config keys: %w", err)
			}
		}

		for k, v := range updateAttrs {
			if err := tx.Query(ctx, upsertStmt, dbKeyValue{Key: k, Value: v}).Run(); err != nil {
				return errors.Trace(err)
			}
		}
		return nil
	})
}

// NamespaceForWatchModelConfig returns the namespace identifier used for
// watching model configuration changes.
func (st *State) NamespaceForWatchModelConfig() string {
	return "model_config"
}

// SpaceExists checks if the space identified by the given space name exists.
func (st *State) SpaceExists(ctx context.Context, spaceName string) (bool, error) {
	db, err := st.DB()
	if err != nil {
		return false, errors.Trace(err)
	}

	stmt, err := st.Prepare(`SELECT &dbSpace.* FROM space WHERE name = $dbSpace.name`, dbSpace{})
	if err != nil {
		return false, errors.Trace(err)
	}

	var exists bool
	return exists, db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if err := tx.Query(ctx, stmt, dbSpace{Space: spaceName}).Get(&dbSpace{}); errors.Is(err, sql.ErrNoRows) {
			return nil
		} else if err != nil {
			return errors.Annotatef(err, "checking space %q exists", spaceName)
		}
		exists = true
		return nil
	})
}

// AllKeysQuery returns a SQL statement that will return all known model config
// keys.
func (st *State) AllKeysQuery() string {
	return "SELECT key from model_config"
}
