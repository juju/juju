// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	"io"
)

// ResourceStorage instances save and retrieve data from an underlying storage implementation.
type ResourceStorage interface {
	// Get returns a reader for the resource located at path.
	Get(path string) (io.ReadCloser, error)

	// Put writes data from the specified reader to path and returns a checksum of the data written.
	Put(path string, r io.Reader, length int64) (checksum string, err error)

	// Remove deletes the data at the specified path.
	Remove(path string) error
}

// ResourceCatalog instances persist Resources.
// Resources with the same hash values are not duplicated; instead a reference count is incremented.
// Similarly, when a Resource is removed, the reference count is decremented. When the reference
// count reaches zero, the Resource is deleted.
type ResourceCatalog interface {
	// Get fetches a Resource with the given id.
	Get(id string) (*Resource, error)

	// Put ensures a Resource entry exists for the given ResourceHash, returning the id, path,
	// and a flag indicating if this is a new record.
	// If the Resource exists, its reference count is incremented, otherwise a new entry is created.
	Put(rh *ResourceHash) (id, path string, isNew bool, err error)

	// UploadComplete records that the underlying resource described by the Resource entry with id
	// is now fully uploaded and the resource is available for use.
	UploadComplete(id string) error

	// Remove decrements the reference count for a Resource with the given id, deleting it
	// if the reference count reaches zero.
	Remove(id string) error
}
