// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package names

import (
	"strings"
)

// IsUser returns whether id is a valid user id.
// TODO(rog) stricter constraints
func IsUser(name string) bool {
	return !strings.Contains(name, "/") && name != ""
}

// UserTag returns the tag for the user with the given name.
func UserTag(userName string) string {
	return makeTag(UserTagKind, userName)
}
