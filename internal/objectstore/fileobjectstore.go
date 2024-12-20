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
	"time"

	"github.com/juju/clock"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/catacomb"

	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/objectstore"
	domainobjectstoreerrors "github.com/juju/juju/domain/objectstore/errors"
	"github.com/juju/juju/internal/errors"
	objectstoreerrors "github.com/juju/juju/internal/objectstore/errors"
	"github.com/juju/juju/internal/objectstore/remote"
)

const (
	defaultFileDirectory = "objectstore"

	// defaultRemoteTimeout is the default timeout for retrieving blobs from
	// remote API servers.
	defaultRemoteTimeout = time.Second * 30
)

// BlobRetriever is the interface for retrieving blobs from remote API servers.
type BlobRetriever interface {
	worker.Worker
	// GetBySHA256 returns a reader for the blob with the given SHA256.
	RetrieveBlobFromRemote(ctx context.Context, sha256 string) (io.ReadCloser, int64, error)
}

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

	// BlobRetriever is the blob retriever for retrieving blobs from remote
	// API servers. This is useful if the current object store doesn't have
	// the blob.
	BlobRetriever BlobRetriever

	// Logger is the logger for the file object store.
	Logger logger.Logger

	// Clock is the clock for the file object store.
	Clock clock.Clock
}

type fileObjectStore struct {
	baseObjectStore
	fs        fs.FS
	namespace string
	requests  chan request

	// progressMarkers is a map of files that have been marked for download, but
	// haven't been downloaded yet, but are in the process of being downloaded.
	// This will prevent Remote API requests from being made for the same file
	// multiple times.
	progressMarkers map[string]struct{}
	blobRetriever   BlobRetriever
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
		fs:              os.DirFS(path),
		namespace:       cfg.Namespace,
		blobRetriever:   cfg.BlobRetriever,
		progressMarkers: make(map[string]struct{}),

		requests: make(chan request),
	}

	if err := catacomb.Invoke(catacomb.Plan{
		Site: &s.catacomb,
		Work: s.loop,
		Init: []worker.Worker{
			cfg.BlobRetriever,
		},
	}); err != nil {
		return nil, errors.Errorf("starting file object store: %w", err)
	}

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
	if reader, size, err := t.get(ctx, path, noFallbackStrategy); err == nil {
		return reader, size, nil
	}

	// Sequence the get request with the put and remove requests.
	response := make(chan response)
	select {
	case <-ctx.Done():
		return nil, -1, ctx.Err()
	case <-t.catacomb.Dying():
		return nil, -1, t.catacomb.ErrDying()
	case t.requests <- request{
		op:       opGet,
		path:     path,
		response: response,
	}:
	}

	select {
	case <-ctx.Done():
		return nil, -1, ctx.Err()
	case <-t.catacomb.Dying():
		return nil, -1, t.catacomb.ErrDying()
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
	if reader, size, err := t.getBySHA256Prefix(ctx, sha256Prefix, noFallbackStrategy); err == nil {
		return reader, size, nil
	}

	// Sequence the get request with the put and remove requests.
	response := make(chan response)
	select {
	case <-ctx.Done():
		return nil, -1, ctx.Err()
	case <-t.catacomb.Dying():
		return nil, -1, t.catacomb.ErrDying()
	case t.requests <- request{
		op:       opGetBySHA256Prefix,
		sha256:   sha256Prefix,
		response: response,
	}:
	}

	select {
	case <-ctx.Done():
		return nil, -1, ctx.Err()
	case <-t.catacomb.Dying():
		return nil, -1, t.catacomb.ErrDying()
	case resp := <-response:
		if resp.err != nil {
			return nil, -1, errors.Errorf("getting blob: %w", resp.err)
		}
		return resp.reader, resp.size, nil
	}
}

