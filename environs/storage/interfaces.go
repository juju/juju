// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	"io"

	"launchpad.net/juju-core/utils"
)

// A StorageReader can retrieve and list files from a storage provider.
type StorageReader interface {
	// Get opens the given storage file and returns a ReadCloser
	// that can be used to read its contents.  It is the caller's
	// responsibility to close it after use.  If the name does not
	// exist, it should return a *NotFoundError.
	Get(name string) (io.ReadCloser, error)

	// List lists all names in the storage with the given prefix, in
	// alphabetical order.  The names in the storage are considered
	// to be in a flat namespace, so the prefix may include slashes
	// and the names returned are the full names for the matching
	// entries.
	List(prefix string) ([]string, error)

	// URL returns a URL that can be used to access the given storage file.
	URL(name string) (string, error)

	// DefaultConsistencyStrategy returns the appropriate polling for waiting
	// for this storage to become consistent.
	// If the storage implementation has immediate consistency, the
	// strategy won't need to wait at all.  But for eventually-consistent
	// storage backends a few seconds of polling may be needed.
	DefaultConsistencyStrategy() utils.AttemptStrategy

	// ShouldRetry returns true is the specified error is such that an
	// operation can be performed again with a chance of success. This is
	// typically the case where the storage implementation does not have
	// immediate consistency and needs to be given a chance to "catch up".
	ShouldRetry(error) bool
}

// A StorageWriter adds and removes files in a storage provider.
type StorageWriter interface {
	// Put reads from r and writes to the given storage file.
	// The length must give the total length of the file.
	Put(name string, r io.Reader, length int64) error

	// Remove removes the given file from the environment's
	// storage. It should not return an error if the file does
	// not exist.
	Remove(name string) error

	// RemoveAll deletes all files that have been stored here.
	// If the underlying storage implementation may be shared
	// with other actors, it must be sure not to delete their
	// file as well.
	// Nevertheless, use with care!  This method is only mean
	// for cleaning up an environment that's being destroyed.
	RemoveAll() error
}

// Storage represents storage that can be both
// read and written.
type Storage interface {
	StorageReader
	StorageWriter
}
