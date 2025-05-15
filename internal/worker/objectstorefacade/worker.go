// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package objectstorefacade

import (
	"context"
	"io"
	"time"

	"github.com/juju/errors"
	"github.com/juju/worker/v4"
	"gopkg.in/tomb.v2"

	coreobjectstore "github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/internal/worker/fortress"
)

const (
	// visitWaitTimeout is the maximum time to wait for a visit to the
	// fortress to complete. This is isn't used to limit the time for the
	// whole operation. Instead, it is used to limit the time for the visit to
	// the fortress to complete. Anything executing during the visit is then
	// allowed to take as long as it needs until it's complete.
	visitWaitTimeout = 5 * time.Second
)

// Config holds the dependencies and configuration for a Worker.
type Config struct {
	FortressVistor    fortress.Guest
	ObjectStoreGetter coreobjectstore.ObjectStoreGetter
}

// Validate returns an error if the config cannot be expected to
// drive a functional Worker.
func (config Config) Validate() error {
	if config.FortressVistor == nil {
		return errors.NotValidf("nil FortressVistor")
	}
	if config.ObjectStoreGetter == nil {
		return errors.NotValidf("nil ObjectStoreGetter")
	}
	return nil
}

// NewWorker returns a Worker that tracks the result of the configured.
func NewWorker(config Config) (worker.Worker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	w := &Worker{
		config: config,
	}
	w.tomb.Go(w.loop)
	return w, nil
}

// Worker watches the object store service for changes to the draining
// phase. If the phase is draining, it locks the guard. If the phase is not
// draining, it unlocks the guard.
// The worker will manage the lifecycle of the watcher and will stop
// watching when the worker is killed or when the context is cancelled.
type Worker struct {
	tomb   tomb.Tomb
	config Config
}

// Kill kills the worker. It will cause the worker to stop if it is
// not already stopped. The worker will transition to the dying state.
func (w *Worker) Kill() {
	w.tomb.Kill(nil)
}

// Wait waits for the worker to finish. It will cause the worker to
// stop if it is not already stopped. It will return an error if the
// worker was killed with an error.
func (w *Worker) Wait() error {
	return w.tomb.Wait()
}

// GetObjectStore returns a object store for the given namespace.
func (w *Worker) GetObjectStore(ctx context.Context, namespace string) (coreobjectstore.ObjectStore, error) {
	objectStore, err := w.config.ObjectStoreGetter.GetObjectStore(ctx, namespace)
	if err != nil {
		return nil, errors.Trace(err)
	}

	return objectStoreFacade{
		ObjectStore:    objectStore,
		FortressVistor: w.config.FortressVistor,
	}, nil
}

func (w *Worker) loop() error {
	<-w.tomb.Dying()
	return tomb.ErrDying
}

// objectStoreFacade is a vaneer over the object store which ensures that every
// method call is guarded by a visit to the fortress. This is necessary because
// the object store can be draining, and we want to be able to wait for the
// draining to complete before we start using the object store.
type objectStoreFacade struct {
	ObjectStore    coreobjectstore.ObjectStore
	FortressVistor fortress.Guest
}

// Get returns an io.ReadCloser for data at path, namespaced to the model.
// The method will block until the fortress is drained or the context
// is cancelled. If the fortress is draining, the method will return
// [objectstore.ErrTimeoutWaitingForDraining] error.
func (o objectStoreFacade) Get(ctx context.Context, path string) (io.ReadCloser, int64, error) {
	visitCtx, cancel := context.WithTimeout(ctx, visitWaitTimeout)
	defer cancel()

	var (
		reader io.ReadCloser
		size   int64
	)
	if visitErr := o.FortressVistor.Visit(visitCtx, func() error {
		var err error
		reader, size, err = o.ObjectStore.Get(ctx, path)
		if err != nil {
			return errors.Trace(err)
		}
		return nil
	}); errors.Is(visitErr, fortress.ErrAborted) {
		return nil, 0, coreobjectstore.ErrTimeoutWaitingForDraining
	} else if visitErr != nil {
		return nil, 0, errors.Trace(visitErr)
	}
	return reader, size, nil
}

