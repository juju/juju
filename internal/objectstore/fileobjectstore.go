// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package objectstore

import (
	"context"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/juju/clock"
	"gopkg.in/tomb.v2"

	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/objectstore"
	domainobjectstoreerrors "github.com/juju/juju/domain/objectstore/errors"
	"github.com/juju/juju/internal/errors"
	objectstoreerrors "github.com/juju/juju/internal/objectstore/errors"
)

const (
	defaultFileDirectory = "objectstore"
)

// FallBackStrategy is the strategy to use when there is no local file to
// retrieve.
type FallbackStrategy string

const (
	// RemoteFallback defines that the fallback strategy is to retrieve the
	// file from a remote source. It doesn't guarantee that there will be any
	// remote request, as the controller might not be configured to be in
	// high-availability mode.
	RemoteFallback FallbackStrategy = "remote"

	// NoFallback defines that there is no fallback strategy.
	NoFallback FallbackStrategy = "none"
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
//
// If the object does not exist, an [objectstore.ObjectNotFound]
// error is returned.
func (t *fileObjectStore) Get(ctx context.Context, path string) (io.ReadCloser, int64, error) {
	// Optimistically try to get the file from the file system. If it doesn't
	// exist, then we'll get an error, and we'll try to get it when sequencing
	// the get request with the put and remove requests.
	// Do not attempt to fallback to a remote source, as we're only trying to
	// get the file from the local file system.
	if reader, size, err := t.get(ctx, path, NoFallback); err == nil {
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
			return nil, -1, errors.Errorf("getting blob: %w", resp.err)
		}
		return resp.reader, resp.size, nil
	}
}

// GetBySHA256 returns an io.ReadCloser for any object with the a SHA256
// hash starting with a given prefix, namespaced to the model.
//
// If no object is found, an [objectstore.ObjectNotFound] error is returned.
func (t *fileObjectStore) GetBySHA256(ctx context.Context, sha256 string) (io.ReadCloser, int64, error) {
	// Optimistically try to get the file from the file system. If it doesn't
	// exist, then we'll get an error, and we'll try to get it when sequencing
	// the get request with the put and remove requests.
	if reader, size, err := t.getBySHA256(ctx, sha256); err == nil {
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
		op:       opGetBySHA256,
		sha256:   sha256,
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
			return nil, -1, errors.Errorf("getting blob: %w", resp.err)
		}
		return resp.reader, resp.size, nil
	}
}

// GetBySHA256Prefix returns an io.ReadCloser for any object with the a SHA256
// hash starting with a given prefix, namespaced to the model.
//
// If no object is found, an [objectstore.ObjectNotFound] error is returned.
func (t *fileObjectStore) GetBySHA256Prefix(ctx context.Context, sha256Prefix string) (io.ReadCloser, int64, error) {
	// Optimistically try to get the file from the file system. If it doesn't
	// exist, then we'll get an error, and we'll try to get it when sequencing
	// the get request with the put and remove requests.
	// Do not attempt to fallback to a remote source, as we're only trying to
	// get the file from the local file system.
	if reader, size, err := t.getBySHA256Prefix(ctx, sha256Prefix, NoFallback); err == nil {
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
		op:       opGetBySHA256Prefix,
		sha256:   sha256Prefix,
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
			return nil, -1, errors.Errorf("getting blob: %w", resp.err)
		}
		return resp.reader, resp.size, nil
	}
}

// Put stores data from reader at path, namespaced to the model.
func (t *fileObjectStore) Put(ctx context.Context, path string, r io.Reader, size int64) (objectstore.UUID, error) {
	response := make(chan response)
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case <-t.tomb.Dying():
		return "", tomb.ErrDying
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
		return "", ctx.Err()
	case <-t.tomb.Dying():
		return "", tomb.ErrDying
	case resp := <-response:
		if resp.err != nil {
			return "", errors.Errorf("putting blob: %w", resp.err)
		}
		return resp.uuid, nil
	}
}

