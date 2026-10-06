// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package database

import (
	"context"
	"database/sql"

	"github.com/canonical/sqlair"

	coredatabase "github.com/juju/juju/core/database"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/domain/schema"
	"github.com/juju/juju/internal/database/app"
	"github.com/juju/juju/internal/errors"
)

// BootstrapNodeManager is an interface for managing the bootstrap of a Dqlite
// node.
type BootstrapNodeManager interface {
	// EnsureDataDir ensures that a directory for Dqlite data exists at
	// a path determined by the agent config, then returns that path.
	EnsureDataDir() (string, error)

	// WithAddressOption returns a Dqlite application Option for binding to the
	// supplied address.
	WithAddressOption(string) app.Option

	// WithTLSOption returns a Dqlite application Option for TLS encryption
	// of traffic between clients and clustered application nodes.
	WithTLSOption() (app.Option, error)

	// WithLogFuncOption returns a Dqlite application Option
	// that will proxy Dqlite log output via this factory's
	// logger where the level is recognised.
	WithLogFuncOption() app.Option

	// WithTracingOption returns a Dqlite application Option
	// that will enable tracing of Dqlite operations.
	WithTracingOption() app.Option
}

// BootstrapOpt is a function run when bootstrapping a database,
// used to insert initial data into the model.
type BootstrapOpt func(
	ctx context.Context,
	controller, model coredatabase.TxnRunner,
) error

// BootstrapDqlite opens a new database for the controller, and runs the
// DDL to create its schema.
//
// It accepts an optional list of functions to perform operations on the
// controller database.
func BootstrapDqlite(
	ctx context.Context,
	mgr BootstrapNodeManager,
	bootstrapAddresses network.ProviderAddresses,
	uuid model.UUID,
	logger logger.Logger,
	opts ...BootstrapOpt,
) error {
	return errors.Capture(WithDqlite(ctx, mgr, bootstrapAddresses, logger, func(ctx context.Context, session *DqliteSession) error {
		controller, err := session.OpenDatabase(ctx, coredatabase.ControllerNS, schema.ControllerDDL())
		if err != nil {
			return errors.Errorf("running controller migration: %w", err)
		}

		// The controller node must exist before the seed operations run,
		// as it is required for referential integrity.
		if err := InsertControllerNodeID(ctx, controller, session.NodeID(), session.BindAddress()); err != nil {
			return errors.Errorf("inserting controller node ID: %w", err)
		}

		model, err := session.OpenDatabase(ctx, uuid.String(), schema.ModelDDL())
		if err != nil {
			return errors.Errorf("running model migration: %w", err)
		}

		for i, op := range opts {
			if err := op(ctx, controller, model); err != nil {
				return errors.Errorf("running bootstrap operation at index %d: %w", i, err)
			}
		}

		return nil
	}))
}

// InsertControllerNodeID inserts the node ID of the controller node
// into the controller_node table.
func InsertControllerNodeID(
	ctx context.Context, runner coredatabase.TxnRunner, nodeID uint64, bindAddress string,
) error {
	q := `
-- TODO (manadart 2023-06-06): At the time of writing, 
-- we have not yet modelled machines. 
-- Accordingly, the controller ID remains the ID of the machine, 
-- but it should probably become a UUID once machines have one.
INSERT INTO controller_node (controller_id, dqlite_node_id, dqlite_bind_address)
VALUES ('0', ?, ?);`
	return runner.StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		result, err := tx.ExecContext(ctx, q, nodeID, bindAddress)
		if err != nil {
			return errors.Capture(err)
		}
		affected, err := result.RowsAffected()
		if err != nil {
			return errors.Capture(err)
		}
		if affected != 1 {
			return errors.Errorf("expected 1 row affected, got %d", affected)
		}
		return nil
	})
}

// txnRunner is the simplest implementation of TxnRunner, wrapping a
// sql.DB reference. It is recruited to run the bootstrap DB migration,
// where we do not yet have access to a transaction runner sourced from
// dbaccessor worker.
type txnRunner struct {
	db *sql.DB
}

func (r *txnRunner) Txn(ctx context.Context, f func(context.Context, *sqlair.TX) error) error {
	return errors.Capture(Txn(ctx, sqlair.NewDB(r.db), f))
}

func (r *txnRunner) StdTxn(ctx context.Context, f func(context.Context, *sql.Tx) error) error {
	return errors.Capture(StdTxn(ctx, r.db, f))
}

func (r *txnRunner) Dying() <-chan struct{} {
	return make(<-chan struct{})
}
