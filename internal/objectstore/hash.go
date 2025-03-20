// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package objectstore

import (
	"context"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/juju/errors"

	"github.com/juju/juju/core/logger"
)

type hashFileSystemAccessor struct {
	fs        fs.FS
	namespace string
	path      string
	logger    logger.Logger
}

func newHashFileSystemAccessor(namespace, rootDir string, logger logger.Logger) *hashFileSystemAccessor {
	path := basePath(rootDir, namespace)
	return &hashFileSystemAccessor{
		fs:        os.DirFS(path),
		path:      path,
		namespace: namespace,
		logger:    logger,
	}
}

// HashExists checks if the file at hash exists in the file storage.
func (t *hashFileSystemAccessor) HashExists(ctx context.Context, hash string) error {
	t.logger.Debugf(ctx, "checking object %q in file storage", hash)

	_, err := os.Stat(t.filePath(hash))
	if err == nil {
		return nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return errors.NotFoundf("hash %q", hash)
	}
	return errors.Trace(err)
}

// GetByHash returns an io.ReadCloser for data at hash, namespaced to the
// model.
func (t *hashFileSystemAccessor) GetByHash(ctx context.Context, hash string) (io.ReadCloser, int64, error) {
	t.logger.Debugf(ctx, "getting object %q from file storage", hash)

	file, err := t.fs.Open(hash)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, -1, errors.NotFoundf("hash %q%w", hash, errors.Hide(err))
		}

		return nil, -1, errors.Annotatef(err, "opening file hash %q", hash)
	}

	stat, err := file.Stat()
	if err != nil {
		return nil, -1, errors.Annotatef(err, "retrieving size: file hash %q", hash)
	}

	return file, stat.Size(), nil
}

// DeleteByHash deletes a file at hash, namespaced to the model.
func (t *hashFileSystemAccessor) DeleteByHash(ctx context.Context, hash string) error {
	t.logger.Debugf(ctx, "deleting object %q from file storage", hash)

	filePath := t.filePath(hash)

	// File doesn't exist, return early, nothing we can do in this case.
	if _, err := os.Stat(filePath); err != nil && errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err := os.Remove(filePath); err != nil {
		return errors.Annotatef(err, "removing file %q", filePath)
	}
	return nil
}

func (t *hashFileSystemAccessor) filePath(hash string) string {
	return filepath.Join(t.path, hash)
}
