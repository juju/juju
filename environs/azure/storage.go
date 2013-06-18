// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package azure

import (
	"io"
	"launchpad.net/juju-core/environs"
)

type azureStorage struct{}

// azureStorage implements Storage.
var _ environs.Storage = (*azureStorage)(nil)

// Get is specified in the StorageReader interface.
func (storage *azureStorage) Get(name string) (io.ReadCloser, error) {
	panic("unimplemented")
}

// List is specified in the StorageReader interface.
func (storage *azureStorage) List(prefix string) ([]string, error) {
	panic("unimplemented")
}

// URL is specified in the StorageReader interface.
func (storage *azureStorage) URL(name string) (string, error) {
	panic("unimplemented")
}

// Put is specified in the StorageWriter interface.
func (storage *azureStorage) Put(name string, r io.Reader, length int64) error {
	panic("unimplemented")
}

// Remove is specified in the StorageWriter interface.
func (storage *azureStorage) Remove(name string) error {
	panic("unimplemented")
}
