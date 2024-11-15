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

// ContainerImageResourceStore is a ResourceStore for container image resource types.
type ContainerImageResourceStore struct {
}

func (f ContainerImageResourceStore) Get(ctx context.Context, path string) (io.ReadCloser, int64, error) {
	return nil, -1, nil
}

// FileResourceStore is a ResourceStore for file resource types.
type FileResourceStore struct {
	objectStore objectstore.ObjectStore
}

func (f FileResourceStore) Get(ctx context.Context, path string) (io.ReadCloser, int64, error) {
	return f.objectStore.Get(ctx, path)
}

type ResourceStoreFactory struct {
	objectStore objectstore.ModelObjectStoreGetter
}

// NewResourceStoreFactory returns a factory which provides the appropriate
// Resource Store for the resource type.
func NewResourceStoreFactory(objectStore objectstore.ModelObjectStoreGetter) ResourceStoreFactory {
	return ResourceStoreFactory{objectStore: objectStore}
}

// GetResourceStore returns the appropriate ResourceStore for the
// give resource type.
func (f ResourceStoreFactory) GetResourceStore(ctx context.Context, t resource.Type) (ResourceStore, error) {
	switch t {
	case resource.TypeContainerImage:
		return ContainerImageResourceStore{}, nil
	case resource.TypeFile:
		store, err := f.objectStore.GetObjectStore(ctx)
		if err != nil {
			return nil, err
		}

		return FileResourceStore{objectStore: store}, nil
	default:
		return nil, fmt.Errorf("unknown resource type %q", t)
	}
}