// Put stores data from reader at path, namespaced to the model.
// It also ensures the stored data has the correct hash.
func (t *fileObjectStore) PutAndCheckHash(ctx context.Context, path string, r io.Reader, size int64, hash string) (objectstore.UUID, error) {
	response := make(chan response)
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case <-t.tomb.Dying():
		return "", tomb.ErrDying
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
		return "", ctx.Err()
	case <-t.tomb.Dying():
		return "", tomb.ErrDying
	case resp := <-response:
		if resp.err != nil {
			return "", errors.Errorf("putting blob and check hash: %w", resp.err)
		}
		return resp.uuid, nil
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
			return errors.Errorf("removing blob: %w", resp.err)
		}
		return nil
	}
}

func (t *fileObjectStore) loop() error {
	// Ensure the namespace directory exists, along with the tmp directory.
	if err := t.ensureDirectories(); err != nil {
		return errors.Errorf("ensuring file store directories exist: %w", err)
	}

	ctx, cancel := t.scopedContext()
	defer cancel()

	// Remove any temporary files that may have been left behind. We don't
	// provide continuation for these operations, so a retry will be required
	// if the operation fails.
	if err := t.cleanupTmpFiles(ctx); err != nil {
		return errors.Errorf("cleaning up temp files: %w", err)
	}

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
				reader, size, err := t.get(ctx, req.path, RemoteFallback)

				select {
				case <-t.tomb.Dying():
					return tomb.ErrDying

				case req.response <- response{
					reader: reader,
					size:   size,
					err:    err,
				}:
				}

			case opGetBySHA256:
				reader, size, err := t.getBySHA256(ctx, req.sha256)

				select {
				case <-t.tomb.Dying():
					return tomb.ErrDying

				case req.response <- response{
					reader: reader,
					size:   size,
					err:    err,
				}:
				}

			case opGetBySHA256Prefix:
				reader, size, err := t.getBySHA256Prefix(ctx, req.sha256, RemoteFallback)

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
				uuid, err := t.put(ctx, req.path, req.reader, req.size, req.hashValidator)

				select {
				case <-t.tomb.Dying():
					return tomb.ErrDying

				case req.response <- response{
					uuid: uuid,
					err:  err,
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
				return errors.Errorf("unknown request type %d", req.op)
			}

		case <-timer.Chan():

			// Reset the timer, as we've jittered the interval at the start of
			// the loop.
			timer.Reset(defaultPruneInterval)

			if err := t.prune(ctx, t.list, t.deleteObject); err != nil {
				t.logger.Errorf(ctx, "prune: %v", err)
				continue
			}
		}
	}
}

func (t *fileObjectStore) get(ctx context.Context, path string, fallbackStrategy FallbackStrategy) (io.ReadCloser, int64, error) {
	t.logger.Debugf(ctx, "getting object %q from file storage", path)

	metadata, err := t.metadataService.GetMetadata(ctx, path)
	if errors.Is(err, domainobjectstoreerrors.ErrNotFound) {
		return nil, -1, errors.Errorf("get metadata: %w", objectstoreerrors.ObjectNotFound)
	} else if err != nil {
		return nil, -1, errors.Errorf("get metadata: %w", err)
	}

	return t.getWithMetadata(ctx, metadata, fallbackStrategy)
}

func (t *fileObjectStore) getBySHA256(ctx context.Context, sha256 string) (io.ReadCloser, int64, error) {
	t.logger.Debugf(ctx, "getting object with SHA256 %q from file storage", sha256)

	metadata, err := t.metadataService.GetMetadataBySHA256(ctx, sha256)
	if errors.Is(err, domainobjectstoreerrors.ErrNotFound) {
		return nil, -1, errors.Errorf("get metadata by SHA256: %w", objectstoreerrors.ObjectNotFound)
	} else if err != nil {
		return nil, -1, errors.Errorf("get metadata by SHA256: %w", err)
	}

	// Getting a file by SHA256 implies that we're looking for a file that
	// has a specific hash. If we can't find the file, then we should return
	// an error, as we can't fallback to a remote source. This is a hard
	// back stop.
	// If we do want to allow for a remote fallback, we should expose that
	// configuration option all the way to the objectstore interface. For now,
	// this is non-configurable.
	// The added benefit is that this prevents infinite loops, where we keep
	// trying to get the file from the remote source, but it doesn't exist.
	return t.getWithMetadata(ctx, metadata, NoFallback)
}

