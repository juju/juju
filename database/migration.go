// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package database

import (
	"database/sql"

	"github.com/juju/errors"
)

// Migration is used to apply a series of deltas to a database.
type Migration struct {
	db     *sql.DB
	logger Logger
	deltas [][]string
}

// NewMigration returns a reference to a new migration that
// is used to apply the input deltas to the input database.
// The deltas are applied in the order supplied.
func NewMigration(db *sql.DB, logger Logger, deltas ...[]string) *Migration {
	return &Migration{
		db:     db,
		logger: logger,
		deltas: deltas,
	}
}

// Apply executes all deltas against the database inside a transaction.
func (m *Migration) Apply() error {
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
