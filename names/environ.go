// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package names

// EnvironTag returns the tag of an environment with the given name.
func EnvironTag(name string) string {
	return makeTag(EnvironTagKind, name)
}
