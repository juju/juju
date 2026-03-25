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

// SetTracingConfig sets the tracing config in the state.
func (st *State) SetTracingConfig(ctx context.Context, insertions map[string]string, deletions []string) error {
	db, err := st.DB(ctx)
	if err != nil {
		return err
	}

	type tracingKeys []string

	deleteQuery := `
DELETE FROM tracing_config
WHERE key IN ($tracingKeys[:])`
	deleteStmt, err := st.Prepare(deleteQuery, tracingKeys{})
	if err != nil {
		return err
	}

	insertQuery := `
INSERT INTO tracing_config (key, value)
VALUES ($tracingConfig.*)
ON CONFLICT (key) DO UPDATE
SET value = $tracingConfig.value`
	insertStmt, err := st.Prepare(insertQuery, tracingConfig{})
	if err != nil {
		return err
	}

	tracingConfigs := make([]tracingConfig, 0, len(insertions))
	for key, value := range insertions {
		tracingConfigs = append(tracingConfigs, tracingConfig{
			Key:   key,
			Value: value,
		})
	}

	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if len(deletions) > 0 {
			if err := tx.Query(ctx, deleteStmt, tracingKeys(deletions)).Run(); err != nil {
				return errors.Errorf("deleting tracing configs: %w", err)
			}
		}

		for _, config := range tracingConfigs {
			if err := tx.Query(ctx, insertStmt, config).Run(); err != nil {
				return errors.Errorf("inserting tracing configs: %w", err)
			}
		}
		return nil
	}); err != nil {
		return err
	}
	return nil
}

// GetTracingConfig returns the tracing config from the state.
func (st *State) GetTracingConfig(ctx context.Context) (map[string]string, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return nil, err
	}

	query := `SELECT &tracingConfig.* FROM tracing_config`
	stmt, err := st.Prepare(query, tracingConfig{})
	if err != nil {
		return nil, err
	}

	var configs []tracingConfig
	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if err := tx.Query(ctx, stmt).GetAll(&configs); err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("getting tracing configs: %w", err)
		}
		return nil
	}); err != nil {
		return nil, err
	}

	tracingConfigMap := make(map[string]string)
	for _, config := range configs {
		tracingConfigMap[config.Key] = config.Value
	}
	return tracingConfigMap, nil
}

type tracingConfig struct {
	Key   string `db:"key"`
	Value string `db:"value"`
}
