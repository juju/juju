// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxd

import (
	"github.com/juju/errors"
)

// IsAuthorisationFailure determines if the given error has an authorisation failure.
func IsAuthorisationFailure(err error) bool {
	if err == nil {
		return false
	} else if errors.Cause(err).Error() == "not authorized" {
		return true
	}
	return false
}
