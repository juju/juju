// Copyright 2014 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package blobstore

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

	// Find returns the resource id for the Resource with the given hash.
	Find(hash string) (id string, err error)

	// Put ensures a Resource entry exists for the given hash,
	// returning the id and path recorded by UploadComplete.
	// If UploadComplete has not been called, path will be empty.
	//
	// If the Resource entry exists, its reference count is incremented,
	// otherwise a new entry is created with a reference count of 1.
	Put(hash string, length int64) (id, path string, err error)

	// UploadComplete records that the underlying resource described by
	// the Resource entry with id is now fully uploaded to the specified
	// storage path, and the resource is available for use. If another
	// uploader already recorded a path, then UploadComplete will return
	// an error satisfiying juju/errors.IsAlreadyExists.
	UploadComplete(id, path string) error

	// Remove decrements the reference count for a Resource with the given id, deleting it
	// if the reference count reaches zero. The path of the Resource is returned.
	// If the Resource is deleted, wasDeleted is returned as true.
	Remove(id string) (wasDeleted bool, path string, err error)
}

// ManagedStorage instances persist data for an environment, for a user, or globally.
// (Only environment storage is currently implemented).
type ManagedStorage interface {
	// GetForEnvironment returns a reader for data at path, namespaced to the environment.
	// If the data is still being uploaded and is not fully written yet,
	// an ErrUploadPending error is returned. This means the path is valid but the caller
	// should try again to retrieve the data.
	GetForEnvironment(envUUID, path string) (r io.ReadCloser, length int64, err error)

	// PutForEnvironment stores data from reader at path, namespaced to the environment.
	//
	// PutForEnvironment is equivalent to PutForEnvironmentAndCheckHash with an empty
	// hash string.
	PutForEnvironment(envUUID, path string, r io.Reader, length int64) error

	// PutForEnvironmentAndCheckHash is the same as PutForEnvironment
	// except that it also checks that the content matches the provided
	// hash. The hash must be hex-encoded SHA-384.
	//
	// If checkHash is empty, then the hash check is elided.
	//
	// If length is < 0, then the reader will be consumed until EOF.
	PutForEnvironmentAndCheckHash(envUUID, path string, r io.Reader, length int64, checkHash string) error

	// RemoveForEnvironment deletes data at path, namespaced to the environment.
	RemoveForEnvironment(envUUID, path string) error

	// PutForEnvironmentRequest requests that data, which may already exist in storage,
	// be saved at path, namespaced to the environment. It allows callers who can
	// demonstrate proof of ownership of the data to store a reference to it without
	// having to upload it all. If no such data exists, a NotFound error is returned
	// and a call to EnvironmentPut is required. If matching data is found, the caller
	// is returned a response indicating the random byte range to for which they must
	// provide a checksum to complete the process.
	PutForEnvironmentRequest(envUUID, path string, hash string) (*RequestResponse, error)

	// ProofOfAccessResponse is called to respond to a Put..Request call in order to
	// prove ownership of data for which a storage reference is created.
	ProofOfAccessResponse(putResponse) error
}
