// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package errors

import (
	"github.com/juju/juju/internal/errors"
)

const (
	// UnitLifePreconditionFailed indicates that a comparison of unit life did
	// not match the expected life.
	UnitLifePreconditionFailed = errors.ConstError("unit life predicate failed")

	// SecretReservationNotOwned indicates that the secret ID being created is
	// reserved for a different unit, so the creation is refused to prevent
	// ownership theft.
	SecretReservationNotOwned = errors.ConstError("secret id reserved by another unit")
)
