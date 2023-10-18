// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package file

import (
	"context"
	"encoding/base64"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/juju/errors"
	"gopkg.in/tomb.v2"
)

// Logger represents the logging methods called.
type Logger interface {
	Errorf(message string, args ...any)
	Warningf(message string, args ...any)
	Infof(message string, args ...any)
	Debugf(message string, args ...any)
	Tracef(message string, args ...any)

	IsTraceEnabled() bool
}

type opType int

const (
	opGet opType = iota
	opPut
	opRemove
)

type request struct {
	op       opType
	path     string
	reader   io.Reader
	size     int64
	response chan response
}

type response struct {
	reader io.ReadCloser
	size   int64
	err    error
}

type fileObjectStore struct {
	tomb tomb.Tomb

	fs        fs.FS
	namespace string
	path      string

	logger Logger

	requests chan request
}

// New returns a new object store worker based on the state
// storage.
func New(root string, namespace string, logger Logger) (*fileObjectStore, error) {
	path := filepath.Join(root, namespace)

	s := &fileObjectStore{
		fs:        os.DirFS(path),
		namespace: namespace,
		path:      path,
		logger:    logger,

		requests: make(chan request),
	}

	s.tomb.Go(s.loop)

	return s, nil
}

// Get returns an io.ReadCloser for data at path, namespaced to the
// model.
func (t *fileObjectStore) Get(ctx context.Context, path string) (io.ReadCloser, int64, error) {
	// The following is an optimisation to avoid the slow path if possible.
	// If the file exists, we can just return it. Otherwise we need to be
	// serialized with the put requests, as the file may be in the process of
	// being written and we want to sequence the Get after the Put.
	if _, err := os.Stat(encodedFileName(path)); err == nil {
		return t.get(path)
	}

	// Sequence the get request with the put requests.
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
		return resp.reader, resp.size, errors.Trace(resp.err)
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
		op:       opPut,
		path:     path,
		reader:   r,
		size:     size,
		response: response,
	}:
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.tomb.Dying():
		return tomb.ErrDying
	case resp := <-response:
		return errors.Trace(resp.err)
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
		return errors.Trace(resp.err)
	}
}

// Kill implements the worker.Worker interface.
func (t *fileObjectStore) Kill() {
	t.tomb.Kill(nil)
}

// Wait implements the worker.Worker interface.
func (t *fileObjectStore) Wait() error {
	return t.tomb.Wait()
}

func (t *fileObjectStore) loop() error {
	// Ensure the namespace directory exists.
	if _, err := os.Stat(t.path); err != nil && errors.Is(err, os.ErrNotExist) {
		if err := os.MkdirAll(t.path, 0755); err != nil {
			return errors.Trace(err)
		}
	}

	// Sequence the get request with the put, remove requests.
	for {
		select {
		case <-t.tomb.Dying():
			return tomb.ErrDying

		case req := <-t.requests:
			switch req.op {
			case opGet:
				reader, size, err := t.get(req.path)

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
				err := t.put(req.path, req.reader, req.size)

				select {
				case <-t.tomb.Dying():
					return tomb.ErrDying

				case req.response <- response{
					err: err,
				}:
				}

			case opRemove:
				err := t.remove(req.path)

				select {
				case <-t.tomb.Dying():
					return tomb.ErrDying

				case req.response <- response{
					err: err,
				}:
				}
			}
		}
	}
}

func (t *fileObjectStore) get(path string) (io.ReadCloser, int64, error) {
	encoded := encodedFileName(path)

	file, err := t.fs.Open(encoded)
	if err != nil {
		return nil, -1, errors.Annotatef(err, "file %q encoded as %q", path, encoded)
	}
	stat, err := file.Stat()
	if err != nil {
		return nil, -1, errors.Trace(err)
	}
	return file, stat.Size(), errors.Trace(err)
}

func (t *fileObjectStore) put(path string, r io.Reader, size int64) error {
	encoded := t.filePath(path)

	// Check to see if the file already exists with the same name.
	if info, err := os.Stat(encoded); err == nil {
		// If the file on disk isn't the same as the one we're trying to write,
		// then we have a problem.
		if info.Size() != size {
			return errors.AlreadyExistsf("file %q encoded as %q", path, encoded)
		}
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		// There is an error attempting to stat the file, and it's not because
		// the file doesn't exist.
		return errors.Trace(err)
	}

	// The following dance is to ensure that we don't end up with a partially
	// written file if we crash while writing it or if we're attempting to
	// read it at the same time.
	tmpFile, err := os.CreateTemp("", "file")
	if err != nil {
		return errors.Trace(err)
	}
	defer func() {
		_ = tmpFile.Close()
		_ = os.Remove(tmpFile.Name())
	}()

	written, err := io.Copy(tmpFile, r)
	if err != nil {
		return errors.Trace(err)
	}

	// Ensure that we write all the data.
	if written != size {
		return errors.Errorf("partially written data: written %d, expected %d", written, size)
	}

	// Swap out the temporary file for the real one.
	return os.Rename(tmpFile.Name(), encoded)
}

func (t *fileObjectStore) remove(path string) error {
	encoded := t.filePath(path)

	if _, err := os.Stat(encoded); err != nil && errors.Is(err, os.ErrNotExist) {
		return errors.AlreadyExistsf("file %q encoded as %q", path, encoded)
	}

	return os.Remove(encoded)
}

func (t *fileObjectStore) filePath(name string) string {
	return filepath.Join(t.path, encodedFileName(name))
}

func encodedFileName(path string) string {
	// Use base64 encoding to ensure that the file name is valid.
	// Note: this doesn't pad the encoded string.
	return base64.RawURLEncoding.EncodeToString([]byte(path))
}
