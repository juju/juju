// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	"context"
	"io"

	"github.com/juju/juju/core/objectstore"
)

// ObjectStore provides an interface for storing objects.
type ObjectStore interface {
	// Put stores data from reader at path, namespaced to the model.
	// It also ensures the stored data has the correct hash.
	PutAndCheckHash(ctx context.Context, path string, r io.Reader, size int64, hash string) (objectstore.UUID, error)
}

// CharmStore provides an interface for storing charms.
type CharmStore struct {
	ObjectStore ObjectStore
}
