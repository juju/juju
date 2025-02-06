// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	"regexp"

	"github.com/juju/juju/internal/errors"
)

const (
	// StorageNameSnippet is the regular expression that describes valid
	// storage names (without the storage instance sequence number).
	StorageNameSnippet = "(?:[a-z][a-z0-9]*(?:-[a-z0-9]*[a-z][a-z0-9]*)*)"
	// NumberSnippet is a non-compiled regexp that can be composed with other
	// snippets for validating small number sequences.
	NumberSnippet = "(?:0|[1-9][0-9]*)"
)

const (
	// InvalidStorageName represents an invalid storage name.
	InvalidStorageName = errors.ConstError("invalid storage name")
	// InvalidStorageID represents an invalid storage id.
	InvalidStorageID = errors.ConstError("invalid storage id")
)

var (
	validStorageID   = regexp.MustCompile("^(" + StorageNameSnippet + ")/" + NumberSnippet + "$")
	validStorageName = regexp.MustCompile(StorageNameSnippet)
)

// Name represents a storage name.
type Name string

// NewName returns a new Name. If the name is invalid, an InvalidStorageName error
// will be returned.
func NewName(name string) (Name, error) {
	n := Name(name)
	return n, n.Validate()
}

// String returns the Name as a string.
func (n Name) String() string {
	return string(n)
}

// Validate returns an error if the Name is invalid.
// The returned error is an InvalidStorageName error.
func (n Name) Validate() error {
	if !validStorageName.MatchString(n.String()) {
		return errors.Errorf("%w: %q", InvalidStorageName, n)
	}
	return nil
}

// ID represents a storage ID which is a name with a sequence number.
type ID string

// NewID returns a new ID. If the id is invalid, an InvalidStorageID error
// will be returned.
func NewID(id string) (ID, error) {
	result := ID(id)
	return result, result.Validate()
}

// String returns the ID as a string.
func (id ID) String() string {
	return string(id)
}

// Validate returns an error if the ID is invalid.
// The returned error is an InvalidStorageID error.
func (id ID) Validate() error {
	if !validStorageID.MatchString(id.String()) {
		return errors.Errorf("%w: %q", InvalidStorageID, id)
	}
	return nil
}
