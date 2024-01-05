// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package objectstore

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/juju/errors"
	"gopkg.in/tomb.v2"

	"github.com/juju/juju/core/objectstore"
)

type opType int

const (
	opGet opType = iota
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
	reader io.ReadCloser
	size   int64
	err    error
}

type fileObjectStore struct {
	baseObjectStore
	fs        fs.FS
	path      string
	namespace string
	requests  chan request
}

// NewFileObjectStore returns a new object store worker based on the file
// storage.
func NewFileObjectStore(ctx context.Context, namespace, rootPath string, metadataService objectstore.ObjectStoreMetadata, locker Locker, logger Logger) (TrackedObjectStore, error) {
	path := filepath.Join(rootPath, namespace)

	s := &fileObjectStore{
		baseObjectStore: baseObjectStore{
			locker:          locker,
			metadataService: metadataService,
			logger:          logger,
		},
		fs:        os.DirFS(path),
		path:      path,
		namespace: namespace,

		requests: make(chan request),
	}

	s.tomb.Go(s.loop)

	return s, nil
}

// Get returns an io.ReadCloser for data at path, namespaced to the
// model.
func (t *fileObjectStore) Get(ctx context.Context, path string) (io.ReadCloser, int64, error) {
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
			return nil, -1, fmt.Errorf("getting file: %w", resp.err)
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
			return fmt.Errorf("putting file: %w", resp.err)
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
			return fmt.Errorf("putting file and check hash: %w", resp.err)
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
			return fmt.Errorf("removing file: %w", resp.err)
		}
		return nil
	}
}

// Kill implements the worker.Worker interface.
func (s *fileObjectStore) Kill() {
	s.tomb.Kill(nil)
}

// Wait implements the worker.Worker interface.
func (s *fileObjectStore) Wait() error {
	return s.tomb.Wait()
}

func (t *fileObjectStore) loop() error {
	// Ensure the namespace directory exists.
	if _, err := os.Stat(t.path); err != nil && errors.Is(err, os.ErrNotExist) {
		if err := os.MkdirAll(t.path, 0755); err != nil {
			return errors.Trace(err)
		}
	}

	ctx, cancel := t.scopedContext()
	defer cancel()

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
			}
		}
	}
}

func (t *fileObjectStore) get(ctx context.Context, path string) (io.ReadCloser, int64, error) {
	metadata, err := t.metadataService.GetMetadata(ctx, path)
	if err != nil {
		return nil, -1, fmt.Errorf("get metadata: %w", err)
	}

	file, err := t.fs.Open(metadata.Hash)
	if err != nil {
		return nil, -1, fmt.Errorf("opening file %q encoded as %q: %w", path, metadata.Hash, err)
	}

	stat, err := file.Stat()
	if err != nil {
		return nil, -1, fmt.Errorf("retrieving size: file %q encoded as %q: %w", path, metadata.Hash, err)
	}

	if metadata.Size != stat.Size() {
		return nil, -1, fmt.Errorf("size mismatch for %q: expected %d, got %d", path, metadata.Size, stat.Size())
	}

	return file, stat.Size(), nil
}

func (t *fileObjectStore) put(ctx context.Context, path string, r io.Reader, size int64, validator hashValidator) error {
	fileName, hash, err := t.writeToTmpFile(ctx, path, r, size)
	if err != nil {
		return errors.Trace(err)
	}

	// Ensure that the hash of the file matches the expected hash.
	if expected, ok := validator(hash); !ok {
		return fmt.Errorf("hash mismatch for %q: expected %q, got %q: %w", path, expected, hash, objectstore.ErrHashMismatch)
	}

	// Lock the file with the given hash, so that we can't remove the file
	// while we're writing it.
	return t.withLock(ctx, hash, func(ctx context.Context) error {
		// Persist the temporary file to the final location.
		if err := t.persistTmpFile(ctx, fileName, hash, size); err != nil {
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

func (t *fileObjectStore) writeToTmpFile(ctx context.Context, path string, r io.Reader, size int64) (string, string, error) {
	// The following dance is to ensure that we don't end up with a partially
	// written file if we crash while writing it or if we're attempting to
	// read it at the same time.
	tmpFile, err := os.CreateTemp("", "file")
	if err != nil {
		return "", "", errors.Trace(err)
	}
	defer func() {
		_ = tmpFile.Close()
	}()

	hasher := sha256.New()
	written, err := io.Copy(tmpFile, io.TeeReader(r, hasher))
	if err != nil {
		_ = os.Remove(tmpFile.Name())
		return "", "", errors.Trace(err)
	}

	// Ensure that we write all the data.
	if written != size {
		_ = os.Remove(tmpFile.Name())
		return "", "", errors.Errorf("partially written data: written %d, expected %d", written, size)
	}

	hash := hex.EncodeToString(hasher.Sum(nil))
	return tmpFile.Name(), hash, nil
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
	metadata, err := t.metadataService.GetMetadata(ctx, path)
	if err != nil {
		return fmt.Errorf("get metadata: %w", err)
	}

	if err := t.metadataService.RemoveMetadata(ctx, path); err != nil {
		return fmt.Errorf("remove metadata: %w", err)
	}

	hash := metadata.Hash
	filePath := t.filePath(hash)

	// File doesn't exist, return early, nothing we can do in this case.
	if _, err := os.Stat(filePath); err != nil && errors.Is(err, os.ErrNotExist) {
		return nil
	}

	return t.withLock(ctx, hash, func(ctx context.Context) error {
		// If we fail to remove the file, we don't want to return an error, as
		// the metadata has already been removed. Manual intervention will be
		// required to remove the file. We may in the future want to prune the
		// file store of files that are no longer referenced by metadata.
		if err := os.Remove(filePath); err != nil {
			t.logger.Errorf("failed to remove file %q for path %q: %v", filePath, path, err)
		}
		return nil
	})
}

func (t *fileObjectStore) filePath(hash string) string {
	return filepath.Join(t.path, hash)
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
