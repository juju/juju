// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package rackspace

import (
	gooseerrors "github.com/go-goose/goose/v3/errors"
	"github.com/juju/errors"
)

// IsAuthorisationFailure determines if the given error has an authorisation failure.
func IsAuthorisationFailure(err error) bool {
	// This should cover most cases.
	if gooseerrors.IsUnauthorised(errors.Cause(err)) {
		return true
	}
	return false
}
