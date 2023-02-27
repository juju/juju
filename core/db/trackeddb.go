// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package db

import "database/sql"

// TrackedDB defines an interface for keeping track of sql.DB. This is useful
// knowing if the underlying DB can be reused after an error has occurred.
type TrackedDB interface {
	// DB returns the raw tracked DB.
	DB() *sql.DB

	// Err returns an error if the underlying tracked DB is in an error
	// condition. Depending on the error, determins of the tracked DB can be
	// requested again, or should be given up and thrown away.
	Err() error
}
