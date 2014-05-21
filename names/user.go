// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package names

import (
	"regexp"
)

var validName = regexp.MustCompile("^[a-zA-Z][a-zA-Z0-9.-]*[a-zA-Z0-9]$")

// IsUser returns whether id is a valid user id.
func IsUser(name string) bool {
	return validName.MatchString(name)
}

// UserTag returns the tag for the user with the given name.
func UserTag(userName string) string {
	return makeTag(UserTagKind, userName)
}
