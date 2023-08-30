// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package database

import (
	"context"
	"database/sql"

	"github.com/canonical/sqlair"
	"github.com/juju/errors"

	coredatabase "github.com/juju/juju/core/database"
	"github.com/juju/juju/database/app"
	"github.com/juju/juju/database/pragma"
	"github.com/juju/juju/domain/schema"
)

type bootstrapOptFactory interface {
	// EnsureDataDir ensures that a directory for Dqlite data exists at
	// a path determined by the agent config, then returns that path.
	EnsureDataDir() (string, error)

	// WithLoopbackAddressOption returns a Dqlite application
	// Option that will bind Dqlite to the loopback IP.
	WithLoopbackAddressOption() app.Option

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

// BootstrapOpt is a function run when bootstrapping a database,
// used to insert initial data into the model.
type BootstrapOpt func(context.Context, coredatabase.TxnRunner) error

// BootstrapDqlite opens a new database for the controller, and runs the
// DDL to create its schema.
//
// It accepts an optional list of functions to perform operations on the
// controller database.
//
// At this point we know there are no peers and that we are the only user
// of Dqlite, so we can eschew external address and clustering concerns.
// Those will be handled by the db-accessor worker.
func BootstrapDqlite(
	ctx context.Context,
	opt bootstrapOptFactory,
	logger Logger,
	ops ...BootstrapOpt,
) error {
	dir, err := opt.EnsureDataDir()
	if err != nil {
		return errors.Trace(err)
	}

	dqlite, err := app.New(dir,
		opt.WithLoopbackAddressOption(),
		opt.WithLogFuncOption(),
	)
	if err != nil {
		return errors.Annotate(err, "creating Dqlite app")
	}
	defer func() {
		if err := dqlite.Close(); err != nil {
			logger.Errorf("closing dqlite: %v", err)
		}
	}()

	if err := dqlite.Ready(ctx); err != nil {
		return errors.Annotatef(err, "waiting for Dqlite readiness")
	}

	db, err := dqlite.Open(ctx, coredatabase.ControllerNS)
	if err != nil {
		return errors.Annotatef(err, "opening controller database")
	}

	if err := pragma.SetPragma(ctx, db, pragma.ForeignKeysPragma, true); err != nil {
		return errors.Annotate(err, "setting foreign keys pragma")
	}

	defer func() {
		if err := db.Close(); err != nil {
			logger.Errorf("closing controller database: %v", err)
		}
	}()

	runner := &txnRunner{db: db}

	if err := NewDBMigration(runner, logger, schema.ControllerDDL(dqlite.ID())).Apply(ctx); err != nil {
		return errors.Annotate(err, "creating controller database schema")
	}

	for i, op := range ops {
		if err := op(ctx, runner); err != nil {
			return errors.Annotatef(err, "running bootstrap operation at index %d", i)
		}
	}

	return nil
}
