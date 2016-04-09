// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import "github.com/juju/errors"

var (
	ParseSettingsCompatible = parseSettingsCompatible
	NewStateStorage         = &newStateStorage
)

func IsMinJujuVersionError(err error) bool {
	_, ok := errors.Cause(err).(minJujuVersionErr)
	return ok
}
