// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package maas

import (
	"github.com/juju/gomaasapi/v2"

	"github.com/juju/juju/internal/provider/common"
)

// IsAuthorisationFailure determines if the given error has an authorisation failure.
func IsAuthorisationFailure(err error) bool {
	// This should cover most cases.
	if gomaasapi.IsPermissionError(err) {
		return true
	}

	// This should cover exceptional circumstances.
	if maasErr, ok := err.(gomaasapi.ServerError); ok {
		return common.AuthorisationFailureStatusCodes.Contains(maasErr.StatusCode)
	}
	return false
}
