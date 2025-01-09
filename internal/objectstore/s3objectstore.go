// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package objectstore

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/juju/clock"
	jujuerrors "github.com/juju/errors"
	"github.com/juju/worker/v4/catacomb"

	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/objectstore"
	domainobjectstoreerrors "github.com/juju/juju/domain/objectstore/errors"
	"github.com/juju/juju/internal/errors"
	objectstoreerrors "github.com/juju/juju/internal/objectstore/errors"
)

const (
	defaultPruneInterval = time.Hour * 6
)

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

// S3ObjectStoreConfig is the configuration for the s3 object store.
type S3ObjectStoreConfig struct {
	// RootDir is the root directory for the object store. This is the location
	// where the tmp directory will be created.
	// This is different than /tmp because the /tmp directory might be
	// mounted on a different file system.
	RootDir string
	// RootBucket is the name of the root bucket.
	RootBucket string
	// Namespace is the namespace for the object store (typically the
	// model UUID).
	Namespace string
	// Client is the object store client (s3 client).
	Client objectstore.Client
	// MetadataService is the metadata service for translating paths to
	// hashes.
	MetadataService objectstore.ObjectStoreMetadata
	// Claimer is the claimer for locking files.
	Claimer Claimer
	// HashFileSystemAccessor is used for draining files from the file backed
	// object store to the s3 object store.
	HashFileSystemAccessor HashFileSystemAccessor
	// AllowDraining is a flag to allow draining files from the file backed
	// object store to the s3 object store.
	AllowDraining bool

	Logger logger.Logger
	Clock  clock.Clock
}

const (
	// States which report the state of the worker.
	stateStarted     = "started"
	stateDrained     = "drained"
	stateFileDrained = "file drained: %s"
)

type s3ObjectStore struct {
	baseObjectStore
	internalStates chan string
	client         objectstore.Client
	rootBucket     string
	namespace      string
	requests       chan request
	drainRequests  chan drainRequest

	// HashFileSystemAccessor is used for draining files from the file backed
	// object store to the s3 object store.
	fileSystemAccessor HashFileSystemAccessor
	allowDraining      bool
}

// NewS3ObjectStore returns a new object store worker based on the s3 backing
// storage.
func NewS3ObjectStore(cfg S3ObjectStoreConfig) (TrackedObjectStore, error) {
	return newS3ObjectStore(cfg, nil)
}

