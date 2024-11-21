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
	"github.com/juju/errors"
	"gopkg.in/tomb.v2"

	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/objectstore"
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

// getAccessorPattern is the type of fallback to use when getting a file.
type getAccessorPattern int

const (
	// useFileAccessor denotes that it's possible to go look in the file
	// system accessor for a file if it's not found in the s3 object store.
	// This can occur when draining files from the file backed object store to
	// the s3 object store.
	useFileAccessor getAccessorPattern = 0

	// noFileFallback denotes that we should not look in the file system
	// accessor for a file if it's not found in the s3 object store.
	noFileFallback getAccessorPattern = 1
)

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

	s.tomb.Go(s.loop)

	return s, nil
}

// Get returns an io.ReadCloser for data at path, namespaced to the
// model.
func (t *s3ObjectStore) Get(ctx context.Context, path string) (io.ReadCloser, int64, error) {
	// Optimistically try to get the file from the file system. If it doesn't
	// exist, then we'll get an error, and we'll try to get it when sequencing
	// the get request with the put and remove requests.
	if reader, size, err := t.get(ctx, path, noFileFallback); err == nil {
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
			return nil, -1, errors.Annotatef(resp.err, "getting blob")
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
			return "", errors.Annotatef(resp.err, "putting blob")
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
			return "", errors.Annotatef(resp.err, "putting blob and check hash")
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
			return errors.Annotatef(resp.err, "removing blob")
		}
		return nil
	}
}

func (t *s3ObjectStore) loop() error {
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

	// Ensure that we have the base directory.
	if err := t.client.Session(ctx, func(ctx context.Context, s objectstore.Session) error {
		err := s.CreateBucket(ctx, t.rootBucket)
		if err != nil && !errors.Is(err, errors.AlreadyExists) {
			return errors.Trace(err)
		}
		return nil
	}); err != nil {
		return errors.Trace(err)
	}

	timer := t.clock.NewTimer(jitter(defaultPruneInterval))
	defer timer.Stop()

	// Drain any files from the file object store to the s3 object store.
	// This will locate any files from the metadata service that are not
	// present in the s3 object store and copy them over.
	// Once done it will terminate the goroutine.
	fileFallback := noFileFallback
	if t.allowDraining {
		// Drain any files from the file object store to the s3 object store.
		// This will locate any files from the metadata service that are not
		// present in the s3 object store and copy them over.
		// Once done it will terminate the goroutine.
		metadata, err := t.metadataService.ListMetadata(ctx)
		if err != nil {
			return errors.Annotatef(err, "listing metadata for draining")
		}

		t.tomb.Go(t.drainFiles(metadata))

		// If we allow draining, then we can attempt to use the file accessor.
		fileFallback = useFileAccessor
	}

	// Report the initial started state.
	t.reportInternalState(stateStarted)

	// Sequence the get request with the put, remove requests.
	for {
		select {
		case <-t.tomb.Dying():
			return tomb.ErrDying

		case req := <-t.requests:
			switch req.op {
			case opGet:
				// We can attempt to use the file accessor to get the file
				// if it's not found in the s3 object store. This can occur
				// during the drain process. As these requests are sequenced
				// with the drain requests we can at least attempt to get the
				// file from the file accessor for intermediate content.
				reader, size, err := t.get(ctx, req.path, fileFallback)

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
				return fmt.Errorf("unknown request type %d", req.op)
			}

		case <-timer.Chan():

			// Reset the timer, as we've jittered the interval at the start of
			// the loop.
			timer.Reset(defaultPruneInterval)

			if err := t.prune(ctx, t.list, t.deleteObject); err != nil {
				t.logger.Errorf(context.TODO(), "prune: %v", err)
				continue
			}

		case req, ok := <-t.drainRequests:
			if !ok {
				// File draining has completed, so we can stop processing
				// requests to the file backed object store.
				fileFallback = noFileFallback
				continue
			}

			select {
			case <-t.tomb.Dying():
				return tomb.ErrDying
			case req.response <- t.drainFile(ctx, req.path, req.hash, req.size):
			}
		}
	}
}

