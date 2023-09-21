// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package database

import (
	"context"
	"database/sql"

	"github.com/canonical/sqlair"
	"github.com/juju/errors"

	coredatabase "github.com/juju/juju/core/database"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/database/app"
	"github.com/juju/juju/database/pragma"
	"github.com/juju/juju/domain/schema"
)

type bootstrapNodeManager interface {
	// EnsureDataDir ensures that a directory for Dqlite data exists at
	// a path determined by the agent config, then returns that path.
	EnsureDataDir() (string, error)

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
type BootstrapOpt func(context.Context, coredatabase.TxnRunner) error

// CloseableTxnRunner is a coredatabase.TxnRunner that can be closed.
type CloseableTxnRunner interface {
	coredatabase.TxnRunner
	Close()
}

// BootstrapDqlite opens a new database for the controller, and runs the
// DDL to create its schema.
//
// It accepts an optional list of functions to perform operations on the
// controller database.
//
// If preferLoopback is true, we bind Dqlite to 127.0.0.1 and eschew TLS
// termination. This is useful primarily in unit testing.
// If it is false, we attempt to identify a unique local-cloud address.
// If we find one, we use it as the bind address. Otherwise, we fall back
// to the loopback binding.
func BootstrapDqlite(
	ctx context.Context,
	mgr bootstrapNodeManager,
	logger Logger,
	preferLoopback bool,
	ops ...BootstrapOpt,
) (CloseableTxnRunner, error) {
	dir, err := mgr.EnsureDataDir()
	if err != nil {
		return nil, errors.Trace(err)
	}

	options := []app.Option{mgr.WithLogFuncOption()}
	if preferLoopback {
		options = append(options, mgr.WithLoopbackAddressOption())
	} else {
		addrOpt, err := mgr.WithPreferredCloudLocalAddressOption(network.DefaultConfigSource())
		if err != nil {
			return nil, errors.Annotate(err, "generating bind address option")
		}

		tlsOpt, err := mgr.WithTLSOption()
		if err != nil {
			return nil, errors.Annotate(err, "generating TLS option")
		}

		options = append(options, addrOpt, tlsOpt)
	}

	dqlite, err := app.New(dir, options...)
	if err != nil {
		return nil, errors.Annotate(err, "creating Dqlite app")
	}

	if err := dqlite.Ready(ctx); err != nil {
		return nil, errors.Annotatef(err, "waiting for Dqlite readiness")
	}

	db, err := dqlite.Open(ctx, coredatabase.ControllerNS)
	if err != nil {
		return nil, errors.Annotatef(err, "opening controller database")
	}

	if err := pragma.SetPragma(ctx, db, pragma.ForeignKeysPragma, true); err != nil {
		return nil, errors.Annotate(err, "setting foreign keys pragma")
	}

	runner := &txnRunner{
		db:     db,
		app:    dqlite,
		logger: logger,
	}

	if err := NewDBMigration(runner, logger, schema.ControllerDDL(dqlite.ID())).Apply(ctx); err != nil {
		return nil, errors.Annotate(err, "creating controller database schema")
	}

	for i, op := range ops {
		if err := op(ctx, runner); err != nil {
			return nil, errors.Annotatef(err, "running bootstrap operation at index %d", i)
		}
	}

	return runner, nil
}

// txnRunner is the simplest implementation of TxnRunner, wrapping a
// sql.DB reference. It is recruited to run the bootstrap DB migration,
// where we do not yet have access to a transaction runner sourced from
// dbaccessor worker.
type txnRunner struct {
	db     *sql.DB
	app    *app.App
	logger Logger
}

func (r *txnRunner) Txn(ctx context.Context, f func(context.Context, *sqlair.TX) error) error {
	return errors.Trace(Retry(ctx, func() error {
		return Txn(ctx, sqlair.NewDB(r.db), f)
	}))
}

func (r *txnRunner) StdTxn(ctx context.Context, f func(context.Context, *sql.Tx) error) error {
	return errors.Trace(Retry(ctx, func() error {
		return StdTxn(ctx, r.db, f)
	}))
}

func (r *txnRunner) Close() {
	if err := r.app.Close(); err != nil {
		r.logger.Errorf("failed to close Dqlite app: %v", err)
	}
	if err := r.db.Close(); err != nil {
		r.logger.Errorf("failed to close controller database: %v", err)
	}
}
