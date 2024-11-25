// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resource

import (
	"context"
	"io"

	"github.com/juju/juju/core/objectstore"
	coreresource "github.com/juju/juju/core/resource"
	"github.com/juju/juju/internal/charm/resource"
	"github.com/juju/juju/internal/errors"
)

// fileResourceStore is a ResourceStore for file resource types.
type fileResourceStore struct {
	objectStore objectstore.ObjectStore
}

// Get the specified resource from the object store.
func (f fileResourceStore) Get(
	ctx context.Context,
	resourceUUID coreresource.ID,
) (io.ReadCloser, int64, error) {
	if err := resourceUUID.Validate(); err != nil {
		return nil, 0, errors.Errorf("validating resource UUID: %w", err)
	}
	return f.objectStore.Get(ctx, resourceUUID.String())
}

// Put the given resource in the object store using the resource UUID as the
// storage path. It returns the UUID of the object store metadata.
func (f fileResourceStore) Put(
	ctx context.Context,
	resourceUUID coreresource.ID,
	r io.Reader,
	size int64,
	fingerprint resource.Fingerprint,
) (ResourceStorageUUID, error) {
	if err := resourceUUID.Validate(); err != nil {
		return nil, errors.Errorf("validating resource UUID: %w", err)
	}
	if r == nil {
		return nil, errors.Errorf("validating resource: reader is nil")
	}
	if size == 0 {
		return nil, errors.Errorf("validating resource: size is 0")
	}
	if err := fingerprint.Validate(); err != nil {
		return nil, errors.Errorf("validating resource fingerprint: %w", err)
	}
	return f.objectStore.PutAndCheckHash(ctx, resourceUUID.String(), r, size, fingerprint.String())
}

// Remove the specified resource from the object store.
func (f fileResourceStore) Remove(
	ctx context.Context,
	resourceUUID coreresource.ID,
) error {
	if err := resourceUUID.Validate(); err != nil {
		return errors.Errorf("validating resource UUID: %w", err)
	}
	return f.objectStore.Remove(ctx, resourceUUID.String())
}
