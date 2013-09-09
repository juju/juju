// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package environs

import (
	"fmt"
	"io"
	"launchpad.net/juju-core/environs/simplestreams"
)

// RemoveAll is a default implementation for StorageWriter.RemoveAll.
// Providers may have more efficient implementations, or better error handling,
// or safeguards against races with other users of the same storage medium.
// But a simple way to implement RemoveAll would be to delegate to here.
func RemoveAll(stor Storage) error {
	files, err := stor.List("")
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

// A httpDataSource retrieves data from an environs.StorageReader.
type storageSimpleStreamsDataSource struct {
	storage StorageReader
}

// NewHttpDataSource returns a new http datasource reading from the specified storage.
func NewStorageSimpleStreamsDataSource(storage StorageReader) simplestreams.DataSource {
	return &storageSimpleStreamsDataSource{storage}
}

// Fetch is defined in simplestreams.DataSource.
func (s *storageSimpleStreamsDataSource) Fetch(path string) (io.ReadCloser, string, error) {
	dataURL := path
	fullURL, err := s.storage.URL(path)
	if err != nil {
		dataURL = fullURL
	}
	rc, err := s.storage.Get(path)
	if err != nil {
		return nil, dataURL, err
	}
	return rc, dataURL, nil
}

// URL is defined in simplestreams.DataSource.
func (s *storageSimpleStreamsDataSource) URL(path string) (string, error) {
	return s.storage.URL(path)
}