func (t *s3ObjectStore) get(ctx context.Context, path string, useAccessor getAccessorPattern) (io.ReadCloser, int64, error) {
	t.logger.Debugf(context.TODO(), "getting object %q from file storage", path)

	metadata, err := t.metadataService.GetMetadata(ctx, path)
	if err != nil {
		return nil, -1, errors.Annotatef(err, "get metadata")
	}

	var reader io.ReadCloser
	var size int64
	if err := t.client.Session(ctx, func(ctx context.Context, s objectstore.Session) error {
		var err error
		reader, size, _, err = s.GetObject(ctx, t.rootBucket, t.filePath(metadata.Hash))
		return err
	}); err != nil && !errors.Is(err, errors.NotFound) {
		return nil, -1, errors.Annotatef(err, "get object")
	} else if errors.Is(err, errors.NotFound) {
		// If we're not allowed to use the file accessor, then we can't
		// attempt to get the file from the file backed object store.
		if useAccessor == noFileFallback {
			return nil, -1, errors.Trace(err)
		}

		var newErr error
		reader, size, newErr = t.fileSystemAccessor.GetByHash(ctx, metadata.Hash)
		if newErr != nil {
			// Ignore the new error, because we want to return the original
			// error.
			t.logger.Debugf(context.TODO(), "unable to get file %q from file object store: %v", path, newErr)
			return nil, -1, errors.Trace(err)
		}

		// This file was located in the file backed object store, the draining
		// process should remove it from the file backed object store.
		t.logger.Tracef(context.TODO(), "located file from file object store that wasn't found in s3 object store: %q", path)
	}

	if metadata.Size != size {
		return nil, -1, fmt.Errorf("size mismatch for %q: expected %d, got %d", path, metadata.Size, size)
	}

	return reader, size, nil
}

func (t *s3ObjectStore) put(ctx context.Context, path string, r io.Reader, size int64, validator hashValidator) (objectstore.UUID, error) {
	t.logger.Debugf(context.TODO(), "putting object %q to s3 storage", path)

	// Charms and resources are coded to use the SHA384 hash. It is possible
	// to move to the more common SHA256 hash, but that would require a
	// migration of all charms and resources during import.
	// I can only assume 384 was chosen over 256 and others, is because it's
	// not susceptible to length extension attacks? In any case, we'll
	// keep using it for now.
	fileHash := sha512.New384()

	// We need two hash sets here, because juju wants to use SHA384, but s3
	// defaults to SHA256. Luckily, we can piggyback on the writing to a tmp
	// file and create TeeReader with a MultiWriter.
	s3Hash := sha256.New()

	// We need to write this to a temp file, because if the client retries
	// then we need seek back to the beginning of the file.
	fileName, tmpFileCleanup, err := t.writeToTmpFile(t.path, io.TeeReader(r, io.MultiWriter(fileHash, s3Hash)), size)
	if err != nil {
		return "", errors.Trace(err)
	}

	// Ensure that we remove the temporary file if we fail to persist it.
	defer func() { _ = tmpFileCleanup() }()

	// Encode the hashes as strings, so we can use them for file and s3 lookups.
	fileEncodedHash := hex.EncodeToString(fileHash.Sum(nil))
	s3EncodedHash := base64.StdEncoding.EncodeToString(s3Hash.Sum(nil))

	// Ensure that the hash of the file matches the expected hash.
	if expected, ok := validator(fileEncodedHash); !ok {
		return "", errors.Annotatef(objectstore.ErrHashMismatch, "hash mismatch for %q: expected %q, got %q", path, expected, fileEncodedHash)
	}

	// Lock the file with the given hash (fileEncodedHash), so that we can't
	// remove the file while we're writing it.
	var uuid objectstore.UUID
	if err := t.withLock(ctx, fileEncodedHash, func(ctx context.Context) error {
		// Persist the temporary file to the final location.
		if err := t.persistTmpFile(ctx, fileName, fileEncodedHash, s3EncodedHash, size); err != nil {
			return errors.Trace(err)
		}

		// Save the metadata for the file after we've written it. That way we
		// correctly sequence the watch events. Otherwise there is a potential
		// race where the watch event is emitted before the file is written.
		var err error
		if uuid, err = t.metadataService.PutMetadata(ctx, objectstore.Metadata{
			Path: path,
			Hash: fileEncodedHash,
			Size: size,
		}); err != nil {
			return errors.Trace(err)
		}
		return nil
	}); err != nil {
		return "", errors.Trace(err)
	}
	return uuid, nil
}

func (t *s3ObjectStore) persistTmpFile(ctx context.Context, tmpFileName, fileEncodedHash, s3EncodedHash string, size int64) error {
	file, err := os.Open(tmpFileName)
	if err != nil {
		return errors.Trace(err)
	}
	defer file.Close()

	// The size is already done when it's written, but to ensure that we have
	// the correct size, we can stat the file. This is very much, belt and
	// braces approach.
	if stat, err := file.Stat(); err != nil {
		return errors.Trace(err)
	} else if stat.Size() != size {
		return fmt.Errorf("size mismatch for %q: expected %d, got %d", tmpFileName, size, stat.Size())
	}

	// The file has been verified, so we can move it to the final location.
	if err := t.putFile(ctx, file, fileEncodedHash, s3EncodedHash); err != nil {
		return errors.Trace(err)
	}

	return nil
}

