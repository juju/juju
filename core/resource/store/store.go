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
	// key. It takes a storageKey which is used as an identifier for the stored
	// resource, a reader for the resource blob, and a size and fingerprint to
	// validate the resource blob against.
	//
	// It returns the storage ID, which can be used to refer to the stored
	// resource in the database, note that this may be different from the
	// storageKey. It also returns the size and fingerprint of the stored
	// resource, which may differ from the size and fingerprint passed in for
	// validation.
	Put(
		ctx context.Context,
		storageKey string,
		r io.Reader,
		size int64,
		fingerprint Fingerprint,
	) (ID, int64, Fingerprint, error)

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

type storeKind int

const (
	unsetStoreKind storeKind = iota
	objectStoreKind
	containerImageMetadataStoreKind
)

// ID is the ID of the stored blob in the database, this can
// be used for adding referential integrity from the resource to the stored
// blob. This can be an object store metadata UUID or a container image metadata
// storage key. It is only one, never both.
type ID struct {
	kind storeKind

	objectStoreUUID               objectstore.UUID
	containerImageMetadataStoreID string
}

// NewFileResourceID creates a new storage ID for a file resource.
func NewFileResourceID(uuid objectstore.UUID) (ID, error) {
	if err := uuid.Validate(); err != nil {
		return ID{}, err
	}
	return ID{
		kind:            objectStoreKind,
		objectStoreUUID: uuid,
	}, nil
}

// GenFileResourceStoreID can be used in testing for generating a file resource
// store ID that is checked for subsequent errors.
func GenFileResourceStoreID(c interface{ Fatal(...any) }, uuid objectstore.UUID) ID {
	id, err := NewFileResourceID(uuid)
	if err != nil {
		c.Fatal(err)
	}
	return id
}

// NewContainerImageMetadataResourceID creates a new storage ID for a container
// image metadata resource.
func NewContainerImageMetadataResourceID(id string) (ID, error) {
	if id == "" {
		return ID{}, errors.Errorf("container image metadata resource id cannot be empty")
	}
	return ID{
		kind:                          containerImageMetadataStoreKind,
		containerImageMetadataStoreID: id,
	}, nil
}

// GenContainerImageMetadataResourceID can be used in testing for generating a
// container image metadata resource store ID that is checked.
func GenContainerImageMetadataResourceID(c interface{ Fatal(...any) }, storageKey string) ID {
	id, err := NewContainerImageMetadataResourceID(storageKey)
	if err != nil {
		c.Fatal(err)
	}
	return id
}

// IsZero is true if ID has not been set.
func (id ID) IsZero() bool {
	return id.kind == unsetStoreKind
}

// IsObjectStoreUUID returns true if the type contains an object store UUID.
func (id ID) IsObjectStoreUUID() bool {
	return id.kind == objectStoreKind
}

// IsContainerImageMetadataID returns true if the type contains a container
// image metadata store ID.
func (id ID) IsContainerImageMetadataID() bool {
	return id.kind == containerImageMetadataStoreKind
}

// ObjectStoreUUID returns the object store UUID or an error if it is not set.
func (id ID) ObjectStoreUUID() (objectstore.UUID, error) {
	if id.kind != objectStoreKind {
		return "", errors.Errorf("object store UUID not set")
	}
	return id.objectStoreUUID, nil
}

// ContainerImageMetadataStoreID returns the container image metadata store ID
// or an error if it is not set.
func (id ID) ContainerImageMetadataStoreID() (string, error) {
	if id.kind != containerImageMetadataStoreKind {
		return "", errors.Errorf("container image metadata store ID not set")
	}
	return id.containerImageMetadataStoreID, nil
}

// ResourceStoreGetter is a function which returns a ResourceStore.
type ResourceStoreGetter func() ResourceStore
