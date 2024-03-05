// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	"context"
	"fmt"
	"io"
	"path"

	"github.com/juju/utils/v4"

	"github.com/juju/juju/environs/simplestreams"
)

// RemoveAll is a default implementation for StorageWriter.RemoveAll.
// Providers may have more efficient implementations, or better error handling,
// or safeguards against races with other users of the same storage medium.
// But a simple way to implement RemoveAll would be to delegate to here.
func RemoveAll(stor Storage) error {
	files, err := List(stor, "")
	if err != nil {
		return fmt.Errorf("unable to list files for deletion: %v", err)
	}

	// Some limited parallellism might be useful in this loop.
	for _, file := range files {
		err = stor.Remove(file)
		if err != nil {
			break
		}
	}
	return err
}

// Get gets the named file from stor using the stor's default consistency strategy.
func Get(stor StorageReader, name string) (io.ReadCloser, error) {
	return GetWithRetry(stor, name, stor.DefaultConsistencyStrategy())
}

// GetWithRetry gets the named file from stor using the specified attempt strategy.
//
// TODO(katco): 2016-08-09: lp:1611427
func GetWithRetry(stor StorageReader, name string, attempt utils.AttemptStrategy) (r io.ReadCloser, err error) {
	for a := attempt.Start(); a.Next(); {
		r, err = stor.Get(name)
		if err == nil || !stor.ShouldRetry(err) {
			break
		}
	}
	return r, err
}

// List lists the files matching prefix from stor using the stor's default consistency strategy.
func List(stor StorageReader, prefix string) ([]string, error) {
	return ListWithRetry(stor, prefix, stor.DefaultConsistencyStrategy())
}

// ListWithRetry lists the files matching prefix from stor using the specified attempt strategy.
//
// TODO(katco): 2016-08-09: lp:1611427
func ListWithRetry(stor StorageReader, prefix string, attempt utils.AttemptStrategy) (list []string, err error) {
	for a := attempt.Start(); a.Next(); {
		list, err = stor.List(prefix)
		if err == nil || !stor.ShouldRetry(err) {
			break
		}
	}
	return list, err
}

// BaseToolsPath is the container where tools tarballs and metadata are found.
var BaseToolsPath = "tools"

// BaseImagesPath is the container where images metadata is found.
var BaseImagesPath = "images"

// A storageSimpleStreamsDataSource retrieves data from a StorageReader.
type storageSimpleStreamsDataSource struct {
	description   string
	basePath      string
	storage       StorageReader
	priority      int
	requireSigned bool
}

// NewStorageSimpleStreamsDataSource returns a new datasource reading from the specified storage.
func NewStorageSimpleStreamsDataSource(description string, storage StorageReader, basePath string, priority int, requireSigned bool) simplestreams.DataSource {
	return &storageSimpleStreamsDataSource{description, basePath, storage, priority, requireSigned}
}

func (s *storageSimpleStreamsDataSource) relpath(storagePath string) string {
	relpath := storagePath
	if s.basePath != "" {
		relpath = path.Join(s.basePath, relpath)
	}
	return relpath
}

// Description is defined in simplestreams.DataSource.
func (s *storageSimpleStreamsDataSource) Description() string {
	return s.description
}

// Fetch is defined in simplestreams.DataSource.
func (s *storageSimpleStreamsDataSource) Fetch(_ context.Context, path string) (io.ReadCloser, string, error) {
	relpath := s.relpath(path)
	dataURL := relpath
	fullURL, err := s.storage.URL(relpath)
	if err == nil {
		dataURL = fullURL
	}
	rc, err := Get(s.storage, relpath)
	if err != nil {
		return nil, dataURL, err
	}
	return rc, dataURL, nil
}

// URL is defined in simplestreams.DataSource.
func (s *storageSimpleStreamsDataSource) URL(path string) (string, error) {
	return s.storage.URL(s.relpath(path))
}

// PublicSigningKey is defined in simplestreams.DataSource.
func (u *storageSimpleStreamsDataSource) PublicSigningKey() string {
	return ""
}

// Priority is defined in simplestreams.DataSource.
func (s *storageSimpleStreamsDataSource) Priority() int {
	return s.priority
}

// RequireSigned is defined in simplestreams.DataSource.
func (s *storageSimpleStreamsDataSource) RequireSigned() bool {
	return s.requireSigned
}
