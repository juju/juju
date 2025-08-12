// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package objectstore

import (
	"context"
	"io"

	"github.com/juju/juju/internal/errors"
)

const (
	// ErrObjectStoreDying is used to indicate to *third parties* that the
	// object store worker is dying, instead of catacomb.ErrDying, which is
	// unsuitable for propagating inter-worker.
	// This error indicates to consuming workers that their dependency has
	// become unmet and a restart by the dependency engine is imminent.
	ErrObjectStoreDying = errors.ConstError("object store worker is dying")

	// ErrTimeoutWaitingForDraining is used to indicate that the object store
	// worker is taking too long to drain. This is used to indicate to
	// *third parties* that the object store worker is draining.
	ErrTimeoutWaitingForDraining = errors.ConstError("timeout waiting for object store draining to complete")

	// ErrObjectStoreNotFound is used to indicate that the object store
	// for the given namespace could not be found.
	ErrObjectStoreNotFound = errors.ConstError("object store not found for namespace")
)

// Client provides access to the object store.
type Client interface {
	// Session calls the given function with a session.
	// The func maybe called multiple times if the underlying session has
	// invalid credentials. Therefore session might not be the same across
	// calls.
	// It is the caller's responsibility to ensure that f is idempotent.
	Session(ctx context.Context, f func(context.Context, Session) error) error
}

// Session provides access to the object store.
type Session interface {
	ReadSession
	WriteSession
	BucketSession
}

// ReadSession provides read access to the object store.
type ReadSession interface {
	// ObjectExists returns nil if the object exists, or an error if it does
	// not.
	ObjectExists(ctx context.Context, bucketName, objectName string) error

	// GetObject returns a reader for the specified object.
	GetObject(ctx context.Context, bucketName, objectName string) (io.ReadCloser, int64, string, error)

	// ListObjects returns a list of objects in the specified bucket.
	ListObjects(ctx context.Context, bucketName string) ([]string, error)
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

// BucketSession provides additional access to the object store. This allows
// the manipulation of buckets.
type BucketSession interface {
	// CreateBucket creates a bucket in the object store based on the bucket name.
	CreateBucket(ctx context.Context, bucketName string) error
}

// ObjectStoreGetter is the interface that is used to get a object store.
type ObjectStoreGetter interface {
	// GetObjectStore returns a object store for the given namespace.
	GetObjectStore(context.Context, string) (ObjectStore, error)
}

// ObjectStoreFlusher is the interface that is used to flush the object store.
type ObjectStoreFlusher interface {
	// FlushWorkers flushes the object store workers.
	FlushWorkers(context.Context) error
}

// ModelObjectStoreGetter is the interface that is used to get a model's
// object store.
type ModelObjectStoreGetter interface {
	NamespacedObjectStoreGetter
}

type NamespacedObjectStoreGetter interface {
	// GetObjectStore returns an object store for the fixed namespace
	// encapsulated by the implementation of this interface, usually
	// a model UUID or the global controller namespace.
	GetObjectStore(context.Context) (ObjectStore, error)
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
	//
	// If the object does not exist, an [objectstore.ObjectNotFound]
	// error is returned.
	Get(context.Context, string) (io.ReadCloser, int64, error)

	// GetBySHA256 returns an io.ReadCloser for the object with the given SHA256
	// hash, namespaced to the model.
	//
	// If no object is found, an [objectstore.ObjectNotFound] error is returned.
	GetBySHA256(context.Context, string) (io.ReadCloser, int64, error)

	// GetBySHA256Prefix returns an io.ReadCloser for any object with the a SHA256
	// hash starting with a given prefix, namespaced to the model.
	//
	// If no object is found, an [objectstore.ObjectNotFound] error is returned.
	GetBySHA256Prefix(context.Context, string) (io.ReadCloser, int64, error)
}

// WriteObjectStore represents an object store that can only be written to.
type WriteObjectStore interface {
	// Put stores data from reader at path, namespaced to the model.
	Put(ctx context.Context, path string, r io.Reader, size int64) (UUID, error)

	// PutAndCheckHash stores data from reader at path, namespaced to the model.
	// It also ensures the stored data has the correct sha384.
	PutAndCheckHash(ctx context.Context, path string, r io.Reader, size int64, sha384 string) (UUID, error)

	// Remove removes data at path, namespaced to the model.
	Remove(ctx context.Context, path string) error
}

// ObjectStoreRemover is an interface that provides a method to remove all
// data for the namespaced model. It is destructive and should be used with
// caution. No objects will be retrievable after this call. This is expected
// to be used when the model is being removed or when the object store has
// been drained and is no longer needed.
//
// It is typically implemented by object stores that support the
// RemoveAll method.
type ObjectStoreRemover interface {
	// RemoveAll removes all data for the namespaced model. It is destructive
	// and should be used with caution. No objects will be retrievable after
	// this call. This is expected to be used when the model is being removed or
	// when the object store has been drained and is no longer needed.
	RemoveAll(ctx context.Context) error
}
