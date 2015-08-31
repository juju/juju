// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import "strings"

// This file contains utility functions related to documents and
// collections that contain data for multiple environments.

// ensureEnvUUID returns an environment UUID prefixed document ID. The
// prefix is only added if it isn't already there.
func ensureEnvUUID(envUUID, id string) string {
	prefix := envUUID + ":"
	if strings.HasPrefix(id, prefix) {
		return id
	}
	return prefix + id
}

// ensureEnvUUIDIfString will call ensureEnvUUID, but only if the id
// is a string. The id will be left untouched otherwise.
func ensureEnvUUIDIfString(envUUID string, id interface{}) interface{} {
	if id, ok := id.(string); ok {
		return ensureEnvUUID(envUUID, id)
	}
	return id
}

// splitDocID returns the 2 parts of environment UUID prefixed
// document ID. If the id is not in the expected format the final
// return value will be false.
func splitDocID(id string) (string, string, bool) {
	parts := strings.SplitN(id, ":", 2)
	if len(parts) != 2 {
		return "", "", false
	}
	return parts[0], parts[1], true
}
