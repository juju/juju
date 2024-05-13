// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package objectstore

import (
	"context"
	"crypto/sha512"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"gopkg.in/tomb.v2"

	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/objectstore"
)

const (
	defaultFileDirectory = "objectstore"
)

// FileObjectStoreConfig is the configuration for the file object store.
type FileObjectStoreConfig struct {
	// RootDir is the root directory for the file object store.
	RootDir string
	// Namespace is the namespace for the file object store (typically the
	// model UUID)
	Namespace string
	// MetadataService is the metadata service for translating paths to
	// hashes.
	MetadataService objectstore.ObjectStoreMetadata
	// Claimer is the claimer for the file object store.
	Claimer Claimer

	Logger logger.Logger
	Clock  clock.Clock
}

type fileObjectStore struct {
	baseObjectStore
	fs        fs.FS
	namespace string
	requests  chan request
}

// NewFileObjectStore returns a new object store worker based on the file
// storage.
func NewFileObjectStore(cfg FileObjectStoreConfig) (TrackedObjectStore, error) {
	path := basePath(cfg.RootDir, cfg.Namespace)

	s := &fileObjectStore{
		baseObjectStore: baseObjectStore{
			path:            path,
			claimer:         cfg.Claimer,
			metadataService: cfg.MetadataService,
			logger:          cfg.Logger,
			clock:           cfg.Clock,
		},
		fs:        os.DirFS(path),
		namespace: cfg.Namespace,

		requests: make(chan request),
	}

	s.tomb.Go(s.loop)

	return s, nil
}

// Get returns an io.ReadCloser for data at path, namespaced to the
// model.
func (t *fileObjectStore) Get(ctx context.Context, path string) (io.ReadCloser, int64, error) {
	// Optimistically try to get the file from the file system. If it doesn't
	// exist, then we'll get an error, and we'll try to get it when sequencing
	// the get request with the put and remove requests.
	if reader, size, err := t.get(ctx, path); err == nil {
		return reader, size, nil
	}

	// Sequence the get request with the put and remove requests.
	response := make(chan response)
	select {
	case <-ctx.Done():
		return nil, -1, ctx.Err()
	case <-t.tomb.Dying():
		return nil, -1, tomb.ErrDying
	case t.requests <- request{
		op:       opGet,
		path:     path,
		response: response,
	}:
	}

	select {
	case <-ctx.Done():
		return nil, -1, ctx.Err()
	case <-t.tomb.Dying():
		return nil, -1, tomb.ErrDying
	case resp := <-response:
		if resp.err != nil {
			if errors.Is(resp.err, os.ErrNotExist) {
				return nil, -1, fmt.Errorf("getting blob: %w", errors.NotFoundf("path %q", path))
			}
			return nil, -1, fmt.Errorf("getting blob: %w", resp.err)
		}
		return resp.reader, resp.size, nil
	}
}

// Put stores data from reader at path, namespaced to the model.
func (t *fileObjectStore) Put(ctx context.Context, path string, r io.Reader, size int64) error {
	response := make(chan response)
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.tomb.Dying():
		return tomb.ErrDying
	case t.requests <- request{
		op:            opPut,
		path:          path,
		reader:        r,
		size:          size,
		hashValidator: ignoreHash,
		response:      response,
	}:
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.tomb.Dying():
		return tomb.ErrDying
	case resp := <-response:
		if resp.err != nil {
			return fmt.Errorf("putting blob: %w", resp.err)
		}
		return nil
	}
}

// Put stores data from reader at path, namespaced to the model.
// It also ensures the stored data has the correct hash.
func (t *fileObjectStore) PutAndCheckHash(ctx context.Context, path string, r io.Reader, size int64, hash string) error {
	response := make(chan response)
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.tomb.Dying():
		return tomb.ErrDying
	case t.requests <- request{
		op:            opPut,
		path:          path,
		reader:        r,
		size:          size,
		hashValidator: checkHash(hash),
		response:      response,
	}:
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.tomb.Dying():
		return tomb.ErrDying
	case resp := <-response:
		if resp.err != nil {
			return fmt.Errorf("putting blob and check hash: %w", resp.err)
		}
		return nil
	}
}

