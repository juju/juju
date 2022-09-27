// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package database

import (
	"database/sql"

	"github.com/juju/errors"
)

// DBMigration is used to apply a series of deltas to a database.
type DBMigration struct {
	db     *sql.DB
	logger Logger
	deltas [][]string
}

// NewDBMigration returns a reference to a new migration that
// is used to apply the input deltas to the input database.
// The deltas are applied in the order supplied.
func NewDBMigration(db *sql.DB, logger Logger, deltas ...[]string) *DBMigration {
	return &DBMigration{
		db:     db,
		logger: logger,
		deltas: deltas,
	}
}

// Apply executes all deltas against the database inside a transaction.
func (m *DBMigration) Apply() error {
	tx, err := m.db.Begin()
	if err != nil {
		return errors.Annotatef(err, "beginning migration transaction")
	}

	for _, delta := range m.deltas {
		for _, stmt := range delta {
			if _, err := tx.Exec(stmt); err != nil {
				if rbErr := tx.Rollback(); rbErr != nil {
					m.logger.Errorf("rolling back transaction: %v", rbErr)
				}
				return errors.Trace(err)
			}
		}
	}

	if err := tx.Commit(); err != nil {
		if rbErr := tx.Rollback(); rbErr != nil {
			m.logger.Errorf("rolling back transaction: %v", rbErr)
		}
		return errors.Trace(err)
	}
	return nil
}
