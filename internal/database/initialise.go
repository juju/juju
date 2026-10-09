// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package database

import (
	"context"
	"database/sql"

	coredatabase "github.com/juju/juju/core/database"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/internal/database/app"
	"github.com/juju/juju/internal/database/pragma"
	"github.com/juju/juju/internal/errors"
)

// initialisationApp describes the application owned by an initialisation
// session.
type initialisationApp interface {
	// Ready waits for the node's startup tasks to complete or the context to be
	// cancelled.
	Ready(context.Context) error

	// Open opens the named database. The caller owns the returned handle and
	// must close it before closing the application.
	Open(context.Context, string) (*sql.DB, error)

	// ID returns the physical Dqlite node ID of the application.
	ID() uint64

	// Close stops the application and releases its resources.
	Close() error
}

// DqliteSession provides database access during exclusive initialisation of a
// Dqlite node. WithDqlite owns the session and all databases opened through it.
type DqliteSession struct {
	app         initialisationApp
	bindAddress string
	logger      logger.Logger
	databases   []*sql.DB
}

// WithDqlite creates a Dqlite application and waits for readiness before calling
// initialise. It closes the session's databases and application before returning,
// including on failure. Close errors are logged without replacing the result of
// initialisation.
//
// The caller must have exclusive ownership of the node's data directory. The
// session and its transaction runners must not be used after initialise returns.
// No schemas or controller data are installed unless initialise requests them.
//
// The caller selects the initialisation workflow: BootstrapDqlite supplies fresh
// controller seeding; a restore workflow can supply archive import instead.
// Keep that choice outside this session's database lifecycle management.
func WithDqlite(
	ctx context.Context,
	mgr BootstrapNodeManager,
	addresses network.ProviderAddresses,
	logger logger.Logger,
	initialise func(context.Context, *DqliteSession) error,
) error {
	dir, err := mgr.EnsureDataDir()
	if err != nil {
		return errors.Capture(err)
	}

	address, ok := addresses.OneMatchingScope(network.ScopeMatchCloudLocal)
	if !ok {
		return errors.New("Dqlite bootstrap address not found")
	}
	bindAddress := address.Value
	tlsOpt, err := mgr.WithTLSOption()
	if err != nil {
		return errors.Errorf("generating TLS option: %w", err)
	}
	options := []app.Option{
		mgr.WithLogFuncOption(),
		mgr.WithAddressOption(bindAddress),
		tlsOpt,
	}

	dqlite, err := app.New(dir, options...)
	if err != nil {
		return errors.Errorf("creating Dqlite app: %w", err)
	}
	return withDqlite(ctx, dqlite, bindAddress, logger, initialise)
}

func withDqlite(
	ctx context.Context,
	dqlite initialisationApp,
	bindAddress string,
	logger logger.Logger,
	initialise func(context.Context, *DqliteSession) error,
) error {
	session := &DqliteSession{
		app:         dqlite,
		bindAddress: bindAddress,
		logger:      logger,
	}
	defer session.close(ctx)

	if err := dqlite.Ready(ctx); err != nil {
		return errors.Errorf("waiting for Dqlite readiness: %w", err)
	}
	return initialise(ctx, session)
}

// NodeID returns the physical Dqlite node ID of the application.
func (s *DqliteSession) NodeID() uint64 {
	return s.app.ID()
}

// BindAddress returns the provider address selected for this node.
func (s *DqliteSession) BindAddress() string {
	return s.bindAddress
}

// OpenDatabase opens a namespace with foreign keys enabled and applies its
// schema. It does not insert bootstrap data. The session owns the database and
// closes it when initialisation finishes, including if schema application fails.
func (s *DqliteSession) OpenDatabase(ctx context.Context, namespace string, schema Schema) (coredatabase.TxnRunner, error) {
	db, err := s.app.Open(ctx, namespace)
	if err != nil {
		return nil, errors.Errorf("opening database for namespace %q: %w", namespace, err)
	}
	s.databases = append(s.databases, db)

	if err := pragma.SetPragma(ctx, db, pragma.ForeignKeysPragma, true); err != nil {
		return nil, errors.Errorf("setting foreign keys pragma for namespace %q: %w", namespace, err)
	}

	runner := &txnRunner{db: db}
	migration := NewDBMigration(runner, s.logger, schema)
	if err := migration.Apply(ctx); err != nil {
		return nil, errors.Errorf("creating database with namespace %q schema: %w", namespace, err)
	}
	return runner, nil
}

func (s *DqliteSession) close(ctx context.Context) {
	for _, db := range s.databases {
		if err := db.Close(); err != nil {
			s.logger.Errorf(ctx, "closing initialisation database: %v", err)
		}
	}
	if err := s.app.Close(); err != nil {
		s.logger.Errorf(ctx, "closing Dqlite: %v", err)
	}
}
