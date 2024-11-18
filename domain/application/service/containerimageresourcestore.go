// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"io"

	"github.com/juju/juju/core/errors"
	coreresource "github.com/juju/juju/core/resources"
	resourcestore "github.com/juju/juju/domain/application/resource"
	"github.com/juju/juju/internal/charm/resource"
)

// ContainerImageResourceState provides methods for interacting
// with the container image resource store.
type ContainerImageResourceState interface {
}

func newContainerImageResourceStore(st ContainerImageResourceState) *containerImageResourceStore {
	return &containerImageResourceStore{st: st}
}

// containerImageResourceStore is a ResourceStore for container image resource types.
type containerImageResourceStore struct {
	st ContainerImageResourceState
}

// Get returns an io.ReadCloser for a resource in the resource store.
func (f containerImageResourceStore) Get(
	ctx context.Context,
	resourceUUID coreresource.ID,
) (r io.ReadCloser, size int64, err error) {
	return nil, -1, errors.NotImplemented
}

// Put stores data from io.Reader in the resource store at the
// path specified in the resource.
func (f containerImageResourceStore) Put(
	ctx context.Context,
	resourceUUID coreresource.ID,
	r io.Reader,
	size int64,
	fingerprint resource.Fingerprint,
) (resourcestore.ResourceStorageUUID, error) {
	return nil, errors.NotImplemented
}

// Remove removes a resource from storage.
func (f containerImageResourceStore) Remove(
	ctx context.Context,
	resourceUUID coreresource.ID,
) error {
	return errors.NotImplemented
}