func (t *s3ObjectStore) putFile(ctx context.Context, file io.ReadSeeker, fileEncodedHash, s3EncodedHash string) error {
	return t.client.Session(ctx, func(ctx context.Context, s objectstore.Session) error {
		// Seek back to the beginning of the file, so that we can read it again.
		if _, err := file.Seek(0, io.SeekStart); err != nil {
			return errors.Trace(err)
		}

		// Now that we've written the file, we can upload it to the object
		// store.
		err := s.PutObject(ctx, t.rootBucket, t.filePath(fileEncodedHash), file, s3EncodedHash)
		// If the file already exists, then we can ignore the error.
		if err == nil || errors.Is(err, errors.AlreadyExists) {
			return nil
		}

		return errors.Trace(err)
	})
}

func (t *s3ObjectStore) remove(ctx context.Context, path string) error {
	t.logger.Debugf(context.TODO(), "removing object %q from s3 storage", path)

	metadata, err := t.metadataService.GetMetadata(ctx, path)
	if err != nil {
		return errors.Annotatef(err, "get metadata")
	}

	hash := metadata.Hash
	return t.withLock(ctx, hash, func(ctx context.Context) error {
		if err := t.metadataService.RemoveMetadata(ctx, path); err != nil {
			return errors.Annotatef(err, "remove metadata")
		}

		return t.deleteObject(ctx, hash)
	})
}

func (t *s3ObjectStore) filePath(hash string) string {
	return fmt.Sprintf("%s/%s", t.namespace, hash)
}

func (t *s3ObjectStore) list(ctx context.Context) ([]objectstore.Metadata, []string, error) {
	t.logger.Debugf(context.TODO(), "listing objects from s3 storage")

	metadata, err := t.metadataService.ListMetadata(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("list metadata: %w", err)
	}

	var objects []string
	if err := t.client.Session(ctx, func(ctx context.Context, s objectstore.Session) error {
		var err error
		objects, err = s.ListObjects(ctx, t.rootBucket)
		if err != nil {
			return errors.Trace(err)
		}
		return nil
	}); err != nil {
		return nil, nil, fmt.Errorf("list objects: %w", err)
	}

	return metadata, objects, nil
}

