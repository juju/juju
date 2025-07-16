// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package objectstoredrainer

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"

	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/catacomb"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/worker/fortress"
)

// SelectFileHashFunc is a function that selects the file hash from the
// metadata.
type SelectFileHashFunc func(objectstore.Metadata) string

// HashFileSystemAccessor is the interface for reading and deleting files from
// the file system.
// The file system accessor is used for draining files from the file backed
// object store to the s3 object store. It should at no point be used for
// writing files to the file system.
type HashFileSystemAccessor interface {
	// HashExists checks if the file exists in the file backed object store.
	// Returns a NotFound error if the file doesn't exist.
	HashExists(ctx context.Context, hash string) error

	// GetByHash returns an io.ReadCloser for the file at the given hash.
	GetByHash(ctx context.Context, hash string) (io.ReadCloser, int64, error)

	// DeleteByHash deletes the file at the given hash.
	DeleteByHash(ctx context.Context, hash string) error
}

// ObjectStoreService provides access to the object store for draining
// operations.
type ObjectStoreService interface {
	// ListMetadata returns the persistence metadata.
	ListMetadata(ctx context.Context) ([]objectstore.Metadata, error)
	// GetDrainingPhase returns the current active draining phase of the
	// object store.
	GetDrainingPhase(ctx context.Context) (objectstore.Phase, error)
	// WatchDraining returns a watcher that watches the draining phase of the
	// object store.
	WatchDraining(ctx context.Context) (watcher.Watcher[struct{}], error)
}

// Config holds the dependencies and configuration for a Worker.
type Config struct {
	Guard                  fortress.Guard
	ObjectStoreService     ObjectStoreService
	HashFileSystemAccessor HashFileSystemAccessor
	S3Client               objectstore.Client
	SelectFileHash         SelectFileHashFunc
	Logger                 logger.Logger
}

// Validate returns an error if the config cannot be expected to
// drive a functional Worker.
func (config Config) Validate() error {
	if config.Guard == nil {
		return errors.Errorf("nil Guard").Add(coreerrors.NotValid)
	}
	if config.ObjectStoreService == nil {
		return errors.Errorf("nil ObjectStoreService").Add(coreerrors.NotValid)
	}
	if config.HashFileSystemAccessor == nil {
		return errors.Errorf("nil HashFileSystemAccessor").Add(coreerrors.NotValid)
	}
	if config.S3Client == nil {
		return errors.Errorf("nil S3Client").Add(coreerrors.NotValid)
	}
	if config.SelectFileHash == nil {
		return errors.Errorf("nil SelectFileHash").Add(coreerrors.NotValid)
	}
	if config.Logger == nil {
		return errors.Errorf("nil Logger").Add(coreerrors.NotValid)
	}
	return nil
}

// NewWorker returns a Worker that tracks the result of the configured.
func NewWorker(ctx context.Context, config Config) (worker.Worker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Capture(err)
	}

	w := &Worker{
		guard: config.Guard,

		metadataService: config.ObjectStoreService,
		fileSystem:      config.HashFileSystemAccessor,
		client:          config.S3Client,

		selectFileHash: config.SelectFileHash,

		logger: config.Logger,
	}

	if err := catacomb.Invoke(catacomb.Plan{
		Name: "objectstoredrainer",
		Site: &w.catacomb,
		Work: w.loop,
	}); err != nil {
		return nil, errors.Capture(err)
	}
	return w, nil
}

// Worker watches the object store service for changes to the draining
// phase. If the phase is draining, it locks the guard. If the phase is not
// draining, it unlocks the guard.
// The worker will manage the lifecycle of the watcher and will stop
// watching when the worker is killed or when the context is cancelled.
type Worker struct {
	catacomb catacomb.Catacomb

	guard fortress.Guard

	metadataService ObjectStoreService
	fileSystem      HashFileSystemAccessor
	client          objectstore.Client

	selectFileHash SelectFileHashFunc

	logger logger.Logger
}

