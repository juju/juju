// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package objectstore

import "github.com/juju/errors"

// BackendType is the type to identify the backend to use for the object store.
type BackendType string

const (
	// StateBackend is the backend type for the state object store.
	StateBackend BackendType = "state"
	// FileBackend is the backend type for the file object store.
	FileBackend BackendType = "file"
)

// ParseObjectStoreType parses the given string into a BackendType.
func ParseObjectStoreType(s string) (BackendType, error) {
	switch s {
	case string(StateBackend):
		return StateBackend, nil
	case string(FileBackend):
		return FileBackend, nil
	default:
		return "", errors.NotValidf("object store type %q", s)
	}
}
