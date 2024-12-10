// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package store

import (
	"context"
	"io"

	"github.com/juju/utils/v4/hash"

	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/internal/errors"
)

// ResourceStore provides a list of methods necessary for interacting with
// a store for the resource.
type ResourceStore interface {
	// Get returns an io.ReadCloser for a resource in the resource store.
	Get(
		ctx context.Context,
		storageKey string,
	) (r io.ReadCloser, size int64, err error)

	// Put stores data from io.Reader in the resource store using the storage
	// key.
	Put(
		ctx context.Context,
		storageKey string,
		r io.Reader,
		size int64,
		fingerprint Fingerprint,
	) (ID, error)

	// Remove removes a resource from storage.
	Remove(
		ctx context.Context,
		storageKey string,
	) error
}

// Fingerprint represents the unique fingerprint value of a resource's data.
type Fingerprint struct {
	hash.Fingerprint
}

// NewFingerprint returns a resource store Fingerprint for the given
// hash Fingerprint.
func NewFingerprint(f hash.Fingerprint) Fingerprint {
	return Fingerprint{f}
}

// ID is the ID of the stored blob in the database, this can
// be used for adding referential integrity from the resource to the stored
// blob. This can be an object store metadata UUID or a container image metadata
// storage key. It is only one, never both.
type ID struct {
	objectStoreUUID               objectstore.UUID
	containerImageMetadataStoreID string
}

// NewFileResourceID creates a new storage ID for a file resource.
func NewFileResourceID(uuid objectstore.UUID) (ID, error) {
	if err := uuid.Validate(); err != nil {
		return ID{}, err
	}
	return ID{
		objectStoreUUID: uuid,
	}, nil
}

// NewContainerImageMetadataResourceID creates a new storage ID for a container
// image metadata resource.
func NewContainerImageMetadataResourceID(id string) (ID, error) {
	if id == "" {
		return ID{}, errors.Errorf("container image metadata resource id cannot be empty")
	}
	return ID{
		containerImageMetadataStoreID: id,
	}, nil
}

// IsZero is true if ID has not been set.
func (id ID) IsZero() bool {
	return id.containerImageMetadataStoreID == "" && id.objectStoreUUID == ""
}

// ObjectStoreUUID returns the object store UUID or an error if it is empty.
func (id ID) ObjectStoreUUID() (objectstore.UUID, error) {
	if id.objectStoreUUID == "" {
		return "", errors.Errorf("object store UUID is empty")
	}
	return id.objectStoreUUID, nil
}

// ContainerImageMetadataStoreID returns the container image metadata store ID
// or an error if it is empty.
func (id ID) ContainerImageMetadataStoreID() (string, error) {
	if id.containerImageMetadataStoreID == "" {
		return "", errors.Errorf("container image metadata store ID is empty")
	}
	return id.containerImageMetadataStoreID, nil
}

// ResourceStoreGetter is a function which returns a ResourceStore.
type ResourceStoreGetter func() ResourceStore
