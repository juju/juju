// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"path"

	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/simplestreams"
	"github.com/juju/juju/environs/storage"
	"github.com/juju/juju/state"
)

// environmentStorageDataSource is a simplestreams.DataSource that
// retrieves simplestreams metadata from environment storage.
type environmentStorageDataSource struct {
	stor state.Storage
}

// NewEnvironmentStorageDataSource returns a new datasource that retrieves
// metadata from environment storage.
func NewEnvironmentStorageDataSource(stor state.Storage) simplestreams.DataSource {
	return environmentStorageDataSource{stor}
}

// Description is defined in simplestreams.DataSource.
func (d environmentStorageDataSource) Description() string {
	return "environment storage"
}

// Fetch is defined in simplestreams.DataSource.
func (d environmentStorageDataSource) Fetch(file string) (io.ReadCloser, string, error) {
	logger.Debugf("fetching %q", file)

	r, _, err := d.stor.Get(path.Join(storage.BaseImagesPath, file))
	if err != nil {
		return nil, "", err
	}
	data, err := ioutil.ReadAll(r)
	if err != nil {
		return nil, "", err
	}

	url, _ := d.URL(file)
	return ioutil.NopCloser(bytes.NewReader(data)), url, nil
}

// URL is defined in simplestreams.DataSource.
func (d environmentStorageDataSource) URL(file string) (string, error) {
	path := path.Join(storage.BaseImagesPath, file)
	return fmt.Sprintf("environment-storage://%s", path), nil
}

// Defined in simplestreams.DataSource.
func (d environmentStorageDataSource) SetAllowRetry(allow bool) {
}

// registerSimplestreamsDataSource registers a environmentStorageDataSource.
func registerSimplestreamsDataSource(stor state.Storage) {
	ds := NewEnvironmentStorageDataSource(stor)
	environs.RegisterImageDataSourceFunc(ds.Description(), func(environs.Environ) (simplestreams.DataSource, error) {
		return ds, nil
	})
}
