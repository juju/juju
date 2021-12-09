// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"context"
	"database/sql"

	"github.com/juju/clock"
	"github.com/juju/errors"
)

// dbMediator encapsulates DB related capabilities to the facades.
type dbMediator struct {
	db     *sql.DB
	logger Logger
	clock  clock.Clock
}

// Txn defines a method for running transactions, dealing with retries,
// commit and rollback semantics.
func (m *dbMediator) Txn(fn func(*sql.Tx) error) error {
	// TODO (stickupkid): Implement retries.
	tx, err := m.db.BeginTx(context.TODO(), nil)
	if err != nil {
		return errors.Trace(err)
	}

	if err := fn(tx); err != nil {
		rollbackErr := tx.Rollback()
		if rollbackErr != nil {
			m.logger.Errorf("unable to rollback transaction %v", rollbackErr)
		}
		return errors.Trace(err)
	}

	return errors.Trace(tx.Commit())
}
