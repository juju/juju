// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	"fmt"
	"io"
	"path/filepath"

	"launchpad.net/juju-core/environs/simplestreams"
	"launchpad.net/juju-core/utils"
)

// RemoveAll is a default implementation for StorageWriter.RemoveAll.
// Providers may have more efficient implementations, or better error handling,
// or safeguards against races with other users of the same storage medium.
// But a simple way to implement RemoveAll would be to delegate to here.
func RemoveAll(stor Storage) error {
	files, err := DefaultList(stor, "")
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

// DefaultGet gets the named file from stor using the stor's default consistency strategy.
func DefaultGet(stor StorageReader, name string) (io.ReadCloser, error) {
	return Get(stor, name, stor.DefaultConsistencyStrategy())
}

// Get gets the named file from stor using the specified attempt strategy.
func Get(stor StorageReader, name string, attempt utils.AttemptStrategy) (r io.ReadCloser, err error) {
	for a := attempt.Start(); a.Next(); {
		r, err = stor.Get(name)
		if err == nil || !stor.ShouldRetry(err) {
			break
		}
	}
	return r, err
}

// DefaultList lists the files matching prefix from stor using the stor's default consistency strategy.
func DefaultList(stor StorageReader, prefix string) ([]string, error) {
	return List(stor, prefix, stor.DefaultConsistencyStrategy())
}

// List lists the files matching prefix from stor using the specified attempt strategy.
func List(stor StorageReader, prefix string, attempt utils.AttemptStrategy) (list []string, err error) {
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

// A storageSimpleStreamsDataSource retrieves data from a StorageReader.
type storageSimpleStreamsDataSource struct {
	basePath string
	storage  StorageReader
}

// NewStorageSimpleStreamsDataSource returns a new datasource reading from the specified storage.
func NewStorageSimpleStreamsDataSource(storage StorageReader, basePath string) simplestreams.DataSource {
	return &storageSimpleStreamsDataSource{basePath, storage}
}

func (s *storageSimpleStreamsDataSource) relpath(path string) string {
	relpath := path
	if s.basePath != "" {
		relpath = filepath.Join(s.basePath, relpath)
	}
	return relpath
}

// Fetch is defined in simplestreams.DataSource.
func (s *storageSimpleStreamsDataSource) Fetch(path string) (io.ReadCloser, string, error) {
	relpath := s.relpath(path)
	dataURL := relpath
	fullURL, err := s.storage.URL(relpath)
	if err == nil {
		dataURL = fullURL
	}
	rc, err := Get(s.storage, relpath, utils.AttemptStrategy{})
	if err != nil {
		return nil, dataURL, err
	}
	return rc, dataURL, nil
}

// URL is defined in simplestreams.DataSource.
func (s *storageSimpleStreamsDataSource) URL(path string) (string, error) {
	return s.storage.URL(s.relpath(path))
}
