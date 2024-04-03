// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package domain

import "github.com/canonical/sqlair"

// Preparer is an interface that prepares SQL statements for sqlair.
type Preparer interface {
	Prepare(query string, typeSamples ...any) (*sqlair.Statement, error)
}
