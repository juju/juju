// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package objectstore

import (
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

	Logger logger.Logger
	Clock  clock.Clock
}

const (
	// States which report the state of the worker.
	stateStarted = "started"
)

type s3ObjectStore struct {
	baseObjectStore

	internalStates chan string
	catacomb       catacomb.Catacomb

	client     objectstore.Client
	rootBucket string
	namespace  string
	requests   chan request
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

		requests: make(chan request),
	}

	if err := catacomb.Invoke(catacomb.Plan{
		Name: "s3-object-store",
		Site: &s.catacomb,
		Work: s.loop,
	}); err != nil {
		return nil, errors.Errorf("starting s3 object store: %w", err)
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
	if reader, size, err := t.get(ctx, path); err == nil {
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

// GetBySHA256 returns an io.ReadCloser for any object with a SHA256
// hash starting with a given prefix, namespaced to the model.
//
// If no object is found, an [objectstore.ObjectNotFound] error is returned.
func (t *s3ObjectStore) GetBySHA256(ctx context.Context, sha256 string) (io.ReadCloser, int64, error) {
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

// GetBySHA256Prefix returns an io.ReadCloser for any object with a SHA256
// hash starting with a given prefix, namespaced to the model.
//
// If no object is found, an [objectstore.ObjectNotFound] error is returned.
func (t *s3ObjectStore) GetBySHA256Prefix(ctx context.Context, sha256Prefix string) (io.ReadCloser, int64, error) {
	// Optimistically try to get the file from the file system. If it doesn't
	// exist, then we'll get an error, and we'll try to get it when sequencing
	// the get request with the put and remove requests.
	if reader, size, err := t.getBySHA256Prefix(ctx, sha256Prefix); err == nil {
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

// Report returns a map of internal state for the s3 object store.
func (t *s3ObjectStore) Report() map[string]any {
	report := make(map[string]any)

	report["namespace"] = t.namespace
	report["path"] = t.path
	report["rootBucket"] = t.rootBucket

	return report
}

// Kill implements the worker.Worker interface.
func (s *s3ObjectStore) Kill() {
	s.catacomb.Kill(nil)
}

// Wait implements the worker.Worker interface.
func (s *s3ObjectStore) Wait() error {
	return s.catacomb.Wait()
}

// scopedContext returns a context that is in the scope of the worker lifetime.
// It returns a cancellable context that is cancelled when the action has
// completed.
func (w *s3ObjectStore) scopedContext() (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(context.Background())
	return w.catacomb.Context(ctx), cancel
}

func (t *s3ObjectStore) loop() error {
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
				reader, size, err := t.get(ctx, req.path)

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
				reader, size, err := t.getBySHA256(ctx, req.sha256)

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
				reader, size, err := t.getBySHA256Prefix(ctx, req.sha256)

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
				t.logger.Errorf(ctx, "prune: %v", err)
				continue
			}
		}
	}
}

func (t *s3ObjectStore) get(ctx context.Context, path string) (io.ReadCloser, int64, error) {
	t.logger.Debugf(ctx, "getting object %q from file storage", path)

	metadata, err := t.metadataService.GetMetadata(ctx, path)
	if errors.Is(err, domainobjectstoreerrors.ErrNotFound) {
		return nil, -1, errors.Errorf("get metadata: %w", objectstoreerrors.ObjectNotFound)
	} else if err != nil {
		return nil, -1, errors.Errorf("get metadata: %w", err)
	}

	return t.getWithMetadata(ctx, metadata)
}

func (t *s3ObjectStore) getBySHA256(ctx context.Context, sha256 string) (io.ReadCloser, int64, error) {
	t.logger.Debugf(ctx, "getting object with SHA256 %q from file storage", sha256)

	metadata, err := t.metadataService.GetMetadataBySHA256(ctx, sha256)
	if errors.Is(err, domainobjectstoreerrors.ErrNotFound) {
		return nil, -1, errors.Errorf("get metadata by SHA256: %w", objectstoreerrors.ObjectNotFound)
	} else if err != nil {
		return nil, -1, errors.Errorf("get metadata by SHA256: %w", err)
	}

	return t.getWithMetadata(ctx, metadata)
}

func (t *s3ObjectStore) getBySHA256Prefix(ctx context.Context, sha256Prefix string) (io.ReadCloser, int64, error) {
	t.logger.Debugf(ctx, "getting object with SHA256 prefix %q from file storage", sha256Prefix)

	metadata, err := t.metadataService.GetMetadataBySHA256Prefix(ctx, sha256Prefix)
	if errors.Is(err, domainobjectstoreerrors.ErrNotFound) {
		return nil, -1, errors.Errorf("get metadata by SHA256 prefix: %w", objectstoreerrors.ObjectNotFound)
	} else if err != nil {
		return nil, -1, errors.Errorf("get metadata by SHA256 prefix: %w", err)
	}

	return t.getWithMetadata(ctx, metadata)
}

func (t *s3ObjectStore) getWithMetadata(ctx context.Context, metadata objectstore.Metadata) (io.ReadCloser, int64, error) {
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
		return nil, -1, objectstoreerrors.ObjectNotFound
	}

	if metadata.Size != size {
		return nil, -1, errors.Errorf("size mismatch for %q: expected %d, got %d", metadata.Path, metadata.Size, size)
	}

	return reader, size, nil
}

func (t *s3ObjectStore) put(ctx context.Context, path string, r io.Reader, size int64, validator hashValidator) (objectstore.UUID, error) {
	t.logger.Debugf(ctx, "putting object %q to s3 storage", path)

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
	t.logger.Debugf(ctx, "removing object %q from s3 storage", path)

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
	t.logger.Debugf(ctx, "listing objects from s3 storage")

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

func (t *s3ObjectStore) reportInternalState(state string) {
	if t.internalStates == nil {
		return
	}
	select {
	case <-t.catacomb.Dying():
	case t.internalStates <- state:
	}
}
