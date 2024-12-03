// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package store

import (
	"context"

	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/core/resource/store"
	"github.com/juju/juju/internal/charm/resource"
	"github.com/juju/juju/internal/errors"
)

// ResourceStoreFactory contains the information to provide the required
// ResourceStore.
type ResourceStoreFactory struct {
	objectStore         objectstore.ModelObjectStoreGetter
	containerImageStore store.ResourceStoreGetter
}

// NewResourceStoreFactory returns a factory which provides the appropriate
// Resource Store for the resource type.
func NewResourceStoreFactory(
	objectStore objectstore.ModelObjectStoreGetter,
	containerImageStore store.ResourceStoreGetter,
) *ResourceStoreFactory {
	return &ResourceStoreFactory{
		objectStore:         objectStore,
		containerImageStore: containerImageStore,
	}
}

// GetResourceStore returns the appropriate ResourceStore for the
// given resource type.
func (f *ResourceStoreFactory) GetResourceStore(ctx context.Context, t resource.Type) (store.ResourceStore, error) {
	switch t {
	case resource.TypeFile:
		objectStore, err := f.objectStore.GetObjectStore(ctx)
		if err != nil {
			return nil, errors.Errorf("getting file resource store: %w", err)
		}
		return fileResourceStore{objectStore: objectStore}, nil
	case resource.TypeContainerImage:
		return f.containerImageStore(), nil
	default:
		return nil, UnknownResourceType
	}
}
