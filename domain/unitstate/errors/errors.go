// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package errors

import (
	"github.com/juju/juju/internal/errors"
)

const (
	// UnitLifePredicateFailed indicates that a comparison of unit life did not
	// match the expected life.
	UnitLifePredicateFailed = errors.ConstError("unit life predicate failed")
)
