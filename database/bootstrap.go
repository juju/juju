// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package database

import (
	"context"
	"database/sql"

	"github.com/juju/errors"

	"github.com/juju/juju/core/network"
	"github.com/juju/juju/database/app"
	"github.com/juju/juju/database/pragma"
	"github.com/juju/juju/database/schema"
)

type bootstrapNodeManager interface {
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
	ops ...func(db *sql.DB) error,
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
