// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package names

import (
	"launchpad.net/juju-core/utils"
)

// EnvironTag returns the tag of an environment with the given environment UUID.
func EnvironTag(uuid string) string {
	return makeTag(EnvironTagKind, uuid)
}

// IsEnvironment returns whether id is a valid environment UUID.
func IsEnvironment(id string) bool {
	return utils.IsValidUUIDString(id)
}