// Kill kills the worker. It will cause the worker to stop if it is
// not already stopped. The worker will transition to the dying state.
func (w *Worker) Kill() {
	w.catacomb.Kill(nil)
}

// Wait waits for the worker to finish. It will cause the worker to
// stop if it is not already stopped. It will return an error if the
// worker was killed with an error.
func (w *Worker) Wait() error {
	return w.catacomb.Wait()
}

func (w *Worker) loop() error {
	ctx := w.catacomb.Context(context.Background())

	watcher, err := w.metadataService.WatchDraining(ctx)
	if err != nil {
		return errors.Capture(err)
	}
	if err := w.catacomb.Add(watcher); err != nil {
		return errors.Capture(err)
	}

	for {
		select {
		case <-w.catacomb.Dying():
			return w.catacomb.ErrDying()
		case <-watcher.Changes():
			phase, err := w.metadataService.GetDrainingPhase(ctx)
			if err != nil {
				return errors.Capture(err)
			}

			// We're not draining, so we can unlock the guard and wait
			// for the next change.
			if !phase.IsDraining() {
				w.logger.Infof(ctx, "object store is not draining, unlocking guard")

				if err := w.guard.Unlock(ctx); err != nil {
					return errors.Errorf("failed to update guard: %v", err)
				}
				continue
			}

			w.logger.Infof(ctx, "object store is draining, locking guard")

			if err := w.guard.Lockdown(ctx); err != nil {
				return errors.Errorf("failed to update guard: %v", err)
			}

			// TODO (stickupkid): Support draining from one s3 object store to
			// another. For now, we just log that we're in the draining phase
			// from file to s3.

			if err := w.drainFiles(ctx); err != nil {
				return errors.Errorf("failed to drain files: %v", err)
			}
		}
	}
}

func (w *Worker) drainFiles(ctx context.Context) error {
	// Drain any files from the file object store to the s3 object store.
	// This will locate any files from the metadata service that are not
	// present in the s3 object store and copy them over.
	metadata, err := w.metadataService.ListMetadata(ctx)
	if err != nil {
		return errors.Errorf("listing metadata for draining: %w", err)
	}

	for _, m := range metadata {
		hash := w.selectFileHash(m)

		if err := w.drainFile(ctx, m.Path, hash, m.Size); err != nil {
			// This will crash the s3ObjectStore worker if this is a fatal
			// error. We don't want to continue processing if we can't drain
			// the files to the s3 object store.

			return errors.Errorf("draining file %q to s3 object store: %w", m.Path, err)
		}
	}

	return nil
}

