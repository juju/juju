// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package base

import (
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/internal/errors"
)

// ValidateBase attempts to validate a base if one is found, otherwise it
// uses the fallback base and validates that one.
// Returns the base it validated against or an error if one is found.
// Note: the selected base will be returned if there is an error to help use
// that for a fallback during error scenarios.
func ValidateBase(supportedBases []Base, base, fallbackPreferredBase Base) (Base, error) {
	// Validate the requested base.
	// Attempt to do the validation in one place, so it makes it easier to
	// reason about where the validation happens. This only happens for IAAS
	// models, as CAAS can't take base as an argument.
	var requestedBase Base
	if !base.Empty() {
		requestedBase = base
	} else {
		// If no bootstrap base is supplied, go and get that information from
		// the fallback. We should still validate the fallback value to ensure
		// that we also work with that base.
		requestedBase = fallbackPreferredBase
	}

	var found bool
	for _, supportedBase := range supportedBases {
		if supportedBase.IsCompatible(requestedBase) {
			found = true
			break
		}
	}
	if !found {
		return requestedBase, errors.Errorf("%s %w", requestedBase.String(), coreerrors.NotSupported)
	}
	return requestedBase, nil
}
