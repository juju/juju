// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package database

import (
	"context"
	"database/sql"
)

// TrackedDB defines an interface for keeping track of sql.DB. This is useful
// knowing if the underlying DB can be reused after an error has occurred.
type TrackedDB interface {
	// DB closes over a raw *sql.DB. Closing over the DB allows the late
	// realization of the database. Allowing retries of DB acquisition if there
	// is a failure that is non-retryable.
	DB(func(*sql.DB) error) error
	// Txn closes over a raw *sql.Tx. This allows retry semantics in only one
	// location. For instances where the underlying sql database is busy or if
	// it's a common retryable error that can be handled cleanly in one place.
	Txn(context.Context, func(context.Context, *sql.Tx) error) error

	// Err returns an error if the underlying tracked DB is in an error
	// condition. Depending on the error, determines of the tracked DB can be
	// requested again, or should be given up and thrown away.
	Err() error
}
