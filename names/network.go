// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package names

// NetworkTag returns the tag of a network with the given id.
func NetworkTag(id string) string {
	return makeTag(NetworkTagKind, id)
}

// IsNetwork returns whether id is a valid network id.
func IsNetwork(id string) bool {
	// TODO(dimitern) Until we have a clear networking specification,
	// we cannot impose further restrictions on the id, because it
	// comes from the provider.
	return id != ""
}
