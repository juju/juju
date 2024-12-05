// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package store

import (
	"context"
	"io"

	"github.com/juju/utils/v4/hash"
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
	) (UUID, error)

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

// UUID is the UUID of the stored blob in the database, this can
// be used for adding referential integrity from the resource to the stored
// blob. This can be an object store metadata UUID or a container image metadata
// storage key.
type UUID string

// ResourceStoreGetter is a function which returns a ResourceStore.
type ResourceStoreGetter func() ResourceStore
