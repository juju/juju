// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package objectstore

import (
	"context"
	"io"

	"github.com/juju/errors"
)

const (
	// ErrObjectStoreDying is used to indicate to *third parties* that the
	// object store worker is dying, instead of catacomb.ErrDying, which is
	// unsuitable for propagating inter-worker.
	// This error indicates to consuming workers that their dependency has
	// become unmet and a restart by the dependency engine is imminent.
	ErrObjectStoreDying = errors.ConstError("object store worker is dying")
)

// Session provides access to the object store.
type Session interface {
	ReadSession
	WriteSession
}

// ReadSession provides read access to the object store.
type ReadSession interface {
	// GetObject returns a reader for the specified object.
	GetObject(ctx context.Context, bucketName, objectName string) (io.ReadCloser, error)
}

// WriteSession provides read access to the object store.
type WriteSession interface {
	// PutObject puts an object into the object store based on the bucket name and
	// object name.
	PutObject(ctx context.Context, bucketName, objectName string, body io.Reader, hash string) error

	// DeleteObject deletes an object from the object store based on the bucket name
	// and object name.
	DeleteObject(ctx context.Context, bucketName, objectName string) error
}

// ObjectStore represents a full object store for both read and write access.
type ObjectStore interface {
	ReadObjectStore
	WriteObjectStore
}

// ReadObjectStore represents an object store that can only be read from.
type ReadObjectStore interface {
	// Get returns an io.ReadCloser for data at path, namespaced to the
	// model.
	Get(context.Context, string) (io.ReadCloser, int64, error)
}

// WriteObjectStore represents an object store that can only be written to.
type WriteObjectStore interface {
	// Put stores data from reader at path, namespaced to the model.
	Put(ctx context.Context, path string, r io.Reader, size int64) error

	// Put stores data from reader at path, namespaced to the model.
	// It also ensures the stored data has the correct hash.
	PutAndCheckHash(ctx context.Context, path string, r io.Reader, size int64, hash string) error

	// Remove removes data at path, namespaced to the model.
	Remove(ctx context.Context, path string) error
}
