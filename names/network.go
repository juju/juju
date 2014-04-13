// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package names

import (
	"fmt"
	"regexp"
)

var validNetwork = regexp.MustCompile("^([a-z0-9]+(-[a-z0-9]+)*)$")

// IsNetwork reports whether name is a valid network name.
func IsNetwork(name string) bool {
	return validNetwork.MatchString(name)
}

// NetworkTag returns the tag of a network with the given name.
func NetworkTag(name string) string {
	if !IsNetwork(name) {
		panic(fmt.Sprintf("%q is not a valid network name", name))
	}
	return makeTag(NetworkTagKind, name)
}
