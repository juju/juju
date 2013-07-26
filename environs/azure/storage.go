// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package azure

import (
	"io"
	"launchpad.net/gwacl"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/errors"
)

type azureStorage struct {
	storageContext
}

// storageContext is an abstraction that is there only to accommodate the need
// for using an azureStorage independently from an environ object in tests.
type storageContext interface {
	getContainer() string
	getStorageContext() (*gwacl.StorageContext, error)
}

// environStorageContext is a storageContext which gets its information from
// an azureEnviron object.
type environStorageContext struct {
	environ *azureEnviron
}

var _ storageContext = (*environStorageContext)(nil)

func (context *environStorageContext) getContainer() string {
	return context.environ.GetEnvPrefix() + -"private"
}

func (context *environStorageContext) getStorageContext() (*gwacl.StorageContext, error) {
	return context.environ.getStorageContext()
}

// azureStorage implements Storage.
var _ environs.Storage = (*azureStorage)(nil)

// Get is specified in the StorageReader interface.
func (storage *azureStorage) Get(name string) (io.ReadCloser, error) {
	context, err := storage.getStorageContext()
	if err != nil {
		return nil, err
	}
	reader, err := context.GetBlob(storage.getContainer(), name)
	if gwacl.IsNotFoundError(err) {
		return nil, errors.NotFoundf("file %q not found", name)
	}
	return reader, err
}

// List is specified in the StorageReader interface.
func (storage *azureStorage) List(prefix string) ([]string, error) {
	context, err := storage.getStorageContext()
	if err != nil {
		return nil, err
	}
	request := &gwacl.ListBlobsRequest{Container: storage.getContainer(), Prefix: prefix, Marker: ""}
	blobList, err := context.ListAllBlobs(request)
	if err != nil {
		return nil, err
	}
	names := make([]string, len(blobList.Blobs))
	for index, blob := range blobList.Blobs {
		names[index] = blob.Name
	}
	return names, nil
}

// URL is specified in the StorageReader interface.
func (storage *azureStorage) URL(name string) (string, error) {
	context, err := storage.getStorageContext()
	if err != nil {
		return "", err
	}
	return context.GetFileURL(storage.getContainer(), name), nil
}

// Put is specified in the StorageWriter interface.
func (storage *azureStorage) Put(name string, r io.Reader, length int64) error {
	limitedReader := io.LimitReader(r, length)
	context, err := storage.getStorageContext()
	if err != nil {
		return err
	}
	return context.UploadBlockBlob(storage.getContainer(), name, limitedReader)
}

// Remove is specified in the StorageWriter interface.
func (storage *azureStorage) Remove(name string) error {
	context, err := storage.getStorageContext()
	if err != nil {
		return err
	}
	return context.DeleteBlob(storage.getContainer(), name)
}

// RemoveAll is specified in the StorageWriter interface.
func (storage *azureStorage) RemoveAll() error {
	context, err := storage.getStorageContext()
	if err != nil {
		return err
	}
	return context.DeleteAllBlobs(&gwacl.DeleteAllBlobsRequest{
		Container: storage.getContainer(),
	})
}

// publicEnvironStorageContext is a storageContext which gets its information
// from an azureEnviron object to create a public storage.
type publicEnvironStorageContext struct {
	environ *azureEnviron
}

var _ storageContext = (*publicEnvironStorageContext)(nil)

func (context *publicEnvironStorageContext) getContainer() string {
	return context.environ.getSnapshot().ecfg.PublicStorageContainerName()
}

func (context *publicEnvironStorageContext) getStorageContext() (*gwacl.StorageContext, error) {
	return context.environ.getPublicStorageContext()
}
