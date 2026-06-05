// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migration

import "github.com/juju/juju/internal/errors"

const (
	// ErrMigrating indicates that the model is currently being migrated.
	ErrMigrating = errors.ConstError("model is being migrated")

	// ErrMinionReportsInvalid indicates that migration minion report counts are
	// internally inconsistent.
	ErrMinionReportsInvalid = errors.ConstError("migration minion reports are invalid")
)
