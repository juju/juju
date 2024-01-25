// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package objectstore

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"gopkg.in/tomb.v2"

	"github.com/juju/juju/core/objectstore"
)

// Claimer is the interface that is used to claim an exclusive lock for a
// particular object store blob.
// The lock is used to prevent concurrent access to the same blob for put
// and remove operations.
type Claimer interface {
	// Claim locks the blob with the given hash.
	Claim(ctx context.Context, hash string) (ClaimExtender, error)
	// Release releases the blob with the given hash.
	Release(ctx context.Context, hash string) error
}

// ClaimExtender is the interface that is used to extend a lock.
type ClaimExtender interface {
	// Extend extends the lock for the given hash.
	Extend(ctx context.Context) error

	// Duration returns the duration of the lock.
	Duration() time.Duration
}

const (
	defaultTempDirectoryName = "tmp"
)

type opType int

const (
	opGet opType = iota
	opList
	opPut
	opRemove
)

type request struct {
	op            opType
	path          string
	reader        io.Reader
	size          int64
	hashValidator hashValidator
	response      chan response
}

type response struct {
	reader       io.ReadCloser
	size         int64
	listMetadata []objectstore.Metadata
	listObjects  []string
	err          error
}

type baseObjectStore struct {
	tomb            tomb.Tomb
	path            string
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

func (t *baseObjectStore) writeToTmpFile(path string, r io.Reader, size int64) (string, func() error, error) {
	// The following dance is to ensure that we don't end up with a partially
	// written file if we crash while writing it or if we're attempting to
	// read it at the same time.
	tmpFile, err := os.CreateTemp(filepath.Join(path, defaultTempDirectoryName), "tmp")
	if err != nil {
		return "", nopCloser, errors.Trace(err)
	}
	defer func() {
		_ = tmpFile.Close()
	}()

	written, err := io.Copy(tmpFile, r)
	if err != nil {
		_ = os.Remove(tmpFile.Name())
		return "", nopCloser, errors.Trace(err)
	}

	// Ensure that we write all the data.
	if written != size {
		_ = os.Remove(tmpFile.Name())
		return "", nopCloser, errors.Errorf("partially written data: written %d, expected %d", written, size)
	}

	return tmpFile.Name(), func() error {
		if _, err := os.Stat(tmpFile.Name()); err == nil {
			return os.Remove(tmpFile.Name())
		}
		return nil
	}, nil
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

func (w *baseObjectStore) ensureDirectories() error {
	if _, err := os.Stat(w.path); err != nil && errors.Is(err, os.ErrNotExist) {
		if err := os.MkdirAll(w.path, 0755); err != nil {
			return errors.Trace(err)
		}
	}

	tmpPath := filepath.Join(w.path, defaultTempDirectoryName)
	if _, err := os.Stat(tmpPath); err != nil && errors.Is(err, os.ErrNotExist) {
		if err := os.MkdirAll(tmpPath, 0755); err != nil {
			return errors.Trace(err)
		}
	}

	return nil
}

// cleanupTmpFiles removes any temporary files that may have been left behind.
// As temporary files are per namespace, we don't have to be concerned about
// removing files that are still being written to by another namespace.
func (w *baseObjectStore) cleanupTmpFiles() error {
	tmpPath := filepath.Join(w.path, defaultTempDirectoryName)

	entries, err := os.ReadDir(tmpPath)
	if err != nil {
		return errors.Trace(err)
	}

	for _, entry := range entries {
		// Ignore directories, this shouldn't happen, but just in case.
		if entry.IsDir() {
			continue
		}

		if err := os.Remove(filepath.Join(tmpPath, entry.Name())); err != nil {
			return errors.Trace(err)
		}
	}

	return nil
}

type hashValidator func(string) (string, bool)

func ignoreHash(string) (string, bool) {
	return "", true
}

func checkHash(expected string) func(string) (string, bool) {
	return func(actual string) (string, bool) {
		return expected, actual == expected
	}
}

func nopCloser() error {
	return nil
}
