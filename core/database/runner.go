// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package database

import (
	"context"
	"database/sql"

	"github.com/canonical/sqlair"
)

// TxnRunner defines an interface for running transactions against a database.
type TxnRunner interface {
	// Txn manages the application of a SQLair transaction within which the
	// input function is executed. See https://github.com/canonical/sqlair.
	// The input context can be used by the caller to cancel this process.
	Txn(context.Context, func(context.Context, *sqlair.TX) error) error

	// StdTxn manages the application of a standard library transaction within
	// which the input function is executed.
	// The input context can be used by the caller to cancel this process.
	StdTxn(context.Context, func(context.Context, *sql.Tx) error) error
}