func newS3ObjectStore(cfg S3ObjectStoreConfig, internalStates chan string) (*s3ObjectStore, error) {
	path := filepath.Join(cfg.RootDir, defaultFileDirectory, cfg.Namespace)

	s := &s3ObjectStore{
		baseObjectStore: baseObjectStore{
			path:            path,
			claimer:         cfg.Claimer,
			metadataService: cfg.MetadataService,
			logger:          cfg.Logger,
			clock:           cfg.Clock,
		},
		internalStates: internalStates,
		client:         cfg.Client,
		rootBucket:     cfg.RootBucket,
		namespace:      cfg.Namespace,

		fileSystemAccessor: cfg.HashFileSystemAccessor,
		allowDraining:      cfg.AllowDraining,

		requests:      make(chan request),
		drainRequests: make(chan drainRequest),
	}

	if err := catacomb.Invoke(catacomb.Plan{
		Site: &s.catacomb,
		Work: s.loop,
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
func (t *s3ObjectStore) Get(ctx context.Context, path string) (io.ReadCloser, int64, error) {
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

// GetBySHA256Prefix returns an io.ReadCloser for any object with a SHA256
// hash starting with a given prefix, namespaced to the model.
//
// If no object is found, an [objectstore.ObjectNotFound] error is returned.
func (t *s3ObjectStore) GetBySHA256Prefix(ctx context.Context, sha256Prefix string) (io.ReadCloser, int64, error) {
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
		op:           opGetByHash,
		sha256Prefix: sha256Prefix,
		response:     response,
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
func (t *s3ObjectStore) Put(ctx context.Context, path string, r io.Reader, size int64) (objectstore.UUID, error) {
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
func (t *s3ObjectStore) PutAndCheckHash(ctx context.Context, path string, r io.Reader, size int64, hash string) (objectstore.UUID, error) {
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
func (t *s3ObjectStore) Remove(ctx context.Context, path string) error {
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

func (t *s3ObjectStore) loop() error {
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

	// Ensure that we have the base directory.
	if err := t.client.Session(ctx, func(ctx context.Context, s objectstore.Session) error {
		err := s.CreateBucket(ctx, t.rootBucket)
		if err != nil && !errors.Is(err, jujuerrors.AlreadyExists) {
			return errors.Capture(err)
		}
		return nil
	}); err != nil {
		return errors.Capture(err)
	}

	timer := t.clock.NewTimer(jitter(defaultPruneInterval))
	defer timer.Stop()

	// Drain any files from the file object store to the s3 object store.
	// This will locate any files from the metadata service that are not
	// present in the s3 object store and copy them over.
	// Once done it will terminate the goroutine.
	strategy := noFallbackStrategy
	if t.allowDraining {
		// Drain any files from the file object store to the s3 object store.
		// This will locate any files from the metadata service that are not
		// present in the s3 object store and copy them over.
		// Once done it will terminate the goroutine.
		metadata, err := t.metadataService.ListMetadata(ctx)
		if err != nil {
			return errors.Errorf("listing metadata for draining: %w", err)
		}

		// It would be nice to use a worker here, but it makes this very
		// complicated. For now just use a goroutine (there is no catacomb.Go
		// which would be nice).
		go func() {
			if err := t.drainFiles(metadata); err != nil {
				t.catacomb.Kill(err)
			}
		}()

		// If we allow draining, then we can attempt to use the file accessor.
		strategy = fileStrategy
	}

	// Report the initial started state.
	t.reportInternalState(stateStarted)

	// Sequence the get request with the put, remove requests.
	for {
		select {
		case <-t.catacomb.Dying():
			return t.catacomb.ErrDying()

		case req := <-t.requests:
			switch req.op {
			case opGet:
				// We can attempt to use the file accessor to get the file
				// if it's not found in the s3 object store. This can occur
				// during the drain process. As these requests are sequenced
				// with the drain requests we can at least attempt to get the
				// file from the file accessor for intermediate content.
				reader, size, err := t.get(ctx, req.path, strategy)

				select {
				case <-t.catacomb.Dying():
					return t.catacomb.ErrDying()

				case req.response <- response{
					reader: reader,
					size:   size,
					err:    err,
				}:
				}

			case opGetByHash:
				// We can attempt to use the file accessor to get the file
				// if it's not found in the s3 object store. This can occur
				// during the drain process. As these requests are sequenced
				// with the drain requests we can at least attempt to get the
				// file from the file accessor for intermediate content.
				reader, size, err := t.getBySHA256Prefix(ctx, req.sha256Prefix, strategy)

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

		case <-timer.Chan():

			// Reset the timer, as we've jittered the interval at the start of
			// the loop.
			timer.Reset(defaultPruneInterval)

			if err := t.prune(ctx, t.list, t.deleteObject); err != nil {
				t.logger.Errorf("prune: %v", err)
				continue
			}

		case req, ok := <-t.drainRequests:
			if !ok {
				// File draining has completed, so we can stop processing
				// requests to the file backed object store.
				strategy = noFallbackStrategy
				continue
			}

			select {
			case <-t.catacomb.Dying():
				return t.catacomb.ErrDying()
			case req.response <- t.drainFile(ctx, req.path, req.hash, req.size):
			}
		}
	}
}

func (t *s3ObjectStore) get(ctx context.Context, path string, strategy accessStrategy) (io.ReadCloser, int64, error) {
	t.logger.Debugf("getting object %q from file storage", path)

	metadata, err := t.metadataService.GetMetadata(ctx, path)
	if errors.Is(err, domainobjectstoreerrors.ErrNotFound) {
		return nil, -1, errors.Errorf("get metadata: %w", objectstoreerrors.ObjectNotFound)
	}
	if err != nil {
		return nil, -1, errors.Errorf("get metadata: %w", err)
	}

	return t.getWithMetadata(ctx, metadata, strategy)
}

func (t *s3ObjectStore) getBySHA256Prefix(ctx context.Context, sha256Prefix string, strategy accessStrategy) (io.ReadCloser, int64, error) {
	t.logger.Debugf("getting object with SHA256 prefix %q from file storage", sha256Prefix)

	metadata, err := t.metadataService.GetMetadataBySHA256Prefix(ctx, sha256Prefix)
	if errors.Is(err, domainobjectstoreerrors.ErrNotFound) {
		return nil, -1, errors.Errorf("get metadata by SHA256 prefix: %w", objectstoreerrors.ObjectNotFound)
	}
	if err != nil {
		return nil, -1, errors.Errorf("get metadata by SHA256 prefix: %w", err)
	}

	return t.getWithMetadata(ctx, metadata, strategy)
}

func (t *s3ObjectStore) getWithMetadata(ctx context.Context, metadata objectstore.Metadata, strategy accessStrategy) (io.ReadCloser, int64, error) {
	hash := selectFileHash(metadata)

	var reader io.ReadCloser
	var size int64
	if err := t.client.Session(ctx, func(ctx context.Context, s objectstore.Session) error {
		var err error
		reader, size, _, err = s.GetObject(ctx, t.rootBucket, t.filePath(hash))
		return err
	}); err != nil && !errors.Is(err, jujuerrors.NotFound) {
		return nil, -1, errors.Errorf("get object: %w", err)
	} else if errors.Is(err, jujuerrors.NotFound) {
		// If we're not allowed to use the file accessor, then we can't
		// attempt to get the file from the file backed object store.
		if strategy != fileStrategy {
			return nil, -1, objectstoreerrors.ObjectNotFound
		}

		var newErr error
		reader, size, newErr = t.fileSystemAccessor.GetByHash(ctx, hash)
		if newErr != nil {
			// Ignore the new error, because we want to return the original
			// error.
			t.logger.Debugf("unable to get file %q from file object store: %v", metadata.Path, newErr)
			return nil, -1, objectstoreerrors.ObjectNotFound
		}

		// This file was located in the file backed object store, the draining
		// process should remove it from the file backed object store.
		t.logger.Tracef("located file from file object store that wasn't found in s3 object store: %q", metadata.Path)
	}

	if metadata.Size != size {
		return nil, -1, errors.Errorf("size mismatch for %q: expected %d, got %d", metadata.Path, metadata.Size, size)
	}

	return reader, size, nil
}

func (t *s3ObjectStore) put(ctx context.Context, path string, r io.Reader, size int64, validator hashValidator) (objectstore.UUID, error) {
	t.logger.Debugf("putting object %q to s3 storage", path)

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
	fileName, tmpFileCleanup, err := t.writeToTmpFile(t.path, io.TeeReader(r, io.MultiWriter(hash384, hash256)), size)
	if err != nil {
		return "", errors.Capture(err)
	}

	// Ensure that we remove the temporary file if we fail to persist it.
	defer func() { _ = tmpFileCleanup() }()

	// Encode the hashes as strings, so we can use them for file, http and s3
	// lookups.
	encoded384 := hex.EncodeToString(hash384.Sum(nil))
	encoded256 := hex.EncodeToString(hash256.Sum(nil))
	s3EncodedHash := base64.StdEncoding.EncodeToString(hash256.Sum(nil))

	// Ensure that the hash of the file matches the expected hash.
	if expected, ok := validator(encoded384); !ok {
		return "", errors.Errorf("hash mismatch for %q: expected %q, got %q: %w", path, expected, encoded384, objectstore.ErrHashMismatch)
	}

	// Lock the file with the given hash (encoded384), so that we can't
	// remove the file while we're writing it.
	var uuid objectstore.UUID
	if err := t.withLock(ctx, encoded384, func(ctx context.Context) error {
		// Persist the temporary file to the final location.
		if err := t.persistTmpFile(ctx, fileName, encoded384, s3EncodedHash, size); err != nil {
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

func (t *s3ObjectStore) persistTmpFile(ctx context.Context, tmpFileName, fileEncodedHash, s3EncodedHash string, size int64) error {
	file, err := os.Open(tmpFileName)
	if err != nil {
		return errors.Capture(err)
	}
	defer file.Close()

	// The size is already done when it's written, but to ensure that we have
	// the correct size, we can stat the file. This is very much, belt and
	// braces approach.
	if stat, err := file.Stat(); err != nil {
		return errors.Capture(err)
	} else if stat.Size() != size {
		return errors.Errorf("size mismatch for %q: expected %d, got %d", tmpFileName, size, stat.Size())
	}

	// The file has been verified, so we can move it to the final location.
	if err := t.putFile(ctx, file, fileEncodedHash, s3EncodedHash); err != nil {
		return errors.Capture(err)
	}

	return nil
}

func (t *s3ObjectStore) putFile(ctx context.Context, file io.ReadSeeker, fileEncodedHash, s3EncodedHash string) error {
	return t.client.Session(ctx, func(ctx context.Context, s objectstore.Session) error {
		// Seek back to the beginning of the file, so that we can read it again.
		if _, err := file.Seek(0, io.SeekStart); err != nil {
			return errors.Capture(err)
		}

		// Now that we've written the file, we can upload it to the object
		// store.
		err := s.PutObject(ctx, t.rootBucket, t.filePath(fileEncodedHash), file, s3EncodedHash)
		// If the file already exists, then we can ignore the error.
		if err == nil || errors.Is(err, jujuerrors.AlreadyExists) {
			return nil
		}

		return errors.Capture(err)
	})
}

func (t *s3ObjectStore) remove(ctx context.Context, path string) error {
	t.logger.Debugf("removing object %q from s3 storage", path)

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

func (t *s3ObjectStore) filePath(hash string) string {
	return fmt.Sprintf("%s/%s", t.namespace, hash)
}

func (t *s3ObjectStore) list(ctx context.Context) ([]objectstore.Metadata, []string, error) {
	t.logger.Debugf("listing objects from s3 storage")

	metadata, err := t.metadataService.ListMetadata(ctx)
	if err != nil {
		return nil, nil, errors.Errorf("list metadata: %w", err)
	}

	var objects []string
	if err := t.client.Session(ctx, func(ctx context.Context, s objectstore.Session) error {
		var err error
		objects, err = s.ListObjects(ctx, t.rootBucket)
		if err != nil {
			return errors.Capture(err)
		}
		return nil
	}); err != nil {
		return nil, nil, errors.Errorf("list objects: %w", err)
	}

	return metadata, objects, nil
}

func (t *s3ObjectStore) deleteObject(ctx context.Context, hash string) error {
	return t.client.Session(ctx, func(ctx context.Context, s objectstore.Session) error {
		err := s.DeleteObject(ctx, t.rootBucket, t.filePath(hash))
		if err == nil || errors.Is(err, jujuerrors.NotFound) {
			return nil
		}
		return errors.Capture(err)
	})
}

// drainRequest is similar to the request type, but is used for draining files
// from the file backed object store to the s3 object store.
type drainRequest struct {
	path     string
	hash     string
	size     int64
	response chan error
}

// The drainFiles method will drain the files from the file object store to the
// s3 object store. This will locate any files from the metadata service that
// are not present in the s3 object store and copy them over.
func (t *s3ObjectStore) drainFiles(metadata []objectstore.Metadata) error {
	ctx, cancel := t.scopedContext()
	defer cancel()

	t.logger.Infof("draining started for %q, processing %d", t.namespace, len(metadata))

	// Process each file in the metadata service, and drain it to the s3 object
	// store.
	// Note: we could do this in parallel, but everything is sequenced with
	// the requests channel, so we don't need to worry about it.
	for _, m := range metadata {
		result := make(chan error)

		hash := selectFileHash(m)

		t.logger.Debugf("draining file %q to s3 object store %q", m.Path, hash)

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-t.catacomb.Dying():
			return t.catacomb.ErrDying()
		case t.drainRequests <- drainRequest{
			path:     m.Path,
			hash:     hash,
			size:     m.Size,
			response: result,
		}:
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-t.catacomb.Dying():
			return t.catacomb.ErrDying()
		case err := <-result:
			t.reportInternalState(fmt.Sprintf(stateFileDrained, hash))
			// This will crash the s3ObjectStore worker if this is a fatal
			// error. We don't want to continue processing if we can't drain
			// the files to the s3 object store.
			if err != nil {
				return errors.Errorf("draining file %q to s3 object store: %w", m.Path, err)
			}
		}
	}

	// Ensure we close the drain requests channel, so that we can prevent
	// any further requests to the local file system.
	close(t.drainRequests)

	t.logger.Infof("draining completed for %q, processed %d", t.namespace, len(metadata))

	// Report the drained state completed.
	t.reportInternalState(stateDrained)

	return nil
}

func (t *s3ObjectStore) drainFile(ctx context.Context, path, hash string, metadataSize int64) error {
	// If the file isn't on the file backed object store, then we can skip it.
	// It's expected that this has already been drained to the s3 object store.
	if err := t.fileSystemAccessor.HashExists(ctx, hash); err != nil {
		if errors.Is(err, jujuerrors.NotFound) {
			return nil
		}
		return errors.Errorf("checking if file %q exists in file object store: %w", path, err)
	}

	// If the file is already in the s3 object store, then we can skip it.
	// Note: we want to check the s3 object store each request, just in
	// case the file was added to the s3 object store while we were
	// draining the files.
	if err := t.objectAlreadyExists(ctx, hash); err != nil && !errors.Is(err, jujuerrors.NotFound) {
		return errors.Errorf("checking if file %q exists in s3 object store: %w", path, err)
	} else if err == nil {
		// File already contains the hash, so we can skip it.
		t.logger.Tracef("file %q already exists in s3 object store, skipping", path)
		return nil
	}

	t.logger.Debugf("draining file %q to s3 object store", path)

	// Grab the file from the file backed object store and drain it to the
	// s3 object store.
	reader, fileSize, err := t.fileSystemAccessor.GetByHash(ctx, hash)
	if err != nil {
		// The file doesn't exist in the file backed object store, but also
		// doesn't exist in the s3 object store. This is a problem, so we
		// should skip it.
		if errors.Is(err, jujuerrors.NotFound) {
			t.logger.Warningf("file %q doesn't exist in file object store, unable to drain", path)
			return nil
		}
		return errors.Errorf("getting file %q from file object store: %w", path, err)
	}

	// Ensure we close the reader when we're done.
	defer reader.Close()

	// If the file size doesn't match the metadata size, then the file is
	// potentially corrupt, so we should skip it.
	if fileSize != metadataSize {
		t.logger.Warningf("file %q has a size mismatch, unable to drain", path)
		return nil
	}

	// We need to compute the sha256 hash here, juju by default uses SHA384,
	// but s3 defaults to SHA256.
	// If the reader is a Seeker, then we can seek back to the beginning of
	// the file, so that we can read it again.
	s3Reader, s3EncodedHash, err := t.computeS3Hash(reader)
	if err != nil {
		return errors.Capture(err)
	}

	// We can drain the file to the s3 object store.
	err = t.client.Session(ctx, func(ctx context.Context, s objectstore.Session) error {
		err := s.PutObject(ctx, t.rootBucket, t.filePath(hash), s3Reader, s3EncodedHash)
		if err != nil {
			return errors.Errorf("putting file %q to s3 object store: %w", path, err)
		}
		return nil
	})
	if err != nil {
		if errors.Is(err, jujuerrors.AlreadyExists) {
			// File already contains the hash, so we can skip it.
			return t.removeDrainedFile(ctx, hash)
		}
		return errors.Capture(err)
	}

	// We can remove the file from the file backed object store, because it
	// has been successfully drained to the s3 object store.
	if err := t.removeDrainedFile(ctx, hash); err != nil {
		return errors.Capture(err)
	}

	return nil
}

func (t *s3ObjectStore) computeS3Hash(reader io.Reader) (io.Reader, string, error) {
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

func (t *s3ObjectStore) objectAlreadyExists(ctx context.Context, hash string) error {
	if err := t.client.Session(ctx, func(ctx context.Context, s objectstore.Session) error {
		err := s.ObjectExists(ctx, t.rootBucket, t.filePath(hash))
		return errors.Capture(err)
	}); err != nil {
		return errors.Errorf("checking if file %q exists in s3 object store: %w", hash, err)
	}
	return nil
}

func (t *s3ObjectStore) removeDrainedFile(ctx context.Context, hash string) error {
	if err := t.fileSystemAccessor.DeleteByHash(ctx, hash); err != nil {
		// If we're unable to remove the file from the file backed object
		// store, then we should log a warning, but continue processing.
		// This is not a terminal case, we can continue processing.
		t.logger.Warningf("unable to remove file %q from file object store: %v", hash, err)
		return nil
	}
	return nil
}

func (t *s3ObjectStore) reportInternalState(state string) {
	if t.internalStates == nil {
		return
	}
	select {
	case <-t.catacomb.Dying():
	case t.internalStates <- state:
	}
}
