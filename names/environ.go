// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package names

import (
	"strings"
)

// EnvironTag returns the tag of an environment with the given name.
func EnvironTag(name string) string {
	return makeTag(EnvironTagKind, name)
}

// IsEnvironment returns whether id is a valid environment id.
// TODO(rog) stricter constraints
func IsEnvironment(name string) bool {
	return !strings.Contains(name, "/")
}
