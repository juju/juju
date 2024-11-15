// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resource

import (
	"context"
	"fmt"
	"io"

	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/internal/charm/resource"
)

// ResourceStore provides a list of methods necessary for interacting with
// a store for the resource.
type ResourceStore interface {
	// Get returns an io.ReadCloser for data at path, namespaced to the
	// model.
	Get(context.Context, string) (io.ReadCloser, int64, error)
}

// FileResourceStore is a ResourceStore for file resource types.
type FileResourceStore struct {
	objectStore objectstore.ObjectStore
}

func (f FileResourceStore) Get(ctx context.Context, path string) (io.ReadCloser, int64, error) {
	return f.objectStore.Get(ctx, path)
}

// ResourceStoreFactory contains the information to provide the required
// ResourceStore.
type ResourceStoreFactory struct {
	objectStore objectstore.ModelObjectStoreGetter
	storeMap    map[resource.Type]ResourceStore
}

// NewResourceStoreFactory returns a factory which provides the appropriate
// Resource Store for the resource type.
func NewResourceStoreFactory(objectStore objectstore.ModelObjectStoreGetter) *ResourceStoreFactory {
	return &ResourceStoreFactory{
		objectStore: objectStore,
		storeMap:    make(map[resource.Type]ResourceStore),
	}
}

// GetResourceStore returns the appropriate ResourceStore for the
// given resource type.
func (f *ResourceStoreFactory) GetResourceStore(ctx context.Context, t resource.Type) (ResourceStore, error) {
	switch t {
	case resource.TypeFile:
		store, err := f.objectStore.GetObjectStore(ctx)
		if err != nil {
			return nil, err
		}
		return FileResourceStore{objectStore: store}, nil
	default:
		store, ok := f.storeMap[t]
		if !ok {
			return nil, fmt.Errorf("unknown resource type %q", t)
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
func (f *ResourceStoreFactory) AddStore(t resource.Type, store ResourceStore) {
	f.storeMap[t] = store
}