// GetBySHA256 returns an io.ReadCloser for the object with the given SHA256
// hash, namespaced to the model.
// The method will block until the fortress is drained or the context
// is cancelled. If the fortress is draining, the method will return
// [objectstore.ErrTimeoutWaitingForDraining] error.
func (o objectStoreFacade) GetBySHA256(ctx context.Context, sha256 string) (io.ReadCloser, int64, error) {
	visitCtx, cancel := context.WithTimeout(ctx, visitWaitTimeout)
	defer cancel()

	var (
		reader io.ReadCloser
		size   int64
	)
	if visitErr := o.FortressVistor.Visit(visitCtx, func() error {
		var err error
		reader, size, err = o.ObjectStore.GetBySHA256(ctx, sha256)
		if err != nil {
			return errors.Trace(err)
		}
		return nil
	}); errors.Is(visitErr, fortress.ErrAborted) {
		return nil, 0, coreobjectstore.ErrTimeoutWaitingForDraining
	} else if visitErr != nil {
		return nil, 0, errors.Trace(visitErr)
	}
	return reader, size, nil
}

// GetBySHA256Prefix returns an io.ReadCloser for any object with the a SHA256
// hash starting with a given prefix, namespaced to the model.
// The method will block until the fortress is drained or the context
// is cancelled. If the fortress is draining, the method will return
// [objectstore.ErrTimeoutWaitingForDraining] error.
func (o objectStoreFacade) GetBySHA256Prefix(ctx context.Context, sha256Prefix string) (io.ReadCloser, int64, error) {
	visitCtx, cancel := context.WithTimeout(ctx, visitWaitTimeout)
	defer cancel()

	var (
		reader io.ReadCloser
		size   int64
	)
	if visitErr := o.FortressVistor.Visit(visitCtx, func() error {
		var err error
		reader, size, err = o.ObjectStore.GetBySHA256Prefix(ctx, sha256Prefix)
		if err != nil {
			return errors.Trace(err)
		}
		return nil
	}); errors.Is(visitErr, fortress.ErrAborted) {
		return nil, 0, coreobjectstore.ErrTimeoutWaitingForDraining
	} else if visitErr != nil {
		return nil, 0, errors.Trace(visitErr)
	}
	return reader, size, nil
}

// Put stores data from reader at path, namespaced to the model.
// The method will block until the fortress is drained or the context
// is cancelled. If the fortress is draining, the method will return
// [objectstore.ErrTimeoutWaitingForDraining] error.
func (o objectStoreFacade) Put(ctx context.Context, path string, r io.Reader, size int64) (coreobjectstore.UUID, error) {
	visitCtx, cancel := context.WithTimeout(ctx, visitWaitTimeout)
	defer cancel()

	var uuid coreobjectstore.UUID
	if visitErr := o.FortressVistor.Visit(visitCtx, func() error {
		var err error
		uuid, err = o.ObjectStore.Put(ctx, path, r, size)
		if err != nil {
			return errors.Trace(err)
		}
		return nil
	}); errors.Is(visitErr, fortress.ErrAborted) {
		return "", coreobjectstore.ErrTimeoutWaitingForDraining
	} else if visitErr != nil {
		return "", errors.Trace(visitErr)
	}
	return uuid, nil
}

// PutAndCheckHash stores data from reader at path, namespaced to the model.
// The method will block until the fortress is drained or the context
// is cancelled. If the fortress is draining, the method will return
// [objectstore.ErrTimeoutWaitingForDraining] error.
func (o objectStoreFacade) PutAndCheckHash(ctx context.Context, path string, r io.Reader, size int64, sha384 string) (coreobjectstore.UUID, error) {
	visitCtx, cancel := context.WithTimeout(ctx, visitWaitTimeout)
	defer cancel()

	var uuid coreobjectstore.UUID
	if visitErr := o.FortressVistor.Visit(visitCtx, func() error {
		var err error
		uuid, err = o.ObjectStore.PutAndCheckHash(ctx, path, r, size, sha384)
		if err != nil {
			return errors.Trace(err)
		}
		return nil
	}); errors.Is(visitErr, fortress.ErrAborted) {
		return "", coreobjectstore.ErrTimeoutWaitingForDraining
	} else if visitErr != nil {
		return "", errors.Trace(visitErr)
	}
	return uuid, nil
}

// Remove removes data at path, namespaced to the model.
// The method will block until the fortress is drained or the context
// is cancelled. If the fortress is draining, the method will return
// [objectstore.ErrTimeoutWaitingForDraining] error.
func (o objectStoreFacade) Remove(ctx context.Context, path string) error {
	visitCtx, cancel := context.WithTimeout(ctx, visitWaitTimeout)
	defer cancel()

	if visitErr := o.FortressVistor.Visit(visitCtx, func() error {
		return o.ObjectStore.Remove(ctx, path)
	}); errors.Is(visitErr, fortress.ErrAborted) {
		return coreobjectstore.ErrTimeoutWaitingForDraining
	} else if visitErr != nil {
		return errors.Trace(visitErr)
	}
	return nil
}
