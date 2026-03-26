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
VALUES ($charmTracingConfig.*)
ON CONFLICT (key) DO UPDATE
SET value = $charmTracingConfig.value`
	insertStmt, err := st.Prepare(insertQuery, charmTracingConfig{})
	if err != nil {
		return err
	}

	charmTracingConfigs := make([]charmTracingConfig, 0, len(insertions))
	for key, value := range insertions {
		charmTracingConfigs = append(charmTracingConfigs, charmTracingConfig{
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

		for _, config := range charmTracingConfigs {
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

// GetCharmTracingConfig returns the tracing config from the state.
func (st *State) GetCharmTracingConfig(ctx context.Context) (map[string]string, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return nil, err
	}

	query := `SELECT &charmTracingConfig.* FROM charm_tracing_config`
	stmt, err := st.Prepare(query, charmTracingConfig{})
	if err != nil {
		return nil, err
	}

	var configs []charmTracingConfig
	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if err := tx.Query(ctx, stmt).GetAll(&configs); err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("getting charm tracing configs: %w", err)
		}
		return nil
	}); err != nil {
		return nil, err
	}

	charmTracingConfigMap := make(map[string]string)
	for _, config := range configs {
		charmTracingConfigMap[config.Key] = config.Value
	}
	return charmTracingConfigMap, nil
}

type charmTracingConfig struct {
	Key   string `db:"key"`
	Value string `db:"value"`
}