// GetBySHA256 returns an io.ReadCloser for any object with the a SHA256
// hash, namespaced to the model.
//
// If no object is found, an [objectstore.ObjectNotFound] error is returned.
func (t *fileObjectStore) GetBySHA256(ctx context.Context, sha256 string) (io.ReadCloser, int64, error) {
	// Optimistically try to get the file from the file system. If it doesn't
	// exist, then we'll get an error, and we'll try to get it when sequencing
	// the get request with the put and remove requests.
	if reader, size, err := t.getBySHA256(ctx, sha256, noFallbackStrategy); err == nil {
		return reader, size, nil
	}

	// Sequence the get request with the put and remove requests.
	response := make(chan response)
	select {
	case <-ctx.Done():
		return nil, -1, ctx.Err()
	case <-t.catacomb.Dying():
		return nil, -1, t.catacomb.ErrDying()
	case t.requests <- request{
		op:       opGetBySHA256,
		sha256:   sha256,
		response: response,
	}:
	}

	select {
	case <-ctx.Done():
		return nil, -1, ctx.Err()
	case <-t.catacomb.Dying():
		return nil, -1, t.catacomb.ErrDying()
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
	case <-t.catacomb.Dying():
		return "", t.catacomb.ErrDying()
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
	case <-t.catacomb.Dying():
		return "", t.catacomb.ErrDying()
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
	case <-t.catacomb.Dying():
		return "", t.catacomb.ErrDying()
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
	case <-t.catacomb.Dying():
		return "", t.catacomb.ErrDying()
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
	case <-t.catacomb.Dying():
		return t.catacomb.ErrDying()
	case t.requests <- request{
		op:       opRemove,
		path:     path,
		response: response,
	}:
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.catacomb.Dying():
		return t.catacomb.ErrDying()
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

	// Remove any temporary files that may have been left behind. We don't
	// provide continuation for these operations, so a retry will be required
	// if the operation fails.
	if err := t.cleanupTmpFiles(); err != nil {
		return errors.Errorf("cleaning up temp files: %w", err)
	}

	ctx, cancel := t.scopedContext()
	defer cancel()

	// Watch for changes to the metadata service.
	watcher, err := t.metadataService.Watch()
	if err != nil {
		return errors.Errorf("watching metadata: %w", err)
	}

	if err := t.catacomb.Add(watcher); err != nil {
		return errors.Errorf("adding watcher to catacomb: %w", err)
	}

	pruneTimer := t.clock.NewTimer(jitter(defaultPruneInterval))
	defer pruneTimer.Stop()

	// Sequence the get request with the put, remove requests.
	for {
		select {
		case <-t.catacomb.Dying():
			return t.catacomb.ErrDying()

		case req := <-t.requests:
			switch req.op {
			case opGet:
				reader, size, err := t.get(ctx, req.path, remoteStrategy)

				select {
				case <-t.catacomb.Dying():
					return t.catacomb.ErrDying()

				case req.response <- response{
					reader: reader,
					size:   size,
					err:    err,
				}:
				}

			case opGetBySHA256:
				reader, size, err := t.getBySHA256(ctx, req.sha256, remoteStrategy)

				select {
				case <-t.catacomb.Dying():
					return t.catacomb.ErrDying()

				case req.response <- response{
					reader: reader,
					size:   size,
					err:    err,
				}:
				}

			case opGetBySHA256Prefix:
				reader, size, err := t.getBySHA256Prefix(ctx, req.sha256, remoteStrategy)

				select {
				case <-t.catacomb.Dying():
					return t.catacomb.ErrDying()

				case req.response <- response{
					reader: reader,
					size:   size,
					err:    err,
				}:
				}

			case opPut:
				uuid, err := t.put(ctx, req.path, req.reader, req.size, req.hashValidator)

				select {
				case <-t.catacomb.Dying():
					return t.catacomb.ErrDying()

				case req.response <- response{
					uuid: uuid,
					err:  err,
				}:
				}

			case opRemove:
				select {
				case <-t.catacomb.Dying():
					return t.catacomb.ErrDying()

				case req.response <- response{
					err: t.remove(ctx, req.path),
				}:
				}

			default:
				return errors.Errorf("unknown request type %d", req.op)
			}

		case changes, ok := <-watcher.Changes():
			if !ok {
				select {
				case <-t.catacomb.Dying():
					return t.catacomb.ErrDying()
				default:
					return errors.Errorf("metadata watcher closed")
				}
			}

			if err := t.handleMetadataChanges(ctx, changes); err != nil {
				// For now, we'll just log the error and continue. We might want
				// to consider retrying the operation. We don't need to fail the
				// worker, as the get request will retry the operation when
				// required.
				t.logger.Errorf("handling metadata changes: %v", err)
			}

		case <-pruneTimer.Chan():

			// Reset the pruneTimer, as we've jittered the interval at the start of
			// the loop.
			pruneTimer.Reset(defaultPruneInterval)

			if err := t.prune(ctx, t.list, t.deleteObject); err != nil {
				t.logger.Errorf("prune: %v", err)
				continue
			}
		}
	}
}

func (t *fileObjectStore) get(ctx context.Context, path string, strategy accessStrategy) (io.ReadCloser, int64, error) {
	t.logger.Debugf("getting object %q from file storage", path)

	metadata, err := t.metadataService.GetMetadata(ctx, path)
	if errors.Is(err, domainobjectstoreerrors.ErrNotFound) {
		return nil, -1, errors.Errorf("get metadata: %w", objectstoreerrors.ObjectNotFound)
	} else if err != nil {
		return nil, -1, errors.Errorf("get metadata: %w", err)
	}

	return t.getWithMetadata(ctx, metadata, strategy)
}

func (t *fileObjectStore) getBySHA256(ctx context.Context, sha256 string, strategy accessStrategy) (io.ReadCloser, int64, error) {
	t.logger.Debugf("getting object with SHA256 %q from file storage", sha256)

	metadata, err := t.metadataService.GetMetadataBySHA256(ctx, sha256)
	if errors.Is(err, domainobjectstoreerrors.ErrNotFound) {
		return nil, -1, errors.Errorf("get metadata by SHA256: %w", objectstoreerrors.ObjectNotFound)
	} else if err != nil {
		return nil, -1, errors.Errorf("get metadata by SHA256: %w", err)
	}

	return t.getWithMetadata(ctx, metadata, strategy)
}

func (t *fileObjectStore) getBySHA256Prefix(ctx context.Context, sha256 string, strategy accessStrategy) (io.ReadCloser, int64, error) {
	t.logger.Debugf("getting object with SHA256 prefix %q from file storage", sha256)

	metadata, err := t.metadataService.GetMetadataBySHA256Prefix(ctx, sha256)
	if errors.Is(err, domainobjectstoreerrors.ErrNotFound) {
		return nil, -1, errors.Errorf("get metadata by SHA256 prefix: %w", objectstoreerrors.ObjectNotFound)
	} else if err != nil {
		return nil, -1, errors.Errorf("get metadata by SHA256 prefix: %w", err)
	}

	return t.getWithMetadata(ctx, metadata, strategy)
}

func (t *fileObjectStore) getWithMetadata(ctx context.Context, metadata objectstore.Metadata, strategy accessStrategy) (io.ReadCloser, int64, error) {
	hash := selectFileHash(metadata)

	file, err := t.fs.Open(hash)
	if errors.Is(err, os.ErrNotExist) {
		if strategy != remoteStrategy {
			return nil, -1, errors.Errorf("file %q encoded as %q: %w", metadata.Path, hash, objectstoreerrors.ObjectNotFound)
		}

		return t.getFromRemote(ctx, metadata)
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
	t.logger.Debugf("putting object %q to file storage", path)

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
	t.logger.Debugf("removing object %q from file storage", path)

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
	t.logger.Debugf("listing objects from file storage")

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

func (t *fileObjectStore) getFromRemote(ctx context.Context, metadata objectstore.Metadata) (io.ReadCloser, int64, error) {
	// If we're already in the process of downloading the file, then we don't
	// need to download it again. In this case, we'll return an error so that
	// the client can retry the request.
	if _, ok := t.progressMarkers[metadata.SHA384]; ok {
		return nil, -1, errors.Errorf("get from remote: %w", objectstoreerrors.ObjectNotFound)
	}

	reader, size, err := t.fetchReaderFromRemote(ctx, metadata)
	if err != nil {
		return nil, -1, errors.Errorf("fetching blob from remote: %w", err)
	}

	// We need to now put the blob into the file store, so that we can
	// retrieve it from the file store next time.
	tmpFileName, tmpFileCleanup, err := t.writeToTmpFile(t.path, reader, size)
	if err != nil {
		return nil, -1, errors.Capture(err)
	}
	defer func() {
		_ = tmpFileCleanup()
	}()

	// Persist the temporary file to the final location.
	if err := t.withLock(ctx, metadata.SHA384, func(ctx context.Context) error {
		return t.persistTmpFile(ctx, tmpFileName, metadata.SHA384, size)
	}); err != nil {
		return nil, -1, errors.Capture(err)
	}

	// Now that we've written the file, we can get the file from the file store.
	return t.getWithMetadata(ctx, metadata, noFallbackStrategy)
}

func (t *fileObjectStore) handleMetadataChanges(ctx context.Context, changes []string) error {
	t.logger.Debugf("handling metadata changes: %v", changes)
	// In theory this could be done in parallel, but we're dealing with paths
	// and not SHA hashes, so we need to ensure that we're not writing to the
	// same file at the same time.
	for _, path := range changes {
		if err := t.handleMetadataChange(ctx, path); err != nil {
			return errors.Errorf("handling metadata change for %q: %w", path, err)
		}
	}
	return nil
}

func (t *fileObjectStore) handleMetadataChange(ctx context.Context, path string) error {
	metadata, err := t.metadataService.GetMetadata(ctx, path)
	if errors.Is(err, domainobjectstoreerrors.ErrNotFound) {
		// We could potentially remove the file here, but
		// we would need to ensure that nothing else is either writing to
		// the underlying hash or linked to the underlying hash.
		// For now, we'll log that it should be cleaned up in the future either
		// by the pruner operation in this worker, or the orphaned file cleanup
		// operation in the object store.
		t.logger.Debugf("metadata for path %q not found, file should be cleaned up", path)
		return nil
	} else if err != nil {
		return errors.Errorf("getting metadata for path %q: %w", path, err)
	}

	// Handle existing requests for the same underlying hash.
	if _, ok := t.progressMarkers[metadata.SHA384]; ok {
		// We're already in the process of downloading the file, so we don't
		// need to do anything.
		return nil
	}
	// Mark the file for download, so that we don't download it multiple times.
	t.progressMarkers[metadata.SHA384] = struct{}{}
	defer func() {
		// Remove the progress marker, as we've either successfully downloaded
		// the file, or we've failed to download the file which will allow it
		// to be retried.
		delete(t.progressMarkers, metadata.SHA384)
	}()

	// If the file already exists for the hash, we don't need to do anything,
	// we've already written the file.
	hash := selectFileHash(metadata)
	_, err = os.Stat(t.filePath(hash))
	if err == nil {
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return errors.Errorf("opening file %q encoded as %q: %w", metadata.Path, hash, err)
	}

	// Fetch the file from a remote API server, if it doesn't exist return
	// not found.
	reader, size, err := t.fetchReaderFromRemote(ctx, metadata)
	if errors.Is(err, objectstoreerrors.ObjectNotFound) {
		t.logger.Warningf("object %q not found remotely", metadata.Path)
		return nil
	} else if err != nil {
		return errors.Errorf("fetching blob from remote: %w", err)
	}

	// We need to now put the blob into the file store, so that we can
	// retrieve it from the file store next time.
	tmpFileName, tmpFileCleanup, err := t.writeToTmpFile(t.path, reader, size)
	if err != nil {
		return errors.Capture(err)
	}
	defer func() {
		_ = tmpFileCleanup()
	}()

	// Persist the temporary file to the final location.
	if err := t.withLock(ctx, metadata.SHA384, func(ctx context.Context) error {
		return t.persistTmpFile(ctx, tmpFileName, metadata.SHA384, size)
	}); err != nil {
		return errors.Capture(err)
	}

	return nil
}

func (t *fileObjectStore) fetchReaderFromRemote(ctx context.Context, metadata objectstore.Metadata) (io.ReadCloser, int64, error) {
	t.logger.Debugf("fetching object %q from remote", metadata.Path)

	ctx, cancel := context.WithTimeout(ctx, defaultRemoteTimeout)
	defer cancel()

	reader, size, err := t.blobRetriever.RetrieveBlobFromRemote(ctx, metadata.SHA256)
	if errors.Is(err, remote.NoRemoteConnection) ||
		errors.Is(err, remote.BlobNotFound) {
		return nil, -1, objectstoreerrors.ObjectNotFound
	} else if err != nil {
		return nil, -1, errors.Capture(err)
	}

	if size != metadata.Size {
		return nil, -1, errors.Errorf("size mismatch for %q: expected %d, got %d", metadata.Path, metadata.Size, size)
	}

	return reader, size, nil
}

// basePath returns the base path for the file object store.
// typically: /var/lib/juju/objectstore/<namespace>
func basePath(rootDir, namespace string) string {
	return filepath.Join(rootDir, defaultFileDirectory, namespace)
}
