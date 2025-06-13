// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	"regexp"

	storageerrors "github.com/juju/juju/domain/storage/errors"
	"github.com/juju/juju/internal/errors"
)

// Name defines a type that represents the name of a storage resource.
type Name string

const (
	// storageNameSnippet is the regular expression that describes valid
	// storage names (without the storage instance sequence number).
	storageNameSnippet = "(?:[a-z][a-z0-9]*(?:-[a-z0-9]*[a-z][a-z0-9]*)*)"
)

var (
	validStorageName = regexp.MustCompile(storageNameSnippet)
)

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
// The returned error is an [storageerrors.InvalidStorageName] error.
func (n Name) Validate() error {
	if !validStorageName.MatchString(n.String()) {
		return errors.Errorf("validating storage name %q", n).Add(
			storageerrors.InvalidStorageName,
		)
	}
	return nil
}
