// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package database

import (
	"context"
	"database/sql"

	"github.com/juju/errors"

	"github.com/juju/juju/database/app"
	"github.com/juju/juju/database/pragma"
	"github.com/juju/juju/database/schema"
)

type bootstrapNodeManager interface {
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
	mgr bootstrapNodeManager,
	logger Logger,
	ops ...func(db *sql.DB) error,
) error {
	dir, err := mgr.EnsureDataDir()
	if err != nil {
		return errors.Trace(err)
	}

	dqlite, err := app.New(dir,
		mgr.WithLoopbackAddressOption(),
		mgr.WithLogFuncOption(),
	)
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

	db, err := dqlite.Open(ctx, "controller")
	if err != nil {
		return errors.Annotatef(err, "opening controller database")
	}
	defer func() {
		if err := db.Close(); err != nil {
			logger.Errorf("closing controller database: %v", err)
		}
	}()

	if err := pragma.SetPragma(ctx, db, pragma.ForeignKeysPragma, true); err != nil {
		return errors.Annotate(err, "setting foreign keys pragma")
	}

	if err := NewDBMigration(db, logger, schema.ControllerDDL()).Apply(); err != nil {
		return errors.Annotate(err, "creating controller database schema")
	}

	for i, op := range ops {
		if err := op(db); err != nil {
			return errors.Annotatef(err, "running bootstrap operation at index %d", i)
		}
	}

	return nil
}
