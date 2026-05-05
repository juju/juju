// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller

import (
	"context"

	"github.com/canonical/sqlair"

	coredb "github.com/juju/juju/core/database"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/domain"
	"github.com/juju/juju/internal/errors"
)

// State provides direct database access to the controller
// database for provisioning info retrieval.
type State struct {
	*domain.StateBase
	logger logger.Logger
}

// NewState returns a new controller state reference.
func NewState(factory coredb.TxnRunnerFactory, logger logger.Logger) *State {
	return &State{
		StateBase: domain.NewStateBase(factory),
		logger:    logger,
	}
}

// controllerConfigRow is a key-value pair from the controller_config table.
type controllerConfigRow struct {
	Key   string `db:"key"`
	Value string `db:"value"`
}

// GetControllerConfig retrieves controller configuration from the
// controller database.
func (st *State) GetControllerConfig(ctx context.Context) (map[string]any, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}

	stmt, err := st.Prepare(`
SELECT &controllerConfigRow.*
FROM controller_config
`, controllerConfigRow{})
	if err != nil {
		return nil, errors.Capture(err)
	}

	var rows []controllerConfigRow
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt).GetAll(&rows)
		if errors.Is(err, sqlair.ErrNoRows) {
			return nil
		}
		return err
	})
	if err != nil {
		return nil, errors.Errorf("getting controller config: %w", err)
	}

	result := make(map[string]any, len(rows))
	for _, row := range rows {
		result[row.Key] = row.Value
	}
	return result, nil
}
