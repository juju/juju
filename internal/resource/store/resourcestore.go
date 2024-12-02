// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package store

import (
	"context"
	"sync"

	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/core/resource/store"
	"github.com/juju/juju/internal/charm/resource"
	"github.com/juju/juju/internal/errors"
)

// ResourceStoreFactory contains the information to provide the required
// ResourceStore.
type ResourceStoreFactory struct {
	objectStore objectstore.ModelObjectStoreGetter
	mu          sync.Mutex
	storeMap    map[resource.Type]store.ResourceStore
}

// NewResourceStoreFactory returns a factory which provides the appropriate
// Resource Store for the resource type.
func NewResourceStoreFactory(objectStore objectstore.ModelObjectStoreGetter) *ResourceStoreFactory {
	return &ResourceStoreFactory{
		objectStore: objectStore,
		storeMap:    make(map[resource.Type]store.ResourceStore),
	}
}

// GetResourceStore returns the appropriate ResourceStore for the
// given resource type.
func (f *ResourceStoreFactory) GetResourceStore(ctx context.Context, t resource.Type) (store.ResourceStore, error) {
	switch t {
	case resource.TypeFile:
		store, err := f.objectStore.GetObjectStore(ctx)
		if err != nil {
			return nil, errors.Errorf("getting file resource store: %w", err)
		}
		return fileResourceStore{objectStore: store}, nil
	default:
		f.mu.Lock()
		defer f.mu.Unlock()
		store, ok := f.storeMap[t]
		if !ok {
			return nil, UnknownResourceType
		}
		return store, nil
	}
}

// AddStore injects a ResourceStore for the given type into the
// ResourceStoreFactory.
//
// Note:
// The store for container image resources is really a DQLite table today.
// This method is a compromise to avoid injecting one service into another
// if the ContainerImageResourceStore was provided as an argument to
// NewResourceStoreFactory. If we get a new implementation of a container image
// resource store this should be re-evaluated and hopefully removed.
func (f *ResourceStoreFactory) AddStore(t resource.Type, store store.ResourceStore) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.storeMap[t] = store
}
