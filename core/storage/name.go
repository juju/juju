// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	"fmt"
	"regexp"

	"github.com/juju/juju/internal/errors"
)

const (
	// storageNameSnippet is the regular expression that describes valid
	// storage names (without the storage instance sequence number).
	storageNameSnippet = "(?:[a-z][a-z0-9]*(?:-[a-z0-9]*[a-z][a-z0-9]*)*)"
	// numberSnippet is a non-compiled regexp that can be composed with other
	// snippets for validating small number sequences.
	numberSnippet = "(?:0|[1-9][0-9]*)"
)

const (
	// InvalidStorageName represents an invalid storage name.
	InvalidStorageName = errors.ConstError("invalid storage name")
	// InvalidStorageID represents an invalid storage id.
	InvalidStorageID = errors.ConstError("invalid storage id")
)

var (
	validStorageID   = regexp.MustCompile("^(" + storageNameSnippet + ")/" + numberSnippet + "$")
	validStorageName = regexp.MustCompile(storageNameSnippet)
)

// Name represents a storage name.
type Name string

// ParseName returns a new Name. If the name is invalid, an [InvalidStorageName] error
// will be returned.
func ParseName(name string) (Name, error) {
	n := Name(name)
	return n, n.Validate()
}

// String returns the Name as a string.
func (n Name) String() string {
	return string(n)
}

// Validate returns an error if the Name is invalid.
// The returned error is an [InvalidStorageName] error.
func (n Name) Validate() error {
	if !validStorageName.MatchString(n.String()) {
		return errors.Errorf("validating storage name %q", n).Add(InvalidStorageName)
	}
	return nil
}

// ID represents a storage ID which is a name with a sequence number.
type ID string

// ParseID returns a new ID. If the id is invalid, an [InvalidStorageID] error
// will be returned.
func ParseID(id string) (ID, error) {
	result := ID(id)
	return result, result.Validate()
}

// String returns the ID as a string.
func (id ID) String() string {
	return string(id)
}

// Validate returns an error if the ID is invalid.
// The returned error is an [InvalidStorageID] error.
func (id ID) Validate() error {
	if !validStorageID.MatchString(id.String()) {
		return errors.Errorf("validating storage ID %q", id).Add(InvalidStorageID)
	}
	return nil
}

// MakeID creates a storage ID from a name and sequence number.
func MakeID(name Name, num uint64) ID {
	return ID(fmt.Sprintf("%s/%d", name, num))
}