func (t *fileObjectStore) getBySHA256Prefix(ctx context.Context, sha256 string, fallbackStrategy FallbackStrategy) (io.ReadCloser, int64, error) {
	t.logger.Debugf(ctx, "getting object with SHA256 %q from file storage", sha256)

	metadata, err := t.metadataService.GetMetadataBySHA256Prefix(ctx, sha256)
	if errors.Is(err, domainobjectstoreerrors.ErrNotFound) {
		return nil, -1, errors.Errorf("get metadata by SHA256 prefix: %w", objectstoreerrors.ObjectNotFound)
	} else if err != nil {
		return nil, -1, errors.Errorf("get metadata by SHA256 prefix: %w", err)
	}

	return t.getWithMetadata(ctx, metadata, fallbackStrategy)
}

func (t *fileObjectStore) getWithMetadata(ctx context.Context, metadata objectstore.Metadata, fallbackStrategy FallbackStrategy) (io.ReadCloser, int64, error) {
	hash := selectFileHash(metadata)

	file, err := t.fs.Open(hash)
	if errors.Is(err, os.ErrNotExist) {
		if fallbackStrategy == RemoteFallback {
			// TODO (stickupkid): Implement remote fallback.
		}
		return nil, -1, objectstoreerrors.ObjectNotFound
	} else if err != nil {
		return nil, -1, errors.Errorf("opening file %q encoded as %q: %w", metadata.Path, hash, err)
	}

	// Verify that the size of the file matches the expected size.
	// This is a sanity check, that the underlying file hasn't changed.
	stat, err := file.Stat()
	if err != nil {
		return nil, -1, errors.Errorf("retrieving size: file %q encoded as %q: %w", metadata.Path, hash, err)
	}

	size := stat.Size()
	if metadata.Size != size {
		return nil, -1, errors.Errorf("size mismatch for %q: expected %d, got %d", metadata.Path, metadata.Size, size)
	}

	return file, size, nil
}

func (t *fileObjectStore) put(ctx context.Context, path string, r io.Reader, size int64, validator hashValidator) (objectstore.UUID, error) {
	t.logger.Debugf(ctx, "putting object %q to file storage", path)

	// Charms and resources are coded to use the SHA384 hash. It is possible
	// to move to the more common SHA256 hash, but that would require a
	// migration of all charms and resources during import.
	// I can only assume 384 was chosen over 256 and others, is because it's
	// not susceptible to length extension attacks? In any case, we'll
	// keep using it for now.
	hash384 := sha512.New384()

	// We need two hash sets here, because juju wants to use SHA384, but s3
	// and http handlers want to use SHA256. We can't change the hash used by
	// default to SHA256. Luckily, we can piggyback on the writing to a tmp
	// file and create TeeReader with a MultiWriter.
	hash256 := sha256.New()

	// We need to write this to a temp file, because if the client retries
	// then we need seek back to the beginning of the file.
	tmpFileName, tmpFileCleanup, err := t.writeToTmpFile(t.path, io.TeeReader(r, io.MultiWriter(hash384, hash256)), size)
	if err != nil {
		return "", errors.Capture(err)
	}

	// Ensure that we remove the temporary file if we fail to persist it.
	defer func() { _ = tmpFileCleanup() }()

	// Encode the hashes as strings, so we can use them for file and http lookups.
	encoded384 := hex.EncodeToString(hash384.Sum(nil))
	encoded256 := hex.EncodeToString(hash256.Sum(nil))

	// Ensure that the hash of the file matches the expected hash.
	if expected, ok := validator(encoded384); !ok {
		return "", errors.Errorf("hash mismatch for %q: expected %q, got %q: %w", path, expected, encoded384, objectstore.ErrHashMismatch)
	}

	// Lock the file with the given hash, so that we can't remove the file
	// while we're writing it.
	var uuid objectstore.UUID
	if err := t.withLock(ctx, encoded384, func(ctx context.Context) error {
		// Persist the temporary file to the final location.
		if err := t.persistTmpFile(ctx, tmpFileName, encoded384, size); err != nil {
			return errors.Capture(err)
		}

		// Save the metadata for the file after we've written it. That way we
		// correctly sequence the watch events. Otherwise there is a potential
		// race where the watch event is emitted before the file is written.
		var err error
		if uuid, err = t.metadataService.PutMetadata(ctx, objectstore.Metadata{
			Path:   path,
			SHA256: encoded256,
			SHA384: encoded384,
			Size:   size,
		}); err != nil {
			return errors.Capture(err)
		}
		return nil
	}); err != nil {
		return "", errors.Capture(err)
	}
	return uuid, nil
}

