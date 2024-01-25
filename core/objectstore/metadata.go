// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package objectstore

import (
	"context"

	"github.com/juju/errors"

	"github.com/juju/juju/core/watcher"
)

const (
	// ErrHashMismatch is returned when the hash of the object does not match
	// the expected hash.
	ErrHashMismatch = errors.ConstError("hash mismatch")
)

// Metadata represents the metadata for an object.
type Metadata struct {
	// Hash is the hash of the object.
	Hash string
	// Path is the path to the object.
	Path string
	// Size is the size of the object.
	Size int64
}

// Metadata represents the metadata for an object store.
type ObjectStoreMetadata interface {

	// GetMetadata returns the persistence metadata for the specified path.
	GetMetadata(ctx context.Context, path string) (Metadata, error)

	// PutMetadata adds a new specified path for the persistence metadata.
	PutMetadata(ctx context.Context, metadata Metadata) error

	// RemoveMetadata removes the specified path for the persistence metadata.
	RemoveMetadata(ctx context.Context, path string) error

	// ListMetadata returns the persistence metadata for all paths.
	ListMetadata(ctx context.Context) ([]Metadata, error)

	// Watch returns a watcher that emits the path changes that either have been
	// added or removed.
	Watch() (watcher.StringsWatcher, error)
}
