// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agent

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"path"

	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/simplestreams"
	"github.com/juju/juju/state/storage"
)

const (
	storageDataSourceId          = "model storage"
	storageDataSourceDescription = storageDataSourceId
	metadataBasePath             = "imagemetadata"
)

// environmentStorageDataSource is a simplestreams.DataSource that
// retrieves simplestreams metadata from environment storage.
type environmentStorageDataSource struct {
	stor          storage.Storage
	priority      int
	requireSigned bool
}

// NewModelStorageDataSource returns a new datasource that retrieves
// metadata from environment storage.
func NewModelStorageDataSource(stor storage.Storage, priority int, requireSigned bool) simplestreams.DataSource {
	return environmentStorageDataSource{stor, priority, requireSigned}
}

// Description is defined in simplestreams.DataSource.
func (d environmentStorageDataSource) Description() string {
	return storageDataSourceDescription
}

// Fetch is defined in simplestreams.DataSource.
func (d environmentStorageDataSource) Fetch(file string) (io.ReadCloser, string, error) {
	logger.Debugf("fetching %q", file)

	r, _, err := d.stor.Get(path.Join(metadataBasePath, file))
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
	path := path.Join(metadataBasePath, file)
	return fmt.Sprintf("model-storage://%s", path), nil
}

// PublicSigningKey is defined in simplestreams.DataSource.
func (d environmentStorageDataSource) PublicSigningKey() string {
	return ""
}

// Defined in simplestreams.DataSource.
func (d environmentStorageDataSource) SetAllowRetry(allow bool) {
}

// Priority is defined in simplestreams.DataSource.
func (d environmentStorageDataSource) Priority() int {
	return d.priority
}

// RequireSigned is defined in simplestreams.DataSource.
func (d environmentStorageDataSource) RequireSigned() bool {
	return d.requireSigned
}

// registerSimplestreamsDataSource registers a environmentStorageDataSource.
func registerSimplestreamsDataSource(stor storage.Storage, requireSigned bool) {
	ds := NewModelStorageDataSource(stor, simplestreams.DEFAULT_CLOUD_DATA, requireSigned)
	environs.RegisterUserImageDataSourceFunc(storageDataSourceId, func(environs.Environ) (simplestreams.DataSource, error) {
		return ds, nil
	})
}

// unregisterSimplestreamsDataSource de-registers an environmentStorageDataSource.
func unregisterSimplestreamsDataSource() {
	environs.UnregisterImageDataSourceFunc(storageDataSourceId)
}
