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
	// Txn executes the input function against the tracked database,
	// within a transaction that depends on the input context.
	// Retry semantics are applied automatically based on transient failures.
	// This is the function that almost all downstream database consumers
	// should use.
	Txn(context.Context, func(context.Context, *sql.Tx) error) error

	// TxnNoRetry executes the input function against the tracked database,
	// within a transaction that depends on the input context.
	// No retries are attempted.
	TxnNoRetry(context.Context, func(context.Context, *sql.Tx) error) error

	// Err returns an error if the underlying tracked DB is in an error
	// condition. Depending on the error, determines of the tracked DB can be
	// requested again, or should be given up and thrown away.
	Err() error
}
