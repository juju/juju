// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package database

import (
	"context"
	"database/sql"

	"github.com/canonical/sqlair"
	"github.com/juju/errors"

	coredatabase "github.com/juju/juju/core/database"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/domain/schema"
	"github.com/juju/juju/internal/database/app"
	"github.com/juju/juju/internal/database/pragma"
)

// BootstrapNodeManager is an interface for managing the bootstrap of a Dqlite
// node.
type BootstrapNodeManager interface {
	// EnsureDataDir ensures that a directory for Dqlite data exists at
	// a path determined by the agent config, then returns that path.
	EnsureDataDir() (string, error)

	// IsLoopbackPreferred returns true if the Dqlite application should
	// be bound to the loopback address.
	IsLoopbackPreferred() bool

	// WithLoopbackAddressOption returns a Dqlite application
	// Option that will bind Dqlite to the loopback IP.
	WithLoopbackAddressOption() app.Option

	// WithPreferredCloudLocalAddressOption uses the input network config
	// source to return a local-cloud address to which to bind Dqlite,
	// provided that a unique one can be determined.
	WithPreferredCloudLocalAddressOption(network.ConfigSource) (app.Option, error)

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
	uuid model.UUID,
	logger logger.Logger,
	opts ...BootstrapOpt,
) error {
	dir, err := mgr.EnsureDataDir()
	if err != nil {
		return errors.Trace(err)
	}

	options := []app.Option{mgr.WithLogFuncOption()}
	if mgr.IsLoopbackPreferred() {
		options = append(options, mgr.WithLoopbackAddressOption())
	} else {
		addrOpt, err := mgr.WithPreferredCloudLocalAddressOption(network.DefaultConfigSource())
		if err != nil {
			return errors.Annotate(err, "generating bind address option")
		}

		tlsOpt, err := mgr.WithTLSOption()
		if err != nil {
			return errors.Annotate(err, "generating TLS option")
		}

		options = append(options, addrOpt, tlsOpt)
	}

	dqlite, err := app.New(dir, options...)
	if err != nil {
		return errors.Annotate(err, "creating Dqlite app")
	}
	defer func() {
		if err := dqlite.Close(); err != nil {
			logger.Errorf(ctx, "closing Dqlite: %v", err)
		}
	}()

	if err := dqlite.Ready(ctx); err != nil {
		return errors.Annotatef(err, "waiting for Dqlite readiness")
	}

	controller, err := runMigration(ctx, dqlite, coredatabase.ControllerNS, schema.ControllerDDL(), controllerBootstrapInit, logger)
	if err != nil {
		return errors.Annotate(err, "running controller migration")
	}

	model, err := runMigration(ctx, dqlite, uuid.String(), schema.ModelDDL(), emptyInit, logger)
	if err != nil {
		return errors.Annotate(err, "running model migration")
	}

	for i, op := range opts {
		if err := op(ctx, controller, model); err != nil {
			return errors.Annotatef(err, "running bootstrap operation at index %d", i)
		}
	}

	return nil
}

func runMigration(ctx context.Context, dqlite *app.App, namespace string, schema Schema, init bootstrapInit, logger logger.Logger) (coredatabase.TxnRunner, error) {
	db, err := dqlite.Open(ctx, namespace)
	if err != nil {
		return nil, errors.Annotatef(err, "opening database for namespace %q", namespace)
	}

	if err := pragma.SetPragma(ctx, db, pragma.ForeignKeysPragma, true); err != nil {
		return nil, errors.Annotatef(err, "setting foreign keys pragma for namespace %q", namespace)
	}

	runner := &txnRunner{db: db}

	migration := NewDBMigration(runner, logger, schema)
	if err := migration.Apply(ctx); err != nil {
		return nil, errors.Annotatef(err, "creating database with namespace %q schema", namespace)
	}

	if err := init(ctx, runner, dqlite); err != nil {
		return nil, errors.Annotatef(err, "running init for database with namespace %q", namespace)
	}

	return runner, nil
}

// InsertControllerNodeID inserts the node ID of the controller node
// into the controller_node table.
func InsertControllerNodeID(ctx context.Context, runner coredatabase.TxnRunner, nodeID uint64) error {
	q := `
-- TODO (manadart 2023-06-06): At the time of writing, 
-- we have not yet modelled machines. 
-- Accordingly, the controller ID remains the ID of the machine, 
-- but it should probably become a UUID once machines have one.
-- While HA is not supported in K8s, this doesn't matter.
INSERT INTO controller_node (controller_id, dqlite_node_id, dqlite_bind_address)
VALUES ('0', ?, '127.0.0.1');`
	return runner.StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		result, err := tx.ExecContext(ctx, q, nodeID)
		if err != nil {
			return errors.Trace(err)
		}
		affected, err := result.RowsAffected()
		if err != nil {
			return errors.Trace(err)
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
	return errors.Trace(Txn(ctx, sqlair.NewDB(r.db), f))
}

func (r *txnRunner) StdTxn(ctx context.Context, f func(context.Context, *sql.Tx) error) error {
	return errors.Trace(StdTxn(ctx, r.db, f))
}

func (r *txnRunner) Dying() <-chan struct{} {
	return make(<-chan struct{})
}

// bootstrapInit is a type for describing a bootstrap operation that
// initialises a database.
type bootstrapInit = func(ctx context.Context, runner coredatabase.TxnRunner, dqlite *app.App) error

// controllerBootstrapInit is used to initialise the controller database with
// a controller node ID. The controller node ID is required to be present in
// the controller_node table as this is used for referential integrity.
func controllerBootstrapInit(ctx context.Context, runner coredatabase.TxnRunner, dqlite *app.App) error {
	if err := InsertControllerNodeID(ctx, runner, dqlite.ID()); err != nil {
		// If the controller node ID already exists, we assume that
		// the database has already been bootstrapped. Mask the unique
		// constraint error with a more user-friendly error.
		if IsErrConstraintUnique(err) {
			return errors.AlreadyExistsf("controller node ID")
		}
		return errors.Annotatef(err, "inserting controller node ID")
	}
	return nil
}

// emptyInit is a BootstrapInit type that does nothing.
func emptyInit(context.Context, coredatabase.TxnRunner, *app.App) error {
	return nil
}
