// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package errors

import "github.com/juju/juju/internal/errors"

const (
	// MissingEndpoints describes an error that occurs when not all of the
	// endpoints for the offer are found.
	MissingEndpoints = errors.ConstError("missing endpoints")

	// OfferNotFound describes an error that occurs when the offer
	// being operated on does not exist.
	OfferNotFound = errors.ConstError("offer not found")

	// OfferAlreadyConsumed describes an error that occurs when trying to
	// create an offer that already exists for the same UUID.
	OfferAlreadyConsumed = errors.ConstError("offer already consumed")
)