// Remove removes data at path, namespaced to the model.
func (t *fileObjectStore) Remove(ctx context.Context, path string) error {
	response := make(chan response)
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.tomb.Dying():
		return tomb.ErrDying
	case t.requests <- request{
		op:       opRemove,
		path:     path,
		response: response,
	}:
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.tomb.Dying():
		return tomb.ErrDying
	case resp := <-response:
		if resp.err != nil {
			return fmt.Errorf("removing blob: %w", resp.err)
		}
		return nil
	}
}

func (t *fileObjectStore) loop() error {
	// Ensure the namespace directory exists, along with the tmp directory.
	if err := t.ensureDirectories(); err != nil {
		return errors.Annotatef(err, "ensuring file store directories exist")
	}

	// Remove any temporary files that may have been left behind. We don't
	// provide continuation for these operations, so a retry will be required
	// if the operation fails.
	if err := t.cleanupTmpFiles(); err != nil {
		return errors.Annotatef(err, "cleaning up temp files")
	}

	ctx, cancel := t.scopedContext()
	defer cancel()

	timer := t.clock.NewTimer(jitter(defaultPruneInterval))
	defer timer.Stop()

	// Sequence the get request with the put, remove requests.
	for {
		select {
		case <-t.tomb.Dying():
			return tomb.ErrDying

		case req := <-t.requests:
			switch req.op {
			case opGet:
				reader, size, err := t.get(ctx, req.path)

				select {
				case <-t.tomb.Dying():
					return tomb.ErrDying

				case req.response <- response{
					reader: reader,
					size:   size,
					err:    err,
				}:
				}

			case opPut:
				select {
				case <-t.tomb.Dying():
					return tomb.ErrDying

				case req.response <- response{
					err: t.put(ctx, req.path, req.reader, req.size, req.hashValidator),
				}:
				}

			case opRemove:
				select {
				case <-t.tomb.Dying():
					return tomb.ErrDying

				case req.response <- response{
					err: t.remove(ctx, req.path),
				}:
				}

			default:
				return fmt.Errorf("unknown request type %d", req.op)
			}

		case <-timer.Chan():

			// Reset the timer, as we've jittered the interval at the start of
			// the loop.
			timer.Reset(defaultPruneInterval)

			if err := t.prune(ctx, t.list, t.deleteObject); err != nil {
				t.logger.Errorf("prune: %v", err)
				continue
			}
		}
	}
}

func (t *fileObjectStore) get(ctx context.Context, path string) (io.ReadCloser, int64, error) {
	t.logger.Debugf("getting object %q from file storage", path)

	metadata, err := t.metadataService.GetMetadata(ctx, path)
	if err != nil {
		return nil, -1, fmt.Errorf("get metadata: %w", err)
	}

	file, err := t.fs.Open(metadata.Hash)
	if err != nil {
		return nil, -1, fmt.Errorf("opening file %q encoded as %q: %w", path, metadata.Hash, err)
	}

	// Verify that the size of the file matches the expected size.
	// This is a sanity check, that the underlying file hasn't changed.
	stat, err := file.Stat()
	if err != nil {
		return nil, -1, fmt.Errorf("retrieving size: file %q encoded as %q: %w", path, metadata.Hash, err)
	}

	size := stat.Size()
	if metadata.Size != size {
		return nil, -1, fmt.Errorf("size mismatch for %q: expected %d, got %d", path, metadata.Size, size)
	}

	return file, size, nil
}

