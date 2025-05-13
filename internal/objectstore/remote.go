// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package objectstore

import (
	"context"
	"io"

	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/catacomb"

	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/worker/apiremotecaller"
)

// remoteFileObjectStore is a facade for the object store that uses a remote
// worker to perform operations. The remoteFileObjectStore is a
// TrackedObjectStore that ties the lifecycle of the objectStore and
// remoteWorker together. To prevent additional complexity in the file
// objectstore implementation, the file objectstore doesn't need to know how to
// manage the remoteWorker.
type remoteFileObjectStore struct {
	catacomb catacomb.Catacomb

	objectStore TrackedObjectStore
}

// newRemoteFileObjectStore returns a new remoteFileObjectStore.
func newRemoteFileObjectStore(objectStore TrackedObjectStore, remoteWorker worker.Worker) (*remoteFileObjectStore, error) {
	w := &remoteFileObjectStore{
		objectStore: objectStore,
	}

	if err := catacomb.Invoke(catacomb.Plan{
		Name: "remote-file-object-store",
		Site: &w.catacomb,
		Work: w.loop,
		Init: []worker.Worker{objectStore, remoteWorker},
	}); err != nil {
		return nil, errors.Capture(err)
	}

	return w, nil
}

// Kill stops the remoteFileObjectStore.
func (c *remoteFileObjectStore) Kill() {
	c.catacomb.Kill(nil)
}

// Wait waits for the remoteFileObjectStore to stop.
func (c *remoteFileObjectStore) Wait() error {
	return c.catacomb.Wait()
}

// Get returns an io.ReadCloser for data at path, namespaced to the model.
//
// If the object does not exist, an [objectstore.ObjectNotFound] error is
// returned.
func (c *remoteFileObjectStore) Get(ctx context.Context, path string) (io.ReadCloser, int64, error) {
	return c.objectStore.Get(ctx, path)
}

// GetBySHA256 returns an io.ReadCloser for the object with the given SHA256
// hash, namespaced to the model.
//
// If no object is found, an [objectstore.ObjectNotFound] error is returned.
func (c *remoteFileObjectStore) GetBySHA256(ctx context.Context, sha256 string) (io.ReadCloser, int64, error) {
	return c.objectStore.GetBySHA256(ctx, sha256)
}

// GetBySHA256Prefix returns an io.ReadCloser for any object with the a SHA256
// hash starting with a given prefix, namespaced to the model.
//
// If no object is found, an [objectstore.ObjectNotFound] error is returned.
func (c *remoteFileObjectStore) GetBySHA256Prefix(ctx context.Context, sha256Prefix string) (io.ReadCloser, int64, error) {
	return c.objectStore.GetBySHA256Prefix(ctx, sha256Prefix)
}

// Put stores data from reader at path, namespaced to the model.
func (c *remoteFileObjectStore) Put(ctx context.Context, path string, r io.Reader, size int64) (objectstore.UUID, error) {
	return c.objectStore.Put(ctx, path, r, size)
}

// PutAndCheckHash stores data from reader at path, namespaced to the model.
// It also ensures the stored data has the correct sha384.
func (c *remoteFileObjectStore) PutAndCheckHash(ctx context.Context, path string, r io.Reader, size int64, sha384 string) (objectstore.UUID, error) {
	return c.objectStore.PutAndCheckHash(ctx, path, r, size, sha384)
}

// Remove removes data at path, namespaced to the model.
func (c *remoteFileObjectStore) Remove(ctx context.Context, path string) error {
	return c.objectStore.Remove(ctx, path)
}

func (c *remoteFileObjectStore) loop() error {
	select {
	case <-c.catacomb.Dying():
		return c.catacomb.ErrDying()
	}
}

type noopAPIRemoteCallers struct{}

// GetAPIRemotes returns no API remotes, this will be default if it's not
// set.
func (noopAPIRemoteCallers) GetAPIRemotes() []apiremotecaller.RemoteConnection {
	return nil
}
