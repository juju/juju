// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"

	"github.com/canonical/sqlair"
	"github.com/juju/juju/core/database"
	"github.com/juju/juju/domain"
	loggingerrors "github.com/juju/juju/domain/logging/errors"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/uuid"
)

// State implements persistence for logging configuration.
type State struct {
	*domain.StateBase
}

// NewState returns a new state reference.
func NewState(factory database.TxnRunnerFactory) *State {
	return &State{
		StateBase: domain.NewStateBase(factory),
	}
}

// SetLokiEndpoint sets the Loki push API endpoint. Any previously stored
// endpoint is replaced.
func (st *State) SetLokiEndpoint(ctx context.Context, endpoint string) error {
	db, err := st.DB(ctx)
	if err != nil {
		return errors.Errorf("getting database: %w", err)
	}

	deleteStmt, err := st.Prepare(`DELETE FROM logging_loki_config`)
	if err != nil {
		return errors.Errorf("preparing delete statement: %w", err)
	}

	insertStmt, err := st.Prepare(`
INSERT INTO logging_loki_config (uuid, endpoint)
VALUES ($lokiConfig.uuid, $lokiConfig.endpoint)`, lokiConfig{})
	if err != nil {
		return errors.Errorf("preparing insert statement: %w", err)
	}

	id, err := uuid.NewUUID()
	if err != nil {
		return errors.Errorf("generating UUID: %w", err)
	}

	config := lokiConfig{
		UUID:     id.String(),
		Endpoint: endpoint,
	}

	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if err := tx.Query(ctx, deleteStmt).Run(); err != nil {
			return errors.Errorf("deleting existing loki endpoint: %w", err)
		}
		if err := tx.Query(ctx, insertStmt, config).Run(); err != nil {
			return errors.Errorf("inserting loki endpoint: %w", err)
		}
		return nil
	}); err != nil {
		return errors.Errorf("setting loki endpoint: %w", err)
	}
	return nil
}

// GetLokiEndpoint returns the configured Loki push API endpoint. If no
// endpoint is configured, an error satisfying [loggingerrors.LokiEndpointNotFound]
// is returned.
func (st *State) GetLokiEndpoint(ctx context.Context) (string, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return "", errors.Errorf("getting database: %w", err)
	}

	stmt, err := st.Prepare(`SELECT &lokiConfig.endpoint FROM logging_loki_config`, lokiConfig{})
	if err != nil {
		return "", errors.Errorf("preparing select statement: %w", err)
	}

	var config lokiConfig
	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if err := tx.Query(ctx, stmt).Get(&config); errors.Is(err, sqlair.ErrNoRows) {
			return loggingerrors.LokiEndpointNotFound
		} else if err != nil {
			return errors.Errorf("getting loki endpoint: %w", err)
		}
		return nil
	}); err != nil {
		return "", errors.Errorf("getting loki endpoint: %w", err)
	}
	return config.Endpoint, nil
}

// DeleteLokiEndpoint removes the configured Loki push API endpoint. If no
// endpoint is configured, this is a no-op.
func (st *State) DeleteLokiEndpoint(ctx context.Context) error {
	db, err := st.DB(ctx)
	if err != nil {
		return errors.Errorf("getting database: %w", err)
	}

	stmt, err := st.Prepare(`DELETE FROM logging_loki_config`)
	if err != nil {
		return errors.Errorf("preparing delete statement: %w", err)
	}

	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if err := tx.Query(ctx, stmt).Run(); err != nil {
			return errors.Errorf("deleting loki endpoint: %w", err)
		}
		return nil
	}); err != nil {
		return errors.Errorf("deleting loki endpoint: %w", err)
	}
	return nil
}

type lokiConfig struct {
	UUID     string `db:"uuid"`
	Endpoint string `db:"endpoint"`
}
