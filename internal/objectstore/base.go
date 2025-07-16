// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package objectstore

import (
	"context"
	"io"
	"math/rand"
	"os"
	"path/filepath"
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"

	"github.com/juju/juju/core/lease"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/objectstore"
)

const (
	// ErrFileLocked is returned when a file is locked.
	ErrFileLocked = errors.ConstError("file locked")
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
	opGetBySHA256
	opGetBySHA256Prefix
	opPut
	opRemove
)

type request struct {
	op            opType
	path          string
	sha256        string
	reader        io.Reader
	size          int64
	hashValidator hashValidator
	response      chan response
}

type response struct {
	reader io.ReadCloser
	size   int64
	uuid   objectstore.UUID
	err    error
}

type baseObjectStore struct {
	path            string
	metadataService objectstore.ObjectStoreMetadata
	claimer         Claimer
	logger          logger.Logger
	clock           clock.Clock
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
		if errors.Is(err, lease.ErrClaimDenied) {
			return ErrFileLocked
		}
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

	// Start a goroutine to extend the lock while the function is running.
	// If we fail to extend the lock, then we cancel the context. This will
	// cause the function to witness a context cancellation.
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-w.clock.After(extender.Duration()):
				// Attempt to extend the lock if the function is still running.
				if err := extender.Extend(ctx); err != nil {
					cancel()

					w.logger.Infof(ctx, "failed to extend lock for %q: %v", hash, err)
					return
				}
			}
		}
	}()

	return errors.Trace(f(runnerCtx))
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
func (w *baseObjectStore) cleanupTmpFiles(ctx context.Context) error {
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

		file := filepath.Join(tmpPath, entry.Name())
		if err := os.Remove(file); err != nil {
			w.logger.Infof(ctx, "failed to remove tmp file %s, will retry later", file)
			continue
		}
	}
	return nil
}

// Define the functions that are used to prune the object store.
type (
	// pruneListFunc is the function that is used to list the objects in the
	// object store. This includes the metadata and the objects themselves.
	pruneListFunc func(ctx context.Context) ([]objectstore.Metadata, []string, error)
	// pruneDeleteFunc is the function that is used to delete an object from
	// the object store.
	pruneDeleteFunc func(ctx context.Context, hash string) error
)

// prune is used to prune any potential stale objects from the object store.
func (w *baseObjectStore) prune(ctx context.Context, list pruneListFunc, delete pruneDeleteFunc) error {
	w.logger.Debugf(ctx, "pruning objects from storage")

	metadata, objects, err := list(ctx)
	if err != nil {
		return errors.Trace(err)
	}
	// Create a map of all the hashes that we know about in the metadata
	// database.
	hashes := make(map[string]struct{})
	for _, m := range metadata {
		hashes[selectFileHash(m)] = struct{}{}
	}

	// Remove any objects that we don't know about.
	for _, object := range objects {
		if _, ok := hashes[object]; ok {
			w.logger.Tracef(ctx, "object %q is referenced", object)
			continue
		}

		w.logger.Debugf(ctx, "attempting to remove unreferenced object %q", object)

		// Attempt to acquire a lock on the object. If we can't acquire
		// the lock, then we'll try again later.
		if err := w.withLock(ctx, object, func(ctx context.Context) error {
			return errors.Trace(delete(ctx, object))
		}); err != nil {
			w.logger.Infof(ctx, "failed to remove unreferenced object %q: %v, will try again later", object, err)
			continue
		}

		w.logger.Debugf(ctx, "removed unreferenced object %q", object)
	}

	return nil
}

// selectFileHash returns the hash that is used to identify the file.
// The file hash is actually the hash of the file itself and is used by Juju
// as the default hash.
// Do not change this function without understanding the implications.
func selectFileHash(m objectstore.Metadata) string {
	return m.SHA384
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

// jitter returns a random duration between 0.5 and 1 times the given duration.
func jitter(d time.Duration) time.Duration {
	h := float64(d) * 0.5
	r := rand.Float64() * h
	return time.Duration(r + h)
}
