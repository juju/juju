// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package database

import (
	"context"
	"database/sql"

	"github.com/canonical/sqlair"
	"github.com/juju/errors"

	coredatabase "github.com/juju/juju/core/database"
	coreschema "github.com/juju/juju/core/database/schema"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/domain/model"
	"github.com/juju/juju/domain/schema"
	"github.com/juju/juju/internal/database/app"
	"github.com/juju/juju/internal/database/pragma"
)

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

// BootstrapConcern is a type for describing a set of bootstrap operations to
// perform on a dqlite application.
type BootstrapConcern = func(ctx context.Context, logger Logger, dqlite *app.App) error

// BootstrapInit is a type for describing a bootstrap operation that
// initialises a database.
type BootstrapInit = func(ctx context.Context, runner coredatabase.TxnRunner, dqlite *app.App) error

// BootstrapControllerConcern is a BootstrapConcern type that will run the
// provided BootstrapOpts on the controller database.
func BootstrapControllerConcern(ops ...BootstrapOpt) BootstrapConcern {
	return bootstrapDBConcern(coredatabase.ControllerNS, schema.ControllerDDL(), controllerBootstrapInit, ops...)
}

// BootstrapModelConcern is a BootstrapConcern type that will run the
// provided BootstrapOpts on the specified model database.
func BootstrapModelConcern(uuid model.UUID, ops ...BootstrapOpt) BootstrapConcern {
	return bootstrapDBConcern(uuid.String(), schema.ModelDDL(), EmptyInit, ops...)
}

// BootstrapControllerInitConcern is a BootstrapConcern type that will run the
// provided BootstrapOpts on the controller database, after first running the
// provided BootstrapInit.
func BootstrapControllerInitConcern(bootstrapInit BootstrapInit, ops ...BootstrapOpt) BootstrapConcern {
	return bootstrapDBConcern(coredatabase.ControllerNS, schema.ControllerDDL(), bootstrapInit, ops...)
}

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

// EmptyInit is a BootstrapInit type that does nothing.
func EmptyInit(context.Context, coredatabase.TxnRunner, *app.App) error {
	return nil
}

func bootstrapDBConcern(
	namespace string,
	namespaceSchema *coreschema.Schema,
	bootstrapInit BootstrapInit,
	ops ...BootstrapOpt,
) BootstrapConcern {
	return func(ctx context.Context, logger Logger, dqlite *app.App) error {
		db, err := dqlite.Open(ctx, namespace)
		if err != nil {
			return errors.Annotatef(err, "opening database for namespace %q", namespace)
		}

		if err := pragma.SetPragma(ctx, db, pragma.ForeignKeysPragma, true); err != nil {
			return errors.Annotatef(err, "setting foreign keys pragma for namespace %q", namespace)
		}

		defer func() {
			if err := db.Close(); err != nil {
				logger.Errorf("closing database with namespace %q: %v", namespace, err)
			}
		}()

		runner := &txnRunner{db: db}

		migration := NewDBMigration(runner, logger, namespaceSchema)
		if err := migration.Apply(ctx); err != nil {
			return errors.Annotatef(err, "creating database with namespace %q schema", namespace)
		}

		if err := bootstrapInit(ctx, runner, dqlite); err != nil {
			return errors.Annotatef(err, "running bootstrap init for database with namespace %q", namespace)
		}

		for i, op := range ops {
			if err := op(ctx, runner); err != nil {
				return errors.Annotatef(
					err,
					"running bootstrap operation at index %d for database with namespace %q",
					i,
					namespace,
				)
			}
		}
		return nil
	}
}

// BootstrapOpt is a function run when bootstrapping a database,
// used to insert initial data into the model.
type BootstrapOpt func(context.Context, coredatabase.TxnRunner) error

// BootstrapDqlite opens a new database for the controller, and runs the
// DDL to create its schema.
//
// It accepts an optional list of functions to perform operations on the
// controller database.
func BootstrapDqlite(
	ctx context.Context,
	mgr BootstrapNodeManager,
	logger Logger,
	concerns ...BootstrapConcern,
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
			logger.Errorf("closing Dqlite: %v", err)
		}
	}()

	if err := dqlite.Ready(ctx); err != nil {
		return errors.Annotatef(err, "waiting for Dqlite readiness")
	}

	for i, concern := range concerns {
		if err := concern(ctx, logger, dqlite); err != nil {
			return errors.Annotatef(err, "running bootstrap concern at index %d", i)
		}
	}

	return nil
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
INSERT INTO controller_node (controller_id, dqlite_node_id, bind_address)
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
