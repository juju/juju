// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package equinix

import (
	"strings"

	"github.com/juju/errors"
)

// IsAuthorisationFailure determines if the given error has an authorisation failure.
func IsAuthorisationFailure(err error) bool {
	if err == nil {
		return false
	} else if strings.Contains(errors.Cause(err).Error(), "not authorized") {
		return true
	}
	return false
}