func (w *Worker) drainFile(ctx context.Context, path, hash string, metadataSize int64) error {
	// If the file isn't on the file backed object store, then we can skip it.
	// It's expected that this has already been drained to the s3 object store.
	if err := w.fileSystem.HashExists(ctx, hash); errors.Is(err, coreerrors.NotFound) {
		return nil
	} else if err != nil {
		return errors.Errorf("checking if file %q exists in file object store: %w", path, err)
	}

	// If the file is already in the s3 object store, then we can skip it.
	// Note: we want to check the s3 object store each request, just in
	// case the file was added to the s3 object store while we were
	// draining the files.
	if err := w.objectAlreadyExists(ctx, hash); err != nil && !errors.Is(err, coreerrors.NotFound) {
		return errors.Errorf("checking if file %q exists in s3 object store: %w", path, err)
	} else if err == nil {
		// File already contains the hash, so we can skip it.
		w.logger.Tracef(ctx, "file %q already exists in s3 object store, skipping", path)
		return nil
	}

	w.logger.Debugf(ctx, "draining file %q to s3 object store", path)

	// Grab the file from the file backed object store and drain it to the
	// s3 object store.
	reader, fileSize, err := w.fileSystem.GetByHash(ctx, hash)
	if err != nil {
		// The file doesn't exist in the file backed object store, but also
		// doesn't exist in the s3 object store. This is a problem, so we
		// should skip it.
		if errors.Is(err, coreerrors.NotFound) {
			w.logger.Warningf(ctx, "file %q doesn't exist in file object store, unable to drain", path)
			return nil
		}
		return errors.Errorf("getting file %q from file object store: %w", path, err)
	}

	// Ensure we close the reader when we're done.
	defer reader.Close()

	// If the file size doesn't match the metadata size, then the file is
	// potentially corrupt, so we should skip it.
	if fileSize != metadataSize {
		w.logger.Warningf(ctx, "file %q has a size mismatch, unable to drain", path)
		return nil
	}

	// We need to compute the sha256 hash here, juju by default uses SHA384,
	// but s3 defaults to SHA256.
	// If the reader is a Seeker, then we can seek back to the beginning of
	// the file, so that we can read it again.
	s3Reader, s3EncodedHash, err := w.computeS3Hash(reader)
	if err != nil {
		return errors.Capture(err)
	}

	// We can drain the file to the s3 object store.
	err = w.client.Session(ctx, func(ctx context.Context, s objectstore.Session) error {
		err := s.PutObject(ctx, w.rootBucket, w.filePath(hash), s3Reader, s3EncodedHash)
		if err != nil {
			return errors.Errorf("putting file %q to s3 object store: %w", path, err)
		}
		return nil
	})
	if errors.Is(err, coreerrors.AlreadyExists) {
		// File already contains the hash, so we can skip it.
		return w.removeDrainedFile(ctx, hash)
	} else if err != nil {
		return errors.Capture(err)
	}

	// We can remove the file from the file backed object store, because it
	// has been successfully drained to the s3 object store.
	if err := w.removeDrainedFile(ctx, hash); err != nil {
		return errors.Capture(err)
	}

	return nil
}

func (w *Worker) computeS3Hash(reader io.Reader) (io.Reader, string, error) {
	s3Hash := sha256.New()

	// This is an optimization for the case where the reader is a Seeker. We
	// can seek back to the beginning of the file, so that we can read it
	// again, without having to copy the entire file into memory.
	if seekReader, ok := reader.(io.Seeker); ok {
		if _, err := io.Copy(s3Hash, reader); err != nil {
			return nil, "", errors.Errorf("computing hash: %w", err)
		}

		if _, err := seekReader.Seek(0, io.SeekStart); err != nil {
			return nil, "", errors.Errorf("seeking back to start: %w", err)
		}

		return reader, base64.StdEncoding.EncodeToString(s3Hash.Sum(nil)), nil
	}

	// If the reader is not a Seeker, then we need to copy the entire file
	// into memory, so that we can compute the hash.
	memReader := new(bytes.Buffer)
	if _, err := io.Copy(io.MultiWriter(s3Hash, memReader), reader); err != nil {
		return nil, "", errors.Errorf("computing hash: %w", err)
	}

	return memReader, base64.StdEncoding.EncodeToString(s3Hash.Sum(nil)), nil
}

func (w *Worker) objectAlreadyExists(ctx context.Context, hash string) error {
	if err := w.client.Session(ctx, func(ctx context.Context, s objectstore.Session) error {
		err := s.ObjectExists(ctx, w.rootBucket, w.filePath(hash))
		return errors.Capture(err)
	}); err != nil {
		return errors.Errorf("checking if file %q exists in s3 object store: %w", hash, err)
	}
	return nil
}

func (w *Worker) removeDrainedFile(ctx context.Context, hash string) error {
	if err := w.fileSystem.DeleteByHash(ctx, hash); err != nil {
		// If we're unable to remove the file from the file backed object
		// store, then we should log a warning, but continue processing.
		// This is not a terminal case, we can continue processing.
		w.logger.Warningf(ctx, "unable to remove file %q from file object store: %v", hash, err)
		return nil
	}
	return nil
}

func (w *Worker) filePath(hash string) string {
	return fmt.Sprintf("%s/%s", w.namespace, hash)
}
