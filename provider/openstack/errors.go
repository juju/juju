// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package openstack

import (
	"github.com/juju/errors"
	gooseerrors "gopkg.in/goose.v2/errors"
)

// IsAuthorisationFailure determines if the given error has an authorisation failure.
func IsAuthorisationFailure(err error) bool {
	// This should cover most cases.
	if gooseerrors.IsUnauthorised(errors.Cause(err)) {
		return true
	}
	return false
}
