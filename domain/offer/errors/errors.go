// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package errors

import "github.com/juju/juju/internal/errors"

const (
	// OfferNotFound describes an error that occurs when the offer
	// being operated on does not exist.
	OfferNotFound = errors.ConstError("offer not found")
)