func (t *fileObjectStore) put(ctx context.Context, path string, r io.Reader, size int64, validator hashValidator) error {
	t.logger.Debugf("putting object %q to file storage", path)

	// Charms and resources are coded to use the SHA384 hash. It is possible
	// to move to the more common SHA256 hash, but that would require a
	// migration of all charms and resources during import.
	// I can only assume 384 was chosen over 256 and others, is because it's
	// not susceptible to length extension attacks? In any case, we'll
	// keep using it for now.
	hasher := sha512.New384()

	tmpFileName, tmpFileCleanup, err := t.writeToTmpFile(t.path, io.TeeReader(r, hasher), size)
	if err != nil {
		return errors.Trace(err)
	}

	// Ensure that we remove the temporary file if we fail to persist it.
	defer func() { _ = tmpFileCleanup() }()

	// Encode the hash as a hex string. This is what the rest of the juju
	// codebase expects. Although we should probably use base64.StdEncoding
	// instead.
	hash := hex.EncodeToString(hasher.Sum(nil))

	// Ensure that the hash of the file matches the expected hash.
	if expected, ok := validator(hash); !ok {
		return fmt.Errorf("hash mismatch for %q: expected %q, got %q: %w", path, expected, hash, objectstore.ErrHashMismatch)
	}

	// Lock the file with the given hash, so that we can't remove the file
	// while we're writing it.
	return t.withLock(ctx, hash, func(ctx context.Context) error {
		// Persist the temporary file to the final location.
		if err := t.persistTmpFile(ctx, tmpFileName, hash, size); err != nil {
			return errors.Trace(err)
		}

		// Save the metadata for the file after we've written it. That way we
		// correctly sequence the watch events. Otherwise there is a potential
		// race where the watch event is emitted before the file is written.
		if err := t.metadataService.PutMetadata(ctx, objectstore.Metadata{
			Path: path,
			Hash: hash,
			Size: size,
		}); err != nil {
			return errors.Trace(err)
		}
		return nil
	})
}

func (t *fileObjectStore) persistTmpFile(ctx context.Context, tmpFileName, hash string, size int64) error {
	filePath := t.filePath(hash)

	// Check to see if the file already exists with the same name.
	if info, err := os.Stat(filePath); err == nil {
		// If the file on disk isn't the same as the one we're trying to write,
		// then we have a problem.
		if info.Size() != size {
			return errors.AlreadyExistsf("encoded as %q", filePath)
		}
		return nil
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		// There is an error attempting to stat the file, and it's not because
		// the file doesn't exist.
		return errors.Trace(err)
	}

	// Swap out the temporary file for the real one.
	if err := os.Rename(tmpFileName, filePath); err != nil {
		return errors.Trace(err)
	}

	return nil
}

func (t *fileObjectStore) remove(ctx context.Context, path string) error {
	t.logger.Debugf("removing object %q from file storage", path)

	metadata, err := t.metadataService.GetMetadata(ctx, path)
	if err != nil {
		return fmt.Errorf("get metadata: %w", err)
	}

	hash := metadata.Hash
	return t.withLock(ctx, hash, func(ctx context.Context) error {
		if err := t.metadataService.RemoveMetadata(ctx, path); err != nil {
			return fmt.Errorf("remove metadata: %w", err)
		}
		return t.deleteObject(ctx, hash)
	})
}

func (t *fileObjectStore) filePath(hash string) string {
	return filepath.Join(t.path, hash)
}

func (t *fileObjectStore) list(ctx context.Context) ([]objectstore.Metadata, []string, error) {
	t.logger.Debugf("listing objects from file storage")

	metadata, err := t.metadataService.ListMetadata(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("list metadata: %w", err)
	}

	// List all the files in the directory.
	entries, err := fs.ReadDir(t.fs, ".")
	if err != nil {
		return nil, nil, fmt.Errorf("reading directory: %w", err)
	}

	// Filter out any directories.
	var files []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		files = append(files, entry.Name())
	}

	return metadata, files, nil
}

func (t *fileObjectStore) deleteObject(ctx context.Context, hash string) error {
	filePath := t.filePath(hash)

	// File doesn't exist. It was probably already removed. Return early,
	// nothing we can do in this case.
	if _, err := os.Stat(filePath); err != nil && errors.Is(err, os.ErrNotExist) {
		t.logger.Debugf("file %q doesn't exist, nothing to do", filePath)
		return nil
	}

	// If we fail to remove the file, we don't want to return an error, as
	// the metadata has already been removed. Manual intervention will be
	// required to remove the file. We may in the future want to prune the
	// file store of files that are no longer referenced by metadata.
	if err := os.Remove(filePath); err != nil {
		t.logger.Errorf("failed to remove file %q: %v", filePath, err)
	}
	return nil
}

// basePath returns the base path for the file object store.
// typically: /var/lib/juju/objectstore/<namespace>
func basePath(rootDir, namespace string) string {
	return filepath.Join(rootDir, defaultFileDirectory, namespace)
}
