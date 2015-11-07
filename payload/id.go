// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package payload

import (
	"strings"
)

// TODO(ericsnow) Add ValidateID()?

// TODO(ericsnow) Add a "composite" ID type?

// BuildID composes an ID from a class and an ID.
func BuildID(class, id string) string {
	if id == "" {
		// TODO(natefinch) remove this special case when we can be sure the ID
		// is never empty (and fix the tests).
		return class
	}
	return class + "/" + id
}

// ParseID extracts the payload name and details ID from the provided string.
// The format is expected to be name/pluginID. If no separator is found, the
// whole string is assumed to be the name.
func ParseID(id string) (name, pluginID string) {
	parts := strings.SplitN(id, "/", 2)
	if len(parts) == 2 {
		return parts[0], parts[1]
	}
	return id, ""
}