func (t *fileObjectStore) persistTmpFile(_ context.Context, tmpFileName, hash string, size int64) error {
	filePath := t.filePath(hash)

	// Check to see if the file already exists with the same name.
	if info, err := os.Stat(filePath); err == nil {
		// If the file on disk isn't the same as the one we're trying to write,
		// then we have a problem.
		if info.Size() != size {
			return errors.Errorf("encoded as %q: %w", hash, objectstoreerrors.ObjectAlreadyExists)
		}
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		// There is an error attempting to stat the file, and it's not because
		// the file doesn't exist.
		return errors.Capture(err)
	}

	// Swap out the temporary file for the real one.
	if err := os.Rename(tmpFileName, filePath); err != nil {
		return errors.Capture(err)
	}

	return nil
}

func (t *fileObjectStore) remove(ctx context.Context, path string) error {
	t.logger.Debugf(ctx, "removing object %q from file storage", path)

	metadata, err := t.metadataService.GetMetadata(ctx, path)
	if err != nil {
		return errors.Errorf("get metadata: %w", err)
	}

	hash := selectFileHash(metadata)
	return t.withLock(ctx, hash, func(ctx context.Context) error {
		if err := t.metadataService.RemoveMetadata(ctx, path); err != nil {
			return errors.Errorf("remove metadata: %w", err)
		}
		return t.deleteObject(ctx, hash)
	})
}

func (t *fileObjectStore) filePath(hash string) string {
	return filepath.Join(t.path, hash)
}

func (t *fileObjectStore) list(ctx context.Context) ([]objectstore.Metadata, []string, error) {
	t.logger.Debugf(ctx, "listing objects from file storage")

	metadata, err := t.metadataService.ListMetadata(ctx)
	if err != nil {
		return nil, nil, errors.Errorf("list metadata: %w", err)
	}

	// List all the files in the directory.
	entries, err := fs.ReadDir(t.fs, ".")
	if err != nil {
		return nil, nil, errors.Errorf("reading directory: %w", err)
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
		t.logger.Debugf(ctx, "file %q doesn't exist, nothing to do", filePath)
		return nil
	}

	// If we fail to remove the file, we don't want to return an error, as
	// the metadata has already been removed. Manual intervention will be
	// required to remove the file. We may in the future want to prune the
	// file store of files that are no longer referenced by metadata.
	if err := os.Remove(filePath); err != nil {
		t.logger.Errorf(ctx, "failed to remove file %q: %v", filePath, err)
	}
	return nil
}

// basePath returns the base path for the file object store.
// typically: /var/lib/juju/objectstore/<namespace>
func basePath(rootDir, namespace string) string {
	return filepath.Join(rootDir, defaultFileDirectory, namespace)
}
