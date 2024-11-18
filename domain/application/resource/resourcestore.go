// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resource

import (
	"context"
	"io"
	"sync"

	"github.com/juju/juju/core/objectstore"
	coreresource "github.com/juju/juju/core/resources"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/internal/charm/resource"
	"github.com/juju/juju/internal/errors"
)

// ResourceStore provides a list of methods necessary for interacting with
// a store for the resource.
type ResourceStore interface {
	// Get returns an io.ReadCloser for a resource in the resource store.
	Get(
		ctx context.Context,
		resourceUUID coreresource.ID,
	) (r io.ReadCloser, size int64, err error)

	// Put stores data from io.Reader in the resource store at the
	// using the resourceUUID as the key.
	Put(
		ctx context.Context,
		resourceUUID coreresource.ID,
		r io.Reader,
		size int64,
		fingerprint resource.Fingerprint,
	) (ResourceStorageUUID, error)

	// Remove removes a resource from storage.
	Remove(
		ctx context.Context,
		resourceUUID coreresource.ID,
	) error
}

// ResourceStoreFactory contains the information to provide the required
// ResourceStore.
type ResourceStoreFactory struct {
	objectStore objectstore.ModelObjectStoreGetter
	mu          sync.Mutex
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
			return nil, errors.Errorf("getting file resource store: %w", err)
		}
		return fileResourceStore{objectStore: store}, nil
	default:
		f.mu.Lock()
		defer f.mu.Unlock()
		store, ok := f.storeMap[t]
		if !ok {
			return nil, applicationerrors.UnknownResourceType
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
	f.mu.Lock()
	defer f.mu.Unlock()
	f.storeMap[t] = store
}
