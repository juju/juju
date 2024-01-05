// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package objectstore

import (
	"context"
	"time"

	"github.com/juju/errors"
	"github.com/juju/juju/core/objectstore"
	"gopkg.in/tomb.v2"
)

// Locker is the interface that is used to lock a file.
type Locker interface {
	// Lock locks the file with the given hash.
	Lock(ctx context.Context, hash string) (LockExtender, error)
	// Unlock unlocks the file with the given hash.
	Unlock(ctx context.Context, hash string) error
}

// LockExtender is the interface that is used to extend a lock.
type LockExtender interface {
	// Extend extends the lock for the given hash.
	Extend(ctx context.Context) error

	// Duration returns the duration of the lock.
	Duration() time.Duration
}

type baseObjectStore struct {
	tomb            tomb.Tomb
	metadataService objectstore.ObjectStoreMetadata
	locker          Locker
	logger          Logger
}

// scopedContext returns a context that is in the scope of the worker lifetime.
// It returns a cancellable context that is cancelled when the action has
// completed.
func (w *baseObjectStore) scopedContext() (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(context.Background())
	return w.tomb.Context(ctx), cancel
}

func (w *baseObjectStore) withLock(ctx context.Context, hash string, f func(context.Context) error) error {
	// Lock the file with the given hash, so that we can't remove the file
	// while we're writing it.
	extender, err := w.locker.Lock(ctx, hash)
	if err != nil {
		return errors.Trace(err)
	}

	defer w.locker.Unlock(ctx, hash)

	newCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Extend the lock for the duration of the operation.
	var runner tomb.Tomb
	runner.Go(func() error {
		defer cancel()

		return f(newCtx)
	})
	runner.Go(func() error {
		defer cancel()

		timer := time.NewTimer(extender.Duration())
		defer timer.Stop()

		for {
			select {
			case <-newCtx.Done():
				return nil
			case <-timer.C:
				if err := extender.Extend(newCtx); err != nil {
					return errors.Trace(err)
				}
			case <-w.tomb.Dying():
				return nil
			}
		}
	})

	select {
	case <-runner.Dying():
		return runner.Err()
	case <-w.tomb.Dying():
		// Ensure that we cancel the context if the runner is dying.
		runner.Kill(nil)
		<-runner.Dying()

		return tomb.ErrDying
	}
}
