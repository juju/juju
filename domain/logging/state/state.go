// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"

	"github.com/canonical/sqlair"

	"github.com/juju/juju/core/database"
	"github.com/juju/juju/domain"
	"github.com/juju/juju/domain/logging"
	loggingerrors "github.com/juju/juju/domain/logging/errors"
	"github.com/juju/juju/internal/errors"
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

// SetLokiConfig sets the Loki push API endpoint and CA certificate. Any
// previously stored config is replaced.
func (st *State) SetLokiConfig(ctx context.Context, id string, config logging.LokiConfig) error {
	db, err := st.DB(ctx)
	if err != nil {
		return errors.Errorf("getting database: %w", err)
	}

	deleteStmt, err := st.Prepare(`DELETE FROM logging_loki_config`)
	if err != nil {
		return errors.Errorf("preparing delete statement: %w", err)
	}

	insertStmt, err := st.Prepare(`
	INSERT INTO logging_loki_config (uuid, endpoint, ca_cert)
	VALUES ($lokiConfig.uuid, $lokiConfig.endpoint, $lokiConfig.ca_cert)`, lokiConfig{})
	if err != nil {
		return errors.Errorf("preparing insert statement: %w", err)
	}

	dbConfig := lokiConfig{
		UUID:          id,
		Endpoint:      config.Endpoint,
		CACertificate: config.CACertificate,
	}

	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if err := tx.Query(ctx, deleteStmt).Run(); err != nil {
			return errors.Errorf("deleting existing loki endpoint: %w", err)
		}
		if err := tx.Query(ctx, insertStmt, dbConfig).Run(); err != nil {
			return errors.Errorf("inserting loki endpoint: %w", err)
		}
		return nil
	}); err != nil {
		return errors.Errorf("setting loki endpoint: %w", err)
	}
	return nil
}

// GetLokiConfig returns the configured Loki push API endpoint and CA
// certificate. If no endpoint is configured, an error satisfying
// [loggingerrors.LokiConfigNotFound] is returned.
func (st *State) GetLokiConfig(ctx context.Context) (logging.LokiConfig, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return logging.LokiConfig{}, errors.Errorf("getting database: %w", err)
	}

	stmt, err := st.Prepare(`
SELECT &lokiConfig.endpoint, &lokiConfig.ca_cert FROM logging_loki_config
`, lokiConfig{})
	if err != nil {
		return logging.LokiConfig{}, errors.Errorf("preparing select statement: %w", err)
	}

	var config lokiConfig
	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if err := tx.Query(ctx, stmt).Get(&config); errors.Is(err, sqlair.ErrNoRows) {
			return loggingerrors.LokiConfigNotFound
		} else if err != nil {
			return errors.Errorf("getting loki config: %w", err)
		}
		return nil
	}); err != nil {
		return logging.LokiConfig{}, errors.Errorf("getting loki config: %w", err)
	}
	return logging.LokiConfig{
		Endpoint:      config.Endpoint,
		CACertificate: config.CACertificate,
	}, nil
}

// DeleteLokiConfig removes the configured Loki push API config. If no config
// is configured, this is a no-op.
func (st *State) DeleteLokiConfig(ctx context.Context) error {
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
	UUID          string `db:"uuid"`
	Endpoint      string `db:"endpoint"`
	CACertificate string `db:"ca_cert"`
}

// NamespaceForWatchLokiConfig returns the namespace identifier used for
// watching Loki config changes.
func (*State) NamespaceForWatchLokiConfig() string {
	return "logging_loki_config"
}