func (t *s3ObjectStore) deleteObject(ctx context.Context, hash string) error {
	return t.client.Session(ctx, func(ctx context.Context, s objectstore.Session) error {
		err := s.DeleteObject(ctx, t.rootBucket, t.filePath(hash))
		if err == nil || errors.Is(err, errors.NotFound) {
			return nil
		}
		return errors.Trace(err)
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
func (t *s3ObjectStore) drainFiles(metadata []objectstore.Metadata) func() error {
	// We require the capture closure to ensure that the metadata is captured
	// at the time of the call, rather than at the time of the execution. This
	// prevents any data races.
	return func() error {
		ctx, cancel := t.scopedContext()
		defer cancel()

		t.logger.Infof(context.TODO(), "draining started for %q, processing %d", t.namespace, len(metadata))

		// Process each file in the metadata service, and drain it to the s3 object
		// store.
		// Note: we could do this in parallel, but everything is sequenced with
		// the requests channel, so we don't need to worry about it.
		for _, m := range metadata {
			result := make(chan error)

			t.logger.Debugf(context.TODO(), "draining file %q to s3 object store %q", m.Path, m.Hash)

			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-t.tomb.Dying():
				return tomb.ErrDying
			case t.drainRequests <- drainRequest{
				path:     m.Path,
				hash:     m.Hash,
				size:     m.Size,
				response: result,
			}:
			}

			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-t.tomb.Dying():
				return tomb.ErrDying
			case err := <-result:
				t.reportInternalState(fmt.Sprintf(stateFileDrained, m.Hash))
				// This will crash the s3ObjectStore worker if this is a fatal
				// error. We don't want to continue processing if we can't drain
				// the files to the s3 object store.
				if err != nil {
					return errors.Annotatef(err, "draining file %q to s3 object store", m.Path)
				}
			}
		}

		// Ensure we close the drain requests channel, so that we can prevent
		// any further requests to the local file system.
		close(t.drainRequests)

		t.logger.Infof(context.TODO(), "draining completed for %q, processed %d", t.namespace, len(metadata))

		// Report the drained state completed.
		t.reportInternalState(stateDrained)

		return nil
	}
}

func (t *s3ObjectStore) drainFile(ctx context.Context, path, hash string, metadataSize int64) error {
	// If the file isn't on the file backed object store, then we can skip it.
	// It's expected that this has already been drained to the s3 object store.
	if err := t.fileSystemAccessor.HashExists(ctx, hash); err != nil {
		if errors.Is(err, errors.NotFound) {
			return nil
		}
		return errors.Annotatef(err, "checking if file %q exists in file object store", path)
	}

	// If the file is already in the s3 object store, then we can skip it.
	// Note: we want to check the s3 object store each request, just in
	// case the file was added to the s3 object store while we were
	// draining the files.
	if err := t.objectAlreadyExists(ctx, hash); err != nil && !errors.Is(err, errors.NotFound) {
		return errors.Annotatef(err, "checking if file %q exists in s3 object store", path)
	} else if err == nil {
		// File already contains the hash, so we can skip it.
		t.logger.Tracef(context.TODO(), "file %q already exists in s3 object store, skipping", path)
		return nil
	}

	t.logger.Debugf(context.TODO(), "draining file %q to s3 object store", path)

	// Grab the file from the file backed object store and drain it to the
	// s3 object store.
	reader, fileSize, err := t.fileSystemAccessor.GetByHash(ctx, hash)
	if err != nil {
		// The file doesn't exist in the file backed object store, but also
		// doesn't exist in the s3 object store. This is a problem, so we
		// should skip it.
		if errors.Is(err, errors.NotFound) {
			t.logger.Warningf(context.TODO(), "file %q doesn't exist in file object store, unable to drain", path)
			return nil
		}
		return errors.Annotatef(err, "getting file %q from file object store", path)
	}

	// Ensure we close the reader when we're done.
	defer reader.Close()

	// If the file size doesn't match the metadata size, then the file is
	// potentially corrupt, so we should skip it.
	if fileSize != metadataSize {
		t.logger.Warningf(context.TODO(), "file %q has a size mismatch, unable to drain", path)
		return nil
	}

	// We need to compute the sha256 hash here, juju by default uses SHA384,
	// but s3 defaults to SHA256.
	// If the reader is a Seeker, then we can seek back to the beginning of
	// the file, so that we can read it again.
	s3Reader, s3EncodedHash, err := t.computeS3Hash(reader)
	if err != nil {
		return errors.Trace(err)
	}

	// We can drain the file to the s3 object store.
	err = t.client.Session(ctx, func(ctx context.Context, s objectstore.Session) error {
		err := s.PutObject(ctx, t.rootBucket, t.filePath(hash), s3Reader, s3EncodedHash)
		if err != nil {
			return errors.Annotatef(err, "putting file %q to s3 object store", path)
		}
		return nil
	})
	if err != nil {
		if errors.Is(err, errors.AlreadyExists) {
			// File already contains the hash, so we can skip it.
			return t.removeDrainedFile(ctx, hash)
		}
		return errors.Trace(err)
	}

	// We can remove the file from the file backed object store, because it
	// has been successfully drained to the s3 object store.
	if err := t.removeDrainedFile(ctx, hash); err != nil {
		return errors.Trace(err)
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
			return nil, "", errors.Annotatef(err, "computing hash")
		}

		if _, err := seekReader.Seek(0, io.SeekStart); err != nil {
			return nil, "", errors.Annotatef(err, "seeking back to start")
		}

		return reader, base64.StdEncoding.EncodeToString(s3Hash.Sum(nil)), nil
	}

	// If the reader is not a Seeker, then we need to copy the entire file
	// into memory, so that we can compute the hash.
	memReader := new(bytes.Buffer)
	if _, err := io.Copy(io.MultiWriter(s3Hash, memReader), reader); err != nil {
		return nil, "", errors.Annotatef(err, "computing hash")
	}

	return memReader, base64.StdEncoding.EncodeToString(s3Hash.Sum(nil)), nil
}

func (t *s3ObjectStore) objectAlreadyExists(ctx context.Context, hash string) error {
	if err := t.client.Session(ctx, func(ctx context.Context, s objectstore.Session) error {
		err := s.ObjectExists(ctx, t.rootBucket, t.filePath(hash))
		return errors.Trace(err)
	}); err != nil {
		return errors.Annotatef(err, "checking if file %q exists in s3 object store", hash)
	}
	return nil
}

func (t *s3ObjectStore) removeDrainedFile(ctx context.Context, hash string) error {
	if err := t.fileSystemAccessor.DeleteByHash(ctx, hash); err != nil {
		// If we're unable to remove the file from the file backed object
		// store, then we should log a warning, but continue processing.
		// This is not a terminal case, we can continue processing.
		t.logger.Warningf(context.TODO(), "unable to remove file %q from file object store: %v", hash, err)
		return nil
	}
	return nil
}

func (t *s3ObjectStore) reportInternalState(state string) {
	if t.internalStates == nil {
		return
	}
	select {
	case <-t.tomb.Dying():
	case t.internalStates <- state:
	}
}
