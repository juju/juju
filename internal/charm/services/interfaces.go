// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package services

import (
	"context"
	"io"

	"github.com/juju/juju/core/objectstore"
)

// StateBackend describes an API for accessing/mutating information in state.
type StateBackend interface {
	PrepareCharmUpload(curl string) (UploadedCharm, error)
	ModelUUID() string
}

// UploadedCharm represents a charm whose upload status can be queried.
type UploadedCharm interface {
	// TODO(nvinuesa): IsUploaded is not implemented yet.
	// See https://warthogs.atlassian.net/browse/JUJU-6845
	// IsUploaded() bool
}

// Storage describes an API for storing and deleting blobs.
type Storage interface {
	// Put stores data from reader at path, namespaced to the model.
	Put(context.Context, string, io.Reader, int64) (objectstore.UUID, error)
	// Remove removes data at path, namespaced to the model.
	Remove(context.Context, string) error
}
