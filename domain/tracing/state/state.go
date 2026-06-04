// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"

	"github.com/canonical/sqlair"

	"github.com/juju/juju/core/database"
	"github.com/juju/juju/domain"
	"github.com/juju/juju/internal/errors"
)

// State implements persistence for tracing configuration.
type State struct {
	*domain.StateBase
}

// NewState returns a new state reference.
func NewState(factory database.TxnRunnerFactory) *State {
	return &State{
		StateBase: domain.NewStateBase(factory),
	}
}

// SetCharmTracingConfig sets the tracing config in the state.
func (st *State) SetCharmTracingConfig(ctx context.Context, insertions map[string]string, deletions []string) error {
	return st.setCharmTracingConfig(ctx, insertions, deletions)
}

// SetWorkloadTracingConfig sets the workload tracing config in the state.
func (st *State) SetWorkloadTracingConfig(ctx context.Context, insertions map[string]string, deletions []string) error {
	return st.setWorkloadTracingConfig(ctx, insertions, deletions)
}

// GetCharmTracingConfig returns the tracing config from the state.
func (st *State) GetCharmTracingConfig(ctx context.Context) (map[string]string, error) {
	return st.getCharmTracingConfig(ctx)
}

// GetWorkloadTracingConfig returns the workload tracing config from the state.
func (st *State) GetWorkloadTracingConfig(ctx context.Context) (map[string]string, error) {
	return st.getWorkloadTracingConfig(ctx)
}

func (st *State) setCharmTracingConfig(
	ctx context.Context,
	insertions map[string]string,
	deletions []string,
) error {
	db, err := st.DB(ctx)
	if err != nil {
		return err
	}

	type tracingKeys []string

	deleteQuery := `
DELETE FROM charm_tracing_config
WHERE key IN ($tracingKeys[:])`
	deleteStmt, err := st.Prepare(deleteQuery, tracingKeys{})
	if err != nil {
		return err
	}

	insertQuery := `
INSERT INTO charm_tracing_config (key, value)
VALUES ($tracingConfigEntry.*)
ON CONFLICT (key) DO UPDATE
SET value = $tracingConfigEntry.value
`
	insertStmt, err := st.Prepare(insertQuery, tracingConfigEntry{})
	if err != nil {
		return err
	}

	tracingConfigs := make([]tracingConfigEntry, 0, len(insertions))
	for key, value := range insertions {
		tracingConfigs = append(tracingConfigs, tracingConfigEntry{
			Key:   key,
			Value: value,
		})
	}

	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if len(deletions) > 0 {
			if err := tx.Query(ctx, deleteStmt, tracingKeys(deletions)).Run(); err != nil {
				return errors.Errorf("deleting charm tracing configs: %w", err)
			}
		}

		for _, config := range tracingConfigs {
			if err := tx.Query(ctx, insertStmt, config).Run(); err != nil {
				return errors.Errorf("inserting charm tracing configs: %w", err)
			}
		}
		return nil
	}); err != nil {
		return err
	}
	return nil
}

func (st *State) setWorkloadTracingConfig(
	ctx context.Context,
	insertions map[string]string,
	deletions []string,
) error {
	db, err := st.DB(ctx)
	if err != nil {
		return err
	}

	type tracingKeys []string

	deleteQuery := `
DELETE FROM workload_tracing_config
WHERE key IN ($tracingKeys[:])`
	deleteStmt, err := st.Prepare(deleteQuery, tracingKeys{})
	if err != nil {
		return err
	}

	insertQuery := `
INSERT INTO workload_tracing_config (key, value)
VALUES ($tracingConfigEntry.*)
ON CONFLICT (key) DO UPDATE
SET value = $tracingConfigEntry.value
`
	insertStmt, err := st.Prepare(insertQuery, tracingConfigEntry{})
	if err != nil {
		return err
	}

	tracingConfigs := make([]tracingConfigEntry, 0, len(insertions))
	for key, value := range insertions {
		tracingConfigs = append(tracingConfigs, tracingConfigEntry{
			Key:   key,
			Value: value,
		})
	}

	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if len(deletions) > 0 {
			if err := tx.Query(ctx, deleteStmt, tracingKeys(deletions)).Run(); err != nil {
				return errors.Errorf("deleting workload tracing configs: %w", err)
			}
		}

		for _, config := range tracingConfigs {
			if err := tx.Query(ctx, insertStmt, config).Run(); err != nil {
				return errors.Errorf("inserting workload tracing configs: %w", err)
			}
		}
		return nil
	}); err != nil {
		return err
	}
	return nil
}

func (st *State) getCharmTracingConfig(ctx context.Context) (map[string]string, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return nil, err
	}

	query := `SELECT &tracingConfigEntry.* FROM charm_tracing_config`
	stmt, err := st.Prepare(query, tracingConfigEntry{})
	if err != nil {
		return nil, err
	}

	var tracingConfigMap map[string]string
	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		// Ensure retry safety by rebuilding config data for each transaction run.
		localConfigs := make([]tracingConfigEntry, 0)
		if err := tx.Query(ctx, stmt).GetAll(&localConfigs); err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("getting charm tracing configs: %w", err)
		}

		tracingConfigMap = configsToMap(localConfigs)
		return nil
	}); err != nil {
		return nil, err
	}

	return tracingConfigMap, nil
}

func (st *State) getWorkloadTracingConfig(ctx context.Context) (map[string]string, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return nil, err
	}

	query := `SELECT &tracingConfigEntry.* FROM workload_tracing_config`
	stmt, err := st.Prepare(query, tracingConfigEntry{})
	if err != nil {
		return nil, err
	}

	var tracingConfigMap map[string]string
	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		// Ensure retry safety by rebuilding config data for each transaction run.
		localConfigs := make([]tracingConfigEntry, 0)
		if err := tx.Query(ctx, stmt).GetAll(&localConfigs); err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("getting workload tracing configs: %w", err)
		}

		tracingConfigMap = configsToMap(localConfigs)
		return nil
	}); err != nil {
		return nil, err
	}

	return tracingConfigMap, nil
}

func configsToMap(configs []tracingConfigEntry) map[string]string {
	result := make(map[string]string, len(configs))
	for _, config := range configs {
		result[config.Key] = config.Value
	}
	return result
}

type tracingConfigEntry struct {
	Key   string `db:"key"`
	Value string `db:"value"`
}
