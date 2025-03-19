// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package store

import (
	"context"
	"io"

	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/core/resource/store"
	"github.com/juju/juju/internal/errors"
)

// fileResourceStore is a ResourceStore for file resource types.
type fileResourceStore struct {
	objectStore objectstore.ObjectStore
}

// Get the specified resource from the object store.
func (f fileResourceStore) Get(
	ctx context.Context,
	storageKey string,
) (io.ReadCloser, int64, error) {
	if storageKey == "" {
		return nil, 0, errors.Errorf("storage key empty")
	}
	return f.objectStore.Get(ctx, storageKey)
}

// Put the given resource in the object store using the storage key as the
// storage path. It returns the UUID of the object store metadata and the size
// and fingerprint of the resource.
// If the size/fingerprint arguments do not match the size/fingerprint of the
// blob the object store should return an error.
func (f fileResourceStore) Put(
	ctx context.Context,
	storageKey string,
	r io.Reader,
	size int64,
	fingerprint store.Fingerprint,
) (store.ID, int64, store.Fingerprint, error) {
	if storageKey == "" {
		return store.ID{}, 0, store.Fingerprint{}, errors.Errorf("storage key empty")
	}
	if r == nil {
		return store.ID{}, 0, store.Fingerprint{}, errors.Errorf("validating resource: reader is nil")
	}
	if size == 0 {
		return store.ID{}, 0, store.Fingerprint{}, errors.Errorf("validating resource size: size is 0")
	}
	if err := fingerprint.Validate(); err != nil {
		return store.ID{}, 0, store.Fingerprint{}, errors.Errorf("validating resource fingerprint: %w", err)
	}
	uuid, err := f.objectStore.PutAndCheckHash(ctx, storageKey, r, size, fingerprint.String())
	if err != nil {
		return store.ID{}, 0, store.Fingerprint{}, errors.Capture(err)
	}
	id, err := store.NewFileResourceID(uuid)
	if err != nil {
		return store.ID{}, 0, store.Fingerprint{}, errors.Capture(err)
	}
	return id, size, fingerprint, nil
}

// Remove the specified resource from the object store.
func (f fileResourceStore) Remove(
	ctx context.Context,
	storageKey string,
) error {
	if storageKey == "" {
		return errors.Errorf("storage key empty")
	}
	return f.objectStore.Remove(ctx, storageKey)
}
