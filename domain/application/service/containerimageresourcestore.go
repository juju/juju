// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"io"
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

func (f containerImageResourceStore) Get(ctx context.Context, path string) (io.ReadCloser, int64, error) {
	return nil, -1, nil
}
