// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package database

import (
	"context"

	"github.com/canonical/go-dqlite/app"
	"github.com/juju/errors"

	"github.com/juju/juju/database/schema"
)

type bootstrapOptFactory interface {
	// EnsureDataDir ensures that a directory for Dqlite data exists at
	// a path determined by the agent config, then returns that path.
	EnsureDataDir() (string, error)

	// WithAddressOption returns a Dqlite application option
	// for specifying the local address:port to use.
	WithAddressOption() (app.Option, error)
}

// BootstrapDqlite opens a new database for the controller, and runs the
// DDL to create its schema.
//
// At this point we know there are no peers and that we are the only user
// of Dqlite, so we can eschew external address and clustering concerns.
// Those will be handled by the db-accessor worker.
func BootstrapDqlite(opt bootstrapOptFactory, logger Logger) error {
	dir, err := opt.EnsureDataDir()
	if err != nil {
		return errors.Trace(err)
	}

	// Although we are not broadcasting this address in the bootstrap phase,
	// the address/port set here can not be changed for this same node later.
	// We are not using the default port, so we need to set it now.
	withAddress, err := opt.WithAddressOption()
	if err != nil {
		return errors.Trace(err)
	}

	dqlite, err := app.New(dir, withAddress)
	if err != nil {
		return errors.Annotate(err, "creating Dqlite app")
	}
	defer func() {
		if err := dqlite.Close(); err != nil {
			logger.Errorf("closing dqlite: %v", err)
		}
	}()

	ctx := context.TODO()

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

	if err := NewMigration(db, logger, schema.ControllerDDL()).Apply(); err != nil {
		return errors.Annotate(err, "creating controller database schema")
	}

	return nil
}
