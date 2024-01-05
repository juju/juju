// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package objectstore

import (
	"context"
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"gopkg.in/tomb.v2"

	"github.com/juju/juju/core/objectstore"
)

// Claimer is the interface that is used to claim an exclusive lock on a file.
// The lock is used to prevent concurrent access to the same file for put
// and remove operations.
type Claimer interface {
	// Claim locks the file with the given hash.
	Claim(ctx context.Context, hash string) (ClaimExtender, error)
	// Release releases the file with the given hash.
	Release(ctx context.Context, hash string) error
}

// ClaimExtender is the interface that is used to extend a lock.
type ClaimExtender interface {
	// Extend extends the lock for the given hash.
	Extend(ctx context.Context) error

	// Duration returns the duration of the lock.
	Duration() time.Duration
}

type baseObjectStore struct {
	tomb            tomb.Tomb
	metadataService objectstore.ObjectStoreMetadata
	claimer         Claimer
	logger          Logger
	clock           clock.Clock
}

// Kill implements the worker.Worker interface.
func (s *baseObjectStore) Kill() {
	s.tomb.Kill(nil)
}

// Wait implements the worker.Worker interface.
func (s *baseObjectStore) Wait() error {
	return s.tomb.Wait()
}

// scopedContext returns a context that is in the scope of the worker lifetime.
// It returns a cancellable context that is cancelled when the action has
// completed.
func (w *baseObjectStore) scopedContext() (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(context.Background())
	return w.tomb.Context(ctx), cancel
}

func (w *baseObjectStore) withLock(ctx context.Context, hash string, f func(context.Context) error) error {
	// If the context is already done, then don't waste any cycles trying
	// to claim the lock.
	if err := ctx.Err(); err != nil {
		return errors.Trace(err)
	}

	// Lock the file with the given hash, so that we can't remove the file
	// while we're writing it.
	extender, err := w.claimer.Claim(ctx, hash)
	if err != nil {
		return errors.Trace(err)
	}

	// Always release the lock when we're done. This is optimistic, because
	// when the duration of the lock has expired, the lock will be released
	// anyway.
	defer func() {
		_ = w.claimer.Release(ctx, hash)
	}()

	runnerCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Extend the lock for the duration of the operation.
	var runner tomb.Tomb
	runner.Go(func() error {
		defer cancel()

		return f(runnerCtx)
	})
	runner.Go(func() error {
		defer cancel()

		for {
			select {
			case <-w.tomb.Dying():
				return nil
			case <-runnerCtx.Done():
				return nil

			case <-w.clock.After(extender.Duration()):
				// Attempt to extend the lock if the function is still running.
				if err := extender.Extend(runnerCtx); err != nil {
					return errors.Trace(err)
				}
			}
		}
	})

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-runner.Dying():
		return runner.Err()
	case <-w.tomb.Dying():
		// Ensure that we cancel the context if the runner is dying.
		runner.Kill(nil)
		if err := runner.Wait(); err != nil {
			return errors.Trace(err)
		}
		return tomb.ErrDying
	}
}
