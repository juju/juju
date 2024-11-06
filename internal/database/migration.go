// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package database

import (
	"context"

	"github.com/juju/errors"

	"github.com/juju/juju/core/database"
	"github.com/juju/juju/core/database/schema"
	"github.com/juju/juju/core/logger"
)

// Schema is used to apply a schema to a database.
type Schema interface {
	// Ensure applies the schema to the database, if it has not already been
	// applied. It returns the list of changes that were applied.
	Ensure(context.Context, database.TxnRunner) (schema.ChangeSet, error)
}

// DBMigration is used to apply a series of deltas to a database.
type DBMigration struct {
	db     database.TxnRunner
	logger logger.Logger
	schema Schema
}

// NewDBMigration returns a reference to a new migration that
// is used to apply the input deltas to the input database.
// The deltas are applied in the order supplied.
func NewDBMigration(db database.TxnRunner, logger logger.Logger, schema Schema) *DBMigration {
	return &DBMigration{
		db:     db,
		logger: logger,
		schema: schema,
	}
}

// Apply executes all deltas against the database inside a transaction.
func (m *DBMigration) Apply(ctx context.Context) error {
	changeSet, err := m.schema.Ensure(ctx, m.db)
	if err != nil {
		return errors.Trace(err)
	}
	m.logger.Debugf(ctx, "Applied %d schema changes", changeSet.Post)
	return nil
}
